package emoji

import (
	"strings"
	"testing"
)

func TestParseManifestFull(t *testing.T) {
	src := `
package {
  name = "acme/json_tools"      # a comment
  version = "0.1.0"
  targets = ["native", "wasm"]
  deps {
    json = "2.3.1"
    http = "1.4.2"
  }
}
`
	m, err := ParseManifest([]byte(src))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	if m.Name != "acme/json_tools" || m.Version != "0.1.0" {
		t.Fatalf("unexpected manifest: %+v", m)
	}
	if m.Owner() != "acme" || m.ShortName() != "json_tools" {
		t.Fatalf("unexpected name split: %q %q", m.Owner(), m.ShortName())
	}
	if len(m.Targets) != 2 || m.Targets[0] != "native" || m.Targets[1] != "wasm" {
		t.Fatalf("unexpected targets: %#v", m.Targets)
	}
	if m.Deps["json"] != "2.3.1" || m.Deps["http"] != "1.4.2" {
		t.Fatalf("unexpected deps: %#v", m.Deps)
	}
}

func TestParseManifestMinimal(t *testing.T) {
	m, err := ParseManifest([]byte(`package { name = "a/b" version = "1.2.3" }`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(m.Targets) != 0 || len(m.Deps) != 0 {
		t.Fatalf("expected empty targets and deps: %+v", m)
	}
}

func TestParseManifestErrors(t *testing.T) {
	cases := []struct {
		name    string
		src     string
		wantSub string
	}{
		{"unknown field", `package { name = "a/b" version = "1.0.0" author = "x" }`, "unknown field"},
		{"missing name", `package { version = "1.0.0" }`, `missing required field "name"`},
		{"missing version", `package { name = "a/b" }`, `missing required field "version"`},
		{"name without owner", `package { name = "plain" version = "1.0.0" }`, "owner/name"},
		{"uppercase name", `package { name = "Acme/Tool" version = "1.0.0" }`, "invalid name"},
		{"bad version", `package { name = "a/b" version = "1.0" }`, "invalid version"},
		{"prerelease version", `package { name = "a/b" version = "1.0.0-rc1" }`, "invalid version"},
		{"unknown target", `package { name = "a/b" version = "1.0.0" targets = ["jvm"] }`, "unknown target"},
		{"range dep version", `package { name = "a/b" version = "1.0.0" deps { json = "^2.3.1" } }`, "invalid dependency version"},
		{"unterminated", `package { name = "a/b"`, "unexpected end of input"},
		{"not a string", `package { name = 5 version = "1.0.0" }`, "unexpected character"},
		{"trailing content", `package { name = "a/b" version = "1.0.0" } extra`, "unexpected content"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseManifest([]byte(tc.src))
			if err == nil {
				t.Fatalf("expected error for %q", tc.src)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error %q does not mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestParseManifestErrorLineNumber(t *testing.T) {
	src := "package {\n  name = \"a/b\"\n  version = \"oops\"\n}"
	_, err := ParseManifest([]byte(src))
	if err == nil || !strings.HasPrefix(err.Error(), "line 3:") {
		t.Fatalf("expected line 3 error, got %v", err)
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.0", "0.2.0", -1},
		{"1.0.0", "0.9.9", 1},
		{"0.10.0", "0.9.0", 1},
		{"2.0.0", "10.0.0", -1},
	}
	for _, tc := range cases {
		if got := CompareVersions(tc.a, tc.b); got != tc.want {
			t.Fatalf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
