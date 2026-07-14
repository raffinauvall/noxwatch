package auth

import (
	"testing"
	"time"
)

func TestPasswordAndTokenSecurity(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	if !verifyPassword(hash, "correct horse battery staple") || verifyPassword(hash, "wrong password") {
		t.Fatal("password verification failed")
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	claims := AccessClaims{UserID: "user-1", SessionID: "session-1", ExpiresAt: now.Add(time.Minute)}
	token := signAccess([]byte("01234567890123456789012345678901"), claims)
	parsed, err := parseAccess([]byte("01234567890123456789012345678901"), token, now)
	if err != nil || parsed.UserID != claims.UserID {
		t.Fatalf("valid token rejected: %+v %v", parsed, err)
	}
	if _, err := parseAccess([]byte("wrong-secret-wrong-secret-wrong-se"), token, now); err == nil {
		t.Fatal("tampered token accepted")
	}
}

func TestRateLimiterResets(t *testing.T) {
	limiter := newRateLimiter(2, time.Minute)
	now := time.Unix(1_700_000_000, 0)
	limiter.now = func() time.Time { return now }
	if !limiter.Allow("ip") || !limiter.Allow("ip") || limiter.Allow("ip") {
		t.Fatal("limit not enforced")
	}
	now = now.Add(time.Minute)
	if !limiter.Allow("ip") {
		t.Fatal("window did not reset")
	}
}
