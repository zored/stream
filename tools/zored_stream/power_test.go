package main

import (
	"strings"
	"testing"
	"time"
)

func TestAwakeLeft(t *testing.T) {
	for left, want := range map[time.Duration]string{
		time.Hour:                                ">1:00<",
		time.Hour + 41*time.Minute + time.Second: ">1:42<",
		30 * time.Second:                         ">0:01<",
	} {
		if got := awakeLeft(left, false); !strings.Contains(got, want) {
			t.Errorf("awakeLeft(%v) = %s, want %s", left, got, want)
		}
	}
}
