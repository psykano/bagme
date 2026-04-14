package htmlbag

import (
	"fmt"
	"os"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/backend/node"
	"github.com/boxesandglue/boxesandglue/frontend"
)

const (
	SettingDisplayFlex       frontend.SettingType = 1000 + iota
	SettingFlexGrow                               // float64
	SettingFlexShrink                             // float64
	SettingFlexBasisAuto                          // bool
	SettingGap                                    // bag.ScaledPoint
	SettingAlignItems                             // string
	SettingJustifyContent                         // string
	SettingMinWidth                               // bag.ScaledPoint
	SettingBoxSizingBorderBox                     // bool
)

type flexChild struct {
	te        *frontend.Text
	flexGrow  float64
	basisAuto bool
	width     bag.ScaledPoint
	vl        *node.VList
}

func (cb *CSSBuilder) buildFlexRow(te *frontend.Text, wd bag.ScaledPoint) (*node.VList, error) {
	settings := te.Settings

	if sWd, ok := settings[frontend.SettingWidth]; ok {
		if wdStr, ok := sWd.(string); ok {
			wd = ParseRelativeSize(wdStr, wd, wd)
		}
	}

	hv := settingsToHTMLValues(settings)
	hasBorderOrBg := hv.hasBorder() || hv.BackgroundColor != nil

	contentWidth := wd
	if hasBorderOrBg {
		contentWidth = wd - hv.BorderLeftWidth - hv.BorderRightWidth - hv.PaddingLeft - hv.PaddingRight
	}

	gap := settingSP(settings[SettingGap])
	alignItems, _ := settings[SettingAlignItems].(string)
	if alignItems == "" {
		alignItems = "stretch"
	}
	justifyContent, _ := settings[SettingJustifyContent].(string)
	if justifyContent == "" {
		justifyContent = "flex-start"
	}

	var children []flexChild
	for _, itm := range te.Items {
		childTe, ok := itm.(*frontend.Text)
		if !ok {
			continue
		}
		if isWhitespaceOnly(childTe) {
			if _, hasTag := childTe.Settings[frontend.SettingDebug]; !hasTag {
				continue
			}
		}

		grow, _ := childTe.Settings[SettingFlexGrow].(float64)
		_, basisAuto := childTe.Settings[SettingFlexBasisAuto].(bool)
		if bAuto, ok := childTe.Settings[SettingFlexBasisAuto].(bool); ok {
			basisAuto = bAuto
		}

		tag, _ := childTe.Settings[frontend.SettingDebug].(string)
		fmt.Fprintf(os.Stderr, "FLEX-DEBUG: child[%d] tag=%q grow=%v basisAuto=%v\n", len(children), tag, grow, basisAuto)

		children = append(children, flexChild{
			te:        childTe,
			flexGrow:  grow,
			basisAuto: basisAuto,
		})
	}

	if len(children) == 0 {
		vls := node.NewVList()
		vls.Width = contentWidth
		if hasBorderOrBg {
			vls = cb.HTMLBorder(vls, hv)
		}
		return vls, nil
	}

	totalGap := gap * bag.ScaledPoint(len(children)-1)
	availableWidth := contentWidth - totalGap

	var totalGrow float64
	var fixedWidth bag.ScaledPoint
	for i := range children {
		if children[i].basisAuto && children[i].flexGrow == 0 {
			childWidth := estimateContentWidth(cb, children[i].te, availableWidth)
			children[i].width = childWidth
			fixedWidth += childWidth
		} else {
			totalGrow += children[i].flexGrow
		}
	}

	remainingWidth := availableWidth - fixedWidth
	if remainingWidth < 0 {
		remainingWidth = 0
	}

	for i := range children {
		if children[i].basisAuto && children[i].flexGrow == 0 {
			continue
		}
		if totalGrow > 0 {
			children[i].width = bag.ScaledPoint(float64(remainingWidth) * children[i].flexGrow / totalGrow)
		} else {
			children[i].width = remainingWidth / bag.ScaledPoint(len(children))
		}
	}

	for i := range children {
		tag, _ := children[i].te.Settings[frontend.SettingDebug].(string)
		fmt.Fprintf(os.Stderr, "FLEX-WIDTH: child[%d] tag=%q width=%v (basisAuto=%v grow=%v)\n", i, tag, children[i].width, children[i].basisAuto, children[i].flexGrow)
	}

	var maxHeight bag.ScaledPoint
	for i := range children {
		vl, err := cb.buildVlistInternal(children[i].te, children[i].width)
		if err != nil {
			return nil, err
		}
		children[i].vl = vl
		childHeight := vl.Height + vl.Depth
		if childHeight > maxHeight {
			maxHeight = childHeight
		}
	}

	var hlHead node.Node
	for i, child := range children {
		if i > 0 && gap > 0 {
			gapKern := node.NewKern()
			gapKern.Kern = gap
			hlHead = node.InsertAfter(hlHead, node.Tail(hlHead), gapKern)
		}

		childHL := node.NewHList()
		childHL.Width = child.width
		childHL.Height = child.vl.Height
		childHL.Depth = child.vl.Depth

		if alignItems == "center" {
			childHeight := child.vl.Height + child.vl.Depth
			if childHeight < maxHeight {
				shift := (maxHeight - childHeight) / 2
				child.vl.ShiftX = 0
				childHL.Height = maxHeight
				childHL.Depth = 0
				topKern := node.NewKern()
				topKern.Kern = shift
				childHL.List = node.InsertAfter(nil, nil, topKern)
				childHL.List = node.InsertAfter(childHL.List, topKern, child.vl)
			} else {
				childHL.List = child.vl
				childHL.Height = maxHeight
				childHL.Depth = 0
			}
		} else {
			childHL.List = child.vl
			if alignItems == "stretch" {
				childHL.Height = maxHeight
				childHL.Depth = 0
			}
		}

		hlHead = node.InsertAfter(hlHead, node.Tail(hlHead), childHL)
	}

	rowHL := node.NewHList()
	rowHL.Width = contentWidth
	rowHL.Height = maxHeight

	if justifyContent == "center" {
		usedWidth := totalGap
		for _, child := range children {
			usedWidth += child.width
		}
		if usedWidth < contentWidth {
			leadKern := node.NewKern()
			leadKern.Kern = (contentWidth - usedWidth) / 2
			rowHL.List = node.InsertAfter(nil, nil, leadKern)
			rowHL.List = node.InsertAfter(rowHL.List, leadKern, hlHead)
		} else {
			rowHL.List = hlHead
		}
	} else {
		rowHL.List = hlHead
	}

	vls := node.NewVList()
	vls.List = rowHL
	vls.Width = contentWidth
	vls.Height = maxHeight
	vls.Attributes = node.H{"origin": "buildFlexRow"}

	if hasBorderOrBg {
		vls = cb.HTMLBorder(vls, hv)
	}

	return vls, nil
}

func estimateContentWidth(cb *CSSBuilder, te *frontend.Text, maxWidth bag.ScaledPoint) bag.ScaledPoint {
	if sWd, ok := te.Settings[frontend.SettingWidth]; ok {
		if wdStr, ok := sWd.(string); ok {
			return ParseRelativeSize(wdStr, maxWidth, maxWidth)
		}
	}
	vl, err := cb.buildVlistInternal(te, maxWidth)
	if err != nil {
		return maxWidth
	}
	if vl.Width > 0 {
		return vl.Width
	}
	return maxWidth
}

func stripFlexSettings(settings frontend.TypesettingSettings) {
	for k := range settings {
		if k >= 1000 {
			delete(settings, k)
		}
	}
}

func warnUnsupportedFlex(property, value string) {
	fmt.Fprintf(os.Stderr, "bagme: unsupported flex value %q — falling back to block\n", property+": "+value)
}
