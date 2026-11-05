package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	vkwsPollMs      = 60000
	vkwsTail        = 50
	vkwsScanWorkers = 6
	vkwsAvatarSize  = "64"
	vkwsBackoff     = 5 * time.Second
)

var errVkwsSession = errors.New("vkws: session expired, run vkws_auth_login in the vkworkspace MCP")

func vkwsSessionPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config/ai-launcher/mcp/lighthouse/generic_vkworkspace/current/bin/session.json")
}

type vkwsSession struct {
	BaseURL      string `json:"base_url"`
	BaseHost     string `json:"base_host"`
	APIVersion   int    `json:"api_version"`
	AIMSID       string `json:"aimsid"`
	FetchBaseURL string `json:"fetch_base_url"`
}

func loadVkwsSession(path string) (*vkwsSession, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vkws: read session: %w", err)
	}
	var s vkwsSession
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("vkws: parse session: %w", err)
	}
	if s.AIMSID == "" || s.FetchBaseURL == "" || s.BaseURL == "" {
		return nil, errVkwsSession
	}
	return &s, nil
}

type vkwsBuddy struct {
	sn, name string
	muted    bool
}

type vkwsEvents struct {
	next    string
	wait    time.Duration
	buddies []vkwsBuddy
	changed []string
}

func parseVkwsEvents(b []byte) (vkwsEvents, error) {
	var r struct {
		Response struct {
			StatusCode int `json:"statusCode"`
			Data       struct {
				FetchBaseURL    string `json:"fetchBaseURL"`
				TimeToNextFetch int64  `json:"timeToNextFetch"`
				Events          []struct {
					Type      string          `json:"type"`
					EventData json.RawMessage `json:"eventData"`
				} `json:"events"`
			} `json:"data"`
		} `json:"response"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return vkwsEvents{}, fmt.Errorf("vkws: decode events: %w", err)
	}
	if c := r.Response.StatusCode; c != http.StatusOK {
		if c == 401 || c == 403 || c == 460 || c == 607 {
			return vkwsEvents{}, errVkwsSession
		}
		return vkwsEvents{}, fmt.Errorf("vkws: fetchEvents status %d", c)
	}
	out := vkwsEvents{next: r.Response.Data.FetchBaseURL, wait: time.Duration(r.Response.Data.TimeToNextFetch) * time.Millisecond}
	for _, e := range r.Response.Data.Events {
		switch e.Type {
		case "buddylist":
			var bl struct {
				Groups []struct {
					Buddies []struct {
						AimID    string `json:"aimId"`
						Friendly string `json:"friendly"`
						Mute     int    `json:"mute"`
						Deleted  bool   `json:"deleted"`
					} `json:"buddies"`
				} `json:"groups"`
			}
			if err := json.Unmarshal(e.EventData, &bl); err != nil {
				return vkwsEvents{}, fmt.Errorf("vkws: decode buddylist: %w", err)
			}
			for _, g := range bl.Groups {
				for _, b := range g.Buddies {
					if b.AimID == "" || b.Deleted {
						continue
					}
					out.buddies = append(out.buddies, vkwsBuddy{sn: b.AimID, name: b.Friendly, muted: b.Mute != 0})
				}
			}
		case "histDlgState":
			var h struct {
				SN string `json:"sn"`
			}
			if err := json.Unmarshal(e.EventData, &h); err != nil {
				return vkwsEvents{}, fmt.Errorf("vkws: decode histDlgState: %w", err)
			}
			if h.SN != "" {
				out.changed = append(out.changed, h.SN)
			}
		}
	}
	return out, nil
}

type vkwsDialog struct {
	unread int
	at     time.Time
	text   string
	name   string
}

func parseVkwsHistory(sn string, b []byte) (vkwsDialog, error) {
	var r struct {
		Status struct {
			Code int `json:"code"`
		} `json:"status"`
		Results struct {
			UnreadCnt int `json:"unreadCnt"`
			Yours     struct {
				LastRead int64 `json:"lastRead"`
			} `json:"yours"`
			Messages []struct {
				MsgID    int64  `json:"msgId"`
				Time     int64  `json:"time"`
				Outgoing bool   `json:"outgoing"`
				Text     string `json:"text"`
				Class    string `json:"class"`
				Parts    []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"messages"`
			Persons []struct {
				SN       string `json:"sn"`
				Friendly string `json:"friendly"`
			} `json:"persons"`
		} `json:"results"`
	}
	if err := json.Unmarshal(b, &r); err != nil {
		return vkwsDialog{}, fmt.Errorf("vkws: decode history: %w", err)
	}
	switch r.Status.Code {
	case 20000:
	case 40001, 40100, 40101, 40102:
		return vkwsDialog{}, errVkwsSession
	default:
		return vkwsDialog{}, fmt.Errorf("vkws: getHistory status %d", r.Status.Code)
	}
	var d vkwsDialog
	res := r.Results
	for _, m := range res.Messages {
		if m.MsgID > res.Yours.LastRead && !m.Outgoing && m.Class != "event" {
			d.unread++
		}
	}
	if res.UnreadCnt > d.unread {
		d.unread = res.UnreadCnt
	}
	if n := len(res.Messages); n > 0 {
		last := res.Messages[0]
		for _, m := range res.Messages {
			if m.MsgID > last.MsgID {
				last = m
			}
		}
		d.at = time.Unix(last.Time, 0)
		text := last.Text
		if text == "" {
			for _, p := range last.Parts {
				text += " " + p.Text
			}
		}
		d.text = strings.Join(strings.Fields(text), " ")
	}
	for _, p := range res.Persons {
		if p.SN == sn {
			d.name = p.Friendly
		}
	}
	return d, nil
}

type vkwsClient struct {
	http *http.Client
	s    *vkwsSession
	seq  int
	mu   sync.Mutex
}

func (c *vkwsClient) get(ctx context.Context, u string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, 0, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, redactAimsid(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return b, resp.StatusCode, err
}

func redactAimsid(err error) error {
	var ue *url.Error
	if errors.As(err, &ue) {
		return fmt.Errorf("%s: %w", ue.Op, ue.Err)
	}
	return err
}

func (c *vkwsClient) fetch(ctx context.Context, base string, first bool) (vkwsEvents, error) {
	u, err := url.Parse(base)
	if err != nil {
		return vkwsEvents{}, fmt.Errorf("vkws: fetch url: %w", err)
	}
	c.mu.Lock()
	c.seq++
	r := c.seq
	c.mu.Unlock()
	q := u.Query()
	q.Set("f", "json")
	q.Set("r", strconv.Itoa(r))
	q.Set("timeout", strconv.Itoa(vkwsPollMs))
	q.Set("peek", "0")
	if first {
		q.Set("first", "1")
	}
	u.RawQuery = q.Encode()
	b, code, err := c.get(ctx, u.String())
	if err != nil {
		return vkwsEvents{}, fmt.Errorf("vkws: fetchEvents: %w", err)
	}
	if code == http.StatusUnauthorized || code == http.StatusForbidden {
		return vkwsEvents{}, errVkwsSession
	}
	if code == http.StatusTooManyRequests {
		return vkwsEvents{next: base, wait: vkwsBackoff}, nil
	}
	if code != http.StatusOK {
		return vkwsEvents{}, fmt.Errorf("vkws: fetchEvents http %d", code)
	}
	return parseVkwsEvents(b)
}

func (c *vkwsClient) history(ctx context.Context, sn string) (vkwsDialog, error) {
	c.mu.Lock()
	c.seq++
	id := fmt.Sprintf("%d-%d", time.Now().UnixMilli(), c.seq)
	c.mu.Unlock()
	body, _ := json.Marshal(map[string]any{
		"reqId":  id,
		"aimsid": c.s.AIMSID,
		"params": map[string]any{"sn": sn, "fromMsgId": -1, "count": -vkwsTail, "patchVersion": "init"},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.s.BaseURL+"/api/v151/rapi/getHistory", strings.NewReader(string(body)))
	if err != nil {
		return vkwsDialog{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return vkwsDialog{}, fmt.Errorf("vkws: getHistory: %w", redactAimsid(err))
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return vkwsDialog{}, fmt.Errorf("vkws: getHistory: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return vkwsDialog{}, fmt.Errorf("vkws: getHistory http %d", resp.StatusCode)
	}
	return parseVkwsHistory(sn, b)
}

func (c *vkwsClient) avatar(ctx context.Context, sn string) ([]byte, error) {
	host := c.s.BaseHost
	if strings.HasPrefix(host, "u.") {
		host = "ub." + host[2:]
	}
	q := url.Values{"aimsid": {c.s.AIMSID}, "targetSn": {sn}, "size": {vkwsAvatarSize}}
	u := fmt.Sprintf("https://%s/api/v%d/files/avatar/get?%s", host, c.s.APIVersion, q.Encode())
	b, code, err := c.get(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("vkws: avatar: %w", err)
	}
	switch code {
	case http.StatusOK:
		return b, nil
	case http.StatusNotFound:
		return nil, nil
	}
	return nil, fmt.Errorf("vkws: avatar http %d", code)
}

func vkwsOpenURL(base, sn string) string {
	return base + "/profile/" + url.PathEscape(sn)
}

type vkwsState struct {
	c       *vkwsClient
	names   map[string]string
	dialogs map[string]vkwsDialog
	avatars map[string][]byte
	muted   map[string]bool
}

// isMuted: a dialog missing from the buddylist is archived (hidden), and the server drops its mute flag with it.
func (st *vkwsState) isMuted(sn string) bool {
	m, listed := st.muted[sn]
	return m || !listed
}

func (st *vkwsState) refresh(ctx context.Context, sns []string) error {
	type res struct {
		sn  string
		d   vkwsDialog
		err error
	}
	jobs := make(chan string)
	out := make(chan res)
	var wg sync.WaitGroup
	for range min(vkwsScanWorkers, len(sns)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for sn := range jobs {
				d, err := st.c.history(ctx, sn)
				out <- res{sn, d, err}
			}
		}()
	}
	go func() {
		for _, sn := range sns {
			jobs <- sn
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()
	var expired error
	skipped := 0
	for r := range out {
		switch {
		case errors.Is(r.err, errVkwsSession):
			expired = r.err
		case r.err != nil:
			skipped++
		default:
			st.dialogs[r.sn] = r.d
		}
	}
	if skipped > 0 {
		log.Printf("vkws: getHistory failed for %d of %d dialogs", skipped, len(sns))
	}
	return expired
}

func (st *vkwsState) entries(ctx context.Context) []chatEntry {
	var es []chatEntry
	for sn, d := range st.dialogs {
		if d.unread == 0 {
			continue
		}
		title := st.names[sn]
		if title == "" {
			title = d.name
		}
		if title == "" {
			title = sn
		}
		av, ok := st.avatars[sn]
		if !ok {
			var err error
			if av, err = st.c.avatar(ctx, sn); err != nil {
				log.Printf("%v", err)
			} else {
				st.avatars[sn] = av
			}
		}
		es = append(es, chatEntry{app: "vkws", id: sn, title: title, unread: d.unread, at: d.at, text: d.text, avatar: av, muted: st.isMuted(sn), open: vkwsOpenURL(st.c.s.BaseURL, sn)})
	}
	sort.Slice(es, func(i, j int) bool { return es[i].at.After(es[j].at) })
	return es
}

func vkwsChats(ctx context.Context, push func([]chatEntry)) error {
	s, err := loadVkwsSession(vkwsSessionPath())
	if err != nil {
		return err
	}
	c := &vkwsClient{http: &http.Client{Timeout: time.Duration(vkwsPollMs)*time.Millisecond + 30*time.Second}, s: s}
	ev, err := c.fetch(ctx, s.FetchBaseURL, true)
	if err != nil {
		return err
	}
	st := &vkwsState{c: c, names: map[string]string{}, muted: map[string]bool{}, dialogs: map[string]vkwsDialog{}, avatars: map[string][]byte{}}
	var scan []string
	for _, b := range ev.buddies {
		st.names[b.sn] = b.name
		st.muted[b.sn] = b.muted
		scan = append(scan, b.sn)
	}
	if err := st.refresh(ctx, scan); err != nil {
		return err
	}
	shown := ""
	emit := func() {
		es := st.entries(ctx)
		key := fmt.Sprint(len(es))
		for _, e := range es {
			key += fmt.Sprintf("|%s:%d:%d:%v:%s", e.id, e.unread, e.at.Unix(), e.muted, e.text)
		}
		if key != shown {
			shown = key
			push(es)
		}
	}
	emit()
	next, wait := ev.next, ev.wait
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
		ev, err := c.fetch(ctx, next, false)
		if err != nil {
			return err
		}
		next, wait = ev.next, ev.wait
		for _, b := range ev.buddies {
			st.names[b.sn] = b.name
			st.muted[b.sn] = b.muted
		}
		var todo []string
		seen := map[string]bool{}
		for _, sn := range ev.changed {
			if !seen[sn] {
				seen[sn] = true
				todo = append(todo, sn)
			}
		}
		if len(todo) == 0 {
			continue
		}
		if err := st.refresh(ctx, todo); err != nil {
			return err
		}
		emit()
	}
}
