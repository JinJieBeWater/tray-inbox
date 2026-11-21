package panel

import (
	"embed"

	"github.com/egoist/mygo/ui"

	"github.com/jinjiebewater/tray-inbox/internal/agent"
)

func svg(body string) *ui.SVG {
	return ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="#000" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + body + `</svg>`))
}

// Lucide, as T3's sidebar draws each state.
var (
	approvalIcon = svg(`<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/><path d="M9.1 9a3 3 0 0 1 5.82 1c0 2-3 3-3 3"/><path d="M12 17h.01"/>`)
	questionIcon = svg(`<path d="M2.992 16.342a2 2 0 0 1 .094 1.167l-1.065 3.29a1 1 0 0 0 1.236 1.168l3.413-.998a2 2 0 0 1 1.099.092 10 10 0 1 0-4.777-4.719"/><path d="M9.09 9a3 3 0 0 1 5.83 1c0 2-3 3-3 3"/><path d="M12 17h.01"/>`)
	requestIcon  = svg(`<circle cx="12" cy="8" r="5"/><path d="M20 21a8 8 0 0 0-16 0"/>`)
	wokeIcon     = svg(`<circle cx="12" cy="13" r="8"/><path d="M12 9v4l2 2"/><path d="M5 3 2 6"/><path d="m22 6-3-3"/><path d="M6.38 18.7 4 21"/><path d="M17.64 18.67 20 21"/>`)
	alertIcon    = svg(`<circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/>`)
	doneIcon     = svg(`<circle cx="12" cy="12" r="10"/><path d="m9 12 2 2 4-4"/>`)
	workingIcon  = svg(`<path d="M10.1 2.182a10 10 0 0 1 3.8 0"/><path d="M13.9 21.818a10 10 0 0 1-3.8 0"/><path d="M17.609 3.721a10 10 0 0 1 2.69 2.7"/><path d="M2.182 13.9a10 10 0 0 1 0-3.8"/><path d="M20.279 17.609a10 10 0 0 1-2.7 2.69"/><path d="M21.818 10.1a10 10 0 0 1 0 3.8"/><path d="M3.721 6.391a10 10 0 0 1 2.7-2.69"/><path d="M6.391 20.279a10 10 0 0 1-2.69-2.7"/>`)
	waitingIcon  = svg(`<path d="M10.1 2.18a9.93 9.93 0 0 1 3.8 0"/><path d="M17.6 3.71a9.95 9.95 0 0 1 2.69 2.7"/><path d="M21.82 10.1a9.93 9.93 0 0 1 0 3.8"/><path d="M20.29 17.6a9.95 9.95 0 0 1-2.7 2.69"/><path d="M13.9 21.82a9.94 9.94 0 0 1-3.8 0"/><path d="M6.4 20.29a9.95 9.95 0 0 1-2.69-2.7"/><path d="M2.18 13.9a9.93 9.93 0 0 1 0-3.8"/><path d="M3.71 6.4a9.95 9.95 0 0 1 2.7-2.69"/><circle cx="12" cy="12" r="1"/>`)
	chevronIcon  = svg(`<path d="m6 9 6 6 6-6"/>`)
)

//go:embed icons
var iconFiles embed.FS

func embedded(name string) ui.ImageSource {
	b, err := iconFiles.ReadFile("icons/" + name)
	if err != nil {
		panic(err)
	}
	img, err := ui.DecodeImage(b)
	if err != nil {
		panic(name + ": " + err.Error())
	}
	return img
}

type mark struct {
	img  ui.ImageSource
	tint bool
}

// Herdr App's defaults/programs.json: a tinted mark takes the text color, the others keep their own.
func agentMarks() map[agent.Kind]mark {
	m := map[agent.Kind]mark{}
	add := func(tint bool, kinds ...agent.Kind) {
		for _, k := range kinds {
			m[k] = mark{embedded(string(k) + ".svg"), tint}
		}
	}
	add(false, agent.KindClaude, agent.KindCodex, agent.KindAntigravity)
	add(true, agent.KindPi, agent.KindOpenCode, agent.KindCursor, agent.KindGrok, agent.KindMuse)
	return m
}

var (
	marks       = agentMarks()
	genericMark = mark{embedded("agent.svg"), true}
)

func agentMark(c *ui.Context, kind agent.Kind, ink ui.Color) {
	m, ok := marks[kind]
	if !ok {
		m = genericMark
	}
	if m.tint {
		ui.Icon(c, m.img.(*ui.SVG)).Size(14, 14).TextColor(ink).Opacity(.6).Shrink(0)
	} else {
		ui.Image(c, m.img).Size(14, 14).Opacity(.6).Shrink(0)
	}
}
