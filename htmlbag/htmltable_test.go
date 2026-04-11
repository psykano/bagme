package htmlbag

import (
	"testing"

	"github.com/boxesandglue/boxesandglue/backend/bag"
	"github.com/boxesandglue/boxesandglue/frontend"
)

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
