package htmlbag

import (
	"bytes"
	"os"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

func TestFlexRowLayout(t *testing.T) {
	df := newTestDocument(t)
	containerWidth := bag.MustSP("600pt")
	sans := df.FindFontFamily("sans")
	fontSize := bag.MustSP("10pt")

	child1 := makeText("div", []any{
		makeText("", []any{"Child 1"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)
	child2 := makeText("div", []any{
		makeText("", []any{"Child 2"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)
	child3 := makeText("div", []any{
		makeText("", []any{"Child 3"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)

	container := makeText("div", []any{child1, child2, child3},
		frontend.SettingBox, true,
		SettingDisplayFlex, true,
	)

	cb := &CSSBuilder{frontend: df, PendingVLists: map[string]*node.VList{}}
	vl, err := cb.buildVlistInternal(container, containerWidth)
	if err != nil {
		t.Fatal("buildVlistInternal:", err)
	}

	expectedChildWidth := containerWidth / 3
	tolerance := bag.MustSP("2pt")

	childWidths := collectFlexChildWidths(t, vl)
	if len(childWidths) < 3 {
		t.Fatalf("expected 3 flex children, got %d", len(childWidths))
	}
	for i, w := range childWidths[:3] {
		diff := w - expectedChildWidth
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("child %d width = %v, want %v ±%v", i, w, expectedChildWidth, tolerance)
		}
	}
}

func TestFlexRowWithGap(t *testing.T) {
	df := newTestDocument(t)
	containerWidth := bag.MustSP("600pt")
	gapSize := bag.MustSP("12px")
	sans := df.FindFontFamily("sans")
	fontSize := bag.MustSP("10pt")

	child1 := makeText("div", []any{
		makeText("", []any{"A"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)
	child2 := makeText("div", []any{
		makeText("", []any{"B"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)
	child3 := makeText("div", []any{
		makeText("", []any{"C"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)

	container := makeText("div", []any{child1, child2, child3},
		frontend.SettingBox, true,
		SettingDisplayFlex, true,
		SettingGap, gapSize,
	)

	cb := &CSSBuilder{frontend: df, PendingVLists: map[string]*node.VList{}}
	vl, err := cb.buildVlistInternal(container, containerWidth)
	if err != nil {
		t.Fatal("buildVlistInternal:", err)
	}

	totalGap := gapSize * 2
	expectedChildWidth := (containerWidth - totalGap) / 3
	tolerance := bag.MustSP("2pt")

	childWidths := collectFlexChildWidths(t, vl)
	if len(childWidths) < 3 {
		t.Fatalf("expected 3 flex children, got %d", len(childWidths))
	}
	for i, w := range childWidths[:3] {
		diff := w - expectedChildWidth
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("child %d width = %v, want %v ±%v", i, w, expectedChildWidth, tolerance)
		}
	}
}

func TestFlexMixedGrowFixed(t *testing.T) {
	df := newTestDocument(t)
	containerWidth := bag.MustSP("600pt")
	fixedWidth := bag.MustSP("80pt")
	sans := df.FindFontFamily("sans")
	fontSize := bag.MustSP("10pt")

	child1 := makeText("div", []any{
		makeText("", []any{"Grow"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)
	child2 := makeText("div", []any{
		makeText("", []any{"Fix"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 0.0, SettingFlexBasisAuto, true,
		frontend.SettingWidth, "80pt")
	child3 := makeText("div", []any{
		makeText("", []any{"Grow"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)

	container := makeText("div", []any{child1, child2, child3},
		frontend.SettingBox, true,
		SettingDisplayFlex, true,
	)

	cb := &CSSBuilder{frontend: df, PendingVLists: map[string]*node.VList{}}
	vl, err := cb.buildVlistInternal(container, containerWidth)
	if err != nil {
		t.Fatal("buildVlistInternal:", err)
	}

	childWidths := collectFlexChildWidths(t, vl)
	if len(childWidths) < 3 {
		t.Fatalf("expected 3 flex children, got %d", len(childWidths))
	}

	tolerance := bag.MustSP("2pt")

	diff := childWidths[1] - fixedWidth
	if diff < 0 {
		diff = -diff
	}
	if diff > tolerance {
		t.Errorf("fixed child width = %v, want %v ±%v", childWidths[1], fixedWidth, tolerance)
	}

	expectedGrowWidth := (containerWidth - fixedWidth) / 2
	for _, i := range []int{0, 2} {
		diff := childWidths[i] - expectedGrowWidth
		if diff < 0 {
			diff = -diff
		}
		if diff > tolerance {
			t.Errorf("grow child %d width = %v, want %v ±%v", i, childWidths[i], expectedGrowWidth, tolerance)
		}
	}
}

func TestNestedFlexCentering(t *testing.T) {
	df := newTestDocument(t)
	containerWidth := bag.MustSP("300pt")
	sans := df.FindFontFamily("sans")
	fontSize := bag.MustSP("10pt")

	innerChild := makeText("span", []any{"centered"},
		frontend.SettingFontFamily, sans,
		frontend.SettingSize, fontSize,
	)
	innerFlex := makeText("div", []any{innerChild},
		frontend.SettingBox, true,
		SettingDisplayFlex, true,
		SettingJustifyContent, "center",
	)
	outerChild := makeText("div", []any{
		makeText("", []any{"left"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true, SettingFlexGrow, 1.0)

	container := makeText("div", []any{outerChild, innerFlex},
		frontend.SettingBox, true,
		SettingDisplayFlex, true,
	)

	cb := &CSSBuilder{frontend: df, PendingVLists: map[string]*node.VList{}}
	vl, err := cb.buildVlistInternal(container, containerWidth)
	if err != nil {
		t.Fatal("buildVlistInternal:", err)
	}
	if vl == nil {
		t.Fatal("expected non-nil VList")
	}
}

func TestUnknownFlexValueFallback(t *testing.T) {
	df := newTestDocument(t)
	containerWidth := bag.MustSP("400pt")
	sans := df.FindFontFamily("sans")
	fontSize := bag.MustSP("10pt")

	child := makeText("div", []any{
		makeText("", []any{"content"}, frontend.SettingFontFamily, sans, frontend.SettingSize, fontSize),
	}, frontend.SettingBox, true)

	container := makeText("div", []any{child},
		frontend.SettingBox, true,
		SettingDisplayFlex, true,
	)

	cb := &CSSBuilder{frontend: df, PendingVLists: map[string]*node.VList{}}

	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	styles := &FormattingStyles{}
	attrs := map[string]string{"flex-direction": "column"}
	_ = StylesToStyles(styles, attrs, df, fontSize)

	w.Close()
	var buf bytes.Buffer
	buf.ReadFrom(r)
	os.Stderr = oldStderr

	if !bytes.Contains(buf.Bytes(), []byte("unsupported flex value")) {
		t.Error("expected stderr warning for flex-direction: column")
	}

	vl, err := cb.buildVlistInternal(container, containerWidth)
	if err != nil {
		t.Fatal("should not panic or error:", err)
	}
	if vl == nil {
		t.Fatal("expected non-nil VList")
	}
}

func TestMinWidthParsing(t *testing.T) {
	df := newTestDocument(t)
	fontSize := bag.MustSP("10pt")

	tests := []struct {
		name  string
		value string
	}{
		{"zero", "0"},
		{"80px", "80px"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			styles := &FormattingStyles{DefaultFontSize: fontSize}
			attrs := map[string]string{"min-width": tt.value}

			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			err := StylesToStyles(styles, attrs, df, fontSize)

			w.Close()
			var buf bytes.Buffer
			buf.ReadFrom(r)
			os.Stderr = oldStderr

			if err != nil {
				t.Fatalf("StylesToStyles error: %v", err)
			}
			if !styles.minWidthSet {
				t.Error("expected minWidthSet to be true")
			}
			if buf.Len() > 0 {
				t.Errorf("unexpected stderr output: %s", buf.String())
			}
		})
	}
}

func collectFlexChildWidths(t *testing.T, vl *node.VList) []bag.ScaledPoint {
	t.Helper()
	var widths []bag.ScaledPoint
	hl := findFirstHList(vl)
	if hl == nil {
		t.Fatal("no HList found in flex VList")
	}
	for cur := hl.List; cur != nil; cur = cur.Next() {
		switch n := cur.(type) {
		case *node.VList:
			widths = append(widths, n.Width)
		case *node.HList:
			widths = append(widths, n.Width)
		}
	}
	return widths
}

func findFirstHList(vl *node.VList) *node.HList {
	for cur := vl.List; cur != nil; cur = cur.Next() {
		switch n := cur.(type) {
		case *node.HList:
			return n
		case *node.VList:
			if hl := findFirstHList(n); hl != nil {
				return hl
			}
		}
	}
	return nil
}
