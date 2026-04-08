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
