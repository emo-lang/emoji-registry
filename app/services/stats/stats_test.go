package stats

import (
	"io"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"
	"github.com/redis/go-redis/v9"

	"github.com/emo-lang/emoji-registry/app/models"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

func setupDB(t *testing.T) {
	t.Helper()

	dsn := "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))

	if err := migrate.Run(migrate.Options{DSN: dsn, Migrations: fstest.MapFS{}, Out: io.Discard}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.SetupDB(dsn); err != nil {
		t.Fatalf("setup db: %v", err)
	}
}

func seedPackage(t *testing.T) (*models.Package, *models.Version) {
	t.Helper()

	pkg, err := repo.CreateFrom[models.Package](sql.H{
		"owner_scope": "alice", "name": "tools", "description": "", "license": "",
		"homepage": "", "repository": "", "user_id": 0, "downloads": 0,
	})
	if err != nil {
		t.Fatalf("create package: %v", err)
	}

	version, err := repo.CreateFrom[models.Version](sql.H{
		"package_id": pkg.ID, "version": "1.0.0", "checksum": "ab", "archive_sha256": "cd",
		"targets": "", "deps": "{}", "size": 10, "storage_key": "k", "user_id": 0,
	})
	if err != nil {
		t.Fatalf("create version: %v", err)
	}

	return pkg, version
}

func TestRedisAggregationAndFlush(t *testing.T) {
	setupDB(t)

	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr()})
	t.Cleanup(func() { _ = client.Close(); Setup(nil) })

	Setup(client)
	if BackendName() != "redis" {
		t.Fatalf("expected redis backend")
	}

	pkg, version := seedPackage(t)
	now := time.Now()

	for i := 0; i < 3; i++ {
		if err := Record(pkg.ID, version.ID, now); err != nil {
			t.Fatalf("record: %v", err)
		}
	}

	// Counters live in Redis; the database is untouched until the flush.
	if got, err := server.Get("stats:dl:pkg:1"); err != nil || got != "3" {
		t.Fatalf("expected redis package counter 3, got %q (%v)", got, err)
	}
	reloaded, _ := repo.FindByID[models.Package](pkg.ID)
	if reloaded.Downloads != 0 {
		t.Fatalf("database should not be updated before flush")
	}

	if err := Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	reloaded, _ = repo.FindByID[models.Package](pkg.ID)
	if reloaded.Downloads != 3 {
		t.Fatalf("expected 3 downloads after flush, got %d", reloaded.Downloads)
	}

	date := now.Format("2006-01-02")
	row, err := repo.FindOneBy[models.Download](sql.H{"version_id": version.ID, "date": date})
	if err != nil || row == nil || row.Count != 3 {
		t.Fatalf("expected downloads row with count 3, got %+v (%v)", row, err)
	}

	if server.Exists("stats:dl:pkg:1") {
		t.Fatalf("flushed keys must be deleted")
	}

	// A second flush must not double-count.
	if err := Flush(); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	reloaded, _ = repo.FindByID[models.Package](pkg.ID)
	if reloaded.Downloads != 3 {
		t.Fatalf("double count: downloads = %d", reloaded.Downloads)
	}

	// Counts arriving between flushes accumulate on a fresh counter.
	_ = Record(pkg.ID, version.ID, now)
	_ = Record(pkg.ID, version.ID, now)
	if err := Flush(); err != nil {
		t.Fatalf("third flush: %v", err)
	}
	reloaded, _ = repo.FindByID[models.Package](pkg.ID)
	if reloaded.Downloads != 5 {
		t.Fatalf("expected 5 downloads total, got %d", reloaded.Downloads)
	}
	row, _ = repo.FindOneBy[models.Download](sql.H{"version_id": version.ID, "date": date})
	if row.Count != 5 {
		t.Fatalf("expected downloads row count 5, got %d", row.Count)
	}
}

func TestSyncModeWritesThrough(t *testing.T) {
	setupDB(t)
	Setup(nil)

	if BackendName() != "database" {
		t.Fatalf("expected database backend")
	}

	pkg, version := seedPackage(t)

	if err := Record(pkg.ID, version.ID, time.Now()); err != nil {
		t.Fatalf("record: %v", err)
	}

	reloaded, _ := repo.FindByID[models.Package](pkg.ID)
	if reloaded.Downloads != 1 {
		t.Fatalf("expected synchronous write, got %d", reloaded.Downloads)
	}
	row, _ := repo.FindOneBy[models.Download](sql.H{"version_id": version.ID, "date": time.Now().Format("2006-01-02")})
	if row == nil || row.Count != 1 {
		t.Fatalf("expected downloads row with count 1, got %+v", row)
	}
}
