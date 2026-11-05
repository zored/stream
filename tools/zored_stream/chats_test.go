package main

import (
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
