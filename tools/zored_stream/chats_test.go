package main

import (
	"testing"
	"time"
)

func TestChatsMergeSortsRecentFirstAndDropsRead(t *testing.T) {
	now := time.Now()
	got := chatsMerge(map[string][]chatEntry{
		"tg":   {{id: "old", unread: 1, at: now.Add(-time.Hour)}, {id: "read", at: now}},
		"vkws": {{id: "new", unread: 2, at: now}, {id: "muted", unread: 5, at: now, muted: true}},
	}, false)
	if len(got) != 2 || got[0].id != "new" || got[1].id != "old" {
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
	c.turn(1)
	c.turn(1)
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

func TestChatsHoldTogglesMutedWithoutPress(t *testing.T) {
	postVar = func(string, string) error { return nil }
	t.Cleanup(func() { postVar = post })
	c := newChats(&vars{sent: map[string]string{}}, func() {}, func(string) {})
	c.set("tg", []chatEntry{{id: "loud", unread: 1}, {id: "quiet", unread: 1, muted: true}})
	c.down()
	time.Sleep(chatsHold + 100*time.Millisecond)
	c.up()
	if !c.muted || c.browsing() || len(c.list) != 1 || c.list[0].id != "quiet" {
		t.Fatalf("muted=%v browsing=%v list=%+v", c.muted, c.browsing(), c.list)
	}
	c.down()
	c.up()
	if !c.browsing() {
		t.Fatal("short press must browse")
	}
}
