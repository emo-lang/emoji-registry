package accounts

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/daqing/airway/lib/repo"
	"github.com/daqing/airway/lib/sql"

	"github.com/emo-lang/emoji-registry/app/models"
	"github.com/emo-lang/emoji-registry/app/services/totp"
)

const RecoveryCodeCount = 10

var recoveryEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// GenerateRecoveryCodes returns fresh one-time codes in display form, e.g.
// XXXXX-XXXXX. Only their hashes are stored.
func GenerateRecoveryCodes() ([]string, error) {
	codes := make([]string, 0, RecoveryCodeCount)
	for range RecoveryCodeCount {
		buf := make([]byte, 7)
		if _, err := rand.Read(buf); err != nil {
			return nil, err
		}
		raw := recoveryEncoding.EncodeToString(buf)[:10]
		codes = append(codes, raw[:5]+"-"+raw[5:])
	}
	return codes, nil
}

// RecoveryCodeHash normalizes a code (case, spaces, dashes) and hashes it.
func RecoveryCodeHash(code string) string {
	normalized := strings.ToUpper(code)
	normalized = strings.NewReplacer("-", "", " ", "").Replace(normalized)
	sum := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(sum[:])
}

// StoreRecoveryCodes replaces every recovery code belonging to userID.
func StoreRecoveryCodes(userID sql.IdType, codes []string) error {
	return repo.WithTx(repo.CurrentDB(), func(tx *repo.Tx) error {
		table := sql.TableFor(models.TOTPRecoveryCode{})
		if err := tx.Delete(sql.DeleteFrom(table).
			Where(sql.FieldEq(sql.FieldFor(models.TOTPRecoveryCode{}, "user_id"), userID))); err != nil {
			return err
		}

		for _, code := range codes {
			if _, err := repo.InsertWith[models.TOTPRecoveryCode](tx.Executor(), sql.Create(models.TOTPRecoveryCode{}, sql.H{
				"user_id":   userID,
				"code_hash": RecoveryCodeHash(code),
			})); err != nil {
				return err
			}
		}
		return nil
	})
}

// VerifyRecoveryCode consumes an unused recovery code. It returns false when
// the code is unknown or was already used; the atomic update keeps two
// concurrent requests from spending the same code.
func VerifyRecoveryCode(userID sql.IdType, code string) (bool, error) {
	row, err := repo.FindOneBy[models.TOTPRecoveryCode](sql.H{
		"user_id":   userID,
		"code_hash": RecoveryCodeHash(code),
	})
	if err != nil || row == nil {
		return false, err
	}

	b := sql.UpdateTable(sql.TableFor(models.TOTPRecoveryCode{})).
		Set(sql.H{"used_at": time.Now()}).
		Where(sql.AllOf(
			sql.FieldEq(sql.FieldFor(models.TOTPRecoveryCode{}, "id"), row.ID),
			sql.IsNull(sql.FieldFor(models.TOTPRecoveryCode{}, "used_at").String()),
		))
	affected, err := repo.UpdateAffected(repo.CurrentDB(), b)
	if err != nil {
		return false, err
	}
	return affected == 1, nil
}

func CountUnusedRecoveryCodes(userID sql.IdType) (int64, error) {
	b := sql.SelectColumns("count(*)").
		FromTable(sql.TableFor(models.TOTPRecoveryCode{})).
		Where(sql.AllOf(
			sql.FieldEq(sql.FieldFor(models.TOTPRecoveryCode{}, "user_id"), userID),
			sql.IsNull(sql.FieldFor(models.TOTPRecoveryCode{}, "used_at").String()),
		))
	return repo.Count(repo.CurrentDB(), b)
}

// EnsureTOTPSecret returns the user's enrollment secret, generating and
// persisting one on first use. The conditional update keeps concurrent setup
// requests on the same secret.
func EnsureTOTPSecret(user *models.User) (string, error) {
	if user.TOTPSecret != "" {
		return user.TOTPSecret, nil
	}

	secret, err := totp.GenerateSecret()
	if err != nil {
		return "", err
	}

	b := sql.UpdateTable(sql.TableFor(models.User{})).
		Set(sql.H{"totp_secret": secret}).
		Where(sql.AllOf(
			sql.FieldEq(sql.FieldFor(models.User{}, "id"), user.ID),
			sql.Eq("totp_secret", ""),
		))
	affected, err := repo.UpdateAffected(repo.CurrentDB(), b)
	if err != nil {
		return "", err
	}
	if affected == 1 {
		user.TOTPSecret = secret
		return secret, nil
	}

	fresh, err := repo.FindByID[models.User](user.ID)
	if err != nil {
		return "", err
	}
	if fresh == nil || fresh.TOTPSecret == "" {
		return "", errors.New("failed to store TOTP secret")
	}
	user.TOTPSecret = fresh.TOTPSecret
	return fresh.TOTPSecret, nil
}

// VerifySecondFactor accepts either a current TOTP code or an unused
// recovery code.
func VerifySecondFactor(user *models.User, code string) (bool, error) {
	if totp.Validate(user.TOTPSecret, code, time.Now()) {
		return true, nil
	}
	return VerifyRecoveryCode(user.ID, code)
}

// ResetTOTP removes a user's second-factor state and signs them out
// everywhere; the next login starts enrollment again.
func ResetTOTP(userID sql.IdType) error {
	if err := repo.UpdateByID[models.User](userID, sql.H{
		"totp_secret":  "",
		"totp_enabled": false,
	}); err != nil {
		return err
	}
	if err := repo.DeleteWhere[models.TOTPRecoveryCode](sql.H{"user_id": userID}); err != nil {
		return err
	}
	return repo.DeleteWhere[models.Session](sql.H{"user_id": userID})
}
