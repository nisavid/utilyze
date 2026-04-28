package tschart

import (
	"math"
	"strings"
	"testing"
)

func TestPlainLines_IsolatedPointRenders(t *testing.T) {
	cases := []struct {
		name   string
		w, h   int
		values []float64
	}{
		{"canvasW_1_single", 1, 5, []float64{50}},
		{"nan_island", 10, 5, []float64{math.NaN(), math.NaN(), 50, math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN()}},
		{"trailing_island_after_gap", 6, 5, []float64{math.NaN(), math.NaN(), math.NaN(), math.NaN(), math.NaN(), 50}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := newLineGrid(tc.w, tc.h)
			g.drawValues(tc.values, 0, 100)
			for _, m := range g.masks {
				if m != 0 {
					return
				}
			}
			t.Fatalf("no cells marked — isolated point rendered blank")
		})
	}
}

func TestPlainLines_NotchClampSkipsTinyCanvas(t *testing.T) {
	g := newLineGrid(4, 2)
	g.drawValues([]float64{1, 1}, 0, 100)
	for x := 0; x < g.canvasW; x++ {
		if g.masks[0*g.canvasW+x] != 0 {
			t.Fatalf("col %d row 0 marked for 1%%, clamp inversion at canvasH=2", x)
		}
	}
	g2 := newLineGrid(4, 2)
	g2.drawValues([]float64{99, 99}, 0, 100)
	for x := 0; x < g2.canvasW; x++ {
		if g2.masks[1*g2.canvasW+x] != 0 {
			t.Fatalf("col %d row 1 marked for 99%%, clamp inversion at canvasH=2", x)
		}
	}
}

func TestPlainLines_TickRowMatchesMapY(t *testing.T) {
	m := &Model{canvasH: 5}
	g := newLineGrid(1, 5)
	for _, pct := range []float64{0.0, 0.1, 0.15, 0.25, 0.33, 0.5, 0.67, 0.85, 1.0} {
		if got, want := m.tickRow(pct), g.mapY(pct); got != want {
			t.Errorf("pct=%.2f: tickRow=%d mapY=%d", pct, got, want)
		}
	}
}

func TestPlainLines_HorizontalSeriesStillRenders(t *testing.T) {
	g := newLineGrid(5, 5)
	g.drawValues([]float64{50, 50, 50, 50, 50}, 0, 100)
	row := g.canvasH / 2
	var s strings.Builder
	for x := 0; x < g.canvasW; x++ {
		r := g.runeAt(x, row)
		if r == 0 {
			t.Fatalf("col %d empty, line broken", x)
		}
		s.WriteRune(r)
	}
	for _, r := range s.String() {
		if r != '─' && r != '╴' && r != '╶' {
			t.Fatalf("unexpected non-horizontal rune %q in %q", r, s.String())
		}
	}
}
