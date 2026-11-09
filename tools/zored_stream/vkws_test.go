package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
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
			t.Logf("%-40s unread=%d muted=%v at=%s text=%q avatar=%dB sender=%dB", e.title, e.unread, e.muted, e.at.Format(time.DateTime), e.text, len(e.avatar), len(e.sender))
		}
		t.Logf("total %d", len(es))
		cancel()
	})
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestVkwsHistoryTextFromParts(t *testing.T) {
	d, err := parseVkwsHistory("bot", []byte(`{"status":{"code":20000},"results":{"messages":[{"msgId":2,"time":5,"parts":[{"mediaType":"text","text":"VKDOC-1\ndone"},{"text":"ok"}]}]}}`))
	if err != nil || d.text != "VKDOC-1 done ok" {
		t.Fatalf("got %q, %v", d.text, err)
	}
}

func TestVkwsMuted(t *testing.T) {
	st := &vkwsState{muted: map[string]bool{"1@chat.agent": false, "2@chat.agent": true, "a@vk.team": false, "1000000569": true}}
	for sn, want := range map[string]bool{"1@chat.agent": false, "2@chat.agent": true, "3@chat.agent": true, "a@vk.team": false, "b@vk.team": false, "1000000569": true} {
		if got := st.isMuted(sn); got != want {
			t.Errorf("%s: muted=%v, want %v", sn, got, want)
		}
	}
}

func TestParseVkwsUserMute(t *testing.T) {
	for body, want := range map[string]bool{
		`{"status":{"code":20000},"results":{"friendly":"JiraBot","mute":true,"bot":true}}`: true,
		`{"status":{"code":20000},"results":{"friendly":"Антон Чертков"}}`:                  false,
	} {
		if got, err := parseVkwsUserMute([]byte(body)); err != nil || got != want {
			t.Errorf("%s: mute=%v, %v, want %v", body, got, err, want)
		}
	}
	if _, err := parseVkwsUserMute([]byte(`{"status":{"code":40400}}`)); err == nil {
		t.Fatal("want error")
	}
	if _, err := parseVkwsUserMute([]byte(`{"status":{"code":40100}}`)); !errors.Is(err, errVkwsSession) {
		t.Fatalf("want errVkwsSession, got %v", err)
	}
}

func TestVkwsReplyShowsOwnTextAndSender(t *testing.T) {
	d, err := parseVkwsHistory("1@chat.agent", []byte(`{"status":{"code":20000},"results":{"messages":[{"msgId":2,"time":5,"chat":{"sender":"b@corp"},"parts":[{"mediaType":"quote","text":"parent"},{"mediaType":"text","text":"answer"}]}]}}`))
	if err != nil || d.text != "answer" || d.sender != "b@corp" {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestVkwsNoSenderForDirectOrOutgoing(t *testing.T) {
	for _, b := range []string{
		`{"status":{"code":20000},"results":{"messages":[{"msgId":2,"time":5,"text":"hi","chat":{"sender":"a@corp"}}]}}`,
		`{"status":{"code":20000},"results":{"messages":[{"msgId":2,"time":5,"text":"hi","outgoing":true,"chat":{"sender":"me@corp"}}]}}`,
		`{"status":{"code":20000},"results":{"messages":[{"msgId":2,"time":5,"text":"hi"}]}}`,
	} {
		if d, err := parseVkwsHistory("a@corp", []byte(b)); err != nil || d.sender != "" {
			t.Errorf("%s: %+v %v", b, d, err)
		}
	}
}

func TestVkwsKnownDialogsPersist(t *testing.T) {
	file := filepath.Join(t.TempDir(), "dir", "vkws-dialogs")
	k, err := loadVkwsKnown(file)
	if err != nil || len(k.list()) != 0 {
		t.Fatalf("fresh: %v, %v", k.list(), err)
	}
	if err := k.add([]string{"b@vk.team", "a@vk.team", "b@vk.team"}); err != nil {
		t.Fatal(err)
	}
	k, err = loadVkwsKnown(file)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(k.list(), ","); got != "a@vk.team,b@vk.team" {
		t.Fatalf("got %q", got)
	}
}
