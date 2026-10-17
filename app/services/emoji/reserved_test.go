package emoji

import (
	"strings"
	"testing"
)

func TestValidReservedEntry(t *testing.T) {
	valid := []string{
		"net",
		"emo_tools",
		"^emo",
		"^Emo",
		"emo.*",
		"^(emo|acme)$",
	}
	for _, entry := range valid {
		if !ValidReservedEntry(entry) {
			t.Fatalf("expected %q to be a valid entry", entry)
		}
	}

	invalid := []string{
		"",
		"BAD NAME",
		"[",
		"emo-tools",
		strings.Repeat("a", MaxReservedEntryLen+1),
	}
	for _, entry := range invalid {
		if ValidReservedEntry(entry) {
			t.Fatalf("expected %q to be rejected", entry)
		}
	}
}

func TestReservedEntryMatches(t *testing.T) {
	cases := []struct {
		entry, name string
		want        bool
	}{
		{"net", "net", true},
		{"net", "netflix", false},
		{"^emo", "emo", true},
		{"^emo", "emotion", true},
		{"^emo", "remotion", false},
		{"^EMO", "emotion", true},
		{"^Emo", "emo", true},
		{"^(EMO|Acme)$", "acme", true},
		{"^(emo|acme)$", "acme", true},
		{"^(emo|acme)$", "acme_tools", false},
		{"[", "acme", false},
	}
	for _, tc := range cases {
		if got := reservedEntryMatches(tc.entry, tc.name); got != tc.want {
			t.Fatalf("reservedEntryMatches(%q, %q) = %v, want %v", tc.entry, tc.name, got, tc.want)
		}
	}
}
