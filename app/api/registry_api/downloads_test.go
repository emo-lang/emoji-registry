package registry_api

import (
	"net/http"
	"testing"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
)

func TestDownloadStats(t *testing.T) {
	r := setupTest(t)

	signup(t, r, "alice")
	token := createToken(t, r, "alice", "push")

	if resp := publish(t, r, token, "alice/tools", "1.0.0"); resp.Code != http.StatusCreated {
		t.Fatalf("publish: %d %s", resp.Code, resp.Body.String())
	}

	for i := 0; i < 3; i++ {
		resp := do(t, r, http.MethodGet, "/downloads/alice--tools--1.0.0.emoji", nil, nil)
		if resp.Code != http.StatusOK {
			t.Fatalf("download %d: expected 200, got %d", i, resp.Code)
		}
	}

	pkg, err := repo.FindOneBy[models.Package](sql.H{"owner_scope": "alice", "name": "tools"})
	if err != nil || pkg == nil {
		t.Fatalf("find package: %v", err)
	}
	if pkg.Downloads != 3 {
		t.Fatalf("expected 3 downloads on package, got %d", pkg.Downloads)
	}

	today := time.Now().Format("2006-01-02")
	rows, err := repo.FindBy[models.Download](sql.H{"date": today})
	if err != nil {
		t.Fatalf("find downloads: %v", err)
	}
	if len(rows) != 1 || rows[0].Count != 3 {
		t.Fatalf("expected one downloads row with count 3, got %+v", rows)
	}

	resp := do(t, r, http.MethodGet, "/api/v1/packages/alice/tools", nil, nil)
	var detail struct {
		Downloads        int64 `json:"downloads"`
		DownloadsLast30d int64 `json:"downloads_last_30d"`
	}
	decodeJSON(t, resp, &detail)
	if detail.Downloads != 3 || detail.DownloadsLast30d != 3 {
		t.Fatalf("unexpected B.4 downloads fields: %s", resp.Body.String())
	}
}
