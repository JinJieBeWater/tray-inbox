package panel

import (
	"strings"
	"unicode"

	"github.com/egoist/mygo/ui"
	"golang.org/x/text/unicode/norm"
	"golang.org/x/text/width"
)

func projectBadge(c *ui.Context, project string, pal palette) {
	if project == "" {
		ui.Box(c).Size(16, 16).Radius(4).Border(1.5, pal.sub).BorderStyle(ui.BorderDashed).Shrink(0)
		return
	}
	ink := tint(project, pal.dark)
	text := monogram(project)
	ui.Box(c).Size(16, 16).Radius(4).Background(ink.Alpha(.14)).AlignItems(ui.Center).Justify(ui.Center).Clip().Shrink(0).Children(func() {
		ui.Text(c, text).Font("monospace").FontSize(8.25).FixedLineHeight(10).Bold().TextColor(ink).NoWrap().Shrink(0).
			TransformOrigin(.5, .5).Scale(squeeze(text), 1)
	})
}

// squeeze fits the glyphs into T3's textLength: 12 for two, 6 for one.
func squeeze(text string) float32 {
	var em, n float32
	for _, r := range text {
		n++
		if k := width.LookupRune(r).Kind(); k == width.EastAsianWide || k == width.EastAsianFullwidth {
			em++
		} else {
			em += .6
		}
	}
	return n * 6 / (em * 8.25)
}

func normalized(name string) string { return strings.TrimSpace(norm.NFKC.String(name)) }

// monogram and tintIndex copy T3's projectIdentity.ts so a project keeps its T3 badge.
func monogram(name string) string {
	words := strings.FieldsFunc(normalized(name), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	if len(words) == 0 {
		return "PR"
	}
	g := []rune(words[0])
	second := g[len(g)-1]
	if len(words) > 1 {
		second = []rune(words[len(words)-1])[0]
	}
	for _, r := range g[1:] {
		if unicode.IsNumber(r) {
			second = r
			break
		}
	}
	m := []rune(strings.ToUpper(string([]rune{g[0], second})))
	return string(m[:min(2, len(m))])
}

func tintIndex(name string) int {
	seed := strings.ToLower(normalized(name))
	if seed == "" {
		seed = "project"
	}
	i := 0
	for _, r := range seed {
		i = (i*31 + int(r)) % len(tints600)
	}
	return i
}

func tint(name string, dark bool) ui.Color {
	if dark {
		return tints400[tintIndex(name)]
	}
	return tints600[tintIndex(name)]
}

// Tailwind gray red orange amber yellow lime green emerald teal cyan sky blue indigo violet purple fuchsia pink rose.
var (
	tints600 = hexes("#4b5563", "#dc2626", "#ea580c", "#d97706", "#ca8a04", "#65a30d", "#16a34a", "#059669", "#0d9488", "#0891b2", "#0284c7", "#2563eb", "#4f46e5", "#7c3aed", "#9333ea", "#c026d3", "#db2777", "#e11d48")
	tints400 = hexes("#9ca3af", "#f87171", "#fb923c", "#fbbf24", "#facc15", "#a3e635", "#4ade80", "#34d399", "#2dd4bf", "#22d3ee", "#38bdf8", "#60a5fa", "#818cf8", "#a78bfa", "#c084fc", "#e879f9", "#f472b6", "#fb7185")
)

func hexes(s ...string) (out [18]ui.Color) {
	for i, h := range s {
		out[i] = ui.Hex(h)
	}
	return out
}
