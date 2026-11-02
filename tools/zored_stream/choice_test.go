package main

import "testing"

func options(n int, selected ...int) []choiceOption {
	out := make([]choiceOption, n)
	for _, i := range selected {
		out[i].Selected = true
	}
	return out
}

func TestChoiceAction(t *testing.T) {
	single := choiceCard{Options: options(3), Index: 1, Count: 1}
	multi := choiceCard{Options: options(3, 1), Multi: true, Index: 1, Count: 1}
	second := choiceCard{Options: options(7), Index: 2, Count: 3}
	seven := choiceCard{Options: options(7), Index: 1, Count: 1}
	tests := []struct {
		name string
		card choiceCard
		b    int
		act  choiceAct
		i    int
	}{
		{"skip on first", single, 1, actSkip, 0},
		{"first option", single, 2, actChoose, 0},
		{"third option", single, 4, actChoose, 2},
		{"past options", single, 5, actNone, 0},
		{"button 8 is option 7 without nav", seven, 8, actChoose, 6},
		{"button 8 finishes multi", multi, 8, actForward, 0},
		{"back on second question", second, 1, actBack, 0},
		{"button 8 pages forward", second, 8, actForward, 0},
		{"button 7 is option 6 with nav", second, 7, actChoose, 5},
	}
	for _, tt := range tests {
		act, i := tt.card.action(tt.b)
		if act != tt.act || i != tt.i {
			t.Errorf("%s: got %v %d, want %v %d", tt.name, act, i, tt.act, tt.i)
		}
	}
}

func TestChoiceLEDs(t *testing.T) {
	multi := choiceCard{Options: options(2, 1), Multi: true, Index: 1, Count: 1}
	want := [choiceButtons]int{choiceNav, choiceOff, choiceOn, 0, 0, 0, 0, choiceWhite}
	if got := multi.leds(); got != want {
		t.Errorf("multi: got %x, want %x", got, want)
	}
	paged := choiceCard{Options: options(1), Index: 1, Count: 2}
	want = [choiceButtons]int{choiceNav, choiceWhite, 0, 0, 0, 0, 0, choiceNav}
	if got := paged.leds(); got != want {
		t.Errorf("paged: got %x, want %x", got, want)
	}
}
