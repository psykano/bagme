package htmlbag

import (
	"strings"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/color"
	"github.com/boxesandglue/boxesandglue/backend/document"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

// recalcVListHeight recomputes the enclosing VList's total vertical extent
// by walking its inner list and summing children via vlistNodeHeight,
// recursing into nested VLists so post-construction drift is visible to the
// ancestor. The freshly computed total is written back to vl.Height (with
// vl.Depth zeroed) so a subsequent vlistNodeHeight(vl) call returns it.
//
// Scope is intentionally narrow: call this only on the pagination hot path
// where dynamic content has materialized after the enclosing VList was
// first built (re-rendered percentage-width SVGs, nested-table closures
// invoked by frontend.BuildTable, re-emitted header overhead on table
// continuation pages). It is NOT a general replacement for the running
// height accumulation in buildVlistInternal — that path is already accurate
// because no drift has occurred yet.
func recalcVListHeight(vl *node.VList) bag.ScaledPoint {
	var total bag.ScaledPoint
	for cur := vl.List; cur != nil; cur = cur.Next() {
		if child, ok := cur.(*node.VList); ok {
			total += recalcVListHeight(child)
			continue
		}
		total += vlistNodeHeight(cur)
	}
	vl.Height = total
	vl.Depth = 0
	return total
}

// CreateVlist builds a vlist (a vertical list) from the Text object.
func (cb *CSSBuilder) CreateVlist(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	vl, err := cb.buildVlistInternal(te, wd)
	if err != nil {
		return nil, err
	}
	return vl, nil
}

// isWhitespaceOnly returns true if the Text element contains only whitespace strings.
func isWhitespaceOnly(te *frontend.Text) bool {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return false
			}
		case *frontend.Text:
			if !isWhitespaceOnly(t) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func (cb *CSSBuilder) buildVlistInternal(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	settings := te.Settings

	// If a CSS width is specified, use it instead of the inherited width.
	if sWd, ok := settings[frontend.SettingWidth]; ok {
		if wdStr, ok := sWd.(string); ok {
			wd = ParseRelativeSize(wdStr, wd, wd)
		}
	}

	// Get padding-left from this element to pass to children (for ul/ol lists)
	paddingLeft := settingSP(settings[frontend.SettingPaddingLeft])

	if boxVal, _ := settings[frontend.SettingBox].(bool); boxVal {
		// PDF/UA: push a container structure element for this block
		var containerSE *document.StructureElement
		var savedStructureCurrent *document.StructureElement
		if cb.enableTagging {
			if tag, ok := settings[frontend.SettingDebug].(string); ok {
				if role := pdfRoleForTag(tag); role != "" {
					containerSE = &document.StructureElement{Role: role}
					cb.structureCurrent.AddChild(containerSE)
					savedStructureCurrent = cb.structureCurrent
					cb.structureCurrent = containerSE
					// LI must contain LBody (PDF/UA 7.2)
					if role == "LI" {
						lbody := &document.StructureElement{Role: "LBody"}
						containerSE.AddChild(lbody)
						cb.structureCurrent = lbody
					}
				}
			}
		}
		// If this box container has a prepend (e.g., list bullet), pass it
		// to the first child Text element so FormatParagraph can render it.
		if prep, ok := settings[frontend.SettingPrepend]; ok {
			for _, itm := range te.Items {
				if t, ok := itm.(*frontend.Text); ok {
					t.Settings[frontend.SettingPrepend] = prep
					break
				}
			}
		}

		if dflex, _ := settings[SettingDisplayFlex].(bool); dflex {
			vl, err := cb.buildFlexRow(te, wd)
			if err != nil {
				return nil, err
			}
			if containerSE != nil {
				cb.structureCurrent = savedStructureCurrent
			}
			return vl, nil
		}

		// Extract border/padding values for this container
		hv := settingsToHTMLValues(settings)
		hasBorderOrBg := hv.hasBorder() || hv.BackgroundColor != nil

		// Calculate effective width for children
		childBaseWidth := wd
		if hasBorderOrBg {
			// HTMLBorder will handle all padding and borders visually
			childBaseWidth = wd - hv.BorderLeftWidth - hv.BorderRightWidth - hv.PaddingLeft - hv.PaddingRight
		}

		vls := node.NewVList()
		vls.Attributes = node.H{"origin": "buildVListInternal"}

		// Track previous element's margin-bottom for margin collapsing
		var prevMarginBottom bag.ScaledPoint

		for i, itm := range te.Items {
			switch t := itm.(type) {
			case *frontend.Text:
				// Skip whitespace-only text elements (e.g. whitespace
				// between </ul> and </li> in the HTML tree).
				if _, hasTag := t.Settings[frontend.SettingDebug]; !hasTag && isWhitespaceOnly(t) {
					continue
				}

				// Get margin-top of current element
				curMarginTop := settingSP(t.Settings[frontend.SettingMarginTop])

				// Calculate collapsed margin (CSS margin collapsing)
				var marginGlue bag.ScaledPoint
				if i == 0 {
					// First element: use margin-top only
					marginGlue = curMarginTop
				} else {
					// Collapsed margin: max of previous bottom and current top
					marginGlue = bag.Max(prevMarginBottom, curMarginTop)
				}

				// Insert margin kern if needed
				if marginGlue > 0 {
					k := node.NewKern()
					k.Kern = marginGlue
					k.Attributes = node.H{"origin": "margin"}
					vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), k)
					vls.Height += marginGlue
				}

				var vl *node.VList
				if dbg, ok := t.Settings[frontend.SettingDebug].(string); ok && dbg == "table" {
					var err error
					vl, err = cb.buildTable(t, childBaseWidth)
					if err != nil {
						return nil, err
					}
				} else {
					// Reduce width for children by padding-left (for lists).
					// When the container has borders/background, HTMLBorder
					// handles all padding, so skip the per-child shift.
					childWidth := childBaseWidth
					if !hasBorderOrBg && paddingLeft > 0 {
						childWidth = childBaseWidth - paddingLeft
					}
					var err error
					vl, err = cb.buildVlistInternal(t, childWidth)
					if err != nil {
						return nil, err
					}

					// Shift content right by padding-left (only for lists
					// without borders — HTMLBorder handles all other cases)
					if !hasBorderOrBg && paddingLeft > 0 {
						for cur := vl.List; cur != nil; cur = cur.Next() {
							switch n := cur.(type) {
							case *node.HList:
								k := node.NewKern()
								k.Kern = paddingLeft
								k.Attributes = node.H{"origin": "padding-left"}
								n.List = node.InsertBefore(n.List, n.List, k)
								n.Width += paddingLeft
							case *node.VList:
								// Box container child (e.g. li with nested ul):
								// shift the entire VList right.
								n.ShiftX += paddingLeft
							}
						}
					}
				}
				// Propagate page-break-after to node attributes
				if pba, ok := t.Settings[frontend.SettingPageBreakAfter]; ok {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["pageBreakAfter"] = pba
				}
				if pbb, ok := t.Settings[frontend.SettingPageBreakBefore]; ok {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["pageBreakBefore"] = pbb
				}
				// page-break-inside / break-inside rides on an htmlbag-
				// private SettingType sentinel; copy it to the VList's
				// Attributes and delete from Settings so the sentinel
				// cannot leak into frontend.FormatParagraph.
				if pbi, ok := t.Settings[settingPageBreakInside]; ok {
					if vl.Attributes == nil {
						vl.Attributes = node.H{}
					}
					vl.Attributes["pageBreakInside"] = pbi
					delete(t.Settings, settingPageBreakInside)
				}

				vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), vl)
				if vl.Width > vls.Width {
					vls.Width = vl.Width
				}
				vls.Height += vl.Height
				vls.Depth = vl.Depth

				if cb.ElementCallback != nil {
					if tag, ok := t.Settings[frontend.SettingDebug].(string); ok {
						cb.ElementCallback(ElementEvent{
							TagName:     tag,
							TextContent: extractTextContent(t),
							VList:       vl,
						})
					}
				}

				// Annotate heading VLists so OutputPages can assign page numbers.
				if tag, ok := t.Settings[frontend.SettingDebug].(string); ok {
					switch tag {
					case "h1", "h2", "h3", "h4", "h5", "h6":
						if vl.Attributes == nil {
							vl.Attributes = node.H{}
						}
						vl.Attributes["_heading_idx"] = cb.headingCount
						cb.Headings = append(cb.Headings, HeadingEntry{Level: tag, Text: extractTextContent(t)})
						cb.headingCount++
					}
				}

				// Store margin-bottom for next iteration
				prevMarginBottom = settingSP(t.Settings[frontend.SettingMarginBottom])
			}
		}

		// Handle final margin-bottom after last element.
		if prevMarginBottom > 0 {
			if hasBorderOrBg {
				// Border/padding blocks margin collapsing: add kern.
				k := node.NewKern()
				k.Kern = prevMarginBottom
				k.Attributes = node.H{"origin": "margin-bottom"}
				vls.List = node.InsertAfter(vls.List, node.Tail(vls.List), k)
				vls.Height += prevMarginBottom
			} else {
				// No border/padding: the last child's margin-bottom
				// collapses through the parent boundary (CSS margin
				// collapsing). Propagate the maximum to the parent.
				parentMB := settingSP(te.Settings[frontend.SettingMarginBottom])
				if prevMarginBottom > parentMB {
					te.Settings[frontend.SettingMarginBottom] = prevMarginBottom
				}
			}
		}

		// Apply borders/background to this block container
		if hasBorderOrBg {
			vls.Width = childBaseWidth
			vls = cb.HTMLBorder(vls, hv)
		}

		// PDF/UA: pop structure element back to parent
		if containerSE != nil {
			cb.structureCurrent = savedStructureCurrent
		}

		return vls, nil
	}

	// Extract border/padding values first to calculate content width
	hv := settingsToHTMLValues(settings)

	// Reduce width by border and padding (CSS box-sizing: border-box behavior)
	contentWidth := wd - hv.BorderLeftWidth - hv.BorderRightWidth - hv.PaddingLeft - hv.PaddingRight

	stripFlexSettings(te.Settings)

	// Capture-and-strip the htmlbag-private settingPageBreakInside
	// sentinel before handing off to FormatParagraph. The sentinel
	// rides on block-level Text.Settings (e.g. a <div> with
	// page-break-inside: avoid) that reach the leaf branch whenever
	// the block only contains inline content — in that case
	// HTMLNodeToText leaves SettingBox off and the enclosing box
	// branch never has a chance to read the sentinel from a child.
	// FormatParagraph → Mknodes → BuildNodelistFromString has a strict
	// "unknown setting" default that would error on the negative
	// sentinel, so we must strip it. We then write the captured value
	// onto the returned VList's Attributes — that is the same place
	// the box-branch read-and-strip above targets, so upstream
	// paginator code (avoidBreakInside / forceBreakBefore /
	// forceBreakAfter) sees a single consistent shape regardless of
	// which branch built the VList.
	pbi, hasPBI := te.Settings[settingPageBreakInside]
	if hasPBI {
		delete(te.Settings, settingPageBreakInside)
	}

	// FormatParagraph -> Mknodes handles SettingPrepend (e.g., bullet points)
	vl, _, err := cb.frontend.FormatParagraph(te, contentWidth)
	if err != nil {
		return nil, err
	}

	// Apply borders if any are defined
	if hv.hasBorder() || hv.BackgroundColor != nil {
		vl = cb.HTMLBorder(vl, hv)
	}

	// Attach the captured page-break-inside value onto the returned
	// VList so the paginator predicate can see it. Done after
	// HTMLBorder so the attribute sits on the outermost wrapper the
	// paginator will actually look at.
	if hasPBI {
		if vl.Attributes == nil {
			vl.Attributes = node.H{}
		}
		vl.Attributes["pageBreakInside"] = pbi
	}

	// PDF/UA: tag leaf block elements (p, h1-h6, pre, code)
	if cb.enableTagging {
		if tag, ok := settings[frontend.SettingDebug].(string); ok {
			role := pdfRoleForTag(tag)

			// If this paragraph contains an image, use Figure role with alt text
			if role == "P" {
				if alt := findImageAlt(te); alt != "" {
					role = "Figure"
				}
			}

			if role != "" {
				se := &document.StructureElement{Role: role}
				if role == "Figure" {
					se.Alt = findImageAlt(te)
				} else {
					se.ActualText = extractTextContent(te)
				}
				// LI must contain exactly one LBody (PDF/UA 7.2)
				if role == "LI" {
					cb.structureCurrent.AddChild(se)
					lbody := &document.StructureElement{Role: "LBody"}
					lbody.ActualText = se.ActualText
					se.ActualText = ""
					se.AddChild(lbody)
					tagVList(vl, lbody)
				} else {
					cb.structureCurrent.AddChild(se)
					tagVList(vl, se)
				}
			}
		}
	}

	return vl, nil
}

// extractTextContent recursively collects string content from a Text tree.
func extractTextContent(te *frontend.Text) string {
	var b strings.Builder
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case string:
			b.WriteString(t)
		case *frontend.Text:
			b.WriteString(extractTextContent(t))
		}
	}
	return b.String()
}

// findImageAlt checks if a Text element contains an image (VList with "alt"
// attribute) and returns its alt text. Returns empty string if no image found.
func findImageAlt(te *frontend.Text) string {
	for _, itm := range te.Items {
		switch t := itm.(type) {
		case *node.VList:
			if t.Attributes != nil {
				if alt, ok := t.Attributes["alt"].(string); ok {
					return alt
				}
			}
		case *frontend.Text:
			if alt := findImageAlt(t); alt != "" {
				return alt
			}
		}
	}
	return ""
}

// settingSP safely extracts a bag.ScaledPoint from a settings value.
// Returns zero if the value is nil or not a ScaledPoint.
func settingSP(v any) bag.ScaledPoint {
	if sp, ok := v.(bag.ScaledPoint); ok {
		return sp
	}
	return 0
}

// settingColor safely extracts a *color.Color from a settings value.
// Returns nil if the value is nil or not a *color.Color.
func settingColor(v any) *color.Color {
	if c, ok := v.(*color.Color); ok {
		return c
	}
	return nil
}

// settingBorderStyle safely extracts a frontend.BorderStyle from a settings value.
func settingBorderStyle(v any) frontend.BorderStyle {
	if bs, ok := v.(frontend.BorderStyle); ok {
		return bs
	}
	return 0
}

// settingsToHTMLValues extracts border/padding/background settings into HTMLValues.
func settingsToHTMLValues(settings frontend.TypesettingSettings) HTMLValues {
	return HTMLValues{
		BackgroundColor:         settingColor(settings[frontend.SettingBackgroundColor]),
		BorderTopWidth:          settingSP(settings[frontend.SettingBorderTopWidth]),
		BorderRightWidth:        settingSP(settings[frontend.SettingBorderRightWidth]),
		BorderBottomWidth:       settingSP(settings[frontend.SettingBorderBottomWidth]),
		BorderLeftWidth:         settingSP(settings[frontend.SettingBorderLeftWidth]),
		BorderTopColor:          settingColor(settings[frontend.SettingBorderTopColor]),
		BorderRightColor:        settingColor(settings[frontend.SettingBorderRightColor]),
		BorderBottomColor:       settingColor(settings[frontend.SettingBorderBottomColor]),
		BorderLeftColor:         settingColor(settings[frontend.SettingBorderLeftColor]),
		BorderTopStyle:          settingBorderStyle(settings[frontend.SettingBorderTopStyle]),
		BorderRightStyle:        settingBorderStyle(settings[frontend.SettingBorderRightStyle]),
		BorderBottomStyle:       settingBorderStyle(settings[frontend.SettingBorderBottomStyle]),
		BorderLeftStyle:         settingBorderStyle(settings[frontend.SettingBorderLeftStyle]),
		BorderTopLeftRadius:     settingSP(settings[frontend.SettingBorderTopLeftRadius]),
		BorderTopRightRadius:    settingSP(settings[frontend.SettingBorderTopRightRadius]),
		BorderBottomLeftRadius:  settingSP(settings[frontend.SettingBorderBottomLeftRadius]),
		BorderBottomRightRadius: settingSP(settings[frontend.SettingBorderBottomRightRadius]),
		PaddingTop:              settingSP(settings[frontend.SettingPaddingTop]),
		PaddingRight:            settingSP(settings[frontend.SettingPaddingRight]),
		PaddingBottom:           settingSP(settings[frontend.SettingPaddingBottom]),
		PaddingLeft:             settingSP(settings[frontend.SettingPaddingLeft]),
	}
}
