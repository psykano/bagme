package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/csshtml"
)

// TestDetailTableContinuation verifies that containsNestedTable detects a
// table VList with _buildHeaders nested inside a wrapper VList (e.g. <section>).
// This is the detection gate that enables grandchild table continuation in
// processNodeList — the fix for executive report rows being silently dropped.
func TestDetailTableContinuation(t *testing.T) {
	// Build a 50-row table with thead so it gets _buildHeaders annotated.
	rows := make([]any, 50)
	for i := range rows {
		rows[i] = makeText("tr", []any{
			makeText("td", nil),
			makeText("td", nil),
			makeText("td", nil),
		})
	}
	table := makeText("table", []any{
		makeText("thead", []any{
			makeText("tr", []any{
				makeText("th", nil),
				makeText("th", nil),
				makeText("th", nil),
			}),
		}),
		makeText("tbody", rows),
	})

	tableVL := buildTableForTest(t, table, bag.MustSP("200pt"))

	// Sanity: table must have _buildHeaders from a previous ticket's write site.
	if _, ok := tableVL.Attributes["_buildHeaders"]; !ok {
		t.Fatal("table VList missing _buildHeaders — write site not wired")
	}

	// containsNestedTable on the table directly must return true.
	if !containsNestedTable(tableVL) {
		t.Error("containsNestedTable: should return true when head node IS the table VList")
	}

	// Wrap the table in a section VList (mirrors <section> in the executive report).
	sectionVL := node.NewVList()
	sectionVL.List = tableVL

	// containsNestedTable on the section's List must detect the nested table.
	if !containsNestedTable(sectionVL.List) {
		t.Error("containsNestedTable: should return true for section VList containing a table with _buildHeaders")
	}

	// A bare VList without nested tables must return false.
	emptyVL := node.NewVList()
	if containsNestedTable(emptyVL) {
		t.Error("containsNestedTable: should return false for VList without nested tables")
	}

	// A VList that wraps a VList without _buildHeaders must return false.
	innerNoHeaders := node.NewVList()
	outerNoHeaders := node.NewVList()
	outerNoHeaders.List = innerNoHeaders
	if containsNestedTable(outerNoHeaders) {
		t.Error("containsNestedTable: should return false when no descendant has _buildHeaders")
	}
}

func makeText(debug string, items []any, extra ...any) *frontend.Text {
	t := &frontend.Text{
		Items:    items,
		Settings: frontend.TypesettingSettings{},
	}
	t.Settings[frontend.SettingDebug] = debug
	for i := 0; i+1 < len(extra); i += 2 {
		t.Settings[extra[i].(frontend.SettingType)] = extra[i+1]
	}
	return t
}

func TestCollectCellWidths_NoWidths(t *testing.T) {
	table := makeText("table", []any{
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil),
				makeText("td", nil),
			}),
		}),
	})
	specs := collectCellWidths(table, bag.MustSP("200pt"))
	if specs != nil {
		t.Fatalf("expected nil ColSpec for table without cell widths, got %d entries", len(specs))
	}
}

func TestCollectCellWidths_AbsoluteWidth(t *testing.T) {
	table := makeText("table", []any{
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil, frontend.SettingWidth, "60px"),
				makeText("td", nil),
			}),
		}),
	})
	maxWidth := bag.MustSP("200pt")
	specs := collectCellWidths(table, maxWidth)
	if len(specs) != 2 {
		t.Fatalf("expected 2 ColSpec entries, got %d", len(specs))
	}
	expected := bag.MustSP("60px")
	if specs[0].ColumnWidth.Width != expected {
		t.Errorf("column 0: expected width %v, got %v", expected, specs[0].ColumnWidth.Width)
	}
	if specs[0].ColumnWidth.Stretch != 0 {
		t.Errorf("column 0: expected Stretch=0 for fixed width, got %v", specs[0].ColumnWidth.Stretch)
	}
	if specs[1].ColumnWidth.Width != 0 {
		t.Errorf("column 1: expected Width=0 for auto column, got %v", specs[1].ColumnWidth.Width)
	}
	if specs[1].ColumnWidth.StretchOrder != 1 {
		t.Errorf("column 1: expected StretchOrder=1 for auto column, got %d", specs[1].ColumnWidth.StretchOrder)
	}
}

func TestCollectCellWidths_PercentageWidth(t *testing.T) {
	table := makeText("table", []any{
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil, frontend.SettingWidth, "50%"),
				makeText("td", nil, frontend.SettingWidth, "50%"),
			}),
		}),
	})
	maxWidth := bag.MustSP("200pt")
	specs := collectCellWidths(table, maxWidth)
	if len(specs) != 2 {
		t.Fatalf("expected 2 ColSpec entries, got %d", len(specs))
	}
	expected := bag.MustSP("100pt")
	if specs[0].ColumnWidth.Width != expected {
		t.Errorf("column 0: expected %v (50%% of 200pt), got %v", expected, specs[0].ColumnWidth.Width)
	}
	if specs[1].ColumnWidth.Width != expected {
		t.Errorf("column 1: expected %v (50%% of 200pt), got %v", expected, specs[1].ColumnWidth.Width)
	}
}

func TestCollectCellWidths_ColspanSkipped(t *testing.T) {
	table := makeText("table", []any{
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil, frontend.SettingWidth, "100px", frontend.SettingColspan, 2),
				makeText("td", nil),
			}),
		}),
	})
	maxWidth := bag.MustSP("200pt")
	specs := collectCellWidths(table, maxWidth)
	if specs != nil {
		t.Fatalf("expected nil ColSpec when only colspan cells have widths, got %d entries", len(specs))
	}
}

func TestCollectCellWidths_MaxAcrossRows(t *testing.T) {
	table := makeText("table", []any{
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil, frontend.SettingWidth, "40px"),
				makeText("td", nil),
			}),
			makeText("tr", []any{
				makeText("td", nil, frontend.SettingWidth, "60px"),
				makeText("td", nil),
			}),
		}),
	})
	maxWidth := bag.MustSP("200pt")
	specs := collectCellWidths(table, maxWidth)
	if len(specs) != 2 {
		t.Fatalf("expected 2 ColSpec entries, got %d", len(specs))
	}
	expected := bag.MustSP("60px")
	if specs[0].ColumnWidth.Width != expected {
		t.Errorf("column 0: expected max width %v, got %v", expected, specs[0].ColumnWidth.Width)
	}
}

func TestCollectCellWidths_DefaultFlex(t *testing.T) {
	table := makeText("table", []any{
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil),
				makeText("td", nil),
				makeText("td", nil),
			}),
		}),
	})
	specs := defaultFlexColSpecs(table)
	if len(specs) != 3 {
		t.Fatalf("expected 3 ColSpec entries for 3-column table, got %d", len(specs))
	}
	for i, spec := range specs {
		if spec.ColumnWidth.Stretch != bag.Factor {
			t.Errorf("column %d: expected Stretch=bag.Factor (%v), got %v", i, bag.Factor, spec.ColumnWidth.Stretch)
		}
		if spec.ColumnWidth.Width != 0 {
			t.Errorf("column %d: expected Width=0 for flex column, got %v", i, spec.ColumnWidth.Width)
		}
		if spec.ColumnWidth.StretchOrder != 1 {
			t.Errorf("column %d: expected StretchOrder=1, got %d", i, spec.ColumnWidth.StretchOrder)
		}
	}
}

func TestCollectCellWidths_TheadAndTbody(t *testing.T) {
	table := makeText("table", []any{
		makeText("thead", []any{
			makeText("tr", []any{
				makeText("th", nil, frontend.SettingWidth, "80px"),
				makeText("th", nil),
			}),
		}),
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil),
				makeText("td", nil, frontend.SettingWidth, "120px"),
			}),
		}),
	})
	maxWidth := bag.MustSP("400pt")
	specs := collectCellWidths(table, maxWidth)
	if len(specs) != 2 {
		t.Fatalf("expected 2 ColSpec entries, got %d", len(specs))
	}
	expectedCol0 := bag.MustSP("80px")
	if specs[0].ColumnWidth.Width != expectedCol0 {
		t.Errorf("column 0: expected %v from thead, got %v", expectedCol0, specs[0].ColumnWidth.Width)
	}
	expectedCol1 := bag.MustSP("120px")
	if specs[1].ColumnWidth.Width != expectedCol1 {
		t.Errorf("column 1: expected %v from tbody, got %v", expectedCol1, specs[1].ColumnWidth.Width)
	}
}

// buildTableForTest creates a CSSBuilder and calls buildTable with the given
// frontend.Text tree, returning the resulting VList.
func buildTableForTest(t *testing.T, te *frontend.Text, wd bag.ScaledPoint) *node.VList {
	t.Helper()
	df := newTestDocument(t)
	sans := df.FindFontFamily("sans")
	fontSize := bag.MustSP("10pt")

	// Ensure every td/th has renderable text content so BuildTable can
	// format cell contents.
	addFontToLeaves(te, sans, fontSize)

	cb := &CSSBuilder{
		frontend:      df,
		PendingVLists: map[string]*node.VList{},
	}
	vl, err := cb.buildTable(te, wd)
	if err != nil {
		t.Fatal("buildTable:", err)
	}
	return vl
}

// addFontToLeaves recursively walks a frontend.Text tree and ensures every
// leaf td/th element has a renderable text child with font settings.
func addFontToLeaves(te *frontend.Text, ff *frontend.FontFamily, fontSize bag.ScaledPoint) {
	elt, _ := te.Settings[frontend.SettingDebug].(string)
	if elt == "td" || elt == "th" {
		if len(te.Items) == 0 {
			child := makeText("", []any{"x"},
				frontend.SettingFontFamily, ff,
				frontend.SettingSize, fontSize,
			)
			te.Items = []any{child}
		} else {
			// Ensure existing children have font settings.
			for _, itm := range te.Items {
				if ct, ok := itm.(*frontend.Text); ok {
					if ct.Settings[frontend.SettingFontFamily] == nil {
						ct.Settings[frontend.SettingFontFamily] = ff
					}
					if ct.Settings[frontend.SettingSize] == nil {
						ct.Settings[frontend.SettingSize] = fontSize
					}
				}
			}
		}
		return
	}
	for _, itm := range te.Items {
		if ct, ok := itm.(*frontend.Text); ok {
			addFontToLeaves(ct, ff, fontSize)
		}
	}
}

// TestBuildTable_TheadAnnotation verifies that a table with <thead> gets
// _buildHeaders and _headerCount attributes on the resulting VList.
func TestBuildTable_TheadAnnotation(t *testing.T) {
	table := makeText("table", []any{
		makeText("thead", []any{
			makeText("tr", []any{
				makeText("th", nil),
				makeText("th", nil),
			}),
		}),
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil),
				makeText("td", nil),
			}),
			makeText("tr", []any{
				makeText("td", nil),
				makeText("td", nil),
			}),
		}),
	})

	vl := buildTableForTest(t, table, bag.MustSP("200pt"))

	if vl.Attributes == nil {
		t.Fatal("table VList has nil Attributes, expected _buildHeaders and _headerCount")
	}

	// Check _headerCount
	hc, ok := vl.Attributes["_headerCount"]
	if !ok {
		t.Fatal("_headerCount attribute missing")
	}
	headerCount, ok := hc.(int)
	if !ok {
		t.Fatalf("_headerCount is %T, expected int", hc)
	}
	if headerCount != 1 {
		t.Errorf("_headerCount = %d, want 1", headerCount)
	}

	// Check _buildHeaders
	bh, ok := vl.Attributes["_buildHeaders"]
	if !ok {
		t.Fatal("_buildHeaders attribute missing")
	}
	buildFn, ok := bh.(func() ([]*node.HList, error))
	if !ok {
		t.Fatalf("_buildHeaders is %T, expected func() ([]*node.HList, error)", bh)
	}

	// Verify the closure is callable and returns header rows.
	headers, err := buildFn()
	if err != nil {
		t.Fatal("_buildHeaders returned error:", err)
	}
	if len(headers) != 1 {
		t.Errorf("_buildHeaders returned %d rows, want 1", len(headers))
	}
}

// TestBuildTable_NoThead verifies that a table without <thead> does NOT get
// the _buildHeaders attribute.
func TestBuildTable_NoThead(t *testing.T) {
	table := makeText("table", []any{
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil),
				makeText("td", nil),
			}),
		}),
	})

	vl := buildTableForTest(t, table, bag.MustSP("200pt"))

	if vl.Attributes != nil {
		if _, ok := vl.Attributes["_buildHeaders"]; ok {
			t.Error("table without <thead> should not have _buildHeaders attribute")
		}
	}
}

// TestBuildTable_TheadEmptyTbody verifies that a table with <thead> and an
// empty <tbody> does not crash.
func TestBuildTable_TheadEmptyTbody(t *testing.T) {
	table := makeText("table", []any{
		makeText("thead", []any{
			makeText("tr", []any{
				makeText("th", nil),
			}),
		}),
		makeText("tbody", []any{}),
	})

	vl := buildTableForTest(t, table, bag.MustSP("200pt"))

	if vl.Attributes == nil {
		t.Fatal("table VList has nil Attributes")
	}
	hc, ok := vl.Attributes["_headerCount"].(int)
	if !ok {
		t.Fatal("_headerCount missing or wrong type")
	}
	if hc != 1 {
		t.Errorf("_headerCount = %d, want 1", hc)
	}
}

// TestBuildTable_TheadSingleRow verifies that a table with <thead> and a
// single <tbody> row (fits on one page) produces a valid VList with
// continuation attributes but doesn't require continuation.
func TestBuildTable_TheadSingleRow(t *testing.T) {
	table := makeText("table", []any{
		makeText("thead", []any{
			makeText("tr", []any{
				makeText("th", nil),
				makeText("th", nil),
			}),
		}),
		makeText("tbody", []any{
			makeText("tr", []any{
				makeText("td", nil),
				makeText("td", nil),
			}),
		}),
	})

	vl := buildTableForTest(t, table, bag.MustSP("200pt"))

	if vl.Attributes == nil {
		t.Fatal("table VList has nil Attributes")
	}
	if _, ok := vl.Attributes["_buildHeaders"]; !ok {
		t.Error("_buildHeaders attribute missing")
	}
	if _, ok := vl.Attributes["_headerCount"]; !ok {
		t.Error("_headerCount attribute missing")
	}

	// Count rows in the VList — should have 2 (1 header + 1 body).
	rowCount := 0
	for n := vl.List; n != nil; n = n.Next() {
		if _, ok := n.(*node.HList); ok {
			rowCount++
		}
	}
	if rowCount != 2 {
		t.Errorf("expected 2 rows (1 header + 1 body), got %d", rowCount)
	}
}

// TestOutputTableRows_ContinuationHeaderFit verifies that outputTableRows does
// not overflow past the page box when a body row is taller than
// (pageContentHeight - headerOverhead). Before the A2 fix the paginator would
// re-emit headers on the continuation page and then place the oversized body
// row below them, causing the row to extend past the page bottom. With the
// fix the paginator folds header overhead into the fit math and skips header
// re-emission for rows that would otherwise overflow, so every placement
// stays within the page box.
func TestOutputTableRows_ContinuationHeaderFit(t *testing.T) {
	df := newTestDocument(t)
	cs := csshtml.NewCSSParserWithDefaults()
	cb, err := New(df, cs)
	if err != nil {
		t.Fatal("New:", err)
	}

	// Small page: 200pt x 150pt, no margins — so pageContent = 150pt.
	if err := cb.ParseCSSString(`@page { size: 200pt 150pt; margin: 0 }`); err != nil {
		t.Fatal("ParseCSSString:", err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatal("InitPage:", err)
	}
	pd, err := cb.PageSize()
	if err != nil {
		t.Fatal("PageSize:", err)
	}

	// Sanity: page must actually be 150pt tall with zero margins.
	wantHeight := bag.MustSP("150pt")
	if pd.Height != wantHeight || pd.MarginTop != 0 || pd.MarginBottom != 0 {
		t.Fatalf("page dims unexpected: height=%v mt=%v mb=%v (want 150pt/0/0)",
			pd.Height, pd.MarginTop, pd.MarginBottom)
	}

	headerHeight := bag.MustSP("40pt")
	// Body row height: 120pt. Chosen so header+body = 160pt > page 150pt,
	// i.e. a body row cannot fit on a continuation page after headers are
	// re-emitted (150 - 40 = 110 < 120). But a bare body row still fits
	// on a full page (120 <= 150), so the fix has room to place it by
	// skipping header re-emission for that page.
	bodyHeight := bag.MustSP("120pt")
	numBodyRows := 3
	tableWidth := bag.MustSP("200pt")

	// makeRow constructs a fresh HList of the given height — synthetic rows
	// bypass buildTable so the heights are deterministic.
	makeRow := func(h bag.ScaledPoint) *node.HList {
		r := node.NewHList()
		r.Width = tableWidth
		r.Height = h
		return r
	}

	var rowNodes []*node.HList
	rowNodes = append(rowNodes, makeRow(headerHeight))
	for i := 0; i < numBodyRows; i++ {
		rowNodes = append(rowNodes, makeRow(bodyHeight))
	}
	for i := 0; i+1 < len(rowNodes); i++ {
		rowNodes[i].SetNext(rowNodes[i+1])
		rowNodes[i+1].SetPrev(rowNodes[i])
	}

	tableVL := node.NewVList()
	tableVL.List = rowNodes[0]
	tableVL.Width = tableWidth
	tableVL.Height = headerHeight + bag.ScaledPoint(numBodyRows)*bodyHeight
	buildHeaders := func() ([]*node.HList, error) {
		return []*node.HList{makeRow(headerHeight)}, nil
	}
	tableVL.Attributes = node.H{
		"_headerCount":  1,
		"_buildHeaders": buildHeaders,
	}

	// InitPage stamped a page-background/container VList on the current
	// page; snapshot the object count on each existing page so we only
	// inspect placements made by outputTableRows itself.
	existingObjCount := make([]int, len(df.Doc.Pages))
	for i, p := range df.Doc.Pages {
		existingObjCount[i] = len(p.Objects)
	}

	y := pd.Height - pd.MarginTop
	yLimit := pd.MarginBottom
	pageHasContent := false

	if err := cb.outputTableRows(tableVL, buildHeaders, &y, &yLimit, &pageHasContent, &pd); err != nil {
		t.Fatal("outputTableRows:", err)
	}

	// Sanity: a 1-header + 3 body-row table across a 150pt page must have
	// forced at least two pages of output.
	if len(df.Doc.Pages) < 2 {
		t.Fatalf("expected multi-page output, got %d page(s)", len(df.Doc.Pages))
	}

	for pi, p := range df.Doc.Pages {
		start := 0
		if pi < len(existingObjCount) {
			start = existingObjCount[pi]
		}
		for oi := start; oi < len(p.Objects); oi++ {
			obj := p.Objects[oi]
			vl := obj.Vlist
			if vl == nil {
				continue
			}
			h := vl.Height + vl.Depth
			bottom := obj.Y - h
			if bottom < pd.MarginBottom {
				t.Errorf("page %d object %d overflows page box: y=%v h=%v bottom=%v (yLimit=%v)",
					pi, oi, obj.Y, h, bottom, pd.MarginBottom)
			}
		}
	}
}

// TestBuildTable_PageBreakInsideOnRow verifies that a <tr> whose Text carries
// the htmlbag-private settingPageBreakInside sentinel has its value copied
// onto the corresponding row HList's Attributes["pageBreakInside"] after
// buildTable runs. This is the plumbing half of Task B: CSS
// page-break-inside: avoid must survive the Text → row HList materialization
// pipeline so that the paginator can act on it.
func TestBuildTable_PageBreakInsideOnRow(t *testing.T) {
	bodyTR := makeText("tr", []any{
		makeText("td", nil),
		makeText("td", nil),
	})
	bodyTR.Settings[settingPageBreakInside] = "avoid"

	table := makeText("table", []any{
		makeText("thead", []any{
			makeText("tr", []any{
				makeText("th", nil),
				makeText("th", nil),
			}),
		}),
		makeText("tbody", []any{bodyTR}),
	})

	vl := buildTableForTest(t, table, bag.MustSP("200pt"))

	var rows []*node.HList
	for n := vl.List; n != nil; n = n.Next() {
		if hl, ok := n.(*node.HList); ok {
			rows = append(rows, hl)
		}
	}
	if len(rows) < 2 {
		t.Fatalf("expected at least 2 row HLists (1 header + 1 body), got %d", len(rows))
	}

	headerRow := rows[0]
	if headerRow.Attributes != nil {
		if _, ok := headerRow.Attributes["pageBreakInside"]; ok {
			t.Error("header row should not have pageBreakInside attribute (sentinel was only on body tr)")
		}
	}

	bodyRow := rows[1]
	if bodyRow.Attributes == nil {
		t.Fatal("body row HList has nil Attributes")
	}
	v, ok := bodyRow.Attributes["pageBreakInside"]
	if !ok {
		t.Fatal("body row HList missing pageBreakInside attribute — sentinel not propagated from tr.Settings")
	}
	if v != "avoid" {
		t.Errorf("body row pageBreakInside = %v, want %q", v, "avoid")
	}
}

// TestOutputTableRows_AvoidBreakInsideForcesNewPage verifies that a row with
// pageBreakInside="avoid" is pushed onto a fresh page instead of being placed
// overflowing, even when the paginator's pageHasContent guard would normally
// suppress the break. This is the behavioral half of Task B: the avoid
// predicate must tighten the fit check so that a row with avoid is not placed
// partially off-page.
//
// Setup: 200pt × 200pt page, zero margins. outputTableRows is invoked with
// y primed to 100pt and pageHasContent=false, modeling an edge case where
// some upstream content has already consumed page space without being tracked
// by the paginator. A 120pt row with avoidBreakInside cannot fit in the
// remaining 100pt slot. Without the fix the existing guard
// (&& pageHasContent) suppresses NewPage, and the row is placed at y=100,
// ending at y=-20 (overflow). With the fix the paginator recognises the
// avoid attribute, forces NewPage, and the row is placed at y=200 where it
// ends at y=80 — well within the page box.
func TestOutputTableRows_AvoidBreakInsideForcesNewPage(t *testing.T) {
	df := newTestDocument(t)
	cs := csshtml.NewCSSParserWithDefaults()
	cb, err := New(df, cs)
	if err != nil {
		t.Fatal("New:", err)
	}

	if err := cb.ParseCSSString(`@page { size: 200pt 200pt; margin: 0 }`); err != nil {
		t.Fatal("ParseCSSString:", err)
	}
	if err := cb.InitPage(); err != nil {
		t.Fatal("InitPage:", err)
	}
	pd, err := cb.PageSize()
	if err != nil {
		t.Fatal("PageSize:", err)
	}

	wantPageHeight := bag.MustSP("200pt")
	if pd.Height != wantPageHeight || pd.MarginTop != 0 || pd.MarginBottom != 0 {
		t.Fatalf("page dims unexpected: height=%v mt=%v mb=%v (want 200pt/0/0)",
			pd.Height, pd.MarginTop, pd.MarginBottom)
	}

	tableWidth := bag.MustSP("200pt")
	rowHeight := bag.MustSP("120pt")

	row := node.NewHList()
	row.Width = tableWidth
	row.Height = rowHeight
	row.Attributes = node.H{"pageBreakInside": "avoid"}

	tableVL := node.NewVList()
	tableVL.List = row
	tableVL.Width = tableWidth
	tableVL.Height = rowHeight
	buildHeaders := func() ([]*node.HList, error) { return nil, nil }
	tableVL.Attributes = node.H{
		"_headerCount":  0,
		"_buildHeaders": buildHeaders,
	}

	// Snapshot existing object counts so we only inspect placements made by
	// the outputTableRows call under test.
	existingObjCount := make([]int, len(df.Doc.Pages))
	for i, p := range df.Doc.Pages {
		existingObjCount[i] = len(p.Objects)
	}

	y := bag.MustSP("100pt")
	yLimit := pd.MarginBottom
	pageHasContent := false

	if err := cb.outputTableRows(tableVL, buildHeaders, &y, &yLimit, &pageHasContent, &pd); err != nil {
		t.Fatal("outputTableRows:", err)
	}

	for pi, p := range df.Doc.Pages {
		start := 0
		if pi < len(existingObjCount) {
			start = existingObjCount[pi]
		}
		for oi := start; oi < len(p.Objects); oi++ {
			obj := p.Objects[oi]
			vl := obj.Vlist
			if vl == nil {
				continue
			}
			h := vl.Height + vl.Depth
			bottom := obj.Y - h
			if bottom < pd.MarginBottom {
				t.Errorf("page %d object %d overflows page box: y=%v h=%v bottom=%v (yLimit=%v)",
					pi, oi, obj.Y, h, bottom, pd.MarginBottom)
			}
		}
	}
}

// TestAvoidBreakInside_Predicate verifies the avoidBreakInside predicate
// returns true for VList and HList nodes carrying the Attributes marker,
// and false otherwise. Narrowly scoped so the predicate contract is pinned
// by a test even if the fit-check call sites evolve.
func TestAvoidBreakInside_Predicate(t *testing.T) {
	vlAvoid := node.NewVList()
	vlAvoid.Attributes = node.H{"pageBreakInside": "avoid"}
	if !avoidBreakInside(vlAvoid) {
		t.Error("avoidBreakInside(VList avoid) = false, want true")
	}

	hlAvoid := node.NewHList()
	hlAvoid.Attributes = node.H{"pageBreakInside": "avoid"}
	if !avoidBreakInside(hlAvoid) {
		t.Error("avoidBreakInside(HList avoid) = false, want true")
	}

	vlAuto := node.NewVList()
	vlAuto.Attributes = node.H{"pageBreakInside": "auto"}
	if avoidBreakInside(vlAuto) {
		t.Error("avoidBreakInside(VList auto) = true, want false")
	}

	vlBare := node.NewVList()
	if avoidBreakInside(vlBare) {
		t.Error("avoidBreakInside(bare VList) = true, want false")
	}
}
