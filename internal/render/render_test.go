package render

import (
	"bytes"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/sfnt"
	"soundcheck/internal/core"
)

func TestEmbeddedCyrillicFonts(t *testing.T) {
	for _, tc := range []struct {
		name string
		ttf  []byte
	}{
		{"goregular", goregular.TTF},
		{"gobold", gobold.TTF},
	} {
		f, err := sfnt.Parse(tc.ttf)
		if err != nil {
			t.Fatalf("failed to parse %s: %v", tc.name, err)
		}
		var buf sfnt.Buffer
		for r := 'А'; r <= 'я'; r++ {
			idx, err := f.GlyphIndex(&buf, r)
			if err != nil || idx == 0 {
				t.Fatalf("%s missing Cyrillic glyph %q (err=%v, idx=%d)", tc.name, r, err, idx)
			}
		}
		for _, r := range []rune{'Ё', 'ё'} {
			idx, err := f.GlyphIndex(&buf, r)
			if err != nil || idx == 0 {
				t.Fatalf("%s missing Cyrillic glyph %q (err=%v, idx=%d)", tc.name, r, err, idx)
			}
		}
	}

	regSrc, err := text.NewGoTextFaceSource(bytes.NewReader(goregular.TTF))
	if err != nil {
		t.Fatalf("failed to load goregular: %v", err)
	}
	boldSrc, err := text.NewGoTextFaceSource(bytes.NewReader(gobold.TTF))
	if err != nil {
		t.Fatalf("failed to load gobold: %v", err)
	}

	regFace := &text.GoTextFace{Source: regSrc, Size: 14}
	boldFace := &text.GoTextFace{Source: boldSrc, Size: 16}

	for _, sample := range []string{"САУНДЧЕК В 19:59", "БАСИСТ", "Ёжик", "ОГЛУШЁН"} {
		w1, h1 := text.Measure(sample, regFace, 0)
		if w1 <= 0 || h1 <= 0 {
			t.Fatalf("expected positive bounds for %q in goregular, got %fx%f", sample, w1, h1)
		}
		w2, h2 := text.Measure(sample, boldFace, 0)
		if w2 <= 0 || h2 <= 0 {
			t.Fatalf("expected positive bounds for %q in gobold, got %fx%f", sample, w2, h2)
		}
	}
}

func TestRendererLanguageToggle(t *testing.T) {
	r := &Renderer{lang: core.LangRU}

	if r.Lang() != core.LangRU {
		t.Fatalf("expected default LangRU, got %v", r.Lang())
	}
	if got := r.tr("ПРИВЕТ", "HELLO"); got != "ПРИВЕТ" {
		t.Fatalf("expected Russian string, got %q", got)
	}

	// Click outside the RU/EN button should not toggle language.
	if r.HandleClick(10, 10) {
		t.Fatalf("expected HandleClick outside button to return false")
	}
	if r.Lang() != core.LangRU {
		t.Fatalf("expected LangRU unchanged after outside click, got %v", r.Lang())
	}

	// Click inside the RU/EN button toggles to EN.
	bx := float64(langBtnX + langBtnW*0.5)
	by := float64(langBtnY + langBtnH*0.5)
	if !r.HandleClick(bx, by) {
		t.Fatalf("expected HandleClick on button to return true")
	}
	if r.Lang() != core.LangEN {
		t.Fatalf("expected LangEN after click, got %v", r.Lang())
	}
	if got := r.tr("ПРИВЕТ", "HELLO"); got != "HELLO" {
		t.Fatalf("expected English string, got %q", got)
	}

	// Second click toggles back to RU.
	if !r.HandleClick(bx, by) {
		t.Fatalf("expected second HandleClick on button to return true")
	}
	if r.Lang() != core.LangRU {
		t.Fatalf("expected LangRU after second click, got %v", r.Lang())
	}
}
