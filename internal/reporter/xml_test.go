package reporter

import (
	"strings"
	"testing"
)

func TestSanitizeXMLText(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain text unchanged", "hello world", "hello world"},
		{"keeps tab/lf/cr", "a\tb\nc\rd", "a\tb\nc\rd"},
		{"drops NUL", "before\x00after", "beforeafter"},
		{"drops assorted C0 controls", "a\x01\x02\x08\x0b\x0c\x1bz", "az"},
		{"keeps unicode", "café — 日本語", "café — 日本語"},
		{"drops only illegal in mixed", "log\x00 line\x07 end", "log line end"},
		{"empty stays empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeXMLText(tc.in)
			if got != tc.want {
				t.Errorf("sanitizeXMLText(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeXMLTextRemovesAllIllegalC0(t *testing.T) {
	var b strings.Builder
	for r := rune(0); r < 0x20; r++ {
		b.WriteRune(r)
	}
	got := sanitizeXMLText(b.String())
	// Only tab (0x09), LF (0x0A) and CR (0x0D) are legal and must survive.
	if got != "\t\n\r" {
		t.Errorf("expected only \\t\\n\\r to survive, got %q", got)
	}
}
