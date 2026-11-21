package trayicon

import (
	"bytes"
	"image"
	"image/png"
	"testing"

	"github.com/jinjiebewater/tray-inbox/internal/inbox"
)

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func width(t *testing.T, c inbox.Counts, disconnected bool) int {
	t.Helper()
	return decode(t, Render(c, disconnected, 2)).Bounds().Dx()
}

func TestRenderIsABlackTemplateOfTheMenuBarHeight(t *testing.T) {
	img := decode(t, Render(inbox.Counts{Blocked: 1, Working: 12}, true, 2))
	if h := img.Bounds().Dy(); h != 44 {
		t.Errorf("height %d at scale 2, want 44", h)
	}
	inked := 0
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, a := img.At(x, y).RGBA()
			if a == 0 {
				continue
			}
			inked++
			if r|g|bl != 0 {
				t.Fatalf("pixel %d,%d is not black: %d %d %d", x, y, r, g, bl)
			}
		}
	}
	if inked == 0 {
		t.Error("nothing drawn")
	}
}

func TestZeroCellsAddNoWidth(t *testing.T) {
	one := width(t, inbox.Counts{Blocked: 2}, false)
	for _, c := range []inbox.Counts{{Error: 2}, {Done: 2}, {Idle: 2}, {Working: 2}} {
		if w := width(t, c, false); w != one {
			t.Errorf("%+v is %d wide, a single blocked cell %d", c, w, one)
		}
	}
	if two := width(t, inbox.Counts{Blocked: 2, Done: 2}, false); two <= one {
		t.Errorf("two cells %d wide, one %d", two, one)
	}
}

func TestAllZeroIsNarrowerThanCounts(t *testing.T) {
	if z, n := width(t, inbox.Counts{}, false), width(t, inbox.Counts{Working: 1}, false); z >= n {
		t.Errorf("all zero %d wide, with a count %d", z, n)
	}
}

func TestDisconnectAddsAGlyph(t *testing.T) {
	if up, down := width(t, inbox.Counts{Working: 1}, false), width(t, inbox.Counts{Working: 1}, true); down <= up {
		t.Errorf("disconnected %d wide, connected %d", down, up)
	}
	if bytes.Equal(Render(inbox.Counts{}, true, 2), Render(inbox.Counts{}, false, 2)) {
		t.Error("all zero looks the same disconnected or not")
	}
}

func TestUpdateSetsOnlyOnChange(t *testing.T) {
	var got [][]byte
	icon := New(func(png []byte) { got = append(got, png) })
	icon.Update(inbox.Counts{Done: 1}, false)
	icon.Update(inbox.Counts{Done: 1}, false)
	if len(got) != 1 {
		t.Fatalf("same input twice set %d times", len(got))
	}
	icon.Update(inbox.Counts{Done: 2}, false)
	icon.Update(inbox.Counts{Done: 2}, true)
	if len(got) != 3 {
		t.Fatalf("two changes set %d times in all", len(got))
	}
	if bytes.Equal(got[0], got[1]) {
		t.Error("a changed count drew the same image")
	}
}
