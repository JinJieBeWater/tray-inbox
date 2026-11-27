package panel

import (
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
	"github.com/jinjiebewater/tray-inbox/internal/inbox"
	"github.com/jinjiebewater/tray-inbox/locales"
)

func (p *Panel) row(c *ui.Context, r inbox.Row, pal palette, now time.Time) {
	b := ui.ButtonBase(c.Key(r.ID)).Cursor(ui.CursorPointer).AlignItems(ui.Start).Padding(8, 10).Radius(6)
	hovered := b.Hovered()
	if hovered {
		b.Background(pal.hover)
	}
	if r.State == agent.Working && !hovered {
		b.Opacity(.7)
	}
	id := r.ID
	b.OnClick(func() { p.jump(id) })
	b.Children(func() {
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).AlignItems(ui.Stretch).Children(func() { p.rowLines(c, r, pal, now, hovered) })
	})
}

func (p *Panel) rowLines(c *ui.Context, r inbox.Row, pal palette, now time.Time, hovered bool) {
	meta, metaInk := p.where(r), pal.sub
	if p.failed[r.ID] {
		meta, metaInk = locales.CantOpen(), pal.failed
	}
	ui.Row(c).Height(20).Gap(6).Children(func() {
		projectBadge(c, r.Project, pal)
		name := r.Project
		if name == "" {
			name = locales.NoProject()
		}
		ui.Text(c, name).FontSize(12).FixedLineHeight(16).TextColor(pal.sub).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		status(c, r, pal, now)
	})
	ui.Row(c).Gap(6).Margin(4, 0, 0, 0).Children(func() {
		title := ui.Text(c, r.Title).FontSize(14).FixedLineHeight(20).SingleLine().Grow(1).Shrink(1).MinWidth(0)
		if r.State == agent.Working || r.State == agent.Idle {
			title.FontWeight(400).TextColor(pal.recede)
			if hovered {
				title.TextColor(pal.text)
			}
		} else {
			title.FontWeight(500).TextColor(pal.text)
		}
		if meta == "" {
			agentMark(c, r.Kind, pal.text)
		}
	})
	if meta != "" {
		ui.Row(c).Gap(6).Margin(2, 0, 0, 0).Children(func() {
			ui.Text(c, meta).FontSize(12).FixedLineHeight(16).TextColor(metaInk).SingleLine().Grow(1).Shrink(1).MinWidth(0)
			agentMark(c, r.Kind, pal.text)
		})
	}
}

func (p *Panel) jump(id agent.ID) {
	opened := p.opened
	p.opts.Jump(id, func(err error) {
		if opened != p.opened {
			return
		}
		if err != nil {
			p.failed[id] = true
			return
		}
		p.opts.Close()
	})
}

func (p *Panel) where(r inbox.Row) string {
	w := r.Where
	if p.home != "" && (w == p.home || strings.HasPrefix(w, p.home+"/")) {
		w = "~" + w[len(p.home):]
	}
	return w
}

// timed is T3's rule: only working/waiting rows (when a start time is known) and idle rows show a time.
func timed(r inbox.Row) bool {
	return (r.State == agent.Working || r.State == agent.Idle) && !r.At.IsZero()
}

func status(c *ui.Context, r inbox.Row, pal palette, now time.Time) {
	var icon *ui.SVG
	var word string
	ink := pal.sub
	switch r.State {
	case agent.Blocked:
		icon, word, ink = alertIcon, locales.NeedsAttention(), pal.needs
		switch r.BlockedWhy {
		case agent.WhyPermission:
			icon, word = approvalIcon, locales.PendingApproval()
		case agent.WhyQuestion:
			icon, word, ink = questionIcon, locales.AwaitingInput(), pal.question
		case agent.WhyRequest:
			icon, word, ink = requestIcon, locales.GuestRequest(), pal.request
		case agent.WhyLimited:
			icon, word = alertIcon, locales.Limited()
		case agent.WhyWoke:
			icon, word = wokeIcon, locales.Woke()
		}
	case agent.Error:
		icon, word, ink = alertIcon, locales.Failed(), pal.failed
	case agent.Done:
		icon, word, ink = doneIcon, locales.Completed(), pal.done
	case agent.Working:
		switch r.WorkingKind {
		case agent.WorkWaiting:
			icon, word, ink = waitingIcon, locales.Waiting(), pal.sub
		case agent.WorkPreparing:
			icon, word, ink = workingIcon, locales.SettingUp(), pal.run
		case agent.WorkGoal:
			icon, word, ink = workingIcon, locales.Goal(), pal.run
		default:
			icon, word, ink = workingIcon, locales.Running(), pal.run
		}
	}
	ui.Row(c).Gap(4).MinWidth(32).Shrink(0).Justify(ui.End).TextColor(ink).FontSize(12).FixedLineHeight(16).FontFeatures("tnum").Children(func() {
		if icon != nil {
			ui.Icon(c, icon).FontSize(16)
			ui.Text(c, word).FontWeight(500).SingleLine()
		}
		if timed(r) {
			label := ago(now.Sub(r.At))
			if r.State == agent.Working {
				label = workingAgo(now.Sub(r.At))
			}
			t := ui.Text(c, label).SingleLine()
			if icon != nil {
				t.FontWeight(500)
			}
		}
	})
}

func workingAgo(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d / time.Second)
	if secs < 60 {
		return locales.Seconds(secs)
	}
	mins := secs / 60
	if mins < 60 {
		return locales.Minutes(mins)
	}
	return locales.HoursMinutes(mins/60, mins%60)
}

func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return locales.JustNow()
	case d < time.Hour:
		return locales.Minutes(int(d / time.Minute))
	case d < 24*time.Hour:
		return locales.Hours(int(d / time.Hour))
	}
	return locales.Days(int(d / (24 * time.Hour)))
}
