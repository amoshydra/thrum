package tui

import (
	"strings"
	"testing"
)

func TestKeyAction(t *testing.T) {
	tests := []struct {
		name string
		k    key
		when Model
		want action
	}{
		{"q quits", key{r: 'q'}, Model{}, action{kind: actQuit}},
		{"ctrl-c quits", key{r: 3}, Model{}, action{kind: actQuit}},
		{"esc quits", key{r: 27}, Model{}, action{kind: actQuit}},

		{"up raises by 2", key{arrow: arrowUp}, Model{}, action{kind: actVolume, delta: 2}},
		{"down lowers by 2", key{arrow: arrowDown}, Model{}, action{kind: actVolume, delta: -2}},
		{"right raises by 10", key{arrow: arrowRight}, Model{}, action{kind: actVolume, delta: 10}},
		{"left lowers by 10", key{arrow: arrowLeft}, Model{}, action{kind: actVolume, delta: -10}},
		{"plus raises by 2", key{r: '+'}, Model{}, action{kind: actVolume, delta: 2}},
		{"equals raises by 2", key{r: '='}, Model{}, action{kind: actVolume, delta: 2}},
		{"minus lowers by 2", key{r: '-'}, Model{}, action{kind: actVolume, delta: -2}},
		{"bracket raises by 10", key{r: ']'}, Model{}, action{kind: actVolume, delta: 10}},
		{"bracket lowers by 10", key{r: '['}, Model{}, action{kind: actVolume, delta: -10}},

		{"m mutes", key{r: 'm'}, Model{}, action{kind: actMute}},
		{"s stops", key{r: 's'}, Model{}, action{kind: actStop}},
		{"n skips", key{r: 'n'}, Model{}, action{kind: actNext}},
		{"b goes back", key{r: 'b'}, Model{}, action{kind: actPrevious}},
		{"r retunes", key{r: 'r'}, Model{}, action{kind: actRetune}},

		// Space is context sensitive: it pauses a playing room and plays a
		// paused one.
		{"space pauses while playing", key{r: ' '}, Model{State: "PLAYING"}, action{kind: actPause}},
		{"space plays while paused", key{r: ' '}, Model{State: "PAUSED_PLAYBACK"}, action{kind: actPlay}},
		{"p plays while stopped", key{r: 'p'}, Model{State: "STOPPED"}, action{kind: actPlay}},

		{"unknown key does nothing", key{r: 'z'}, Model{}, action{kind: actNone}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := keyAction(tc.k, tc.when); got != tc.want {
				t.Errorf("keyAction(%v) = %+v, want %+v", tc.k, got, tc.want)
			}
		})
	}
}

func TestDecodeKeys(t *testing.T) {
	var got []key
	decodeKeys(strings.NewReader("\x1b[A\x1b[B\x1b[C\x1b[Dmq"), func(k key) {
		got = append(got, k)
	})

	want := []key{
		{arrow: arrowUp},
		{arrow: arrowDown},
		{arrow: arrowRight},
		{arrow: arrowLeft},
		{r: 'm'},
		{r: 'q'},
	}
	if len(got) != len(want) {
		t.Fatalf("decoded %d keys, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("key %d = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestDecodeKeysTreatsBareEscapeAsEsc(t *testing.T) {
	var got []key
	decodeKeys(strings.NewReader("\x1b"), func(k key) { got = append(got, k) })

	if len(got) != 1 || got[0].r != 27 {
		t.Fatalf("got %v, want a single Esc key", got)
	}
	if keyAction(got[0], Model{}).kind != actQuit {
		t.Error("a bare Esc should quit")
	}
}

// A sequence split across reads cannot be told apart from Esc followed by a
// '[' keypress, so the decoder reports what actually arrived instead of
// guessing and blocking for more bytes.
func TestDecodeKeysDoesNotGuessASplitSequence(t *testing.T) {
	var got []key
	decodeKeys(strings.NewReader("\x1b["), func(k key) { got = append(got, k) })

	if len(got) != 2 || got[0].r != 27 || got[1].r != '[' {
		t.Fatalf("got %v, want Esc then '['", got)
	}
	if keyAction(got[0], Model{}).kind != actQuit {
		t.Error("the leading Esc should quit")
	}
}
