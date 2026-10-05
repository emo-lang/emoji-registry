package emoji

// reservedNames are top-level short names held back for the official Emo
// stdlib; packages under these owner scopes cannot be registered by ordinary
// accounts.
// TODO: move the reserved list into the database once governance tooling lands.
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

// IsReserved reports whether name is a reserved short name.
func IsReserved(name string) bool {
	return reservedNames[name]
}
