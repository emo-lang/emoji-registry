package emoji

import (
	"regexp"
	"sort"
	"strings"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
)

// MaxReservedEntryLen bounds a stored reserved entry; it matches the
// reserved_names.name column width.
const MaxReservedEntryLen = 64

// reservedPatternChars limits regular-expression entries to the characters a
// name can contain plus the regex operators that make sense over them. Letters
// are accepted in either case; matching is case-insensitive. It keeps typos
// like "BAD NAME" from being accepted as a literal pattern.
var reservedPatternChars = regexp.MustCompile(`^[a-zA-Z0-9_^$.*+?()\[\]{}|\\-]+$`)

// reservedNames are top-level short names held back for the official Emo
// stdlib; packages under these owner scopes cannot be registered by ordinary
// accounts. This hardcoded list doubles as the compiler-side constant and as
// the seed data for the reserved_names table.
var reservedNames = map[string]bool{
	"net":    true,
	"http":   true,
	"core":   true,
	"std":    true,
	"sys":    true,
	"io":     true,
	"os":     true,
	"log":    true,
	"time":   true,
	"math":   true,
	"json":   true,
	"http2":  true,
	"ws":     true,
	"db":     true,
	"cache":  true,
	"queue":  true,
	"mail":   true,
	"test":   true,
	"debug":  true,
	"unsafe": true,
}

// ReservedSeedReason is stored on rows seeded from the hardcoded list.
const ReservedSeedReason = "official stdlib"

// BuiltinReservedNames returns the hardcoded stdlib list, sorted. The admin
// UI shows these as read-only; they cannot be removed at runtime.
func BuiltinReservedNames() []string {
	names := make([]string, 0, len(reservedNames))
	for name := range reservedNames {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidReservedEntry reports whether entry can be stored in the managed
// reserved list: a plain lowercase identifier (matched exactly) or a regular
// expression using the name alphabet and regex operators, e.g. "^emo".
func ValidReservedEntry(entry string) bool {
	if ValidNamePart(entry) {
		return true
	}

	if entry == "" || len(entry) > MaxReservedEntryLen || !reservedPatternChars.MatchString(entry) {
		return false
	}
	// A bare name is matched exactly, so an entry that is not a valid name
	// must actually use a regex operator to be meaningful.
	if !strings.ContainsAny(entry, "^$.*+?()[]{}|\\") {
		return false
	}

	_, err := compileReservedPattern(entry)
	return err == nil
}

// compileReservedPattern compiles a reserved-list regex, case-insensitively so
// operators can type names in any case.
func compileReservedPattern(entry string) (*regexp.Regexp, error) {
	return regexp.Compile("(?i)" + entry)
}

// IsReserved reports whether name is reserved: the union of the hardcoded
// stdlib list and the reserved_names table. Plain entries match exactly;
// entries with regex syntax match case-insensitively anywhere in the name, so
// "^emo" reserves every name starting with "emo". When no database is set up
// (unit tests, tooling) only the hardcoded list applies.
func IsReserved(name string) bool {
	if reservedNames[name] {
		return true
	}

	if _, ok := repo.CurrentDBOK(); !ok {
		return false
	}

	entries, err := repo.FindAll[models.ReservedName]()
	if err != nil {
		return false
	}

	for _, entry := range entries {
		if reservedEntryMatches(entry.Name, name) {
			return true
		}
	}

	return false
}

func reservedEntryMatches(entry, name string) bool {
	if entry == name {
		return true
	}
	if ValidNamePart(entry) {
		return false
	}

	re, err := compileReservedPattern(entry)
	return err == nil && re.MatchString(name)
}

// SeedReservedNames inserts the hardcoded stdlib list into reserved_names.
// Existing rows are left untouched, so it is safe to call on every boot.
func SeedReservedNames() error {
	for name := range reservedNames {
		exists, err := repo.ExistsWhere[models.ReservedName](sql.H{"name": name})
		if err != nil {
			return err
		}
		if exists {
			continue
		}

		if _, err := repo.CreateFrom[models.ReservedName](sql.H{
			"name":   name,
			"reason": ReservedSeedReason,
		}); err != nil {
			return err
		}
	}

	return nil
}
