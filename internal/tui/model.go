package tui

import (
	"fmt"
	"strings"

	"github.com/amoshydra/sinkrill/internal/sink"
)

// Model is everything the panel draws. It is a plain value, so both the key
// mapping and the renderer work without a terminal, a speaker, or a context.
type Model struct {
	Room    string       // display name of the room, as resolved
	Station string       // the stream's title
	Volume  int          // -1 until the room answers
	Muted   bool         //
	State   string       // PLAYING, PAUSED_PLAYBACK, STOPPED, ...
	Playing string       // URI the room is on right now
	Stream  string       // URI this instance advertises
	Local   *sink.Status // /status of the local instance, nil when it is not running
	Message string       // result of the last action, or the last error
	Failed  bool         // Message is an error
}

// stateLabel turns a UPnP transport state into something readable.
func (m Model) stateLabel() string {
	switch m.State {
	case "":
		return "unknown"
	case "PLAYING":
		return "playing"
	case "PAUSED_PLAYBACK":
		return "paused"
	case "STOPPED":
		return "stopped"
	case "TRANSITIONING":
		return "buffering"
	}
	return strings.ToLower(m.State)
}

// playingLabel says whether the room is on this stream, without printing a
// URI nobody can read at a glance.
func (m Model) playingLabel() string {
	switch {
	case m.Stream == "":
		return "no stream advertised yet"
	case m.Playing == "":
		return "unknown"
	case m.Playing == m.Stream:
		return "this stream"
	}
	return "another source"
}

// volumeLine is the bar, the number, and the mute marker.
func (m Model) volumeLine(barWidth int) string {
	level := "--"
	if m.Volume >= 0 {
		level = fmt.Sprintf("%d", m.Volume)
	}
	line := fmt.Sprintf("%s  %s", volumeBar(m.Volume, barWidth), level)
	if m.Muted {
		line += "  (muted)"
	}
	return line
}

func (m Model) streamLine() string {
	if m.Local == nil {
		return "no running instance on this port"
	}
	return fmt.Sprintf("%s, %d kbps", m.Local.Format, m.Local.Bitrate)
}

func (m Model) captureLine() string {
	if m.Local == nil {
		return "-"
	}
	c := m.Local.Capture
	switch {
	case c.LastError != "" && !c.Running:
		return "stopped: " + c.LastError
	case !c.Running:
		return "stopped"
	case c.Stalled:
		return fmt.Sprintf("stalled, last data %d ms ago", c.LastDataMS)
	}
	return fmt.Sprintf("running, last data %d ms ago, %d restarts", c.LastDataMS, c.Restarts)
}

func (m Model) bufferLine() string {
	if m.Local == nil {
		return "-"
	}
	noun := "listeners"
	if m.Local.Listeners == 1 {
		noun = "listener"
	}
	return fmt.Sprintf("%d B queued, %d dropped, %d %s",
		m.Local.QueuedBytes, m.Local.DroppedChunks, m.Local.Listeners, noun)
}

// volumeBar draws a fixed-width level bar. A negative volume (device not
// answering yet) draws as empty.
func volumeBar(volume, width int) string {
	if width < 6 {
		width = 6
	}
	if volume < 0 {
		volume = 0
	}
	if volume > 100 {
		volume = 100
	}
	filled := volume * width / 100
	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + "]"
}
