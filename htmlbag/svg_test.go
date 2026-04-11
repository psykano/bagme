package htmlbag

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
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

	// Initially rendered at 50% of DefaultPageWidth (400pt) = 200pt.
	initialWant := bag.MustSP("200pt")
	if vl.Width != initialWant {
		t.Fatalf("initial VList.Width = %v pt, want %v pt", vl.Width.ToPT(), initialWant.ToPT())
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
