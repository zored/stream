package main

import (
	"context"
	"encoding/base64"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/tg"
)

func TestTelethonSession(t *testing.T) {
	raw := []byte{2, 149, 154, 167, 50, 1, 187}
	key := make([]byte, 256)
	key[0] = 7
	s := "1" + base64.URLEncoding.EncodeToString(append(raw, key...))
	d, err := session.TelethonSession(s)
	if err != nil {
		t.Fatal(err)
	}
	if d.DC != 2 || d.Addr != "149.154.167.50:443" || d.AuthKey[0] != 7 || len(d.AuthKeyID) != 8 {
		t.Fatalf("got %+v", d)
	}
	if _, err := session.TelethonSession("2" + s[1:]); err == nil {
		t.Fatal("bad version accepted")
	}
}

func TestTGEntries(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	dlg := func(p tg.PeerClass, top, unread int, folder int, mute int) *tg.Dialog {
		d := &tg.Dialog{Peer: p, TopMessage: top, UnreadCount: unread, FolderID: folder}
		if mute > 0 {
			d.NotifySettings.SetMuteUntil(mute)
		}
		return d
	}
	d := &tg.MessagesDialogs{
		Dialogs: []tg.DialogClass{
			dlg(&tg.PeerUser{UserID: 1}, 10, 2, 0, 0),
			dlg(&tg.PeerUser{UserID: 2}, 11, 1, 0, 0),
			dlg(&tg.PeerChat{ChatID: 3}, 12, 5, 0, 0),
			dlg(&tg.PeerChannel{ChannelID: 4}, 13, 1, 0, 0),
			dlg(&tg.PeerChannel{ChannelID: 5}, 14, 9, 0, 2_000_000),
			dlg(&tg.PeerUser{UserID: 6}, 15, 3, 1, 0),
			dlg(&tg.PeerUser{UserID: 7}, 16, 0, 0, 0),
		},
		Messages: []tg.MessageClass{
			&tg.Message{ID: 10, PeerID: &tg.PeerUser{UserID: 1}, Date: 100, Message: "hi\nthere"},
			&tg.Message{ID: 11, PeerID: &tg.PeerUser{UserID: 2}, Date: 300, Media: &tg.MessageMediaPhoto{}},
			&tg.Message{ID: 12, PeerID: &tg.PeerChat{ChatID: 3}, Date: 200, Message: "g"},
			&tg.Message{ID: 13, PeerID: &tg.PeerChannel{ChannelID: 4}, Date: 50, Message: "c"},
		},
		Chats: []tg.ChatClass{
			&tg.Chat{ID: 3, Title: "group"},
			&tg.Channel{ID: 4, Title: "chan"},
			&tg.Channel{ID: 5, Title: "muted"},
		},
		Users: []tg.UserClass{
			&tg.User{ID: 1, FirstName: "Ann", LastName: "B", Username: "ann"},
			&tg.User{ID: 2, FirstName: "Bob"},
			&tg.User{ID: 6, FirstName: "Arch"},
			&tg.User{ID: 7, FirstName: "Read"},
		},
	}
	got, muted := tgEntries(d, now)
	want := []chatEntry{
		{app: "tg", id: "u2", title: "Bob", unread: 1, at: time.Unix(300, 0), text: "photo", open: "tg://user?id=2"},
		{app: "tg", id: "c3", title: "group", unread: 5, at: time.Unix(200, 0), text: "g", open: "tg://openmessage?chat_id=3"},
		{app: "tg", id: "u1", title: "Ann B", unread: 2, at: time.Unix(100, 0), text: "hi there", open: "tg://resolve?domain=ann"},
		{app: "tg", id: "ch4", title: "chan", unread: 1, at: time.Unix(50, 0), text: "c", open: "tg://privatepost?channel=4&post=13"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries: %+v", len(got), got)
	}
	for i := range want {
		if got[i].id != want[i].id || got[i].title != want[i].title || got[i].unread != want[i].unread ||
			!got[i].at.Equal(want[i].at) || got[i].text != want[i].text || got[i].open != want[i].open {
			t.Errorf("%d: got %+v want %+v", i, got[i], want[i])
		}
	}
	if !muted["ch5"] || len(muted) != 1 {
		t.Errorf("muted %v", muted)
	}
	if tgRelevant(&tg.UpdateShort{Update: &tg.UpdateNewChannelMessage{Message: &tg.Message{PeerID: &tg.PeerChannel{ChannelID: 5}}}}, muted) {
		t.Error("muted update relevant")
	}
	if !tgRelevant(&tg.UpdateShortMessage{UserID: 1}, muted) {
		t.Error("dm not relevant")
	}
}

func TestTGLive(t *testing.T) {
	if os.Getenv("TG_LIVE") == "" {
		t.Skip("TG_LIVE=1 to run against the live account")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	pushed := false
	err := tgChats(ctx, func(es []chatEntry) {
		pushed = true
		for _, e := range es {
			t.Logf("%-14s unread=%-4d age=%-10s avatar=%-6d text=%-4d open=%s", e.id, e.unread,
				time.Since(e.at).Round(time.Minute), len(e.avatar), len(e.text), e.open[:strings.Index(e.open, "?")])
		}
		t.Logf("%d chats", len(es))
		cancel()
	})
	if !pushed {
		t.Fatal(err)
	}
}
