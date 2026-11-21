// Package trayicon draws the menu bar counts as a macOS template image.
package trayicon

import (
	"bytes"
	"image/png"
	"math"
	"strconv"

	"github.com/egoist/mygo/ui"
	"github.com/jinjiebewater/tray-inbox/internal/inbox"
)

// MyGo shows tray images 18pt tall, so the 22pt canvas is drawn at 22/18 of the sizes meant on screen.
const (
	height = 22
	glyph  = 16
	digits = 16
	inner  = 4
	outer  = 11
	pad    = 1
)

var font = ui.Font{Size: digits, Weight: 500, Features: "tnum"}

var (
	errorIcon   = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><mask id="m"><rect width="24" height="24" fill="#fff"/><path d="M8.8 8.8l6.4 6.4M15.2 8.8l-6.4 6.4" stroke="#000" stroke-width="2.4" stroke-linecap="round"/></mask><path mask="url(#m)" d="M8 2h8l6 6v8l-6 6H8l-6-6V8z"/></svg>`))
	blockedIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><rect x="6.6" y="5.5" width="2.8" height="10" rx="1.4"/><rect x="9.8" y="2.5" width="2.8" height="12" rx="1.4"/><rect x="13" y="3.5" width="2.8" height="11" rx="1.4"/><rect x="16.2" y="6" width="2.8" height="9.5" rx="1.4"/><path d="M6.6 12h12.4v3.5a7 7 0 0 1-7 7h-.6a6 6 0 0 1-4.6-2.2L2.9 15.6a1.6 1.6 0 0 1 2.4-2.1L6.6 15z"/></svg>`))
	doneIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><mask id="m"><rect width="24" height="24" fill="#fff"/><path d="M7.5 12.3l3.1 3.1 5.9-6.4" fill="none" stroke="#000" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round"/></mask><circle mask="url(#m)" cx="12" cy="12" r="10"/></svg>`))
	workingIcon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><circle cx="12" cy="12" r="8.8" fill="none" stroke="#000" stroke-width="2.4" stroke-dasharray="4.1 2.81" transform="rotate(-11 12 12)"/></svg>`))
	offIcon     = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9.5"/><path d="M6.5 12.5l3.5-3 4 5 3.5-3"/></svg>`))
	idleIcon    = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M2 13h5.6l1.7 2.6h5.4l1.7-2.6H22v5a3 3 0 0 1-3 3H5a3 3 0 0 1-3-3z"/><path d="M2.8 12.4 5.7 6a1.9 1.9 0 0 1 1.7-1.1h9.2A1.9 1.9 0 0 1 18.3 6l2.9 6.4" fill="none" stroke="#000" stroke-width="2.2" stroke-linejoin="round"/></svg>`))
)

type cell struct {
	icon *ui.SVG
	text string
	ink  ui.Color
}

// Render draws error, blocked, done, idle and working counts left to right, leaving out zeros, with a disconnect glyph at the end.
func Render(c inbox.Counts, disconnected bool, scale float32) []byte {
	black := ui.RGB(0, 0, 0)
	var cells []cell
	for _, n := range []struct {
		icon *ui.SVG
		n    int
	}{{errorIcon, c.Error}, {blockedIcon, c.Blocked}, {doneIcon, c.Done}, {idleIcon, c.Idle}, {workingIcon, c.Working}} {
		if n.n > 0 {
			cells = append(cells, cell{n.icon, strconv.Itoa(n.n), black})
		}
	}
	if disconnected {
		cells = append(cells, cell{offIcon, "", black.Alpha(0.45)})
	}
	if len(cells) == 0 {
		cells = append(cells, cell{idleIcon, "", black})
	}

	w := float32(2 * pad)
	for i, cl := range cells {
		if i > 0 {
			w += outer
		}
		w += glyph
		if cl.text != "" {
			w += inner + advance(cl.text)
		}
	}
	img := ui.Render(func(ctx *ui.Context) {
		ctx.Root().Background(ui.Transparent)
		ui.Row(ctx).Height(height).PaddingX(pad).Gap(outer).Children(func() {
			for _, cl := range cells {
				ui.Row(ctx).Gap(inner).TextColor(cl.ink).Children(func() {
					ui.Icon(ctx, cl.icon).FontSize(glyph)
					if cl.text != "" {
						ui.Text(ctx, cl.text).FontSize(font.Size).FontWeight(font.Weight).FontFeatures(font.Features).SingleLine()
					}
				})
			}
		})
	}, int(math.Ceil(float64(w))), height, scale)
	var b bytes.Buffer
	_ = png.Encode(&b, img)
	return b.Bytes()
}

func advance(s string) float32 {
	g := ui.Shape(s, font)
	if len(g) == 0 {
		return 0
	}
	last := g[len(g)-1]
	return last.X + last.Advance
}

// Icon redraws the menu bar image when its input changes.
type Icon struct {
	set   func(png []byte)
	last  inbox.Counts
	off   bool
	drawn bool
}

func New(set func(png []byte)) *Icon { return &Icon{set: set} }

func (i *Icon) Update(c inbox.Counts, disconnected bool) {
	if i.drawn && c == i.last && disconnected == i.off {
		return
	}
	i.last, i.off, i.drawn = c, disconnected, true
	i.set(Render(c, disconnected, 2))
}
