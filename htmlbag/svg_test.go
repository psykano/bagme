package htmlbag

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
	"github.com/boxesandglue/svgreader"
	"golang.org/x/net/html"
)

// findSVGNode walks an html.Node tree and returns the first <svg> element.
func findSVGNode(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && n.Data == "svg" {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findSVGNode(c); found != nil {
			return found
		}
	}
	return nil
}

func TestSerializeSVGNodeRoundTrip(t *testing.T) {
	input := `<html><body><svg viewBox="0 0 100 100" width="100" height="100"><circle cx="50" cy="50" r="40" fill="red"/></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}

	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found in parsed HTML")
	}

	var buf bytes.Buffer
	serializeSVGNode(&buf, svgNode)

	xmlOut := buf.String()
	t.Logf("serialized XML: %s", xmlOut)

	// Verify the XML can be parsed by svgreader.
	svgDoc, err := svgreader.Parse(&buf)
	if err != nil {
		t.Fatal("svgreader.Parse:", err)
	}

	// Check viewBox.
	if svgDoc.ViewBox.Width != 100 || svgDoc.ViewBox.Height != 100 {
		t.Fatalf("viewBox = %+v, want {0 0 100 100}", svgDoc.ViewBox)
	}

	// Check that child elements survived the round-trip.
	if len(svgDoc.Elements) == 0 {
		t.Fatal("no elements parsed from inline SVG")
	}

	// Assert specific element type.
	circle, ok := svgDoc.Elements[0].(svgreader.Circle)
	if !ok {
		t.Fatalf("element[0] is %T, want svgreader.Circle", svgDoc.Elements[0])
	}
	if circle.Cx != 50 || circle.Cy != 50 || circle.R != 40 {
		t.Fatalf("circle = {Cx:%v Cy:%v R:%v}, want {50 50 40}", circle.Cx, circle.Cy, circle.R)
	}
}

func TestSerializeSVGNodeWithText(t *testing.T) {
	input := `<html><body><svg viewBox="0 0 200 200"><text x="10" y="80">Hello</text></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}

	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found")
	}

	var buf bytes.Buffer
	serializeSVGNode(&buf, svgNode)

	svgDoc, err := svgreader.Parse(&buf)
	if err != nil {
		t.Fatal("svgreader.Parse:", err)
	}
	if len(svgDoc.Elements) == 0 {
		t.Fatal("no elements parsed")
	}

	text, ok := svgDoc.Elements[0].(svgreader.Text)
	if !ok {
		t.Fatalf("element[0] is %T, want svgreader.Text", svgDoc.Elements[0])
	}
	if text.Content != "Hello" {
		t.Fatalf("text content = %q, want %q", text.Content, "Hello")
	}
}

func TestSerializeSVGNodeEscaping(t *testing.T) {
	// Attribute value with quotes and ampersand.
	input := `<html><body><svg viewBox="0 0 100 100"><rect x="0" y="0" width="100" height="100" data-info="a&amp;b"/></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}

	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found")
	}

	var buf bytes.Buffer
	serializeSVGNode(&buf, svgNode)

	// Should produce valid XML (svgreader.Parse only cares about known
	// elements, but the XML decoder would reject malformed XML).
	_, err = svgreader.Parse(&buf)
	if err != nil {
		t.Fatal("svgreader.Parse on escaped SVG:", err)
	}
}

func TestCollectHorizontalNodesSVG(t *testing.T) {
	// Parse HTML to get the SVG html.Node.
	input := `<html><body><svg viewBox="0 0 100 100" width="100" height="100"><circle cx="50" cy="50" r="40" fill="red"/></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}

	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found")
	}

	// Build an HTMLItem that mimics what selection.go would produce.
	item := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "svg",
		Dir:        ModeHorizontal,
		Attributes: map[string]string{},
		Styles:     map[string]string{},
		OrigNode:   svgNode,
	}

	// Set up minimal dependencies.
	df, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}

	var ss StylesStack
	ss.PushStyles()

	te := frontend.NewText()
	defaultFontsize := bag.MustSP("10pt")
	currentFontsize := defaultFontsize

	err = collectHorizontalNodes(te, item, ss, currentFontsize, defaultFontsize, df)
	if err != nil {
		t.Fatal("collectHorizontalNodes:", err)
	}

	// Assert a VList with origin "inline-svg" was emitted.
	if len(te.Items) == 0 {
		t.Fatal("no items emitted")
	}
	vl, ok := te.Items[0].(*node.VList)
	if !ok {
		t.Fatalf("te.Items[0] is %T, want *node.VList", te.Items[0])
	}
	origin, ok := vl.Attributes["origin"]
	if !ok || origin != "inline-svg" {
		t.Fatalf("VList origin = %v, want %q", origin, "inline-svg")
	}
}

func TestCollectHorizontalNodesSVGNilOrigNode(t *testing.T) {
	item := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "svg",
		Dir:        ModeHorizontal,
		Attributes: map[string]string{},
		Styles:     map[string]string{},
		// OrigNode intentionally nil.
	}

	df, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}

	var ss StylesStack
	ss.PushStyles()

	te := frontend.NewText()
	defaultFontsize := bag.MustSP("10pt")
	currentFontsize := defaultFontsize

	err = collectHorizontalNodes(te, item, ss, currentFontsize, defaultFontsize, df)
	if err == nil {
		t.Fatal("expected error for nil OrigNode, got nil")
	}
	if !strings.Contains(err.Error(), "missing original node") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSVGPercentageWidth(t *testing.T) {
	// SVG with no explicit width/height — viewBox only.
	input := `<html><body><svg viewBox="0 0 100 100"><rect x="0" y="0" width="100" height="100"/></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}
	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found")
	}

	item := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "svg",
		Dir:        ModeHorizontal,
		Attributes: map[string]string{},
		Styles:     map[string]string{"width": "50%"},
		OrigNode:   svgNode,
	}

	df, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}
	df.Doc.DefaultPageWidth = bag.MustSP("200pt")

	var ss StylesStack
	ss.PushStyles()

	te := frontend.NewText()
	defaultFontsize := bag.MustSP("10pt")

	if err = collectHorizontalNodes(te, item, ss, defaultFontsize, defaultFontsize, df); err != nil {
		t.Fatal("collectHorizontalNodes:", err)
	}
	if len(te.Items) == 0 {
		t.Fatal("no items emitted")
	}
	vl, ok := te.Items[0].(*node.VList)
	if !ok {
		t.Fatalf("te.Items[0] is %T, want *node.VList", te.Items[0])
	}
	want := bag.MustSP("100pt")
	if vl.Width != want {
		t.Errorf("VList.Width = %v (%v pt), want %v (%v pt)", vl.Width, vl.Width.ToPT(), want, want.ToPT())
	}
}

func TestSVGExplicitDimensionsUnaffected(t *testing.T) {
	// SVG with explicit px/pt dimensions via CSS — existing code path unchanged.
	input := `<html><body><svg viewBox="0 0 72 72"><circle cx="36" cy="36" r="36"/></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}
	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found")
	}

	item := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "svg",
		Dir:        ModeHorizontal,
		Attributes: map[string]string{},
		Styles:     map[string]string{"width": "72pt", "height": "72pt"},
		OrigNode:   svgNode,
	}

	df, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}

	var ss StylesStack
	ss.PushStyles()

	te := frontend.NewText()
	defaultFontsize := bag.MustSP("10pt")

	if err = collectHorizontalNodes(te, item, ss, defaultFontsize, defaultFontsize, df); err != nil {
		t.Fatal("collectHorizontalNodes:", err)
	}
	if len(te.Items) == 0 {
		t.Fatal("no items emitted")
	}
	vl, ok := te.Items[0].(*node.VList)
	if !ok {
		t.Fatalf("te.Items[0] is %T, want *node.VList", te.Items[0])
	}
	want := bag.MustSP("72pt")
	if vl.Width != want {
		t.Errorf("VList.Width = %v pt, want %v pt", vl.Width.ToPT(), want.ToPT())
	}
}

func TestSVGHeightPercentageIsAuto(t *testing.T) {
	// SVG height="50%" must be treated as auto (ht=0); no error expected.
	input := `<html><body><svg viewBox="0 0 100 100"><rect x="0" y="0" width="100" height="100"/></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}
	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found")
	}

	item := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "svg",
		Dir:        ModeHorizontal,
		Attributes: map[string]string{},
		Styles:     map[string]string{"height": "50%"},
		OrigNode:   svgNode,
	}

	df, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}

	var ss StylesStack
	ss.PushStyles()

	te := frontend.NewText()
	defaultFontsize := bag.MustSP("10pt")

	if err = collectHorizontalNodes(te, item, ss, defaultFontsize, defaultFontsize, df); err != nil {
		t.Fatal("collectHorizontalNodes:", err)
	}
	if len(te.Items) == 0 {
		t.Fatal("no items emitted")
	}
	if _, ok := te.Items[0].(*node.VList); !ok {
		t.Fatalf("te.Items[0] is %T, want *node.VList", te.Items[0])
	}
}

// TestSVGText verifies that SVG text rendering is not distorted by a wrong
// aspect ratio when the SVG element carries width="100%".
//
// Root cause: parseDimension("100%") stripped "%" and returned 100, so
// doc.Width was set to 100 instead of 0. The viewBox fallback (which would
// have set doc.Width=500 from the viewBox) never triggered. This caused the
// CTM scale factors sx=wPt/100 and sy=hPt/160 to be wildly wrong, making
// glyphs appear 5x taller than intended and inserting apparent gaps between
// characters in SVG text elements that had text-anchor="middle".
func TestSVGText(t *testing.T) {
	t.Run("PercentageWidthUsesViewBox", func(t *testing.T) {
		// Mirrors the real report SVGs: width="100%" with a viewBox.
		// After the fix, doc.Width must equal the viewBox width (500), not 100.
		svgXML := `<svg xmlns="http://www.w3.org/2000/svg" width="100%" height="160" viewBox="0 0 500 160">` +
			`<text text-anchor="middle" x="250" y="80">2.5</text></svg>`
		doc, err := svgreader.Parse(strings.NewReader(svgXML))
		if err != nil {
			t.Fatal("svgreader.Parse:", err)
		}
		if doc.Width != 500 {
			t.Errorf("doc.Width = %v, want 500 (viewBox width); got 100 means the %% fix is missing", doc.Width)
		}
		if doc.Height != 160 {
			t.Errorf("doc.Height = %v, want 160", doc.Height)
		}
	})

	t.Run("FixedWidthUnchanged", func(t *testing.T) {
		// SVG with an explicit numeric width must continue to parse correctly.
		// This is the code path used by donut-chart SVGs (width="220"), which
		// rendered correctly before the fix and must not regress.
		svgXML := `<svg xmlns="http://www.w3.org/2000/svg" width="220" height="220" viewBox="0 0 220 220">` +
			`<text x="110" y="110">test</text></svg>`
		doc, err := svgreader.Parse(strings.NewReader(svgXML))
		if err != nil {
			t.Fatal("svgreader.Parse:", err)
		}
		if doc.Width != 220 {
			t.Errorf("doc.Width = %v, want 220", doc.Width)
		}
		if doc.Height != 220 {
			t.Errorf("doc.Height = %v, want 220", doc.Height)
		}
	})
}

func TestSVGInTableCell(t *testing.T) {
	// SVG with viewBox only — CSS width="50%" should resolve to 50% of container.
	input := `<html><body><svg viewBox="0 0 100 100"><rect x="0" y="0" width="100" height="100"/></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}
	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found")
	}

	item := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "svg",
		Dir:        ModeHorizontal,
		Attributes: map[string]string{},
		Styles:     map[string]string{"width": "50%"},
		OrigNode:   svgNode,
	}

	df, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}
	df.Doc.DefaultPageWidth = bag.MustSP("400pt")

	var ss StylesStack
	ss.PushStyles()

	te := frontend.NewText()
	defaultFontsize := bag.MustSP("10pt")

	if err = collectHorizontalNodes(te, item, ss, defaultFontsize, defaultFontsize, df); err != nil {
		t.Fatal("collectHorizontalNodes:", err)
	}
	if len(te.Items) == 0 {
		t.Fatal("no items emitted")
	}
	vl, ok := te.Items[0].(*node.VList)
	if !ok {
		t.Fatalf("te.Items[0] is %T, want *node.VList", te.Items[0])
	}

	// Task C / D3: percentage-width SVGs are sized lazily. At
	// construction time the wrapper uses the SVG's natural dimensions
	// (derived from viewBox — 100×100 here) as a placeholder; the
	// real size is resolved later by resolveSVGWidths / materializeSVG
	// at the consumer's known container width. Before Task C this
	// assertion read 200pt (50% of DefaultPageWidth=400pt) because the
	// construction path eagerly resolved against page width; it now
	// reads the natural viewBox width instead, which is also what
	// CreateSVGNodeFromDocument returns when called with wd=0, ht=0.
	initialWant := bag.MustSP("100pt")
	if vl.Width != initialWant {
		t.Fatalf("initial VList.Width = %v pt, want %v pt (natural viewBox width; Task C defers percentage resolution to the consumer)", vl.Width.ToPT(), initialWant.ToPT())
	}

	// Verify percentage metadata was stored.
	pct, ok := vl.Attributes["svg-width-pct"].(float64)
	if !ok || pct != 50 {
		t.Fatalf("svg-width-pct = %v, want 50", vl.Attributes["svg-width-pct"])
	}
	if _, ok := vl.Attributes["svg-doc"]; !ok {
		t.Fatal("svg-doc attribute missing")
	}

	// Simulate table cell layout: resolve SVGs at a cell width of 200pt.
	// 50% of 200pt = 100pt.
	cellWidth := bag.MustSP("200pt")
	resolveSVGWidths(te.Items, cellWidth, df)

	resolved, ok := te.Items[0].(*node.VList)
	if !ok {
		t.Fatalf("after resolve, te.Items[0] is %T, want *node.VList", te.Items[0])
	}
	resolvedWant := bag.MustSP("100pt")
	if resolved.Width != resolvedWant {
		t.Errorf("resolved VList.Width = %v pt, want %v pt (50%% of 200pt)", resolved.Width.ToPT(), resolvedWant.ToPT())
	}
	// Verify it's still marked as inline-svg.
	if origin, ok := resolved.Attributes["origin"]; !ok || origin != "inline-svg" {
		t.Errorf("resolved VList origin = %v, want %q", resolved.Attributes["origin"], "inline-svg")
	}
}

// TestSVGWithTextSibling is a regression test for the bug where an inline SVG
// followed by a text sibling produced garbled output. The SVG case in
// collectHorizontalNodes previously lacked a return nil, causing SVG internal
// nodes to be processed as text and appended to te.Items alongside the span text.
func TestSVGWithTextSibling(t *testing.T) {
	// SVG with a <title> child that contains text — this is the key: the SVG
	// HTMLItem has Children, which the bug would process as inline text.
	input := `<html><body><svg viewBox="0 0 100 100" width="100" height="100"><title>elastio</title><circle cx="50" cy="50" r="40"/></svg></body></html>`
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal("html.Parse:", err)
	}

	svgNode := findSVGNode(doc)
	if svgNode == nil {
		t.Fatal("no <svg> element found")
	}

	// Build SVG HTMLItem WITH children to simulate the real parsed structure.
	// The title child contains a text node "elastio" — without the fix this
	// would leak into te.Items as a stray string.
	svgItem := &HTMLItem{
		Typ:        html.ElementNode,
		Data:       "svg",
		Dir:        ModeHorizontal,
		Attributes: map[string]string{},
		Styles:     map[string]string{},
		OrigNode:   svgNode,
		Children: []*HTMLItem{
			{
				Typ:  html.ElementNode,
				Data: "title",
				Dir:  ModeHorizontal,
				Children: []*HTMLItem{
					{Typ: html.TextNode, Data: "elastio"},
				},
			},
		},
	}

	// The span text sibling.
	spanTextItem := &HTMLItem{
		Typ:  html.TextNode,
		Data: "elastio",
	}

	df, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}

	var ss StylesStack
	ss.PushStyles()

	te := frontend.NewText()
	defaultFontsize := bag.MustSP("10pt")
	currentFontsize := defaultFontsize

	// Process the SVG item.
	if err = collectHorizontalNodes(te, svgItem, ss, currentFontsize, defaultFontsize, df); err != nil {
		t.Fatal("collectHorizontalNodes (svg):", err)
	}
	// Process the span text sibling.
	if err = collectHorizontalNodes(te, spanTextItem, ss, currentFontsize, defaultFontsize, df); err != nil {
		t.Fatal("collectHorizontalNodes (text):", err)
	}

	// Must have exactly 2 items: VList (SVG) + string ("elastio").
	if len(te.Items) != 2 {
		t.Fatalf("te.Items has %d items, want 2; items: %v", len(te.Items), te.Items)
	}

	// First item must be the SVG VList.
	vl, ok := te.Items[0].(*node.VList)
	if !ok {
		t.Fatalf("te.Items[0] is %T, want *node.VList", te.Items[0])
	}
	if origin, _ := vl.Attributes["origin"].(string); origin != "inline-svg" {
		t.Fatalf("VList origin = %q, want %q", origin, "inline-svg")
	}

	// Second item must be the span text string — no stray SVG-internal strings.
	if s, ok := te.Items[1].(string); !ok || s != "elastio" {
		t.Fatalf("te.Items[1] = %v (%T), want string %q", te.Items[1], te.Items[1], "elastio")
	}
}

// TestBuildTDInlineSVGBypass verifies that buildTD passes an inline-SVG VList
// through directly without routing it through CreateVlist/FormatParagraph, which
// would serialize the SVG nodes as text runs instead of rendering the graphic.
func TestBuildTDInlineSVGBypass(t *testing.T) {
	// svgVL represents what collectHorizontalNodes produces for an <svg> child:
	// a *node.VList with origin "inline-svg".
	svgVL := node.NewVList()
	svgVL.Attributes = node.H{"origin": "inline-svg"}
	svgVL.Width = bag.MustSP("20pt")
	svgVL.Height = bag.MustSP("20pt")

	// cld1 is the *frontend.Text wrapper that collectHorizontalNodes creates for
	// the SVG child (each child gets a new Text at inheritablestyles.go:1115).
	cld1 := frontend.NewText()
	cld1.Items = append(cld1.Items, svgVL)

	// te is the TD's Text; its Items contains the child wrappers.
	te := frontend.NewText()
	te.Items = append(te.Items, cld1)

	// Minimal CSSBuilder: only PendingVLists is needed by buildTD.
	// cb.frontend is accessed via resolveSVGWidths, but the SVG has no
	// svg-width-pct attribute so that path is not taken.
	cb := &CSSBuilder{PendingVLists: map[string]*node.VList{}}
	row := &frontend.TableRow{}

	cb.buildTD(te, row, false)

	if len(row.Cells) != 1 {
		t.Fatalf("expected 1 cell, got %d", len(row.Cells))
	}
	cell := row.Cells[0]
	if len(cell.Contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(cell.Contents))
	}
	ftv, ok := cell.Contents[0].(frontend.FormatToVList)
	if !ok {
		t.Fatalf("Contents[0] is %T, want frontend.FormatToVList", cell.Contents[0])
	}

	// Invoke the closure — must return the original SVG VList directly,
	// not a new one produced by CreateVlist/FormatParagraph.
	got, err := ftv(bag.MustSP("200pt"))
	if err != nil {
		t.Fatal("FormatToVList closure:", err)
	}
	if got != svgVL {
		t.Fatal("FormatToVList closure returned different VList; SVG bypass did not activate")
	}
}

func TestIsCSSLength(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"200px", true},
		{"10em", true},
		{"12pt", true},
		{"2.5cm", true},
		{"1in", true},
		{".5rem", true},
		{"0", true},
		{"0.0", true},
		{".0", true},
		{"+3mm", true},
		{"-1.5pc", true},
		{"auto", false},
		{"inherit", false},
		{"unset", false},
		{"none", false},
		{"50%", false},
		{"boguspx", false},
		{"", false},
		{"100", false},   // unitless non-zero
		{"px", false},    // unit without number
		{" 10px ", true}, // whitespace trimmed
		{"10PX", true},   // case insensitive
	}

	for _, tt := range tests {
		got := isCSSLength(tt.input)
		if got != tt.want {
			t.Errorf("isCSSLength(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

// TestInlineSVG_PercentWidth_ResolvedAgainstContainer is the Task C TDD
// anchor (D3, Section C). It asserts the new deferred-sizing contract:
// a percentage-width inline SVG must be sized against the real container
// width it ends up inside, not against df.Doc.DefaultPageWidth.
//
// The PRD test wording targets a 50% <td>; that path already worked
// before Task C because buildTD's FormatToVList closure calls
// resolveSVGWidths at the final cell width. The path this test exercises
// instead is the non-table block path — a <div><svg style="width:100%"/>
// </div> flows through buildVlistInternal's leaf branch into
// FormatParagraph without ever touching buildTD, and therefore never
// re-materialized its SVG. That is the direct cause of the summary-page
// Data Integrity donut and any other block-level percentage SVG
// rendering at page width instead of the container it lives inside.
//
// The test uses a <svg style="width:100%"> rather than 50% specifically
// because collectHorizontalNodes wraps <svg> in an enclosing
// *frontend.Text (svgTe) that inherits the same width style via
// ApplySettings. buildVlistInternal re-applies that percentage to compute
// the wrapper's own width, so a 50%-wide SVG inside a 400pt container
// would end up at 100pt (50% of 50%) and blur the very thing this test
// is checking. Using 100% keeps the wrapper width equal to the container
// and lets the assertion speak unambiguously about the SVG itself.
//
// Failing mode before Task C: construction caches
// wd = DefaultPageWidth * 100 / 100 = DefaultPageWidth (800pt in this
// test). The block path never calls resolveSVGWidths, so the final SVG
// VList width stays at 800pt — not the 400pt the 400pt container should
// yield.
//
// Passing mode after Task C: construction stores svg-width-pct only and
// leaves the SVG at its natural (viewBox) dimensions; the leaf branch of
// buildVlistInternal materializes the SVG against contentWidth via
// resolveSVGWidths right before FormatParagraph runs, so the final width
// matches contentWidth * widthPct / 100 = 400pt.
func TestInlineSVG_PercentWidth_ResolvedAgainstContainer(t *testing.T) {
	df := newTestDocument(t)
	// DefaultPageWidth is intentionally set much larger than the
	// CreateVlist container width below so that the two values cannot
	// coincide — a passing final width must prove the SVG was sized
	// against the container, not against DefaultPageWidth.
	df.Doc.DefaultPageWidth = bag.MustSP("800pt")

	cs := csshtml.NewCSSParserWithDefaults()
	cb, err := New(df, cs)
	if err != nil {
		t.Fatal("New:", err)
	}
	if err := cb.ParseCSSString(`@page { size: 800pt 600pt; margin: 0 }`); err != nil {
		t.Fatal("ParseCSSString:", err)
	}

	const htmlSrc = `<html><body><div>` +
		`<svg style="width:100%" viewBox="0 0 100 100"><rect x="0" y="0" width="100" height="100" fill="red"/></svg>` +
		`</div></body></html>`

	te, err := cb.HTMLToText(htmlSrc)
	if err != nil {
		t.Fatal("HTMLToText:", err)
	}

	containerWidth := bag.MustSP("400pt")
	rootVL, err := cb.CreateVlist(te, containerWidth)
	if err != nil {
		t.Fatal("CreateVlist:", err)
	}

	// Walk the result tree and locate the first node.VList carrying the
	// inline-svg origin — that is the wrapper collectHorizontalNodes
	// produced for <svg> and whose width the fix updates.
	var svgVL *node.VList
	var walk func(n node.Node)
	walk = func(n node.Node) {
		for cur := n; cur != nil; cur = cur.Next() {
			switch v := cur.(type) {
			case *node.VList:
				if svgVL == nil && v.Attributes != nil {
					if origin, _ := v.Attributes["origin"].(string); origin == "inline-svg" {
						svgVL = v
						return
					}
				}
				walk(v.List)
			case *node.HList:
				walk(v.List)
			}
		}
	}
	walk(rootVL.List)

	if svgVL == nil {
		t.Fatal("no inline-svg VList found in CreateVlist output — " +
			"block path did not materialize the SVG at all")
	}

	// The div sits at the top level of the body with no borders or
	// padding, so the effective contentWidth reaching the leaf branch
	// equals containerWidth. A 100% SVG inside a 400pt container should
	// be 400pt.
	want := bag.MustSP("400pt")
	if svgVL.Width != want {
		t.Errorf("inline SVG final Width = %v pt, want %v pt (100%% of 400pt container); "+
			"if Width == 800pt the SVG is still sized against DefaultPageWidth (800pt × 100%%) — "+
			"block path does not re-materialize percentage SVGs against the real container",
			svgVL.Width.ToPT(), want.ToPT())
	}
}
