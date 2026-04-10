package htmlbag

// TestMirroringSpike renders a bordered table cell containing "ABC" and
// inspects the PDF content stream for the Tm (text matrix) operator.
//
// PDF text matrix format: a b c d e f Tm
//   a = horizontal scaling; negative a means the text is horizontally mirrored.
//
// Hypothesis under test (html.go:333):
//   HTMLBorder constructs border geometry in negative X space:
//     x0 := 0 - width - padding - border  (x0 < 0)
//     x3 := 0
//   If this causes a negative CTM scale, the Tm operator will show a < 0.
//
// RESULT (2026-04-10): HYPOTHESIS NOT CONFIRMED.
//   Tm: a=1 b=0 c=0 d=1 e=28.35 f=805.53
//   The 'a' component is +1 (identity scale). Text is not mirrored at the Tm level.
//   No 'cm' operators found in the content stream either.
//   Text glyph order in the TJ array: A→B→C (correct, not reversed).
//
// Secondary finding: table cell borders (border:1px solid red) do not appear
//   in the page content stream of the minimal spike PDF, despite CSS settings
//   being correctly parsed and propagated to the table cell (0.75pt border, red).
//   This suggests HTMLBorder is not the rendering path for table borders in the
//   minimal test case — table borders go through frontend.BuildTable instead.
//   The rule nodes created by frontend.BuildTable are present but their Pre
//   content is either not reaching the content stream or is being output in a
//   separate stream (none found via decodePDFStreams).
//
// Next steps for mirroring investigation:
//   1. Reproduce the actual page 3 mirrored text from blue-stack-reporting.
//   2. Inspect that PDF's content stream for the Tm/cm operators.
//   3. Check font glyph encoding (reverse encoding in CFF/Type2 font, or
//      reversed TJ glyph index array).
//
// Exit states:
//   a < 0  → HYPOTHESIS CONFIRMED. Fix in ticket 691d1a41.
//   a >= 0 → HYPOTHESIS REFUTED. Mirroring has a different root cause.

import (
	"bytes"
	"compress/zlib"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// decodePDFStreams extracts and decompresses all FlateDecode content streams
// from raw PDF bytes. Non-compressed streams are returned as-is.
// This is intentionally simple — good enough for a spike.
func decodePDFStreams(data []byte) []string {
	var decoded []string

	// The stream boundary marker written by baseline-pdf is "\nstream\n".
	streamMarker := []byte("\nstream\n")
	endMarker := []byte("\nendstream")

	rest := data
	for {
		si := bytes.Index(rest, streamMarker)
		if si < 0 {
			break
		}
		bodyStart := si + len(streamMarker)

		// Look in the 4 KB before the stream marker for the dict.
		lookbackStart := si - 4096
		if lookbackStart < 0 {
			lookbackStart = 0
		}
		header := rest[lookbackStart:si]

		// Check for FlateDecode filter.
		isFlate := bytes.Contains(header, []byte("FlateDecode"))

		// Find the last /Length (not /Length1) entry before the stream.
		// /Length1 is the uncompressed size written alongside /Length for some streams.
		type lenMatch struct {
			val int
			is1 bool
		}
		var lenMatches []lenMatch
		allLenRE := regexp.MustCompile(`/Length(1?)\s+(\d+)`)
		for _, m := range allLenRE.FindAllSubmatchIndex(header, -1) {
			is1 := string(header[m[2]:m[3]]) == "1"
			val, _ := strconv.Atoi(string(header[m[4]:m[5]]))
			lenMatches = append(lenMatches, lenMatch{val: val, is1: is1})
		}

		streamLen := -1
		for i := len(lenMatches) - 1; i >= 0; i-- {
			if !lenMatches[i].is1 {
				streamLen = lenMatches[i].val
				break
			}
		}

		var streamBody []byte
		if streamLen > 0 && bodyStart+streamLen <= len(rest) {
			streamBody = rest[bodyStart : bodyStart+streamLen]
			rest = rest[bodyStart+streamLen:]
		} else {
			// Fallback: find \nendstream after bodyStart.
			ei := bytes.Index(rest[bodyStart:], endMarker)
			if ei < 0 {
				break
			}
			streamBody = rest[bodyStart : bodyStart+ei]
			rest = rest[bodyStart+ei+len(endMarker):]
		}

		if isFlate && len(streamBody) > 0 {
			r, err := zlib.NewReader(bytes.NewReader(streamBody))
			if err == nil {
				plain, err := io.ReadAll(r)
				r.Close()
				if err == nil {
					decoded = append(decoded, string(plain))
					continue
				}
			}
		}
		// Not compressed or decompression failed — include raw if printable-ish.
		decoded = append(decoded, string(streamBody))
	}
	return decoded
}

func TestMirroringSpike(t *testing.T) {
	var buf bytes.Buffer
	fe, err := frontend.NewForWriter(&buf)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}
	if err := LoadIncludedFonts(fe); err != nil {
		t.Fatal("LoadIncludedFonts:", err)
	}

	cs := csshtml.NewCSSParserWithDefaults()
	cb, err := New(fe, cs)
	if err != nil {
		t.Fatal("New:", err)
	}

	html := `<table><tr><td style="border:1px solid red;padding:4px">ABC</td></tr></table>`

	if err := cb.InitPage(); err != nil {
		t.Fatal("InitPage:", err)
	}
	te, err := cb.HTMLToText(html)
	if err != nil {
		t.Fatal("HTMLToText:", err)
	}
	if err := cb.OutputPagesFromText(te); err != nil {
		t.Fatal("OutputPagesFromText:", err)
	}
	if err := fe.Doc.Finish(); err != nil {
		t.Fatal("fe.Doc.Finish:", err)
	}

	pdfData := buf.Bytes()
	if len(pdfData) == 0 {
		t.Fatal("PDF output is empty")
	}
	t.Logf("PDF size: %d bytes", len(pdfData))

	streams := decodePDFStreams(pdfData)
	t.Logf("Extracted %d content stream(s)", len(streams))

	// PDF text matrix: a b c d e f Tm
	// Numbers can be integers or floats, possibly negative.
	tmRE := regexp.MustCompile(`(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+Tm`)

	type tmEntry struct {
		a, b, c, d, e, f float64
		raw               string
	}

	var allTm []tmEntry
	for i, s := range streams {
		matches := tmRE.FindAllStringSubmatch(s, -1)
		for _, m := range matches {
			a, _ := strconv.ParseFloat(m[1], 64)
			b, _ := strconv.ParseFloat(m[2], 64)
			c, _ := strconv.ParseFloat(m[3], 64)
			d, _ := strconv.ParseFloat(m[4], 64)
			e, _ := strconv.ParseFloat(m[5], 64)
			f, _ := strconv.ParseFloat(m[6], 64)
			allTm = append(allTm, tmEntry{a, b, c, d, e, f, m[0]})
			t.Logf("stream[%d] Tm: a=%v b=%v c=%v d=%v e=%v f=%v", i, a, b, c, d, e, f)
		}
		// Also dump any cm operators (current transformation matrix changes).
		cmRE := regexp.MustCompile(`(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+(-?[\d.]+)\s+cm`)
		cmMatches := cmRE.FindAllStringSubmatch(s, -1)
		for _, m := range cmMatches {
			t.Logf("stream[%d] cm: a=%s b=%s c=%s d=%s e=%s f=%s", i, m[1], m[2], m[3], m[4], m[5], m[6])
		}
	}

	if len(allTm) == 0 {
		// Dump first 2KB of each stream for debugging.
		for i, s := range streams {
			preview := s
			if len(preview) > 2000 {
				preview = preview[:2000]
			}
			t.Logf("stream[%d] preview:\n%s", i, preview)
		}
		t.Fatal("No Tm operator found in PDF content streams — cannot confirm or refute hypothesis")
	}

	// Analyse: does any Tm have a < 0?
	var negCount, posCount int
	for _, tm := range allTm {
		if tm.a < 0 {
			negCount++
		} else {
			posCount++
		}
	}

	t.Logf("Tm operators found: %d total (%d with a<0, %d with a>=0)", len(allTm), negCount, posCount)

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n=== SPIKE RESULT ===\n")
	if negCount > 0 {
		fmt.Fprintf(&sb, "HYPOTHESIS CONFIRMED: %d/%d Tm operators have a < 0 (negative horizontal scale)\n", negCount, len(allTm))
		fmt.Fprintf(&sb, "Root cause: HTMLBorder (html.go:333) sets x0 = 0 - width - padding - border\n")
		fmt.Fprintf(&sb, "The negative-X coordinate space causes a negative horizontal scale in the text matrix.\n")
		fmt.Fprintf(&sb, "Fix: ticket 691d1a41 — flip the coordinate construction so x0=0, x3=width+padding+border.\n")
	} else {
		fmt.Fprintf(&sb, "HYPOTHESIS NOT CONFIRMED: all %d Tm operators have a >= 0\n", len(allTm))
		fmt.Fprintf(&sb, "The negative-X border geometry does NOT directly produce a negative Tm 'a' component.\n")
		fmt.Fprintf(&sb, "Investigate: check cm operators above, or inspect how text glyphs are encoded.\n")
		fmt.Fprintf(&sb, "The mirroring reported on page 3 may stem from font encoding, glyph order,\n")
		fmt.Fprintf(&sb, "or a cm transform in the page content stream rather than the Tm operator.\n")
	}
	t.Log(sb.String())

	// The test always passes — it's a spike for documentation, not a pass/fail gate.
	// The findings are logged above and in the output of:
	//   go test -v -run TestMirroringSpike ./htmlbag/
}
