package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// TestBuildTableInBorderedContainer verifies that a table inside a container
// with borders/background receives the content-area width (container width
// minus borders and padding), not the full container width.
func TestBuildTableInBorderedContainer(t *testing.T) {
	df := newTestDocument(t)

	borderWidth := bag.MustSP("2pt")
	padding := bag.MustSP("10pt")
	containerWidth := bag.MustSP("200pt")

	// Expected content width = 200pt - 2*2pt(borders) - 2*10pt(padding) = 176pt
	expectedContentWidth := containerWidth - 2*borderWidth - 2*padding

	// Build a container div with border and background settings, containing a
	// width:100% table. The table's MaxWidth should equal the content area.
	// Cell content must be wrapped in *frontend.Text with font settings so
	// FormatParagraph can process text.
	sans := df.FindFontFamily("sans")
	fontSize := bag.MustSP("10pt")
	cell1Content := makeText("", []any{"cell1"},
		frontend.SettingFontFamily, sans,
		frontend.SettingSize, fontSize,
	)
	cell2Content := makeText("", []any{"cell2"},
		frontend.SettingFontFamily, sans,
		frontend.SettingSize, fontSize,
	)
	table := makeText("table", []any{
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", []any{cell1Content}),
				makeText("td", []any{cell2Content}),
			}),
		}),
	}, frontend.SettingWidth, "100%")

	container := makeText("div", []any{table},
		frontend.SettingBox, true,
		frontend.SettingBorderLeftWidth, borderWidth,
		frontend.SettingBorderRightWidth, borderWidth,
		frontend.SettingPaddingLeft, padding,
		frontend.SettingPaddingRight, padding,
		frontend.SettingBackgroundColor, &color.Color{Space: color.ColorGray, A: 1, R: 0.9},
	)

	cb := &CSSBuilder{
		frontend:      df,
		PendingVLists: map[string]*node.VList{},
	}

	vl, err := cb.buildVlistInternal(container, containerWidth)
	if err != nil {
		t.Fatal("buildVlistInternal:", err)
	}

	// The resulting VList should have width equal to the content area
	// (HTMLBorder adds border/padding back visually, but the content fits
	// within expectedContentWidth).
	// The outer VList width after HTMLBorder should equal containerWidth.
	if vl.Width != containerWidth {
		t.Errorf("outer VList width = %v, want %v (container width after HTMLBorder)", vl.Width, containerWidth)
	}

	// HTMLBorder wraps the content in: outer VList → HList → inner VList
	// (with Glue nodes for padding/borders). Walk through to find the
	// table VList inside the inner content VList.
	var tableVL *node.VList
	for cur := vl.List; cur != nil; cur = cur.Next() {
		hl, ok := cur.(*node.HList)
		if !ok {
			continue
		}
		for hcur := hl.List; hcur != nil; hcur = hcur.Next() {
			inner, ok := hcur.(*node.VList)
			if !ok {
				continue
			}
			// inner is the "buildVListInternal" content VList.
			// The table VList is a direct child of it.
			for vcur := inner.List; vcur != nil; vcur = vcur.Next() {
				if tvl, ok := vcur.(*node.VList); ok {
					tableVL = tvl
					break
				}
			}
			if tableVL != nil {
				break
			}
		}
		if tableVL != nil {
			break
		}
	}
	if tableVL == nil {
		t.Fatal("no table VList found inside bordered container")
	}
	if tableVL.Width > expectedContentWidth+bag.MustSP("1pt") {
		t.Errorf("table VList width = %v, exceeds content area %v", tableVL.Width, expectedContentWidth)
	}
}
