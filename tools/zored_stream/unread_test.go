package main

import (
	"testing"
	"time"
)

func TestUnreadAttribution(t *testing.T) {
	postVar = func(string, string) error { return nil }
	u := newUnread(&vars{sent: map[string]string{}})
	now := time.Now()
	u.update(now, map[string]string{"Telegram": "1"}, [][2]string{{"a", "Мама"}})
	u.update(now.Add(time.Second), map[string]string{"Telegram": "2"}, [][2]string{{"a", "Мама"}, {"b", "Работа"}})
	u.update(now.Add(time.Minute), map[string]string{"Telegram": "2"}, [][2]string{{"c", "Чужое"}})
	got := u.items()
	if len(got) != 2 || got[0].text != "Работа" || got[1].text != "Мама" {
		t.Fatalf("items: %+v", got)
	}
	u.update(now.Add(2*time.Minute), map[string]string{}, nil)
	if u.active() || u.vars.sent["unread_svg"] != "" {
		t.Fatal("zero badge must clear the tile")
	}
}

func TestUnreadLetterFallback(t *testing.T) {
	postVar = func(string, string) error { return nil }
	u := newUnread(&vars{sent: map[string]string{}})
	u.update(time.Now(), map[string]string{"VK WorkSpace Beta": "•"}, nil)
	it, _, _, ok := u.current()
	if !ok || it.text != "w" || it.app.badge != 1 {
		t.Fatalf("got %+v", it)
	}
}
