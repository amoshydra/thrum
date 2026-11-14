package tui

import (
	"bufio"
	"io"
)

// arrow is one of the four cursor keys.
type arrow int

const (
	arrowNone arrow = iota
	arrowUp
	arrowDown
	arrowLeft
	arrowRight
)

// key is one logical keystroke: a byte the user typed, or an arrow key.
type key struct {
	r     rune
	arrow arrow
}

type kind int

const (
	actNone kind = iota
	actVolume
	actMute
	actPlay
	actPause
	actStop
	actNext
	actPrevious
	actRetune
	actQuit
)

// action is what a key asks for, decided without touching the speaker.
type action struct {
	kind  kind
	delta int // actVolume only
}

const (
	stepSmall = 2
	stepLarge = 10
)

// keyAction maps a keystroke to an action. It reads the current state only
// where the meaning depends on it: space is pause while playing, play
// otherwise.
func keyAction(k key, m Model) action {
	switch k.arrow {
	case arrowUp:
		return action{kind: actVolume, delta: stepSmall}
	case arrowDown:
		return action{kind: actVolume, delta: -stepSmall}
	case arrowRight:
		return action{kind: actVolume, delta: stepLarge}
	case arrowLeft:
		return action{kind: actVolume, delta: -stepLarge}
	}

	switch k.r {
	case 'q', 'Q', 3, 27: // 3 is Ctrl+C, 27 a lone Esc; raw mode delivers both as bytes
		return action{kind: actQuit}
	case '+', '=':
		return action{kind: actVolume, delta: stepSmall}
	case '-':
		return action{kind: actVolume, delta: -stepSmall}
	case ']':
		return action{kind: actVolume, delta: stepLarge}
	case '[':
		return action{kind: actVolume, delta: -stepLarge}
	case 'm', 'M':
		return action{kind: actMute}
	case ' ':
		if m.State == "PLAYING" {
			return action{kind: actPause}
		}
		return action{kind: actPlay}
	case 'p', 'P':
		if m.State == "PLAYING" {
			return action{kind: actPause}
		}
		return action{kind: actPlay}
	case 's', 'S':
		return action{kind: actStop}
	case 'n', 'N':
		return action{kind: actNext}
	case 'b', 'B':
		return action{kind: actPrevious}
	case 'r', 'R':
		return action{kind: actRetune}
	}
	return action{kind: actNone}
}

// decodeKeys turns raw terminal bytes into keys and hands each to emit until
// the reader ends. Arrow keys arrive as ESC [ X in a single write, so a lone
// ESC with nothing behind it is the Esc key.
func decodeKeys(r io.Reader, emit func(key)) {
	br := bufio.NewReader(r)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return
		}
		if b != 27 {
			emit(key{r: rune(b)})
			continue
		}

		// A terminal sends the whole sequence at once, so if this is really an
		// arrow key the rest is already buffered.
		if br.Buffered() < 2 {
			emit(key{r: 27})
			continue
		}
		open, err := br.ReadByte()
		if err != nil {
			return
		}
		final, err := br.ReadByte()
		if err != nil {
			return
		}
		if open != '[' {
			continue
		}
		switch final {
		case 'A':
			emit(key{arrow: arrowUp})
		case 'B':
			emit(key{arrow: arrowDown})
		case 'C':
			emit(key{arrow: arrowRight})
		case 'D':
			emit(key{arrow: arrowLeft})
		}
	}
}
