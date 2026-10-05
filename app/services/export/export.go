// Package export renders the whole registry as a static file tree matching
// the protocol A layout (plus the downloads/ archive tree), so a registry can
// be served by nginx, a CDN or object storage with EMO_REGISTRY pointed at it.
//
// Only public packages are exported. The export overwrites files in place and
// never deletes: for an exact mirror, clear the target directory first.
package export

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
)

// Stats summarizes one export run.
type Stats struct {
	Packages       int
	Versions       int
	Archives       int
	Files          int
	SkippedPrivate int
}

// Run exports every public package into dir.
func Run(dir string) (Stats, error) {
	var stats Stats

	pkgs, err := repo.FindAll[models.Package]()
	if err != nil {
		return stats, err
	}

	sort.Slice(pkgs, func(i, j int) bool { return pkgs[i].FullName() < pkgs[j].FullName() })

	index := []indexEntry{}

	for _, pkg := range pkgs {
		if pkg.Private() {
			stats.SkippedPrivate++
			continue
		}
		stats.Packages++

		versions, err := repo.FindBy[models.Version](sql.H{"package_id": pkg.ID})
		if err != nil {
			return stats, err
		}
		sort.Slice(versions, func(i, j int) bool {
			return emoji.CompareVersions(versions[i].Version, versions[j].Version) < 0
		})

		// The versions index: ascending semver, yanked excluded — byte-identical
		// to the protocol A response.
		listed := []string{}
		for _, v := range versions {
			if !v.Yanked() {
				listed = append(listed, v.Version)
			}
		}

		versionsJSON, err := json.Marshal(listed)
		if err != nil {
			return stats, err
		}
		if err := writeFile(filepath.Join(dir, pkg.OwnerScope, pkg.Name, "versions"), versionsJSON); err != nil {
			return stats, err
		}

		latest := ""
		for _, v := range versions {
			if v.Yanked() {
				continue
			}
			latest = v.Version

			// Source files come out of the stored archive itself: the storage
			// layer has no listing, and the archive is the canonical content.
			files, err := readArchive(v.StorageKey)
			if err != nil {
				return stats, err
			}
			stats.Versions++

			for name, content := range files {
				if !strings.HasSuffix(name, ".emo") {
					continue
				}
				if err := writeFile(filepath.Join(dir, pkg.OwnerScope, pkg.Name, v.Version, name), content); err != nil {
					return stats, err
				}
				stats.Files++
			}
		}

		// Archives are exported for every version, yanked included — matching
		// the live download endpoint.
		for _, v := range versions {
			data, err := readBlob(v.StorageKey)
			if err != nil {
				return stats, err
			}
			name := pkg.OwnerScope + "--" + pkg.Name + "--" + v.Version + ".emoji"
			if err := writeFile(filepath.Join(dir, "downloads", name), data); err != nil {
				return stats, err
			}
			stats.Archives++
		}

		index = append(index, indexEntry{Name: pkg.FullName(), Latest: latest})
	}

	if err := writeFile(filepath.Join(dir, "index.html"), []byte(renderIndex(index))); err != nil {
		return stats, err
	}

	return stats, nil
}

// readArchive fetches and unpacks a stored .emoji archive.
func readArchive(key string) (map[string][]byte, error) {
	data, err := readBlob(key)
	if err != nil {
		return nil, err
	}
	return emoji.Unpack(data)
}

func readBlob(key string) ([]byte, error) {
	r, err := storage.Current().Get(context.Background(), key)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", key, err)
	}
	defer r.Close()
	return io.ReadAll(r)
}

func writeFile(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0644)
}

// indexEntry is one row of the human-browsable index page.
type indexEntry struct {
	Name   string
	Latest string
}

// renderIndex builds the minimal human-browsable index page. It is not part
// of the protocol.
func renderIndex(entries []indexEntry) string {
	var buf bytes.Buffer
	buf.WriteString("<!DOCTYPE html>\n<html><head><meta charset=\"utf-8\"><title>Emo Registry</title></head><body>\n")
	buf.WriteString("<h1>Emo Registry</h1>\n<ul>\n")
	for _, e := range entries {
		buf.WriteString(`<li><a href="` + e.Name + `/versions">` + e.Name + `</a>`)
		if e.Latest != "" {
			buf.WriteString(` — <a href="` + e.Name + `/` + e.Latest + `/package.emo">` + e.Latest + `</a>`)
		}
		buf.WriteString("</li>\n")
	}
	buf.WriteString("</ul>\n</body></html>\n")
	return buf.String()
}
