package htmlbag

import (
	"strconv"
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
	"github.com/boxesandglue/svgreader"
)

// parseColumnWidth parses a column width specification and returns a Glue node.
// Supports:
//   - fixed widths: "3cm", "50mm", "2in", "100pt"
//   - flexible widths: "*" (1 share), "2*" (2 shares), "3*" (3 shares)
func parseColumnWidth(width string) *node.Glue {
	g := node.NewGlue()
	width = strings.TrimSpace(width)

	if width == "" {
		// No width specified - auto
		g.Stretch = bag.Factor
		g.StretchOrder = 1
		return g
	}

	if strings.HasSuffix(width, "*") {
		// Flexible width: "*", "2*", "3*", etc.
		multiplier := 1.0
		prefix := strings.TrimSuffix(width, "*")
		if prefix != "" {
			if m, err := strconv.ParseFloat(prefix, 64); err == nil {
				multiplier = m
			}
		}
		g.Stretch = bag.ScaledPoint(multiplier * float64(bag.Factor))
		g.StretchOrder = 1
		return g
	}

	// Fixed width
	if sp, err := bag.SP(width); err == nil {
		g.Width = sp
	}
	return g
}

func (cb *CSSBuilder) buildTable(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	tbl := &frontend.Table{}
	tbl.MaxWidth = wd
	if sWd, ok := te.Settings[frontend.SettingWidth]; ok {
		if wdStr, ok := sWd.(string); ok && strings.HasSuffix(wdStr, "%") {
			tbl.MaxWidth = ParseRelativeSize(wdStr, wd, wd)
			tbl.Stretch = true
		}
	}

	// Process colgroup for column specifications
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "colgroup" {
				cb.buildColgroup(t, tbl)
			}
		}
	}

	// Derive ColSpec from td/th CSS widths when no colgroup is present.
	if len(tbl.ColSpec) == 0 {
		if specs := collectCellWidths(te, tbl.MaxWidth); specs != nil {
			tbl.ColSpec = specs
		} else if tbl.Stretch {
			// width:100% table with no explicit column widths — distribute equally
			tbl.ColSpec = defaultFlexColSpecs(te)
		}
	}

	// First pass: process thead (header rows come first)
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "thead" {
				rowsBefore := len(tbl.Rows)
				cb.buildTBody(t, tbl)
				tbl.HeaderRows = len(tbl.Rows) - rowsBefore
			}
		}
	}
	// Second pass: process tbody (body rows come after header)
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "tbody" {
				cb.buildTBody(t, tbl)
			}
		}
	}
	vls, err := cb.frontend.BuildTable(tbl)
	if err != nil {
		return nil, err
	}

	vl := vls[0]

	// PDF/UA: tag the table structure.
	// Repeated headers on continuation pages are left untagged
	// (the backend will wrap them as artifacts in PDF/UA mode).
	if cb.enableTagging {
		cb.tagTable(vl, tbl)
	}

	return vl, nil
}

// collectCellWidths scans the table's frontend.Text tree for td/th elements
// with CSS width declarations and converts them to ColSpec entries.
// Cells with colspan are skipped. When multiple rows declare different widths
// for the same column, the maximum is used.
func collectCellWidths(te *frontend.Text, maxWidth bag.ScaledPoint) []frontend.ColSpec {
	type colWidth struct {
		width bag.ScaledPoint
		set   bool
	}
	var widths []colWidth
	nCols := 0

	// processRow extracts widths from td/th elements in a tr.
	processRow := func(tr *frontend.Text) {
		col := 0
		for _, itm := range tr.Items {
			t, ok := itm.(*frontend.Text)
			if !ok {
				continue
			}
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok || (elt != "td" && elt != "th") {
				continue
			}
			colspan := 0
			if v, ok := t.Settings[frontend.SettingColspan]; ok && v != nil {
				if cs, ok := v.(int); ok && cs > 1 {
					colspan = cs - 1
				}
			}
			if colspan == 0 {
				if wdVal, ok := t.Settings[frontend.SettingWidth]; ok {
					if wdStr, ok := wdVal.(string); ok && wdStr != "" {
						sp := ParseRelativeSize(wdStr, maxWidth, maxWidth)
						if sp > 0 {
							for col >= len(widths) {
								widths = append(widths, colWidth{})
							}
							if !widths[col].set || sp > widths[col].width {
								widths[col] = colWidth{width: sp, set: true}
							}
						}
					}
				}
			}
			col += 1 + colspan
		}
		if col > nCols {
			nCols = col
		}
	}

	// processSection walks thead or tbody to find tr elements.
	processSection := func(section *frontend.Text) {
		for _, itm := range section.Items {
			t, ok := itm.(*frontend.Text)
			if !ok {
				continue
			}
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok || elt != "tr" {
				continue
			}
			processRow(t)
		}
	}

	for _, itm := range te.Items {
		t, ok := itm.(*frontend.Text)
		if !ok {
			continue
		}
		elt, ok := t.Settings[frontend.SettingDebug].(string)
		if !ok {
			continue
		}
		if elt == "thead" || elt == "tbody" {
			processSection(t)
		}
	}

	hasAny := false
	for _, w := range widths {
		if w.set {
			hasAny = true
			break
		}
	}
	if !hasAny {
		return nil
	}

	specs := make([]frontend.ColSpec, nCols)
	for i := 0; i < nCols; i++ {
		g := node.NewGlue()
		if i < len(widths) && widths[i].set {
			g.Width = widths[i].width
		} else {
			g.Stretch = bag.Factor
			g.StretchOrder = 1
		}
		specs[i] = frontend.ColSpec{ColumnWidth: g}
	}
	return specs
}

// countTableColumns returns the column count of the table by scanning the first
// row found in thead or tbody, summing cell counts accounting for colspan.
func countTableColumns(te *frontend.Text) int {
	for _, itm := range te.Items {
		t, ok := itm.(*frontend.Text)
		if !ok {
			continue
		}
		elt, ok := t.Settings[frontend.SettingDebug].(string)
		if !ok || (elt != "thead" && elt != "tbody") {
			continue
		}
		for _, rowItm := range t.Items {
			tr, ok := rowItm.(*frontend.Text)
			if !ok {
				continue
			}
			trElt, ok := tr.Settings[frontend.SettingDebug].(string)
			if !ok || trElt != "tr" {
				continue
			}
			n := 0
			for _, cellItm := range tr.Items {
				cell, ok := cellItm.(*frontend.Text)
				if !ok {
					continue
				}
				cellElt, ok := cell.Settings[frontend.SettingDebug].(string)
				if !ok || (cellElt != "td" && cellElt != "th") {
					continue
				}
				colspan := 1
				if v, ok := cell.Settings[frontend.SettingColspan].(int); ok && v > 1 {
					colspan = v
				}
				n += colspan
			}
			if n > 0 {
				return n
			}
		}
	}
	return 0
}

// defaultFlexColSpecs generates equal flex ColSpec entries for a table with
// no explicit column widths. Returns nil for empty tables (0 columns).
func defaultFlexColSpecs(te *frontend.Text) []frontend.ColSpec {
	n := countTableColumns(te)
	if n == 0 {
		return nil
	}
	specs := make([]frontend.ColSpec, n)
	for i := range specs {
		g := node.NewGlue()
		g.Stretch = bag.Factor
		g.StretchOrder = 1
		specs[i] = frontend.ColSpec{ColumnWidth: g}
	}
	return specs
}

func (cb *CSSBuilder) buildColgroup(te *frontend.Text, tbl *frontend.Table) {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "col" {
				width := ""
				if w, ok := t.Settings[frontend.SettingColumnWidth].(string); ok {
					width = w
				}
				colSpec := frontend.ColSpec{
					ColumnWidth: parseColumnWidth(width),
				}
				tbl.ColSpec = append(tbl.ColSpec, colSpec)
			}
		}
	}
}

func (cb *CSSBuilder) buildTBody(te *frontend.Text, tbl *frontend.Table) {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "tr" {
				cb.buildTR(t, tbl)
			}
		}
	}
}

func (cb *CSSBuilder) buildTR(te *frontend.Text, tbl *frontend.Table) {
	tr := &frontend.TableRow{}
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			elt, ok := t.Settings[frontend.SettingDebug].(string)
			if !ok {
				continue
			}
			if elt == "td" || elt == "th" {
				cb.buildTD(t, tr, elt == "th")
			}
		}
	}
	tbl.Rows = append(tbl.Rows, tr)
}

func (cb *CSSBuilder) buildTD(te *frontend.Text, row *frontend.TableRow, isHeader bool) {
	td := &frontend.TableCell{}
	td.IsHeader = isHeader

	// Extract colspan and rowspan
	settings := te.Settings
	if v, ok := settings[frontend.SettingColspan]; ok && v != nil {
		if colspan, ok := v.(int); ok && colspan > 1 {
			td.ExtraColspan = colspan - 1
		}
	}
	if v, ok := settings[frontend.SettingRowspan]; ok && v != nil {
		if rowspan, ok := v.(int); ok && rowspan > 1 {
			td.ExtraRowspan = rowspan - 1
		}
	}

	// Extract border/padding/background settings from CSS.
	td.BorderTopWidth = settingSP(settings[frontend.SettingBorderTopWidth])
	td.BorderBottomWidth = settingSP(settings[frontend.SettingBorderBottomWidth])
	td.BorderLeftWidth = settingSP(settings[frontend.SettingBorderLeftWidth])
	td.BorderRightWidth = settingSP(settings[frontend.SettingBorderRightWidth])
	td.BorderTopColor = settingColor(settings[frontend.SettingBorderTopColor])
	td.BorderBottomColor = settingColor(settings[frontend.SettingBorderBottomColor])
	td.BorderLeftColor = settingColor(settings[frontend.SettingBorderLeftColor])
	td.BorderRightColor = settingColor(settings[frontend.SettingBorderRightColor])
	td.PaddingTop = settingSP(settings[frontend.SettingPaddingTop])
	td.PaddingBottom = settingSP(settings[frontend.SettingPaddingBottom])
	td.PaddingLeft = settingSP(settings[frontend.SettingPaddingLeft])
	td.PaddingRight = settingSP(settings[frontend.SettingPaddingRight])
	td.BackgroundColor = settingColor(settings[frontend.SettingBackgroundColor])

	// If this cell references a pre-rendered VList, use it directly as content.
	if vlid, ok := settings[frontend.SettingPrerenderedVListID].(string); ok {
		if vl, vlOK := cb.PendingVLists[vlid]; vlOK {
			td.Contents = append(td.Contents, frontend.FormatToVList(func(wd bag.ScaledPoint) (*node.VList, error) {
				return vl, nil
			}))
		}
	}

	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *frontend.Text:
			// Wrap all Text items in FormatToVList closures so cell content
			// receives the actual cell width at layout time. This is critical
			// for inline SVGs whose dimensions may depend on container width.
			textCopy := t
			ftv := func(wd bag.ScaledPoint) (*node.VList, error) {
				// Re-render any percentage-width SVGs at the actual cell width.
				resolveSVGWidths(textCopy.Items, wd, cb.frontend)
				// If this Text item wraps a single inline-SVG VList, return it
				// directly. FormatParagraph (called by CreateVlist) is a text
				// formatter and cannot handle a VList item — it would serialize
				// the SVG nodes as text instead of rendering the graphic.
				if len(textCopy.Items) == 1 {
					if svgVL, ok := textCopy.Items[0].(*node.VList); ok {
						if origin, _ := svgVL.Attributes["origin"].(string); origin == "inline-svg" {
							return svgVL, nil
						}
					}
				}
				vl, err := cb.CreateVlist(textCopy, wd)
				if err != nil {
					return nil, err
				}
				// Margin-bottom may have been propagated from a child
				// through a borderless parent (CSS margin collapsing).
				// In a table cell, materialize it as a kern.
				if mb, ok := textCopy.Settings[frontend.SettingMarginBottom]; ok {
					if mbSP, ok := mb.(bag.ScaledPoint); ok && mbSP > 0 {
						k := node.NewKern()
						k.Kern = mbSP
						k.Attributes = node.H{"origin": "margin-bottom"}
						vl.List = node.InsertAfter(vl.List, node.Tail(vl.List), k)
						vl.Height += mbSP
					}
				}
				return vl, nil
			}
			td.Contents = append(td.Contents, frontend.FormatToVList(ftv))
		default:
			td.Contents = append(td.Contents, itm)
		}
	}
	row.Cells = append(row.Cells, td)
}

// tagTable walks the table VList and creates Table/TR/TH/TD structure elements.
func (cb *CSSBuilder) tagTable(tableVL *node.VList, tbl *frontend.Table) {
	tableSE := &document.StructureElement{Role: "Table"}
	cb.structureCurrent.AddChild(tableSE)

	// Create THead/TBody grouping SEs
	var theadSE, tbodySE *document.StructureElement
	if tbl.HeaderRows > 0 {
		theadSE = &document.StructureElement{Role: "THead"}
		tableSE.AddChild(theadSE)
	}
	tbodySE = &document.StructureElement{Role: "TBody"}
	tableSE.AddChild(tbodySE)

	// Walk rows: each child of the table VList is an HList (row)
	rowIdx := 0
	for cur := tableVL.List; cur != nil; cur = cur.Next() {
		rowHL, ok := cur.(*node.HList)
		if !ok {
			continue
		}
		if rowIdx >= len(tbl.Rows) {
			break
		}

		// Determine parent: THead for header rows, TBody otherwise
		rowParent := tbodySE
		if theadSE != nil && rowIdx < tbl.HeaderRows {
			rowParent = theadSE
		}

		trSE := &document.StructureElement{Role: "TR"}
		rowParent.AddChild(trSE)

		// Walk cells in this row
		row := tbl.Rows[rowIdx]
		cellIdx := 0
		for cellCur := rowHL.List; cellCur != nil; cellCur = cellCur.Next() {
			cellVL, ok := cellCur.(*node.VList)
			if !ok {
				continue
			}
			if cellIdx >= len(row.Cells) {
				break
			}

			cell := row.Cells[cellIdx]
			role := "TD"
			if cell.IsHeader {
				role = "TH"
			}
			cellSE := &document.StructureElement{Role: role}
			// Set Scope for TH cells
			if cell.IsHeader {
				if rowIdx < tbl.HeaderRows {
					cellSE.Scope = "Column"
				} else {
					cellSE.Scope = "Row"
				}
			}
			cellSE.ActualText = extractCellText(cell)
			trSE.AddChild(cellSE)
			tagVList(cellVL, cellSE)
			cellIdx++
		}
		rowIdx++
	}
}

// resolveSVGWidths walks a slice of frontend items and replaces any
// percentage-width SVG VLists with versions rendered at the correct container
// width. SVG VLists created during collectHorizontalNodes use DefaultPageWidth
// for percentage resolution; this function re-renders them using the actual
// cell/container width determined at layout time.
func resolveSVGWidths(items []any, containerWidth bag.ScaledPoint, df *frontend.Document) {
	for i, itm := range items {
		switch v := itm.(type) {
		case *node.VList:
			pct, ok := v.Attributes["svg-width-pct"].(float64)
			if !ok {
				continue
			}
			svgDoc, ok := v.Attributes["svg-doc"].(*svgreader.Document)
			if !ok {
				continue
			}
			ht, _ := v.Attributes["svg-height"].(bag.ScaledPoint)
			tr, _ := v.Attributes["svg-text-renderer"].(*frontend.SVGTextRenderer)
			if tr == nil {
				tr = frontend.NewSVGTextRenderer(df)
				tr.DefaultFamily = df.FindFontFamily("sans")
			}
			newWd := bag.ScaledPoint(float64(containerWidth) * pct / 100)
			svgNode := df.Doc.CreateSVGNodeFromDocument(svgDoc, newWd, ht, tr)
			newVL := node.Vpack(svgNode)
			newVL.Attributes = node.H{
				"origin":              "inline-svg",
				"svg-width-pct":       pct,
				"svg-doc":             svgDoc,
				"svg-height":          ht,
				"svg-text-renderer":   tr,
			}
			items[i] = newVL
		case *frontend.Text:
			resolveSVGWidths(v.Items, containerWidth, df)
		}
	}
}

// extractCellText extracts text content from a table cell's contents.
func extractCellText(cell *frontend.TableCell) string {
	var b strings.Builder
	for _, cc := range cell.Contents {
		switch t := cc.(type) {
		case *frontend.Text:
			b.WriteString(extractTextContent(t))
		}
	}
	return b.String()
}
