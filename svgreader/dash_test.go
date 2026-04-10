package svgreader

import (
	"strings"
	"testing"
)

func TestParseDashArray(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []float64
	}{
		{"basic two values", "389.2 440", []float64{389.2, 440}},
		{"comma separated", "389.2,440", []float64{389.2, 440}},
		{"comma and space", "389.2, 440", []float64{389.2, 440}},
		{"odd length doubled", "5 10 15", []float64{5, 10, 15, 5, 10, 15}},
		{"single value doubled", "10", []float64{10, 10}},
		{"none", "none", nil},
		{"None mixed case", "None", nil},
		{"NONE upper", "NONE", nil},
		{"empty string", "", nil},
		{"whitespace only", "   ", nil},
		{"negative value", "-5 10", nil},
		{"unparseable", "abc 10", nil},
		{"four values even", "5 3 9 2", []float64{5, 3, 9, 2}},
		{"decimal precision", "0.5 1.25", []float64{0.5, 1.25}},
		{"zero values", "0 10", []float64{0, 10}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseDashArray(tt.in)
			if tt.want == nil {
				if got != nil {
					t.Errorf("parseDashArray(%q) = %v, want nil", tt.in, got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("parseDashArray(%q) len = %d, want %d; got %v", tt.in, len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Errorf("parseDashArray(%q)[%d] = %v, want %v", tt.in, i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestDashOffsetParsing(t *testing.T) {
	tests := []struct {
		name       string
		dasharray  string
		dashoffset string
		wantOffset float64
	}{
		{"negative offset", "10 20", "-389.2", -389.2},
		{"positive offset", "10 20", "50", 50},
		{"zero offset", "10 20", "0", 0},
		{"no offset", "10 20", "", 0},
		{"invalid offset", "10 20", "abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parent := defaultStyle()
			attrs := StyleAttrs{
				StrokeDasharray:  tt.dasharray,
				StrokeDashoffset: tt.dashoffset,
			}
			s := mergeStyle(parent, attrs)
			if s.dashOffset != tt.wantOffset {
				t.Errorf("dashOffset = %v, want %v", s.dashOffset, tt.wantOffset)
			}
		})
	}
}

func TestDashInheritance(t *testing.T) {
	parent := defaultStyle()
	parent.dashArray = []float64{5, 10}
	parent.dashOffset = 3

	// Child with no dash attrs should inherit
	child := mergeStyle(parent, StyleAttrs{})
	if len(child.dashArray) != 2 || child.dashArray[0] != 5 || child.dashArray[1] != 10 {
		t.Errorf("inherited dashArray = %v, want [5 10]", child.dashArray)
	}
	if child.dashOffset != 3 {
		t.Errorf("inherited dashOffset = %v, want 3", child.dashOffset)
	}

	// Child with "none" should clear
	child2 := mergeStyle(parent, StyleAttrs{StrokeDasharray: "none"})
	if child2.dashArray != nil {
		t.Errorf("dashArray after 'none' = %v, want nil", child2.dashArray)
	}
}

func TestDashPDFEmission(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		<circle cx="50" cy="50" r="40" fill="none" stroke="black" stroke-width="2"
			stroke-dasharray="389.2 440" stroke-dashoffset="-389.2"/>
	</svg>`

	doc, err := Parse(strings.NewReader(svg))
	if err != nil {
		t.Fatal(err)
	}

	pdf := doc.RenderPDF(RenderOptions{Width: 100, Height: 100})

	if !strings.Contains(pdf, "d") {
		t.Error("PDF stream missing 'd' (setdash) operator")
	}
	// Check for the specific dash pattern
	if !strings.Contains(pdf, "[389.2 440] -389.2 d") {
		t.Errorf("PDF stream missing expected dash pattern; got:\n%s", pdf)
	}
}

func TestDashNonePDFEmission(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		<circle cx="50" cy="50" r="40" fill="none" stroke="black" stroke-dasharray="none"/>
	</svg>`

	doc, err := Parse(strings.NewReader(svg))
	if err != nil {
		t.Fatal(err)
	}

	pdf := doc.RenderPDF(RenderOptions{Width: 100, Height: 100})

	// "none" should produce the reset pattern, not a dash array
	lines := strings.Split(pdf, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasSuffix(line, " d") && line != "[] 0 d" {
			t.Errorf("unexpected dash operator for 'none': %q", line)
		}
	}
}

func TestDashEmptyPDFEmission(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		<circle cx="50" cy="50" r="40" fill="none" stroke="black"/>
	</svg>`

	doc, err := Parse(strings.NewReader(svg))
	if err != nil {
		t.Fatal(err)
	}

	pdf := doc.RenderPDF(RenderOptions{Width: 100, Height: 100})

	// No dasharray attr → should emit reset "[] 0 d"
	if !strings.Contains(pdf, "[] 0 d") {
		t.Errorf("expected '[] 0 d' reset for stroke without dasharray; got:\n%s", pdf)
	}
}

func TestDashViewBoxScaling(t *testing.T) {
	// Non-1:1 viewBox: 200x200 viewBox rendered into 100x100pt
	// Dash values should be in SVG user units (CTM scales them)
	svg := `<svg xmlns="http://www.w3.org/2000/svg" width="100" height="100" viewBox="0 0 200 200">
		<circle cx="100" cy="100" r="80" fill="none" stroke="black" stroke-width="4"
			stroke-dasharray="20 10"/>
	</svg>`

	doc, err := Parse(strings.NewReader(svg))
	if err != nil {
		t.Fatal(err)
	}

	pdf := doc.RenderPDF(RenderOptions{Width: 100, Height: 100})

	// Dash values should be 20 and 10 (SVG user units), NOT scaled
	if !strings.Contains(pdf, "[20 10] 0 d") {
		t.Errorf("expected dash values in SVG user units [20 10]; got:\n%s", pdf)
	}

	// Verify CTM is present (0.5 scale factor = 100/200)
	if !strings.Contains(pdf, "0.5 0 0") {
		t.Errorf("expected CTM with 0.5 scale; got:\n%s", pdf)
	}
}

func TestDashOddDoublingIntegration(t *testing.T) {
	svg := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 100 100">
		<line x1="0" y1="50" x2="100" y2="50" stroke="black"
			stroke-dasharray="5 10 15"/>
	</svg>`

	doc, err := Parse(strings.NewReader(svg))
	if err != nil {
		t.Fatal(err)
	}

	pdf := doc.RenderPDF(RenderOptions{Width: 100, Height: 100})

	// "5 10 15" → doubled to [5 10 15 5 10 15]
	if !strings.Contains(pdf, "[5 10 15 5 10 15] 0 d") {
		t.Errorf("expected doubled dash array; got:\n%s", pdf)
	}
}
