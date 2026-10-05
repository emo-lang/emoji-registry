package emoji

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// Digest computes the SHA-256 content digest shared with the compiler: over
// the given files sorted by path, each entry fed to the hash as
// `path \x00 content \x00`. The hex-encoded digest is what clients write
// into emo.lock.
func Digest(files map[string][]byte) string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	sort.Strings(paths)

	h := sha256.New()
	for _, path := range paths {
		h.Write([]byte(path))
		h.Write([]byte{0})
		h.Write(files[path])
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))
}

// SHA256Hex returns the hex-encoded SHA-256 of raw bytes.
func SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
