package panel

import "github.com/egoist/mygo/ui"

// Surfaces and text follow the MyGo theme; status colors are T3's sidebar ones.
type palette struct {
	panel, hover                           ui.Color
	text, recede, sub                      ui.Color
	needs, question, request, failed, done ui.Color
	run, dcBack, dcText                    ui.Color
	dark                                   bool
}

func colors(t *ui.Theme) palette {
	p := palette{
		panel: t.Background, hover: t.Surface,
		text: t.Text, recede: t.Text.Alpha(.75), sub: t.TextMuted, dark: t.Dark,
	}
	if t.Dark {
		p.needs, p.question, p.request, p.failed, p.done, p.run = ui.Hex("#fbbf24"), ui.Hex("#a5b4fc"), ui.Hex("#f0abfc"), ui.Hex("#f87171"), ui.Hex("#34d399"), ui.Hex("#60a5fa")
		p.dcBack, p.dcText = ui.RGBA(249, 115, 22, .14), ui.Hex("#fdba74")
	} else {
		p.needs, p.question, p.request, p.failed, p.done, p.run = ui.Hex("#b45309"), ui.Hex("#4f46e5"), ui.Hex("#a21caf"), ui.Hex("#b91c1c"), ui.Hex("#047857"), ui.Hex("#1d4ed8")
		p.dcBack, p.dcText = ui.Hex("#fff7ed"), ui.Hex("#9a3412")
	}
	return p
}
