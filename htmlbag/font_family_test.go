package htmlbag

import (
	"io"
	"testing"

	"github.com/boxesandglue/boxesandglue/frontend"
)

func newTestDocument(t *testing.T) *frontend.Document {
	t.Helper()
	df, err := frontend.NewForWriter(io.Discard)
	if err != nil {
		t.Fatal("frontend.NewForWriter:", err)
	}
	if err := LoadIncludedFonts(df); err != nil {
		t.Fatal("LoadIncludedFonts:", err)
	}
	return df
}

func TestResolveFontFamilyGenericMapping(t *testing.T) {
	df := newTestDocument(t)

	tests := []struct {
		input    string
		wantName string
	}{
		{"sans-serif", "sans"},
		{"serif", "serif"},
		{"monospace", "monospace"},
		{"sans", "sans"},
	}

	for _, tt := range tests {
		ff := resolveFontFamily(df, tt.input)
		if ff == nil {
			t.Errorf("resolveFontFamily(%q) = nil, want %q", tt.input, tt.wantName)
			continue
		}
		if ff.Name != tt.wantName {
			t.Errorf("resolveFontFamily(%q).Name = %q, want %q", tt.input, ff.Name, tt.wantName)
		}
	}
}

func TestResolveFontFamilyFallbackList(t *testing.T) {
	df := newTestDocument(t)

	// Full Apple/Windows/Linux stack; only sans-serif (→"sans") matches.
	ff := resolveFontFamily(df, `-apple-system, "Segoe UI", Helvetica, Arial, sans-serif`)
	if ff == nil {
		t.Fatal("resolveFontFamily returned nil for Apple font stack")
	}
	if ff.Name != "sans" {
		t.Errorf("got %q, want %q", ff.Name, "sans")
	}
}

func TestResolveFontFamilyQuoteStripping(t *testing.T) {
	df := newTestDocument(t)

	// Quoted names that don't exist fall through; trailing sans-serif maps to sans.
	ff := resolveFontFamily(df, `"Segoe UI", sans-serif`)
	if ff == nil {
		t.Fatal("resolveFontFamily returned nil")
	}
	if ff.Name != "sans" {
		t.Errorf("got %q, want %q", ff.Name, "sans")
	}
}

func TestResolveFontFamilyCustomFallthrough(t *testing.T) {
	df := newTestDocument(t)

	// Unknown font falls through to registered generic.
	ff := resolveFontFamily(df, `"CustomFont", sans-serif`)
	if ff == nil {
		t.Fatal("resolveFontFamily returned nil")
	}
	if ff.Name != "sans" {
		t.Errorf("got %q, want %q", ff.Name, "sans")
	}
}

func TestResolveFontFamilyNoMatch(t *testing.T) {
	df := newTestDocument(t)

	// Totally unknown family → nil (caller logs and falls back to serif).
	ff := resolveFontFamily(df, "Wingdings, Zapf Dingbats")
	if ff != nil {
		t.Errorf("resolveFontFamily returned %q, want nil", ff.Name)
	}
}

func TestResolveFontFamilySingleSansSerif(t *testing.T) {
	df := newTestDocument(t)

	ff := resolveFontFamily(df, "sans-serif")
	if ff == nil {
		t.Fatal("resolveFontFamily(\"sans-serif\") returned nil")
	}
	if ff.Name != "sans" {
		t.Errorf("got %q, want %q", ff.Name, "sans")
	}
}

// TestFontFamilyParsing is the acceptance-criteria entry point named in the ticket.
func TestFontFamilyParsing(t *testing.T) {
	t.Run("AppleFontStack", TestResolveFontFamilyFallbackList)
	t.Run("GenericMapping", TestResolveFontFamilyGenericMapping)
	t.Run("QuoteStripping", TestResolveFontFamilyQuoteStripping)
	t.Run("CustomFallthrough", TestResolveFontFamilyCustomFallthrough)
	t.Run("NoMatch", TestResolveFontFamilyNoMatch)
}
