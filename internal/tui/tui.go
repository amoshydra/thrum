package tui

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"golang.org/x/term"

	"github.com/amoshydra/sinkrill/internal/sink"
	"github.com/amoshydra/sinkrill/internal/sonos"
)

// room is the slice of the speaker the panel uses. *sonos.Client is the real
// implementation; tests supply a recording fake so every key can be checked
// without a speaker on the network.
type room interface {
	Volume(context.Context) (int, error)
	Muted(context.Context) (bool, error)
	SetVolume(context.Context, int) error
	SetMute(context.Context, bool) error
	TransportState(context.Context) (string, error)
	Position(context.Context) (sonos.Position, error)
	Play(context.Context) error
	Pause(context.Context) error
	Stop(context.Context) error
	Next(context.Context) error
	Previous(context.Context) error
	SetAVTransportURI(ctx context.Context, uri, metadata string) error
}

// Options is what the panel needs before it can draw anything.
type Options struct {
	Room   string // display name of the room
	Device string // IP the room answers on
	Port   int    // port of the local instance
	Local  *sink.Status
}

const (
	enterScreen  = "\x1b[?1049h\x1b[?25l" // alternate screen, hide cursor
	leaveScreen  = "\x1b[?25h\x1b[?1049l" // show cursor, restore screen
	homeAndClear = "\x1b[H\x1b[J"
)

// Run draws the panel until the user quits or ctx ends. It owns the terminal:
// raw mode and the alternate screen are both restored on the way out.
func Run(ctx context.Context, opts Options) error {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		return errors.New("the control panel needs an interactive terminal")
	}
	old, err := term.MakeRaw(fd)
	if err != nil {
		return fmt.Errorf("could not put the terminal in raw mode: %w", err)
	}

	fmt.Fprint(os.Stdout, enterScreen)
	defer func() {
		fmt.Fprint(os.Stdout, leaveScreen)
		_ = term.Restore(fd, old)
	}()

	var speaker room = sonos.NewClient(opts.Device)
	model := Model{Room: opts.Room, Volume: -1}
	if opts.Local != nil {
		model.Local = opts.Local
		model.Station = opts.Local.Station
		model.Stream = opts.Local.URI
	}

	keys := make(chan key, 8)
	go decodeKeys(os.Stdin, func(k key) {
		select {
		case keys <- k:
		case <-ctx.Done():
		}
	})

	snapshots := make(chan snapshot, 1)
	refresh := make(chan struct{}, 1)
	go poll(ctx, speaker, opts.Port, snapshots, refresh)
	select {
	case refresh <- struct{}{}:
	default:
	}

	draw(model)
	for {
		select {
		case <-ctx.Done():
			return nil

		case snap := <-snapshots:
			model.apply(snap)
			draw(model)

		case k := <-keys:
			act := keyAction(k, model)
			if act.kind == actQuit {
				return nil
			}
			if act.kind == actNone {
				continue
			}
			actCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			model.Message, model.Failed = runAction(actCtx, speaker, model, act)
			cancel()
			// Pick up the consequence now instead of a second later.
			select {
			case refresh <- struct{}{}:
			default:
			}
			draw(model)
		}
	}
}

// snapshot is one poll of the room and the local instance.
type snapshot struct {
	volume  int
	muted   bool
	state   string
	playing string
	local   *sink.Status
	err     error // the room did not answer
}

// apply folds a poll into the model. A room that answers clears a stale error;
// a room that does not answer leaves the last known values on screen.
func (m *Model) apply(s snapshot) {
	if s.err != nil {
		m.Message, m.Failed = "room unreachable: "+shortError(s.err), true
		if s.local != nil {
			m.updateLocal(s.local)
		}
		return
	}
	if m.Failed {
		m.Message, m.Failed = "", false
	}
	m.Volume, m.Muted, m.State, m.Playing = s.volume, s.muted, s.state, s.playing
	if s.local != nil {
		m.updateLocal(s.local)
	}
}

// shortError drops the SOAP URL that net/http wraps around the actual cause.
// The URL is the same for every call, so it only pushes the useful part off
// the end of a narrow terminal.
func shortError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) && ue.Err != nil {
		return ue.Err.Error()
	}
	return err.Error()
}

func (m *Model) updateLocal(st *sink.Status) {
	m.Local = st
	m.Stream = st.URI
	m.Station = st.Station
}

// poll refreshes the model once a second, and immediately when an action asks
// for it.
func poll(ctx context.Context, speaker room, port int, out chan<- snapshot, refresh <-chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-refresh:
		}

		snap := collect(ctx, speaker, port)
		select {
		case out <- snap:
		default: // the loop is busy drawing; the next tick will do
		}
	}
}

func collect(ctx context.Context, speaker room, port int) snapshot {
	cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	snap := snapshot{volume: -1}
	if st, ok := sink.LocalStatus(port); ok {
		snap.local = st
	}

	volume, err := speaker.Volume(cctx)
	if err != nil {
		snap.err = err
		return snap
	}
	snap.volume = volume
	if muted, err := speaker.Muted(cctx); err == nil {
		snap.muted = muted
	}
	if state, err := speaker.TransportState(cctx); err == nil {
		snap.state = state
	}
	if pos, err := speaker.Position(cctx); err == nil {
		snap.playing = pos.URI
	}
	return snap
}

// runAction performs one keystroke's worth of work and returns what to show in
// the footer. A message with failed set is drawn as an error.
func runAction(ctx context.Context, speaker room, m Model, a action) (string, bool) {
	fail := func(what string, err error) (string, bool) {
		return fmt.Sprintf("%s: %v", what, err), true
	}

	switch a.kind {
	case actVolume:
		level := m.Volume
		if level < 0 {
			current, err := speaker.Volume(ctx)
			if err != nil {
				return fail("could not read the volume", err)
			}
			level = current
		}
		target := clampVolume(level + a.delta)
		if target == level {
			return fmt.Sprintf("volume %d", target), false
		}
		if err := speaker.SetVolume(ctx, target); err != nil {
			return fail("could not set the volume", err)
		}
		return fmt.Sprintf("volume %d", target), false

	case actMute:
		muted := !m.Muted
		if err := speaker.SetMute(ctx, muted); err != nil {
			return fail("could not mute", err)
		}
		if muted {
			return "muted", false
		}
		return "unmuted", false

	case actPlay:
		if err := speaker.Play(ctx); err != nil {
			return fail("could not play", err)
		}
		return "playing", false

	case actPause:
		if err := speaker.Pause(ctx); err != nil {
			return fail("could not pause", err)
		}
		return "paused", false

	case actStop:
		if err := speaker.Stop(ctx); err != nil {
			return fail("could not stop", err)
		}
		return "stopped", false

	case actNext:
		if err := speaker.Next(ctx); err != nil {
			return fail("could not skip", err)
		}
		return "next track", false

	case actPrevious:
		if err := speaker.Previous(ctx); err != nil {
			return fail("could not go back", err)
		}
		return "previous track", false

	case actRetune:
		if m.Stream == "" {
			return "no stream to tune to; is the instance started?", true
		}
		format, station := "wav", m.Station
		if m.Local != nil {
			format, station = m.Local.Format, m.Local.Station
		}
		meta := sonos.StreamMetadata(format, station, m.Stream)
		if err := speaker.SetAVTransportURI(ctx, m.Stream, meta); err != nil {
			return fail("could not retune", err)
		}
		if err := speaker.Play(ctx); err != nil {
			return fail("retuned, but playback did not start", err)
		}
		return "tuned back to this stream", false
	}
	return "", false
}

func clampVolume(v int) int {
	if v < 0 {
		return 0
	}
	if v > 100 {
		return 100
	}
	return v
}

// draw paints the panel in place, in one write so a slow terminal cannot show
// a half-finished frame.
func draw(m Model) {
	width, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || width < 44 {
		width = 80
	}

	var buf bytes.Buffer
	buf.WriteString(homeAndClear)
	render(crlfWriter{&buf}, m, width)
	_, _ = os.Stdout.Write(buf.Bytes())
}

// crlfWriter turns "\n" into "\r\n". Raw mode disables output processing, so
// a bare newline would move down without returning to the first column.
type crlfWriter struct{ w *bytes.Buffer }

func (c crlfWriter) Write(p []byte) (int, error) {
	if _, err := c.w.Write(bytes.ReplaceAll(p, []byte("\n"), []byte("\r\n"))); err != nil {
		return 0, err
	}
	return len(p), nil
}
