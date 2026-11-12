package main

import (
	"encoding/base64"
	"math"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestAgentOrder(t *testing.T) {
	t0 := time.Unix(1000, 0)
	list := []*agentSession{
		{ID: "old", Kind: agentInput, Updated: t0},
		{ID: "kid-new", Kind: agentDone, Updated: t0.Add(3 * time.Minute), Child: true},
		{ID: "new", Kind: agentWorking, Updated: t0.Add(2 * time.Minute)},
		{ID: "kid-old", Kind: agentInput, Updated: t0, Child: true},
		{ID: "mid", Kind: agentRead, Updated: t0.Add(time.Minute)},
	}
	slices.SortFunc(list, agentLess)
	var got []string
	for _, s := range list {
		got = append(got, s.ID)
	}
	if want := []string{"kid-old", "old", "kid-new", "new", "mid"}; !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestDesktopDone(t *testing.T) {
	for _, c := range []struct {
		s    agentSession
		want bool
	}{
		{agentSession{Kind: agentDone, Host: "local_abc-1"}, true},
		{agentSession{Kind: agentInput, Host: "local_abc-1"}, false},
		{agentSession{Kind: agentDone}, false},
		{agentSession{Kind: agentDone, Host: "local_a;rm"}, false},
	} {
		if got := c.s.desktopDone(); got != c.want {
			t.Errorf("%+v: got %v want %v", c.s, got, c.want)
		}
	}
}

func TestAgentLayout(t *testing.T) {
	for _, sizes := range [][]int{{1}, {3}, {1, 1, 1, 1, 1}, {2, 1, 4, 1}, {1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1}} {
		pos := agentLayout(sizes)
		for i, p := range pos {
			if p.x-p.r-2.5 < 4 || p.x+p.r+2.5 > 96 || p.y-p.r-2.5 < 4 || p.y+p.r+2.5 > 96 {
				t.Errorf("%v: dot %d %+v leaves the tile", sizes, i, p)
			}
			for j, q := range pos[:i] {
				if math.Hypot(p.x-q.x, p.y-q.y) < p.r+q.r {
					t.Errorf("%v: dots %d and %d overlap", sizes, j, i)
				}
			}
		}
	}
	if one, five := agentLayout([]int{1})[0].r, agentLayout([]int{1, 1, 1, 1, 1})[0].r; one <= five {
		t.Errorf("a lone dot r=%.1f is not bigger than one of five r=%.1f", one, five)
	}
}

func TestAgentGroups(t *testing.T) {
	at := func(m int) time.Time { return time.Unix(int64(m)*60, 0) }
	nodes := []agentNode{
		{&agentSession{ID: "old", Updated: at(1)}, "old", 0},
		{&agentSession{ID: "parent", Updated: at(2)}, "parent", 0},
		{&agentSession{ID: "child", Updated: at(3)}, "parent", 1},
		{&agentSession{ID: "grandchild", Updated: at(4)}, "parent", 2},
		{&agentSession{ID: "child2", Updated: at(5)}, "parent", 1},
		{&agentSession{ID: "mid", Updated: at(4)}, "mid", 0},
	}
	list, sizes := agentGroups(nodes)
	var ids []string
	for _, s := range list {
		ids = append(ids, s.ID)
	}
	want := []string{"grandchild", "child2", "child", "parent", "mid", "old"}
	if !slices.Equal(ids, want) || !slices.Equal(sizes, []int{4, 1, 1}) {
		t.Errorf("got %v %v, want %v [4 1 1]", ids, sizes, want)
	}
}

func TestFrontDesktop(t *testing.T) {
	recs := map[string]desktopRecord{
		"local_a": {SessionID: "local_a", LastFocusedAt: 10},
		"local_b": {SessionID: "local_b", LastFocusedAt: 30, IsArchived: true},
		"local_c": {SessionID: "local_c", LastFocusedAt: 20},
	}
	if got := frontDesktop(recs); got != "local_c" {
		t.Errorf("got %q", got)
	}
	if got := frontDesktop(nil); got != "" {
		t.Errorf("empty: got %q", got)
	}
	if got := itermUID("w0t1p0:AB-12"); got != "AB-12" {
		t.Errorf("iterm uid %q", got)
	}
}

func TestAgentStep(t *testing.T) {
	for _, c := range []struct{ i, d, n, want int }{
		{-1, 1, 3, 0}, {-1, -1, 3, 2}, {0, 1, 3, 1}, {2, 1, 3, 0}, {0, -1, 3, 2}, {1, -2, 3, 2},
	} {
		if got := agentStep(c.i, c.d, c.n); got != c.want {
			t.Errorf("agentStep(%d,%d,%d) = %d, want %d", c.i, c.d, c.n, got, c.want)
		}
	}
}

func TestTitleEmoji(t *testing.T) {
	for title, want := range map[string]string{
		"🟢 59599: зелёный | 15 окт 🎫": "🟢",
		"⚙️ суть | 28 окт":            "⚙️",
		"👩‍💻 код":                     "👩‍💻",
		"🎛 stream: суть":              "🎛️",
		"misc: суть":                  "",
		"":                            "",
	} {
		if got := titleEmoji(title); got != want {
			t.Errorf("titleEmoji(%q) = %q, want %q", title, got, want)
		}
	}
}

func TestAgentsSVGEmoji(t *testing.T) {
	uri := agentsSVG([]agentDotSpec{{agentInput, "🟢"}, {agentRead, ""}}, []int{1, 1}, -1, -1, 1)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(uri, "data:image/svg+xml;base64,"))
	if err != nil {
		t.Fatal(err)
	}
	svg := string(raw)
	if !strings.Contains(svg, ">🟢</text>") || !strings.Contains(svg, `fill="`+amber+`"`) || !strings.Contains(svg, `stroke="`+gray+`"`) {
		t.Fatalf("want amber-backed emoji and a hollow dot: %s", svg)
	}
}

func TestAgentRowPos(t *testing.T) {
	want := []float64{0, emojiPitch, 2*emojiPitch + agentGroupGap, 3*emojiPitch + 2*agentGroupGap}
	if got := agentRowPos([]int{2, 1, 1}); !slices.Equal(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}
