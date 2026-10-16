package middlewares

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"strconv"
	"strings"
	"time"

	"github.com/daqing/airway/lib/utils"
	"github.com/gin-gonic/gin"
)

// AdminCookie carries the env-credentials admin session issued by
// /admin/login. It is self-contained: an expiry timestamp plus an HMAC
// keyed by the configured credentials, so rotating ADMIN_USERNAME or
// ADMIN_PASSWORD in .env invalidates every outstanding cookie.
const AdminCookie = "emo_admin"

// AdminSessionTTL is how long an env-credentials admin sign-in stays valid.
const AdminSessionTTL = 24 * time.Hour

// AdminEnvConfigured reports whether .env supplies both admin credentials;
// only then does /admin/login answer. Without them the env sign-in path
// stays invisible (404) and only `admin:grant` site accounts reach /admin.
func AdminEnvConfigured() bool {
	return adminEnvUsername() != "" && adminEnvPassword() != ""
}

func adminEnvUsername() string {
	return strings.TrimSpace(utils.GetEnvMulti("ADMIN_USERNAME"))
}

func adminEnvPassword() string {
	return utils.GetEnvMulti("ADMIN_PASSWORD")
}

// CheckAdminEnv reports whether the pair matches the configured credentials.
// Both comparisons are constant-time; the password is compared verbatim
// (surrounding spaces are significant).
func CheckAdminEnv(username, password string) bool {
	if !AdminEnvConfigured() {
		return false
	}
	userOK := subtle.ConstantTimeCompare([]byte(strings.TrimSpace(username)), []byte(adminEnvUsername())) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(adminEnvPassword())) == 1
	return userOK && passOK
}

func adminSigningKey() []byte {
	// The credentials themselves are the secret: hashing them into the key
	// keeps the HMAC input fixed-size and never puts the password on the wire.
	sum := sha256.Sum256([]byte(adminEnvUsername() + "\x1f" + adminEnvPassword()))
	return sum[:]
}

func adminMAC(expiry time.Time) string {
	mac := hmac.New(sha256.New, adminSigningKey())
	mac.Write([]byte("emo-admin|" + strconv.FormatInt(expiry.Unix(), 10)))
	return hex.EncodeToString(mac.Sum(nil))
}

// AdminCookieValue mints the signed cookie value for a session expiring at
// expiresAt. Split out of SignInAdmin so tests can mint expired values.
func AdminCookieValue(expiresAt time.Time) string {
	return strconv.FormatInt(expiresAt.Unix(), 10) + "." + adminMAC(expiresAt)
}

// SignInAdmin issues the admin session cookie for AdminSessionTTL.
func SignInAdmin(c *gin.Context) {
	c.SetCookie(AdminCookie, AdminCookieValue(time.Now().Add(AdminSessionTTL)),
		int(AdminSessionTTL.Seconds()), "/", "", false, true)
}

// SignOutAdmin clears the admin session cookie.
func SignOutAdmin(c *gin.Context) {
	c.SetCookie(AdminCookie, "", -1, "/", "", false, true)
}

// CurrentAdmin reports whether the request carries a valid, unexpired
// env-credentials admin cookie.
func CurrentAdmin(c *gin.Context) bool {
	if !AdminEnvConfigured() {
		return false
	}
	cookie, err := c.Cookie(AdminCookie)
	if err != nil || cookie == "" {
		return false
	}
	rawExpiry, sig, ok := strings.Cut(cookie, ".")
	if !ok {
		return false
	}
	expiry, err := strconv.ParseInt(rawExpiry, 10, 64)
	if err != nil || time.Now().Unix() > expiry {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(adminMAC(time.Unix(expiry, 0))))
}
