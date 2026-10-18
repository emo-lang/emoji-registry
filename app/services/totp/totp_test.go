package totp

import (
	"strings"
	"testing"
	"time"
)

// RFC 6238 appendix B, SHA-1 secret "12345678901234567890".
const rfcSecret = "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"

func TestRFC6238Vectors(t *testing.T) {
	cases := []struct {
		unix int64
		want string
	}{
		{59, "287082"},
		{1111111109, "081804"},
		{1111111111, "050471"},
		{1234567890, "005924"},
		{2000000000, "279037"},
		{20000000000, "353130"},
	}

	for _, tc := range cases {
		got, err := Code(rfcSecret, time.Unix(tc.unix, 0))
		if err != nil {
			t.Fatalf("Code(%d): %v", tc.unix, err)
		}
		if got != tc.want {
			t.Fatalf("Code(%d) = %s, want %s", tc.unix, got, tc.want)
		}
	}
}

func TestValidateAcceptanceWindow(t *testing.T) {
	at := time.Unix(1234567890, 0)

	current, err := Code(rfcSecret, at)
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if !Validate(rfcSecret, current, at) {
		t.Fatalf("expected current code to validate")
	}

	previous, err := Code(rfcSecret, at.Add(-Step))
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	next, err := Code(rfcSecret, at.Add(Step))
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if !Validate(rfcSecret, previous, at) || !Validate(rfcSecret, next, at) {
		t.Fatalf("expected ±1 step codes to validate")
	}

	stale, err := Code(rfcSecret, at.Add(-2*Step))
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if Validate(rfcSecret, stale, at) {
		t.Fatalf("expected a code two steps old to be rejected")
	}
}

func TestValidateRejectsMalformedCodes(t *testing.T) {
	at := time.Unix(1234567890, 0)

	for _, code := range []string{"", "12345", "1234567", "abcdef", "12 456"} {
		if Validate(rfcSecret, code, at) {
			t.Fatalf("expected %q to be rejected", code)
		}
	}
}

func TestGenerateSecretRoundTrip(t *testing.T) {
	secret, err := GenerateSecret()
	if err != nil {
		t.Fatalf("GenerateSecret: %v", err)
	}
	if len(secret) != 32 {
		t.Fatalf("expected a 32-character Base32 secret, got %q", secret)
	}

	code, err := Code(secret, time.Now())
	if err != nil {
		t.Fatalf("Code: %v", err)
	}
	if len(code) != 6 || strings.Trim(code, "0123456789") != "" {
		t.Fatalf("unexpected code %q", code)
	}
}

func TestProvisioningURI(t *testing.T) {
	uri := ProvisioningURI("Emo Registry", "alice@example.com", rfcSecret)

	for _, want := range []string{
		"otpauth://totp/Emo%20Registry:alice@example.com?",
		"secret=" + rfcSecret,
		"issuer=Emo+Registry",
		"algorithm=SHA1",
		"digits=6",
		"period=30",
	} {
		if !strings.Contains(uri, want) {
			t.Fatalf("expected %q in %q", want, uri)
		}
	}
}
