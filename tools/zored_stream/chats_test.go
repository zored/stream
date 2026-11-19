package main

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"
)

func TestChatsMergeLoudFirstRecentFirstDropsRead(t *testing.T) {
	now := time.Now()
	got := chatsMerge(map[string][]chatEntry{
		"tg":   {{id: "old", unread: 1, at: now.Add(-time.Hour)}, {id: "read", at: now}},
		"vkws": {{id: "new", unread: 2, at: now.Add(-time.Minute)}, {id: "muted", unread: 5, at: now, muted: true}},
	})
	if len(got) != 3 || got[0].id != "new" || got[1].id != "old" || got[2].id != "muted" {
		t.Fatalf("got %+v", got)
	}
}

func TestChatsBrowseThenOpenPick(t *testing.T) {
	var opened string
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = post })
	c := newChats(&vars{sent: map[string]string{}}, func() {}, func(string) {})
	c.opener = func(url string) error { opened = url; return nil }
	now := time.Now()
	c.set("tg", []chatEntry{{id: "a", unread: 1, at: now, open: "tg://a"}, {id: "b", unread: 1, at: now.Add(-time.Minute), open: "tg://b"}})
	if c.turn(1) {
		t.Fatal("turn outside browse must pass through")
	}
	if err := c.press(); err != nil || opened != "" {
		t.Fatalf("first press must browse, opened %q", opened)
	}
	c.step(1)
	c.step(1)
	if err := c.press(); err != nil || opened != "tg://b" {
		t.Fatalf("opened %q, err %v", opened, err)
	}
	if c.browsing() {
		t.Fatal("open must end browse")
	}
}

func TestChatsInitialSkipsSpaceAndLowercases(t *testing.T) {
	if got := chatsInitial("  Робот"); got != "р" {
		t.Fatalf("got %q", got)
	}
}

func TestChatsRepublishesAfterFailedPost(t *testing.T) {
	var posts []string
	fail := true
	postVar = func(name, value string) error {
		if name == "unread_svg" {
			posts = append(posts, value)
		}
		if fail {
			return fmt.Errorf("413")
		}
		return nil
	}
	t.Cleanup(func() { postVar = post })
	c := newChats(&vars{sent: map[string]string{}}, func() {}, func(string) {})
	c.publish()
	fail = false
	c.publish()
	c.publish()
	if len(posts) != 2 || posts[0] != posts[1] {
		t.Fatalf("posts %d", len(posts))
	}
}

func TestChatsLines(t *testing.T) {
	if got := chatsLines("Команда бэкенда — VKDoc", 3, 4); !slices.Equal(got, []string{"кома", "бэке", "vkdo"}) {
		t.Fatal(got)
	}
	if got := chatsLines("!!", 2, 4); !slices.Equal(got, []string{"·"}) {
		t.Fatal(got)
	}
}

func TestChatsSenderBottomLeft(t *testing.T) {
	if chatsSender(chatEntry{}, 0, 0, 88, 13) != nil {
		t.Fatal("no sender, no disc")
	}
	d := chatsSender(chatEntry{sender: []byte{1}}, 10, 20, 88, 13)
	if len(d) != 1 || d[0].cx != 23 || d[0].cy != 95 || d[0].r != 13 {
		t.Fatalf("%+v", d)
	}
}

func TestChatsKnobTurnStartsBrowse(t *testing.T) {
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = post })
	c := newChats(&vars{sent: map[string]string{}}, func() {}, func(string) {})
	now := time.Now()
	c.set("tg", []chatEntry{{id: "a", unread: 1, at: now}, {id: "b", unread: 1, at: now.Add(-time.Minute)}})
	c.knobTurn(1)
	if !c.browsing() || c.pick != 0 {
		t.Fatalf("browsing=%v pick=%d", c.browsing(), c.pick)
	}
	c.knobTurn(1)
	if c.pick != 1 {
		t.Fatalf("pick=%d", c.pick)
	}
}

func TestChatsViewWalksMessagesBadgeIsRemainder(t *testing.T) {
	e := chatEntry{unread: 3, text: "last", msgs: []chatMsg{{text: "a", sender: []byte{1}}, {text: "b"}, {text: "c"}}}
	if v := chatsView(e, 0); v.text != "a" || v.unread != 2 || len(v.sender) != 1 {
		t.Fatalf("%+v", v)
	}
	if v := chatsView(e, 9); v.text != "c" || v.unread != 0 || v.sender != nil {
		t.Fatalf("%+v", v)
	}
	if v := chatsView(chatEntry{unread: 4, text: "x"}, 0); v.text != "x" || v.unread != 4 {
		t.Fatalf("%+v", v)
	}
}

func TestChatsMsgTurnOnlyWhileBrowsing(t *testing.T) {
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = post })
	c := newChats(&vars{sent: map[string]string{}}, func() {}, func(string) {})
	now := time.Now()
	c.set("tg", []chatEntry{{id: "a", unread: 2, at: now, msgs: []chatMsg{{text: "1"}, {text: "2"}}}, {id: "b", unread: 1, at: now.Add(-time.Minute)}})
	if c.msgTurn(1) || c.turn(1) {
		t.Fatal("right knobs must pass through outside browse")
	}
	c.knobTurn(1)
	if !c.msgTurn(1) || c.msg != 1 || !c.msgTurn(1) || c.msg != 1 {
		t.Fatalf("msg=%d", c.msg)
	}
	c.msgTurn(-1)
	c.msgTurn(-1)
	if c.msg != 0 {
		t.Fatalf("msg=%d", c.msg)
	}
	c.msgTurn(1)
	c.step(1)
	if c.pick != 1 || c.msg != 0 {
		t.Fatalf("pick=%d msg=%d", c.pick, c.msg)
	}
}

func TestChatsHoldMarksReadAndEatsRelease(t *testing.T) {
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = post })
	c := newChats(&vars{sent: map[string]string{}}, func() {}, func(string) {})
	var opened string
	c.opener = func(url string) error { opened = url; return nil }
	read := make(chan string, 1)
	now := time.Now()
	reader := func(id string) func(context.Context) error {
		return func(context.Context) error { read <- id; return nil }
	}
	list := []chatEntry{{app: "tg", id: "a", unread: 1, at: now, open: "a", read: reader("a")}, {app: "tg", id: "b", unread: 1, at: now.Add(-time.Minute), open: "b", read: reader("b")}}
	c.set("tg", list)
	c.down()
	time.Sleep(chatsHoldFor + 100*time.Millisecond)
	if err := c.up(); err != nil || opened != "" || !c.browsing() || len(read) != 0 {
		t.Fatal("hold outside browse: release must start browse only, nothing marked")
	}
	c.down()
	time.Sleep(chatsHoldFor + 100*time.Millisecond)
	if err := c.up(); err != nil || opened != "" {
		t.Fatalf("release after hold must do nothing, opened %q", opened)
	}
	if got := <-read; got != "a" {
		t.Fatalf("read %q", got)
	}
	if len(c.list) != 1 || c.list[0].id != "b" || !c.browsing() {
		t.Fatalf("list %+v browsing %v", c.list, c.browsing())
	}
	c.set("tg", list)
	if len(c.list) != 1 {
		t.Fatal("a stale push must not bring a read chat back")
	}
	list[0].at = now.Add(time.Second)
	c.set("tg", list)
	if len(c.list) != 2 {
		t.Fatal("a newer message must bring it back")
	}
	c.down()
	if err := c.up(); err != nil || opened != "a" {
		t.Fatalf("tap in browse must open, opened %q", opened)
	}
}
