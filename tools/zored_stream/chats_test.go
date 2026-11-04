package main

import (
	"strings"
	"testing"
	"time"
)

func TestChatsMergeSortsRecentFirstAndDropsRead(t *testing.T) {
	now := time.Now()
	got := chatsMerge(map[string][]chatEntry{
		"tg":   {{id: "old", unread: 1, at: now.Add(-time.Hour)}, {id: "read", at: now}},
		"vkws": {{id: "new", unread: 2, at: now}},
	})
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

func TestChatsTileUsesInitialWithoutAvatar(t *testing.T) {
	svg := chatsAvatar(chatEntry{title: "  Робот"}, 10, 10, 5, "x")
	if !strings.Contains(svg, ">р<") {
		t.Fatalf("got %s", svg)
	}
}
