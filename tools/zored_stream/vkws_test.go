package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

func TestParseVkwsEvents(t *testing.T) {
	b := []byte(`{"response":{"statusCode":200,"data":{"fetchBaseURL":"https://x/fetch?seqNum=2","timeToNextFetch":500,"events":[
		{"type":"buddylist","eventData":{"groups":[{"name":"g","buddies":[
			{"aimId":"a@corp.mail.ru","friendly":"Anna"},
			{"aimId":"1@chat.agent","friendly":"Team","mute":1280},
			{"aimId":"gone@corp.mail.ru","deleted":true}]}]}},
		{"type":"histDlgState","eventData":{"sn":"1@chat.agent","unreadCnt":3}},
		{"type":"presence","eventData":{"aimId":"a@corp.mail.ru"}}]}}}`)
	ev, err := parseVkwsEvents(b)
	if err != nil {
		t.Fatal(err)
	}
	if ev.next != "https://x/fetch?seqNum=2" || ev.wait != 500*time.Millisecond {
		t.Fatalf("next %q wait %v", ev.next, ev.wait)
	}
	want := []vkwsBuddy{{sn: "a@corp.mail.ru", name: "Anna"}, {sn: "1@chat.agent", name: "Team", muted: true}}
	if len(ev.buddies) != 2 || ev.buddies[0] != want[0] || ev.buddies[1] != want[1] {
		t.Fatalf("buddies %+v", ev.buddies)
	}
	if len(ev.changed) != 1 || ev.changed[0] != "1@chat.agent" {
		t.Fatalf("changed %v", ev.changed)
	}
}

func TestParseVkwsEventsExpired(t *testing.T) {
	_, err := parseVkwsEvents([]byte(`{"response":{"statusCode":401}}`))
	if !errors.Is(err, errVkwsSession) {
		t.Fatalf("err %v", err)
	}
}

func TestParseVkwsHistory(t *testing.T) {
	b := []byte(`{"status":{"code":20000},"results":{"yours":{"lastRead":11},"messages":[
		{"msgId":10,"time":100,"text":"old"},
		{"msgId":12,"time":200,"outgoing":true,"text":"mine"},
		{"msgId":13,"time":300,"class":"event","text":"joined"},
		{"msgId":14,"time":400,"text":"  hello\n  world "}],
		"persons":[{"sn":"a@corp.mail.ru","friendly":"Anna"}]}}`)
	d, err := parseVkwsHistory("a@corp.mail.ru", b)
	if err != nil {
		t.Fatal(err)
	}
	if d.unread != 1 || d.text != "hello world" || d.at.Unix() != 400 || d.name != "Anna" {
		t.Fatalf("%+v", d)
	}
}

func TestParseVkwsHistoryUnreadCnt(t *testing.T) {
	d, err := parseVkwsHistory("x", []byte(`{"status":{"code":20000},"results":{"unreadCnt":120,"yours":{"lastRead":1},"messages":[{"msgId":2,"time":1,"text":"a"}]}}`))
	if err != nil || d.unread != 120 {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestParseVkwsHistoryStatus(t *testing.T) {
	if _, err := parseVkwsHistory("x", []byte(`{"status":{"code":40000}}`)); err == nil {
		t.Fatal("want error")
	}
}

func TestVkwsLive(t *testing.T) {
	if os.Getenv("VKWS_LIVE") == "" {
		t.Skip("VKWS_LIVE=1 to list unread VK WorkSpace chats once")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	err := vkwsChats(ctx, func(es []chatEntry) {
		for _, e := range es {
			t.Logf("%-40s unread=%d at=%s text=%dch avatar=%dB", e.title, e.unread, e.at.Format(time.DateTime), len(e.text), len(e.avatar))
		}
		t.Logf("total %d", len(es))
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}
