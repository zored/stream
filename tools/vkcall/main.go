package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sys/unix"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protowire"
)

const (
	companionVars = "http://127.0.0.1:8000/api/custom-variable/"
	listenAddr    = "127.0.0.1:47114"
	callAgent     = "/com.vk.calls.sdk.v1.CallAgent/"
	processPrefix = "vkws-call-serv"
	holdMin       = 300 * time.Millisecond
	confirmIn     = 5 * time.Second
	rescanEvery   = 2 * time.Second
	resendEvery   = 10 * time.Second
)

const (
	on      = "on"
	off     = "off"
	none    = "none"
	unknown = "unknown"
	pending = "pending"
)

var errNoCall = errors.New("no call with known media state")

type rawCodec struct{}

func (rawCodec) Marshal(v any) ([]byte, error) { return *v.(*[]byte), nil }
func (rawCodec) Unmarshal(data []byte, v any) error {
	*v.(*[]byte) = append([]byte(nil), data...)
	return nil
}
func (rawCodec) Name() string { return "proto" }

type message map[protowire.Number][]any

func parse(b []byte) (message, error) {
	m := message{}
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		b = b[n:]
		var v any
		switch typ {
		case protowire.VarintType:
			v, n = protowire.ConsumeVarint(b)
		case protowire.BytesType:
			v, n = protowire.ConsumeBytes(b)
		default:
			n = protowire.ConsumeFieldValue(num, typ, b)
		}
		if n < 0 {
			return nil, protowire.ParseError(n)
		}
		b = b[n:]
		if v != nil {
			m[num] = append(m[num], v)
		}
	}
	return m, nil
}

func (m message) has(n protowire.Number) bool { return len(m[n]) > 0 }

func (m message) flag(n protowire.Number) bool {
	for _, v := range m[n] {
		if x, ok := v.(uint64); ok {
			return x != 0
		}
	}
	return false
}

func (m message) strings(n protowire.Number) []string {
	var out []string
	for _, v := range m[n] {
		if b, ok := v.([]byte); ok {
			out = append(out, string(b))
		}
	}
	return out
}

func (m message) sub(n protowire.Number) (message, error) {
	for _, v := range m[n] {
		if b, ok := v.([]byte); ok {
			return parse(b)
		}
	}
	return nil, nil
}

var reqSeq atomic.Uint64

func header(callID string) []byte {
	h := protowire.AppendTag(nil, 1, protowire.VarintType)
	h = protowire.AppendVarint(h, uint64(time.Now().UnixMicro()))
	h = appendString(h, 2, fmt.Sprintf("zs_%d", reqSeq.Add(1)))
	if callID != "" {
		h = appendString(h, 3, callID)
	}
	return protowire.AppendBytes(protowire.AppendTag(nil, 1, protowire.BytesType), h)
}

func appendString(b []byte, n protowire.Number, s string) []byte {
	return protowire.AppendString(protowire.AppendTag(b, n, protowire.BytesType), s)
}

func appendBool(b []byte, n protowire.Number, v bool) []byte {
	return protowire.AppendVarint(protowire.AppendTag(b, n, protowire.VarintType), protowire.EncodeBool(v))
}

func subscribe(ctx context.Context, conn *grpc.ClientConn, method string, req []byte) (grpc.ClientStream, error) {
	stream, err := conn.NewStream(ctx, &grpc.StreamDesc{ServerStreams: true}, callAgent+method)
	if err != nil {
		return nil, err
	}
	if err := stream.SendMsg(&req); err != nil {
		return nil, err
	}
	return stream, stream.CloseSend()
}

func receive(stream grpc.ClientStream) (message, error) {
	var b []byte
	if err := stream.RecvMsg(&b); err != nil {
		return nil, err
	}
	return parse(b)
}

func findPort() (int, error) {
	procs, err := unix.SysctlKinfoProcSlice("kern.proc.all")
	if err != nil {
		return 0, err
	}
	for _, p := range procs {
		if !strings.HasPrefix(unix.ByteSliceToString(p.Proc.P_comm[:]), processPrefix) {
			continue
		}
		raw, err := unix.SysctlRaw("kern.procargs2", int(p.Proc.P_pid))
		if err != nil {
			return 0, fmt.Errorf("pid %d args: %w", p.Proc.P_pid, err)
		}
		args := strings.Split(string(raw[4:]), "\x00")
		for i, a := range args {
			if a == "--rpc-port" && i+1 < len(args) {
				return strconv.Atoi(args[i+1])
			}
		}
		return 0, fmt.Errorf("pid %d: no --rpc-port", p.Proc.P_pid)
	}
	return 0, nil
}

type daemon struct {
	mu       sync.Mutex
	conn     *grpc.ClientConn
	callID   string
	mic, cam string
	pressed  time.Time
	micWas   bool
	want     [2]bool
	confirm  *time.Timer
	resync   context.CancelFunc
	changed  chan struct{}
}

func (d *daemon) notify() {
	select {
	case d.changed <- struct{}{}:
	default:
	}
}

func (d *daemon) setLocked(mic, cam string) {
	if d.mic != mic || d.cam != cam {
		log.Printf("call %q: mic %s, cam %s", d.callID, mic, cam)
	}
	d.mic, d.cam = mic, cam
	d.notify()
}

func (d *daemon) set(callID, state string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.callID = callID
	d.stopConfirmLocked()
	d.setLocked(state, state)
}

func (d *daemon) setMedia(callID string, mic, cam bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.callID != callID {
		return
	}
	actual, states := [2]bool{mic, cam}, [2]string{onOff(mic), onOff(cam)}
	if d.confirm != nil && d.want == actual {
		d.stopConfirmLocked()
	}
	if d.confirm != nil {
		for i := range states {
			if d.want[i] != actual[i] {
				states[i] = pending
			}
		}
	}
	d.setLocked(states[0], states[1])
}

func (d *daemon) isOn(i int) bool {
	state := [2]string{d.mic, d.cam}[i]
	return state == on || state == pending && d.want[i]
}

func (d *daemon) stopConfirmLocked() {
	if d.confirm != nil {
		d.confirm.Stop()
		d.confirm = nil
	}
}

func onOff(v bool) string {
	if v {
		return on
	}
	return off
}

func (d *daemon) run() {
	for {
		port, err := findPort()
		switch {
		case err != nil:
			log.Printf("find call service: %v", err)
			d.set("", unknown)
		case port == 0:
			d.set("", none)
		default:
			log.Printf("call service on port %d", port)
			err = d.session(port)
			log.Printf("call service session ended: %v", err)
			d.set("", unknown)
			continue
		}
		time.Sleep(rescanEvery)
	}
}

func (d *daemon) session(port int) error {
	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(rawCodec{})))
	if err != nil {
		return err
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream, err := subscribe(ctx, conn, "SubscribeToStatusNotifications", header(""))
	if err != nil {
		return err
	}
	d.mu.Lock()
	d.conn = conn
	d.mu.Unlock()
	defer func() {
		d.mu.Lock()
		d.conn = nil
		d.mu.Unlock()
	}()
	watching, stopWatch, snapshot := "", context.CancelFunc(func() {}), true
	defer func() { stopWatch() }()
	for {
		report, err := receive(stream)
		if err != nil {
			return err
		}
		var calls []string
		switch {
		case report.has(5):
			calls = report.strings(5)
		case report.flag(6), snapshot:
		default:
			snapshot = false
			continue
		}
		id := ""
		if len(calls) > 0 {
			id = calls[0]
		}
		isNew := !snapshot
		snapshot = false
		if id == watching {
			continue
		}
		stopWatch()
		watching = id
		if id == "" {
			d.set("", none)
			continue
		}
		d.set(id, unknown)
		stopWatch = d.startWatch(ctx, conn, id, isNew)
	}
}

func (d *daemon) startWatch(parent context.Context, conn *grpc.ClientConn, id string, mute bool) context.CancelFunc {
	ctx, cancel := context.WithCancel(parent)
	go d.watch(ctx, conn, id, mute)
	return cancel
}

func (d *daemon) watch(ctx context.Context, conn *grpc.ClientConn, id string, mute bool) {
	for ctx.Err() == nil {
		err := d.watchOnce(ctx, conn, id, &mute)
		if ctx.Err() != nil {
			return
		}
		log.Printf("call %q stream: %v", id, err)
		d.mu.Lock()
		if d.callID == id {
			d.setLocked(unknown, unknown)
		}
		d.mu.Unlock()
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

func (d *daemon) watchOnce(ctx context.Context, conn *grpc.ClientConn, id string, mute *bool) error {
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	d.mu.Lock()
	d.resync = cancel
	d.mu.Unlock()
	stream, err := subscribe(streamCtx, conn, "SubscribeToCallNotifications", header(id))
	if err != nil {
		return err
	}
	for {
		event, err := receive(stream)
		if err != nil {
			return err
		}
		if event.has(8) {
			d.set(id, none)
			continue
		}
		state, err := event.sub(4)
		if err != nil {
			return fmt.Errorf("call state: %w", err)
		}
		if !state.has(2) {
			continue
		}
		media, err := state.sub(2)
		if err != nil {
			return fmt.Errorf("media state: %w", err)
		}
		mic, cam := media.flag(1), media.flag(2)
		d.setMedia(id, mic, cam)
		if *mute {
			*mute = false
			if mic || cam {
				log.Printf("call %q: new call, muting", id)
				if err := d.change(func(bool, bool) (bool, bool) { return false, false }); err != nil {
					log.Printf("auto-mute: %v", err)
				}
			}
		}
	}
}

func (d *daemon) change(next func(mic, cam bool) (bool, bool)) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.changeLocked(next)
}

func (d *daemon) changeLocked(next func(mic, cam bool) (bool, bool)) error {
	if d.conn == nil || d.callID == "" || !known(d.mic) || !known(d.cam) {
		return errNoCall
	}
	mic, cam := next(d.isOn(0), d.isOn(1))
	req := appendBool(appendBool(header(d.callID), 2, mic), 3, cam)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var resp []byte
	if err := d.conn.Invoke(ctx, callAgent+"ChangeMyMedia", &req, &resp); err != nil {
		d.setLocked(unknown, unknown)
		return fmt.Errorf("ChangeMyMedia: %w", err)
	}
	r, err := parse(resp)
	if err != nil || !r.flag(2) {
		d.setLocked(unknown, unknown)
		return fmt.Errorf("ChangeMyMedia rejected: %x", resp)
	}
	d.want = [2]bool{mic, cam}
	states := [2]string{d.mic, d.cam}
	for i, state := range states {
		if state != onOff(d.want[i]) {
			states[i] = pending
		}
	}
	d.stopConfirmLocked()
	if states != [2]string{d.mic, d.cam} {
		d.confirm = time.AfterFunc(confirmIn, d.confirmTimeout)
	}
	d.setLocked(states[0], states[1])
	return nil
}

func known(s string) bool { return s == on || s == off || s == pending }

func (d *daemon) hangUp() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil || d.callID == "" {
		return errNoCall
	}
	req := header(d.callID)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	var resp []byte
	if err := d.conn.Invoke(ctx, callAgent+"EndCall", &req, &resp); err != nil {
		return fmt.Errorf("EndCall: %w", err)
	}
	if r, err := parse(resp); err != nil || !r.flag(2) {
		return fmt.Errorf("EndCall rejected: %x", resp)
	}
	log.Printf("call %q: hung up", d.callID)
	return nil
}

func (d *daemon) confirmTimeout() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.confirm == nil {
		return
	}
	d.confirm = nil
	log.Printf("call %q: change not confirmed in %s, resyncing", d.callID, confirmIn)
	d.setLocked(unknown, unknown)
	if d.resync != nil {
		d.resync()
	}
}

func (d *daemon) micDown() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !known(d.mic) {
		return errNoCall
	}
	d.pressed, d.micWas = time.Now(), d.isOn(0)
	return d.changeLocked(func(mic, cam bool) (bool, bool) { return !d.micWas, cam })
}

func (d *daemon) micUp() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pressed.IsZero() {
		return nil
	}
	held := time.Since(d.pressed) >= holdMin
	d.pressed = time.Time{}
	if !held {
		return nil
	}
	return d.changeLocked(func(mic, cam bool) (bool, bool) { return d.micWas, cam })
}

func (d *daemon) camDown() error {
	return d.change(func(mic, cam bool) (bool, bool) { return mic, !cam })
}

func (d *daemon) publish() {
	sent := map[string]string{}
	resend := time.NewTicker(resendEvery)
	for {
		select {
		case <-d.changed:
		case <-resend.C:
			clear(sent)
		}
		d.mu.Lock()
		values := map[string]string{"call_mic": d.mic, "call_cam": d.cam}
		d.mu.Unlock()
		for name, value := range values {
			if sent[name] == value {
				continue
			}
			if err := setVariable(name, value); err != nil {
				log.Printf("companion %s=%s: %v", name, value, err)
				delete(sent, name)
				continue
			}
			sent[name] = value
		}
	}
}

var client = &http.Client{Timeout: time.Second}

func setVariable(name, value string) error {
	resp, err := client.Post(companionVars+name+"/value", "application/json", strings.NewReader(strconv.Quote(value)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s: %s", resp.Status, body)
	}
	return nil
}

func handle(mux *http.ServeMux, path string, action func() error) {
	mux.HandleFunc("POST "+path, func(w http.ResponseWriter, _ *http.Request) {
		if err := action(); err != nil {
			log.Printf("%s: %v", path, err)
			http.Error(w, err.Error(), http.StatusConflict)
		}
	})
}

func main() {
	d := &daemon{mic: unknown, cam: unknown, changed: make(chan struct{}, 1)}
	go d.publish()
	go d.run()
	mux := http.NewServeMux()
	handle(mux, "/mic/down", d.micDown)
	handle(mux, "/mic/up", d.micUp)
	handle(mux, "/cam/down", d.camDown)
	handle(mux, "/call/end", d.hangUp)
	log.Fatal(http.ListenAndServe(listenAddr, mux))
}
