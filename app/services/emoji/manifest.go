// Package emoji implements the registry-side package primitives of the Emo
// ecosystem: parsing of the restricted package.emo manifest, the SHA-256
// content digest shared with the compiler, and deterministic handling of the
// .emoji (gzip tar) archive format.
package emoji

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	namePartPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)
	versionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

var validTargets = map[string]bool{
	"native":     true,
	"wasm":       true,
	"typescript": true,
	"beam":       true,
	"riscv64":    true,
}

// Manifest is the parsed content of a package.emo file. The schema is fixed
// by the compiler: exactly these four fields, literal values only.
type Manifest struct {
	Name    string
	Version string
	Targets []string
	Deps    map[string]string
}

// Owner returns the scope part of Name ("acme" in "acme/json_tools").
func (m *Manifest) Owner() string {
	owner, _, _ := strings.Cut(m.Name, "/")
	return owner
}

// ShortName returns the package part of Name ("json_tools" in
// "acme/json_tools").
func (m *Manifest) ShortName() string {
	_, name, _ := strings.Cut(m.Name, "/")
	return name
}

// ValidNamePart reports whether s is a legal owner or package name segment.
func ValidNamePart(s string) bool {
	return namePartPattern.MatchString(s)
}

// ValidVersion reports whether s is a strict major.minor.patch version.
func ValidVersion(s string) bool {
	return versionPattern.MatchString(s)
}

// CompareVersions orders two strict semver strings numerically per segment.
// It returns -1, 0 or 1. Callers must only pass strings accepted by
// ValidVersion.
func CompareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")

	for i := 0; i < 3; i++ {
		ai, bi := as[i], bs[i]
		// Both are digit-only, so length then lexicographic order matches
		// numeric order without overflow concerns.
		if len(ai) != len(bi) {
			if len(ai) < len(bi) {
				return -1
			}
			return 1
		}
		if ai != bi {
			if ai < bi {
				return -1
			}
			return 1
		}
	}

	return 0
}

// ParseManifest parses package.emo source under the restricted rules the
// compiler enforces: a single `package { ... }` block with literal `name`,
// `version`, `targets` and `deps` fields. Errors carry a line number.
func ParseManifest(src []byte) (*Manifest, error) {
	tokens, err := tokenize(string(src))
	if err != nil {
		return nil, err
	}

	p := &parser{tokens: tokens}

	if err := p.expectIdent("package"); err != nil {
		return nil, err
	}
	if err := p.expect(tokLBrace, "{"); err != nil {
		return nil, err
	}

	m := &Manifest{Deps: map[string]string{}}
	seen := map[string]bool{}

	for p.peek().kind != tokRBrace {
		if p.peek().kind == tokEOF {
			return nil, p.errorf("unexpected end of input, expected }")
		}

		fieldTok := p.next()
		if fieldTok.kind != tokIdent {
			return nil, p.errorAt(fieldTok, "expected a field name")
		}
		if seen[fieldTok.text] {
			return nil, p.errorAt(fieldTok, fmt.Sprintf("duplicate field %q", fieldTok.text))
		}
		seen[fieldTok.text] = true

		switch fieldTok.text {
		case "name":
			value, err := p.stringField()
			if err != nil {
				return nil, err
			}
			if err := validatePackageName(value); err != nil {
				return nil, p.errorAt(fieldTok, err.Error())
			}
			m.Name = value
		case "version":
			value, err := p.stringField()
			if err != nil {
				return nil, err
			}
			if !ValidVersion(value) {
				return nil, p.errorAt(fieldTok, fmt.Sprintf("invalid version %q: must be major.minor.patch with digits only", value))
			}
			m.Version = value
		case "targets":
			targets, err := p.targetsField()
			if err != nil {
				return nil, err
			}
			m.Targets = targets
		case "deps":
			deps, err := p.depsField()
			if err != nil {
				return nil, err
			}
			m.Deps = deps
		default:
			return nil, p.errorAt(fieldTok, fmt.Sprintf("unknown field %q: only name, version, targets and deps are allowed", fieldTok.text))
		}
	}

	p.next() // consume }

	if p.peek().kind != tokEOF {
		return nil, p.errorf("unexpected content after package block")
	}

	if m.Name == "" {
		return nil, fmt.Errorf("line 1: missing required field \"name\"")
	}
	if m.Version == "" {
		return nil, fmt.Errorf("line 1: missing required field \"version\"")
	}

	return m, nil
}

func validatePackageName(name string) error {
	owner, short, found := strings.Cut(name, "/")
	if !found || owner == "" || short == "" {
		return fmt.Errorf("invalid name %q: must be in owner/name form", name)
	}
	if !ValidNamePart(owner) || !ValidNamePart(short) {
		return fmt.Errorf("invalid name %q: owner and name must be 1-64 chars of [a-z0-9_-]", name)
	}
	return nil
}

// validateDepName accepts short names ("json") and scoped names
// ("acme/json") as dependency keys.
func validateDepName(name string) error {
	if strings.Contains(name, "/") {
		return validatePackageName(name)
	}
	if !ValidNamePart(name) {
		return fmt.Errorf("invalid dependency name %q", name)
	}
	return nil
}

type tokenKind int

const (
	tokEOF tokenKind = iota
	tokIdent
	tokString
	tokAssign
	tokLBrace
	tokRBrace
	tokLBracket
	tokRBracket
	tokComma
)

type token struct {
	kind tokenKind
	text string
	line int
}

func tokenize(src string) ([]token, error) {
	var tokens []token

	line := 1
	i := 0

	for i < len(src) {
		c := src[i]

		switch {
		case c == '\n':
			line++
			i++
		case c == ' ' || c == '\t' || c == '\r':
			i++
		case c == '#':
			for i < len(src) && src[i] != '\n' {
				i++
			}
		case c == '=':
			tokens = append(tokens, token{kind: tokAssign, text: "=", line: line})
			i++
		case c == '{':
			tokens = append(tokens, token{kind: tokLBrace, text: "{", line: line})
			i++
		case c == '}':
			tokens = append(tokens, token{kind: tokRBrace, text: "}", line: line})
			i++
		case c == '[':
			tokens = append(tokens, token{kind: tokLBracket, text: "[", line: line})
			i++
		case c == ']':
			tokens = append(tokens, token{kind: tokRBracket, text: "]", line: line})
			i++
		case c == ',':
			tokens = append(tokens, token{kind: tokComma, text: ",", line: line})
			i++
		case c == '"':
			startLine := line
			i++
			var sb strings.Builder
			for {
				if i >= len(src) {
					return nil, fmt.Errorf("line %d: unterminated string", startLine)
				}
				ch := src[i]
				if ch == '"' {
					i++
					break
				}
				if ch == '\n' {
					return nil, fmt.Errorf("line %d: unterminated string", startLine)
				}
				if ch == '\\' {
					if i+1 >= len(src) {
						return nil, fmt.Errorf("line %d: unterminated string", startLine)
					}
					next := src[i+1]
					if next != '"' && next != '\\' {
						return nil, fmt.Errorf("line %d: unsupported escape \\%c", startLine, next)
					}
					sb.WriteByte(next)
					i += 2
					continue
				}
				sb.WriteByte(ch)
				i++
			}
			tokens = append(tokens, token{kind: tokString, text: sb.String(), line: startLine})
		case isIdentStart(c):
			start := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			tokens = append(tokens, token{kind: tokIdent, text: src[start:i], line: line})
		default:
			return nil, fmt.Errorf("line %d: unexpected character %q", line, string(c))
		}
	}

	tokens = append(tokens, token{kind: tokEOF, line: line})
	return tokens, nil
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

type parser struct {
	tokens []token
	pos    int
}

func (p *parser) peek() token {
	return p.tokens[p.pos]
}

func (p *parser) next() token {
	t := p.tokens[p.pos]
	if p.pos < len(p.tokens)-1 {
		p.pos++
	}
	return t
}

func (p *parser) errorf(format string, args ...any) error {
	return p.errorAt(p.peek(), fmt.Sprintf(format, args...))
}

func (p *parser) errorAt(t token, msg string) error {
	return fmt.Errorf("line %d: %s", t.line, msg)
}

func (p *parser) expect(kind tokenKind, what string) error {
	t := p.next()
	if t.kind != kind {
		return p.errorAt(t, fmt.Sprintf("expected %s", what))
	}
	return nil
}

func (p *parser) expectIdent(name string) error {
	t := p.next()
	if t.kind != tokIdent || t.text != name {
		return p.errorAt(t, fmt.Sprintf("expected %q", name))
	}
	return nil
}

func (p *parser) stringField() (string, error) {
	if err := p.expect(tokAssign, "="); err != nil {
		return "", err
	}
	t := p.next()
	if t.kind != tokString {
		return "", p.errorAt(t, "expected a string literal")
	}
	return t.text, nil
}

func (p *parser) targetsField() ([]string, error) {
	if err := p.expect(tokAssign, "="); err != nil {
		return nil, err
	}
	if err := p.expect(tokLBracket, "["); err != nil {
		return nil, err
	}

	targets := []string{}
	seen := map[string]bool{}

	for p.peek().kind != tokRBracket {
		t := p.next()
		if t.kind != tokString {
			return nil, p.errorAt(t, "expected a target string")
		}
		if !validTargets[t.text] {
			return nil, p.errorAt(t, fmt.Sprintf("unknown target %q", t.text))
		}
		if seen[t.text] {
			return nil, p.errorAt(t, fmt.Sprintf("duplicate target %q", t.text))
		}
		seen[t.text] = true
		targets = append(targets, t.text)

		if p.peek().kind == tokComma {
			p.next()
		} else if p.peek().kind != tokRBracket {
			return nil, p.errorf("expected , or ]")
		}
	}

	p.next() // consume ]
	return targets, nil
}

func (p *parser) depsField() (map[string]string, error) {
	if err := p.expect(tokLBrace, "{"); err != nil {
		return nil, err
	}

	deps := map[string]string{}

	for p.peek().kind != tokRBrace {
		if p.peek().kind == tokEOF {
			return nil, p.errorf("unexpected end of input, expected }")
		}

		nameTok := p.next()
		if nameTok.kind != tokIdent && nameTok.kind != tokString {
			return nil, p.errorAt(nameTok, "expected a dependency name")
		}
		if err := validateDepName(nameTok.text); err != nil {
			return nil, p.errorAt(nameTok, err.Error())
		}
		if _, dup := deps[nameTok.text]; dup {
			return nil, p.errorAt(nameTok, fmt.Sprintf("duplicate dependency %q", nameTok.text))
		}

		if err := p.expect(tokAssign, "="); err != nil {
			return nil, err
		}

		versionTok := p.next()
		if versionTok.kind != tokString {
			return nil, p.errorAt(versionTok, "expected a version string")
		}
		if !ValidVersion(versionTok.text) {
			return nil, p.errorAt(versionTok, fmt.Sprintf("invalid dependency version %q: exact major.minor.patch required", versionTok.text))
		}

		deps[nameTok.text] = versionTok.text
	}

	p.next() // consume }
	return deps, nil
}
