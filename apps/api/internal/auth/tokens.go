package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
	"time"
)

type AccessClaims struct {
	UserID    string
	SessionID string
	ExpiresAt time.Time
}

func newOpaqueToken(prefix string) (string, error) {
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(random), nil
}

func tokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func signAccess(secret []byte, claims AccessClaims) string {
	payload := strings.Join([]string{claims.UserID, claims.SessionID, strconv.FormatInt(claims.ExpiresAt.Unix(), 10)}, "|")
	encoded := base64.RawURLEncoding.EncodeToString([]byte(payload))
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(encoded))
	return "nox_access_" + encoded + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func parseAccess(secret []byte, token string, now time.Time) (AccessClaims, error) {
	token = strings.TrimPrefix(token, "nox_access_")
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return AccessClaims{}, errors.New("invalid access token")
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return AccessClaims{}, errors.New("invalid access token")
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return AccessClaims{}, errors.New("invalid access token")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return AccessClaims{}, errors.New("invalid access token")
	}
	values := strings.Split(string(payload), "|")
	if len(values) != 3 {
		return AccessClaims{}, errors.New("invalid access token")
	}
	expiresUnix, err := strconv.ParseInt(values[2], 10, 64)
	if err != nil {
		return AccessClaims{}, errors.New("invalid access token")
	}
	claims := AccessClaims{UserID: values[0], SessionID: values[1], ExpiresAt: time.Unix(expiresUnix, 0).UTC()}
	if claims.UserID == "" || claims.SessionID == "" || !now.Before(claims.ExpiresAt) {
		return AccessClaims{}, errors.New("expired access token")
	}
	return claims, nil
}
