package main

import "testing"

func TestModesNewOneClosesOthers(t *testing.T) {
	open := map[string]bool{}
	var closed []string
	md := func(name string) mode {
		return mode{name: name, active: func() bool { return open[name] }, close: func() { closed = append(closed, name); open[name] = false }}
	}
	m := &modes{was: map[string]bool{}}
	for _, name := range []string{"emoji", "chats", "wifi"} {
		m.add(md(name))
	}
	open["emoji"] = true
	m.check()
	if len(closed) != 0 {
		t.Fatal(closed)
	}
	m.check()
	open["chats"] = true
	m.check()
	if len(closed) != 1 || closed[0] != "emoji" || open["emoji"] {
		t.Fatalf("emoji must close when chats open: %v", closed)
	}
	open["emoji"] = true
	m.check()
	if len(closed) != 2 || closed[1] != "chats" {
		t.Fatalf("chats must close when emoji opens: %v", closed)
	}
	open["emoji"] = false
	m.check()
	open["wifi"], open["chats"] = true, true
	m.check()
	if len(closed) != 2 {
		t.Fatalf("two modes opening together stay: %v", closed)
	}
}
