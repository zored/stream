package main

import (
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

const orcaFixture = `{"result":{"worktrees":[
 {"worktreeId":"r1::/a","displayName":"Razer","workspaceKind":"git","isActive":true,"lastActivityAt":9000,"agents":[
  {"orcaSessionId":"a1","conversationName":"old","lastAssistantMessage":"done","activity":{"listed":true,"unread":false,"sortAt":1000,"title":"old"}},
  {"orcaSessionId":"a2","conversationName":"new","prompt":"go","activity":{"listed":true,"unread":true,"sortAt":3000,"title":"new"}},
  {"orcaSessionId":"a3","conversationName":"hidden","activity":{"listed":false,"unread":true,"sortAt":9999}}]},
 {"worktreeId":"r2::/b","displayName":"macos","workspaceKind":"git","isActive":false,"lastActivityAt":1,"agents":[
  {"orcaSessionId":"b1","conversationName":"b","lastAssistantMessage":"hi","activity":{"listed":true,"unread":true,"sortAt":5000,"title":"b"}}]},
 {"worktreeId":"f1","displayName":"notes","workspaceKind":"folder","isActive":false,"agents":[
  {"orcaSessionId":"c1","conversationName":"c","activity":{"listed":true,"unread":false,"sortAt":500,"title":"c"}}]},
 {"worktreeId":"r3::/c","displayName":"empty","workspaceKind":"git","agents":[]},
 {"worktreeId":"r4::/d","displayName":"gone","workspaceKind":"git","isArchived":true,"agents":[
  {"orcaSessionId":"d1","activity":{"listed":true,"sortAt":99999}}]}]}}`

func TestOrcaParseOrdersByChatActivity(t *testing.T) {
	spaces, err := orcaParse([]byte(orcaFixture))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, s := range spaces {
		names = append(names, s.name)
	}
	if !slices.Equal(names, []string{"macos", "Razer", "notes"}) {
		t.Fatalf("workspaces %v, want freshest chat activity first, empty and archived out", names)
	}
	r := spaces[1]
	if len(r.chats) != 2 || r.chats[0].name != "new" || r.chats[1].name != "old" {
		t.Fatalf("chats %+v, want listed ones, newest first", r.chats)
	}
	if r.chats[0].text != "go" || r.unread() != 1 || !r.at().Equal(time.UnixMilli(3000)) {
		t.Fatalf("chat text falls back to the prompt, unread counts chats: %+v", r)
	}
}

type orcaCalls struct {
	mu    sync.Mutex
	calls []string
	done  chan struct{}
}

func (c *orcaCalls) add(s string) {
	c.mu.Lock()
	c.calls = append(c.calls, s)
	c.mu.Unlock()
}

func testOrca(t *testing.T) (*orca, *orcaCalls, *vars) {
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = post })
	v := &vars{sent: map[string]string{}}
	var strip []string
	o := newOrca(v, func(uri string) { strip = append(strip, uri) })
	calls := &orcaCalls{done: make(chan struct{}, 1)}
	o.run = func(args ...string) ([]byte, error) {
		if args[0] == "agent" {
			return []byte(`{"result":{"entries":[]}}`), nil
		}
		calls.add(strings.Join(args, " "))
		return []byte(orcaFixture), nil
	}
	o.front = func() error { calls.add("front"); return nil }
	o.press = func(name string) error { calls.add("press " + name); calls.done <- struct{}{}; return nil }
	spaces, err := orcaParse([]byte(orcaFixture))
	if err != nil {
		t.Fatal(err)
	}
	o.spaces = spaces
	return o, calls, v
}

func TestOrcaKnobsCaptureOnlyWhileBrowsing(t *testing.T) {
	o, _, _ := testOrca(t)
	if o.chatStep(1) {
		t.Fatal("idle chat knob captured scroll")
	}
	if took, _ := o.knob(true); took {
		t.Fatal("idle press captured scroll")
	}
	o.step(-1)
	if o.space != "f1" || !o.capturing() {
		t.Fatal("backward step did not open the last workspace")
	}
	o.step(1)
	if o.space != "r2::/b" {
		t.Fatal("workspace step did not wrap")
	}
	o.step(1)
	if !o.chatStep(1) || o.space != "r1::/a" || o.chat != 1 || !o.chatStep(1) || o.space != "f1" || o.chat != 0 {
		t.Fatal("chat knob did not walk into the next workspace")
	}
	o.chatStep(1)
	if o.space != "r2::/b" || o.chat != 0 {
		t.Fatal("chat knob did not loop past the last workspace")
	}
	o.chatStep(-1)
	if o.space != "f1" || o.chat != 0 {
		t.Fatal("chat knob back did not land on the previous workspace's last chat")
	}
	o.chatStep(-1)
	if o.space != "r1::/a" || o.chat != 1 {
		t.Fatal("chat knob back did not land on the last chat")
	}
	o.step(-1)
	if o.chat != 0 {
		t.Fatal("a workspace step did not reset to its freshest chat")
	}
	o.till = time.Now().Add(-time.Second)
	o.expire()
	if o.space != "" || o.capturing() || o.chatStep(1) {
		t.Fatal("expired browse kept the knobs")
	}
}

func TestOrcaOpenActivatesThenPressesTab(t *testing.T) {
	o, calls, _ := testOrca(t)
	if err := o.tap(); err != nil || o.space != "r2::/b" {
		t.Fatalf("first tap did not browse the freshest workspace: %v", err)
	}
	o.step(1)
	o.chatStep(1)
	if took, err := o.knob(true); !took || err != nil {
		t.Fatal("press while browsing not taken")
	}
	if took, err := o.knob(false); !took || err != nil {
		t.Fatalf("release did not open: %v", err)
	}
	<-calls.done
	want := []string{"front", "worktree activate --worktree id:r1::/a --json", "press old"}
	if !slices.Equal(calls.calls, want) {
		t.Fatalf("calls %v, want %v", calls.calls, want)
	}
	if o.capturing() {
		t.Fatal("open did not close the browse")
	}
}

func TestOrcaFolderWorkspaceFailsLoudly(t *testing.T) {
	o, calls, _ := testOrca(t)
	o.step(-1)
	err := o.tap()
	if err == nil || !strings.Contains(err.Error(), "cannot be activated") {
		t.Fatalf("folder workspace: got %v", err)
	}
	if len(calls.calls) != 0 {
		t.Fatalf("folder workspace ran %v", calls.calls)
	}
}

func TestOrcaLastReply(t *testing.T) {
	text, err := orcaLastReply([]byte(`{"result":{"entries":[{"kind":"message","role":"assistant","text":"first"},
		{"kind":"message","role":"user","text":"q"},{"kind":"message","role":"assistant","text":"last"},{"kind":"tool","name":"Bash"}]}}`))
	if err != nil || text != "last" {
		t.Fatalf("got %q %v, want the last assistant message", text, err)
	}
}

func TestOrcaScrollOnlyWhileBrowsingAndClamped(t *testing.T) {
	o, _, _ := testOrca(t)
	if o.scroll(1) {
		t.Fatal("idle scroll captured the brightness knob")
	}
	o.run = func(args ...string) ([]byte, error) {
		return []byte(`{"result":{"entries":[{"kind":"message","role":"assistant","text":"` + strings.Repeat("long reply ", 60) + `"}]}}`), nil
	}
	o.step(1)
	o.step(1)
	for {
		o.mu.Lock()
		n := len(o.textLocked(o.shownLocked()))
		o.mu.Unlock()
		if n >= 600 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !o.scroll(-1) || o.scrollTo != 0 {
		t.Fatal("scroll went above the top")
	}
	if o.push != -1 {
		t.Fatal("scroll above the top did not start the back bar")
	}
	o.scroll(1)
	for o.push == 0 {
		o.scroll(1)
	}
	end := o.scrollTo
	o.mu.Lock()
	want := o.scrollEndLocked()
	o.mu.Unlock()
	if end <= 0 || end != want {
		t.Fatalf("scroll not clamped at the end: %v", end)
	}
	o.chatStep(1)
	if o.scrollTo != 0 {
		t.Fatal("a new chat did not start at the top")
	}
}

func TestOrcaIconEmojiThenModel(t *testing.T) {
	for _, c := range []orcaChat{{name: "🎛 tile", model: "claude"}, {name: "tile", model: "claude"}, {name: "tile", model: "openai"}} {
		if b := orcaIconPNG(c); len(b) < 8 || string(b[1:4]) != "PNG" {
			t.Fatalf("%+v: no PNG icon", c)
		}
	}
	if orcaIconPNG(orcaChat{name: "tile"}) != nil {
		t.Fatal("a chat with no emoji and no known model got an icon")
	}
	spaces, err := orcaParse([]byte(orcaFixture))
	if err != nil || spaces[0].chats[0].model != "" {
		t.Fatalf("fixture has no agentType: %v", err)
	}
}

func TestOrcaScrollPushesThroughChats(t *testing.T) {
	o, _, _ := testOrca(t)
	o.run = func(args ...string) ([]byte, error) {
		return []byte(`{"result":{"entries":[{"kind":"message","role":"assistant","text":"short"}]}}`), nil
	}
	o.step(1)
	o.step(1)
	for {
		o.mu.Lock()
		text := o.textLocked(o.shownLocked())
		o.mu.Unlock()
		if text == "short" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !o.scroll(1) || o.push != 1 || o.pushTo != 1.0/chatsPushTurns {
		t.Fatalf("first detent past the end: push %d bar %v", o.push, o.pushTo)
	}
	o.scroll(-1)
	if o.push != -1 {
		t.Fatal("a turn back did not restart the count from the top")
	}
	for range chatsPushTurns {
		o.scroll(1)
	}
	if o.space != "r1::/a" || o.chat != 1 || o.push != 0 || o.pushTo != 0 {
		t.Fatalf("last detent did not move to the next chat: %s %d push %d", o.space, o.chat, o.push)
	}
	for range chatsPushTurns {
		o.scroll(-1)
	}
	o.mu.Lock()
	end := o.scrollEndLocked()
	o.mu.Unlock()
	if o.chat != 0 || o.scrollTo != end {
		t.Fatal("back did not land at the end of the previous chat's reply")
	}
}
