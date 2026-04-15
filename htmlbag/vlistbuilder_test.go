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

// TestRecalcVListHeight verifies that recalcVListHeight recomputes an
// enclosing VList's Height+Depth by summing children via vlistNodeHeight,
// overwriting any stale cached value that was captured before dynamic
// content (e.g. a percentage-width SVG re-rendered at its real container
// width) changed a child's height. This is the height-propagation helper
// the pagination fit check relies on.
func TestRecalcVListHeight(t *testing.T) {
	vl := node.NewVList()

	r1 := node.NewRule()
	r1.Height = bag.MustSP("10pt")
	r2 := node.NewRule()
	r2.Height = bag.MustSP("20pt")

	vl.List = node.InsertAfter(vl.List, node.Tail(vl.List), r1)
	vl.List = node.InsertAfter(vl.List, node.Tail(vl.List), r2)
	// Correct cached height at construction time.
	vl.Height = bag.MustSP("30pt")
	vl.Depth = 0

	// Simulate dynamic-content drift: the second child grows to 50pt after
	// a post-construction re-layout (e.g. resolveSVGWidths swapping in a
	// re-rendered SVG, or a nested table closure producing a taller VList).
	r2.Height = bag.MustSP("50pt")

	// vlistNodeHeight still reads the stale cached value.
	if got := vlistNodeHeight(vl); got != bag.MustSP("30pt") {
		t.Fatalf("pre-recalc vlistNodeHeight = %v, want 30pt (stale cached value)", got)
	}

	got := recalcVListHeight(vl)
	want := bag.MustSP("60pt") // 10pt + 50pt
	if got != want {
		t.Errorf("recalcVListHeight returned %v, want %v", got, want)
	}
	if vlistNodeHeight(vl) != want {
		t.Errorf("post-recalc vlistNodeHeight = %v, want %v (should read the freshly written value)", vlistNodeHeight(vl), want)
	}
}

// TestRecalcVListHeight_NestedVList verifies the helper descends into child
// VLists so that drift inside a nested VList is reflected in the ancestor's
// recomputed total. This matches the real-world case where a mixed-content
// cell's wrapper VList holds a heading + SVG child whose height was stale.
func TestRecalcVListHeight_NestedVList(t *testing.T) {
	// Inner VList with one rule that will grow.
	inner := node.NewVList()
	innerRule := node.NewRule()
	innerRule.Height = bag.MustSP("10pt")
	inner.List = node.InsertAfter(inner.List, node.Tail(inner.List), innerRule)
	inner.Height = bag.MustSP("10pt")

	// Outer VList with a sibling rule plus the inner VList.
	outer := node.NewVList()
	sibling := node.NewRule()
	sibling.Height = bag.MustSP("15pt")
	outer.List = node.InsertAfter(outer.List, node.Tail(outer.List), sibling)
	outer.List = node.InsertAfter(outer.List, node.Tail(outer.List), inner)
	outer.Height = bag.MustSP("25pt") // 15pt + 10pt

	// Inner rule grows after construction — outer.Height is now doubly stale.
	innerRule.Height = bag.MustSP("40pt")

	got := recalcVListHeight(outer)
	want := bag.MustSP("55pt") // 15pt + 40pt
	if got != want {
		t.Errorf("recalcVListHeight returned %v, want %v", got, want)
	}
	if inner.Height != bag.MustSP("40pt") {
		t.Errorf("nested inner VList Height = %v, want 40pt (should have been updated)", inner.Height)
	}
}
