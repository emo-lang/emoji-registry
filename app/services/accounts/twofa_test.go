package accounts

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/daqing/airway/lib/migrate"
	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
	_ "github.com/emo-lang/emoji-registry/db/migrate"
)

func setupTwoFATest(t *testing.T) *models.User {
	t.Helper()

	dsn := "sqlite://" + filepath.ToSlash(filepath.Join(t.TempDir(), "test.db"))
	if err := migrate.Run(migrate.Options{DSN: dsn, Migrations: fstest.MapFS{}, Out: io.Discard}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if _, err := repo.SetupDB(dsn); err != nil {
		t.Fatalf("setup db: %v", err)
	}

	user, err := repo.CreateFrom[models.User](sql.H{
		"username":        "alice",
		"email":           "alice@example.com",
		"password_digest": "x",
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	return user
}

func TestRecoveryCodesAreFormattedAndUnique(t *testing.T) {
	codes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if len(codes) != RecoveryCodeCount {
		t.Fatalf("expected %d codes, got %d", RecoveryCodeCount, len(codes))
	}

	seen := map[string]bool{}
	for _, code := range codes {
		parts := strings.Split(code, "-")
		if len(parts) != 2 || len(parts[0]) != 5 || len(parts[1]) != 5 {
			t.Fatalf("unexpected code format %q", code)
		}
		if seen[code] {
			t.Fatalf("duplicate code %q", code)
		}
		seen[code] = true
	}
}

func TestRecoveryCodeHashNormalizes(t *testing.T) {
	if RecoveryCodeHash("abcde-fghij") != RecoveryCodeHash("ABCDE FGHIJ") {
		t.Fatalf("expected case, spaces and dashes to be ignored")
	}
	if RecoveryCodeHash("abcde-fghij") == "abcde-fghij" {
		t.Fatalf("expected the stored value to be a hash")
	}
}

func TestRecoveryCodeIsSingleUse(t *testing.T) {
	user := setupTwoFATest(t)

	codes, err := GenerateRecoveryCodes()
	if err != nil {
		t.Fatalf("GenerateRecoveryCodes: %v", err)
	}
	if err := StoreRecoveryCodes(user.ID, codes); err != nil {
		t.Fatalf("StoreRecoveryCodes: %v", err)
	}

	remaining, err := CountUnusedRecoveryCodes(user.ID)
	if err != nil || remaining != RecoveryCodeCount {
		t.Fatalf("expected %d unused codes, got %d (%v)", RecoveryCodeCount, remaining, err)
	}

	ok, err := VerifyRecoveryCode(user.ID, codes[0])
	if err != nil || !ok {
		t.Fatalf("first use: expected success, got %v (%v)", ok, err)
	}

	ok, err = VerifyRecoveryCode(user.ID, codes[0])
	if err != nil || ok {
		t.Fatalf("second use: expected rejection, got %v (%v)", ok, err)
	}

	remaining, err = CountUnusedRecoveryCodes(user.ID)
	if err != nil || remaining != RecoveryCodeCount-1 {
		t.Fatalf("expected %d unused codes, got %d (%v)", RecoveryCodeCount-1, remaining, err)
	}
}

func TestEnsureTOTPSecretIsStable(t *testing.T) {
	user := setupTwoFATest(t)

	first, err := EnsureTOTPSecret(user)
	if err != nil || first == "" {
		t.Fatalf("first secret: %q (%v)", first, err)
	}
	second, err := EnsureTOTPSecret(user)
	if err != nil || second != first {
		t.Fatalf("expected the same secret, got %q then %q (%v)", first, second, err)
	}
}

func TestResetTOTPClearsStateAndSessions(t *testing.T) {
	user := setupTwoFATest(t)

	if _, err := EnsureTOTPSecret(user); err != nil {
		t.Fatalf("EnsureTOTPSecret: %v", err)
	}
	if err := repo.UpdateByID[models.User](user.ID, sql.H{"totp_enabled": true}); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if err := StoreRecoveryCodes(user.ID, []string{"AAAAA-BBBBB"}); err != nil {
		t.Fatalf("store codes: %v", err)
	}
	if _, err := repo.CreateFrom[models.Session](sql.H{
		"token": "session-token", "user_id": user.ID, "expires_at": time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	if err := ResetTOTP(user.ID); err != nil {
		t.Fatalf("ResetTOTP: %v", err)
	}

	fresh, err := repo.FindByID[models.User](user.ID)
	if err != nil || fresh == nil {
		t.Fatalf("reload user: %v", err)
	}
	if fresh.TOTPEnabled || fresh.TOTPSecret != "" {
		t.Fatalf("expected TOTP to be cleared, got %+v", fresh)
	}
	if remaining, _ := CountUnusedRecoveryCodes(user.ID); remaining != 0 {
		t.Fatalf("expected recovery codes to be deleted")
	}
	if sessions, _ := repo.FindBy[models.Session](sql.H{"user_id": user.ID}); len(sessions) != 0 {
		t.Fatalf("expected sessions to be deleted")
	}
}
