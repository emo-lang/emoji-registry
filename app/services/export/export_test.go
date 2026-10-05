package export

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/daqing/airway/lib/storage"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/emoji"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

func setupTest(t *testing.T) {
	t.Helper()

	dsn := "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))
	if err := migrate.Run(migrate.Options{DSN: dsn, Migrations: fstest.MapFS{}, Out: io.Discard}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.SetupDB(dsn); err != nil {
		t.Fatalf("setup db: %v", err)
	}
	if _, err := storage.Setup(storage.Config{Driver: storage.DriverLocal, Root: t.TempDir()}); err != nil {
		t.Fatalf("setup storage: %v", err)
	}
}

// publishSeed stores a package version the way the publish pipeline does:
// ProcessUpload archive under packages/<o>/<n>/<v>/package.emoji.
func publishSeed(t *testing.T, fullName, version string, yanked bool, private bool) {
	t.Helper()

	owner, name, _ := splitName(fullName)

	archive, err := emoji.BuildArchive(map[string][]byte{
		"package.emo": []byte("package {\n  name = \"" + fullName + "\"\n  version = \"" + version + "\"\n}\n"),
		"main.emo":    []byte("let main = 1 // " + version),
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}

	result, err := emoji.ProcessUpload(archive)
	if err != nil {
		t.Fatalf("process: %v", err)
	}

	pkg, err := repo.FindOneBy[models.Package](sql.H{"owner_scope": owner, "name": name})
	if err != nil {
		t.Fatalf("find package: %v", err)
	}
	if pkg == nil {
		pkg, err = repo.CreateFrom[models.Package](sql.H{
			"owner_scope": owner, "name": name, "description": "", "license": "",
			"homepage": "", "repository": "", "user_id": 0, "downloads": 0,
			"visibility": models.VisibilityPublic,
		})
		if err != nil {
			t.Fatalf("create package: %v", err)
		}
	}

	if private {
		if err := repo.UpdateByID[models.Package](pkg.ID, sql.H{"visibility": models.VisibilityPrivate}); err != nil {
			t.Fatalf("privatize: %v", err)
		}
	}

	storageKey := "packages/" + owner + "/" + name + "/" + version + "/package.emoji"
	if err := storage.Current().Put(t.Context(), storageKey, storage.Object{
		Reader: bytes.NewReader(result.Archive), Size: int64(len(result.Archive)), ContentType: "application/gzip",
	}); err != nil {
		t.Fatalf("store archive: %v", err)
	}

	vals := sql.H{
		"package_id": pkg.ID, "version": version, "checksum": result.Digest,
		"archive_sha256": result.ArchiveSHA256, "targets": "", "deps": "{}",
		"size": int64(len(result.Archive)), "storage_key": storageKey, "user_id": 0,
	}
	row, err := repo.CreateFrom[models.Version](vals)
	if err != nil {
		t.Fatalf("create version: %v", err)
	}

	if yanked {
		now := sql.H{"yanked_at": "2026-10-05 00:00:00"}
		if err := repo.UpdateByID[models.Version](row.ID, now); err != nil {
			t.Fatalf("yank: %v", err)
		}
	}
}

func splitName(fullName string) (string, string, bool) {
	for i := 0; i < len(fullName); i++ {
		if fullName[i] == '/' {
			return fullName[:i], fullName[i+1:], true
		}
	}
	return fullName, "", false
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestExport(t *testing.T) {
	setupTest(t)

	publishSeed(t, "alice/tools", "0.2.0", false, false)
	publishSeed(t, "alice/tools", "0.1.0", false, false)
	publishSeed(t, "alice/old", "1.0.0", true, false)
	publishSeed(t, "alice/secret", "1.0.0", false, true)

	dir := t.TempDir()

	stats, err := Run(dir)
	if err != nil {
		t.Fatalf("export: %v", err)
	}

	if stats.Packages != 2 || stats.SkippedPrivate != 1 || stats.Archives != 3 {
		t.Fatalf("unexpected stats: %+v", stats)
	}

	// The versions index is byte-identical to the protocol A response
	// (ascending semver, yanked excluded, no trailing newline).
	if got := readFile(t, filepath.Join(dir, "alice/tools/versions")); got != `["0.1.0","0.2.0"]` {
		t.Fatalf("unexpected versions file: %q", got)
	}
	if got := readFile(t, filepath.Join(dir, "alice/old/versions")); got != `[]` {
		t.Fatalf("yanked-only package should list no versions: %q", got)
	}

	// Source files, manifest included, only for live versions.
	manifest := readFile(t, filepath.Join(dir, "alice/tools/0.1.0/package.emo"))
	if !bytes.Contains([]byte(manifest), []byte(`name = "alice/tools"`)) {
		t.Fatalf("manifest content wrong: %q", manifest)
	}
	if got := readFile(t, filepath.Join(dir, "alice/tools/0.2.0/main.emo")); got != "let main = 1 // 0.2.0" {
		t.Fatalf("source content wrong: %q", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "alice/old/1.0.0/package.emo")); !os.IsNotExist(err) {
		t.Fatalf("yanked sources must not be exported")
	}

	// Archives are exported for every version, yanked included.
	for _, name := range []string{
		"alice--tools--0.1.0.emoji",
		"alice--tools--0.2.0.emoji",
		"alice--old--1.0.0.emoji",
	} {
		data, err := os.ReadFile(filepath.Join(dir, "downloads", name))
		if err != nil {
			t.Fatalf("missing archive %s: %v", name, err)
		}
		if _, err := emoji.Unpack(data); err != nil {
			t.Fatalf("archive %s does not unpack: %v", name, err)
		}
	}

	// The private package is absent entirely.
	if _, err := os.Stat(filepath.Join(dir, "alice/secret")); !os.IsNotExist(err) {
		t.Fatalf("private package must not be exported")
	}
	if _, err := os.Stat(filepath.Join(dir, "downloads/alice--secret--1.0.0.emoji")); !os.IsNotExist(err) {
		t.Fatalf("private archive must not be exported")
	}

	// The human index page lists public packages.
	index := readFile(t, filepath.Join(dir, "index.html"))
	if !bytes.Contains([]byte(index), []byte("alice/tools")) || bytes.Contains([]byte(index), []byte("alice/secret")) {
		t.Fatalf("index page wrong: %s", index)
	}
}
