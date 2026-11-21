// Package panel draws the inbox panel from the aggregated view.
package panel

import (
	"fmt"
	"math"
	"os"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/jinjiebewater/tray-inbox/internal/agent"
	"github.com/jinjiebewater/tray-inbox/internal/inbox"
	"github.com/jinjiebewater/tray-inbox/locales"
)

type Options struct {
	Now   func() time.Time
	Jump  func(id agent.ID, done func(err error))
	Close func()
}

// Panel is used only on the UI thread.
type Panel struct {
	opts    Options
	home    string
	view    inbox.View
	running *bool // the user's choice since the panel opened; nil: open while the inbox is empty
	failed  map[agent.ID]bool
	opened  int
	maxH    float32
}

func New(opts Options) *Panel {
	home, _ := os.UserHomeDir()
	return &Panel{opts: opts, home: home, failed: map[agent.ID]bool{}}
}

func (p *Panel) SetView(v inbox.View) { p.view = v }

// SetMaxHeight is the height the window stops growing at. Below it the content overflows only for the frame before the window grows to fit, which must not flash a scroll bar.
func (p *Panel) SetMaxHeight(h int) { p.maxH = float32(h) }

// Reset is for when the panel closes: it forgets failed jumps and the running toggle, and ignores the answers of jumps still on their way.
func (p *Panel) Reset() {
	p.opened++
	p.running = nil
	clear(p.failed)
}

type dcKey int

func (p *Panel) View(c *ui.Context) {
	pal := colors(c.Theme())
	c.Root().Background(pal.panel)
	now := p.opts.Now()
	next := time.Duration(math.MaxInt64)
	show := func(r inbox.Row) {
		p.row(c, r, pal, now)
		if timed(r) {
			next = min(next, time.Minute-now.Sub(r.At)%time.Minute)
		}
	}
	_, h := c.Size()
	ui.Scroll(c).Fill().Scrollbars(p.maxH > 0 && h >= p.maxH-1).Children(func() {
		ui.Column(c).Padding(8).Gap(8).AlignItems(ui.Stretch).Children(func() {
			for i, d := range p.view.Disconnected {
				disconnect(c.Key(dcKey(i)), d, pal)
			}
			if len(p.view.Inbox) > 0 || len(p.view.Disconnected) == 0 {
				section(c).Children(func() {
					n := 0
					for _, r := range p.view.Inbox {
						if unread(r) {
							n++
						}
					}
					headerStyle(ui.Row(c), pal).Children(func() {
						ui.Text(c, locales.Inbox()).SingleLine().Grow(1)
						if n > 0 {
							ui.Text(c, locales.Unread(n)).FontWeight(400).FontFeatures("tnum")
						}
					})
					for _, r := range p.view.Inbox {
						show(r)
					}
					if len(p.view.Inbox) == 0 {
						allDone(c, pal)
					}
				})
			}
			if n := len(p.view.Running); n > 0 {
				section(c).Children(func() {
					open := p.runningOpen()
					p.header(c, n, open, pal)
					if open {
						for _, r := range p.view.Running {
							show(r)
						}
					}
				})
			}
		})
	})
	if next != time.Duration(math.MaxInt64) {
		c.After(next)
	}
}

func reason(r agent.Reason) string {
	switch r {
	case agent.ReasonReconnecting:
		return locales.Reconnecting()
	case agent.ReasonAppNotRunning:
		return locales.AppNotRunning()
	case agent.ReasonNoCommand:
		return locales.NoCommand()
	}
	return ""
}

func section(c *ui.Context) ui.Element { return ui.Column(c).Gap(5).AlignItems(ui.Stretch) }

// headerStyle is T3's section header: the headers are the dividers, the rows carry no background.
func headerStyle(e ui.Element, pal palette) ui.Element {
	return e.Gap(8).Height(32).PaddingX(8).Margin(0, 2).Radius(6).TextColor(pal.sub).FontSize(12).FixedLineHeight(16).FontWeight(500)
}

func allDone(c *ui.Context, pal palette) {
	ui.Column(c).AlignItems(ui.Center).Padding(16, 8, 20, 8).Children(func() {
		ui.Text(c, locales.AllDone()).FontSize(12).FixedLineHeight(16).TextColor(pal.sub.Alpha(.6)).SingleLine()
	})
}

func unread(r inbox.Row) bool {
	return r.State == agent.Blocked || r.State == agent.Error || r.State == agent.Done
}

func disconnect(c *ui.Context, d inbox.Disconnect, pal palette) {
	ui.Row(c).Gap(6).Padding(10, 12).Radius(10).Background(pal.dcBack).TextColor(pal.dcText).FontSize(12).FixedLineHeight(16).Children(func() {
		ui.Icon(c, alertIcon).FontSize(16).Shrink(0)
		ui.Text(c, reason(d.Reason)).SingleLine().Shrink(1).MinWidth(0)
	})
}

func (p *Panel) runningOpen() bool {
	if p.running != nil {
		return *p.running
	}
	return len(p.view.Inbox) == 0
}

func (p *Panel) header(c *ui.Context, n int, open bool, pal palette) {
	headerStyle(ui.ButtonBase(c.Key("running")), pal).Cursor(ui.CursorPointer).Expanded(open).OnClick(func() { p.running = new(!open) }).Children(func() {
		ui.Text(c, locales.Running()).SingleLine().Grow(1)
		ui.Text(c, fmt.Sprint(n)).FontWeight(400).FontFeatures("tnum")
		chev := ui.Icon(c, chevronIcon).FontSize(12)
		if !open {
			chev.Rotate(-90)
		}
	})
}
