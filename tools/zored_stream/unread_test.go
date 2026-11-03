package main

import (
	"testing"
	"time"
)

func TestUnreadTiles(t *testing.T) {
	postVar = func(string, string) error { return nil }
	u := newUnread(&vars{sent: map[string]string{}})
	now := time.Now()
	u.update(now.Add(-time.Hour), map[string]string{})
	u.update(now, map[string]string{"Telegram": "1"})
	u.update(now.Add(time.Minute), map[string]string{"Telegram": "1", "VK WorkSpace Beta": "•"})
	got := u.waiting()
	fresh := newUnread(&vars{sent: map[string]string{}})
	fresh.update(now, map[string]string{"Telegram": "3"})
	if !fresh.apps[0].lastAt.IsZero() {
		t.Fatal("badges present at start have no known age")
	}
	if len(got) != 2 || got[1].badge != 1 || !got[0].lastAt.Equal(now) {
		t.Fatalf("waiting: %+v", got)
	}
	u.update(now.Add(2*time.Minute), map[string]string{})
	if u.vars.sent["unread_svg"] != unreadSVG(nil, now) {
		t.Fatal("no badges must show the eye")
	}
}

func TestUnreadLabels(t *testing.T) {
	now := time.Now()
	for _, c := range []struct{ got, want string }{
		{capped(150, 99), "99+"},
		{ago(now.Add(-5*time.Minute), now), "5m"},
		{ago(now.Add(-3*time.Hour), now), "3h"},
		{ago(now.Add(-30*24*time.Hour), now), "9+d"},
		{ago(time.Time{}, now), ""},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}
