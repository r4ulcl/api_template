package utils

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"golang.org/x/crypto/bcrypt"
)

// HashPassword hashes a plaintext password using bcrypt.
//
// It returns the hashed password as a string and any error encountered
// during hashing.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)

	return string(bytes), err
}

// CheckPassword verifies whether a given plaintext password matches a stored
// hashed password.
//
// It returns nil if the passwords match, otherwise returns an error.
func CheckPassword(hashedPassword, password string) error {
	return bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
}

// GenerateJWT generates a signed JWT token containing a username and role.
//
// The token is signed using the provided secret key and has a validity period
// of 240 hours.
//
// Returns the generated JWT token as a string and an error if signing fails.
func GenerateJWT(username string, role string, secret string) (string, error) {
	claims := jwt.MapClaims{
		"username": username,
		"role":     role,
		"exp":      time.Now().Add(time.Hour * 240).Unix(), // 24-hour expiration
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	return token.SignedString([]byte(secret))
}

// ParseJWT validates and parses a JWT token using the given secret key.
//
// It checks for a valid signing method and returns the token claims as a `jwt.MapClaims`
// if valid. If the token is invalid, it returns an error.
func ParseJWT(tokenString, secret string) (jwt.MapClaims, error) {
	token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("invalid signing method")
		}

		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims, nil
	}

	return nil, errors.New("invalid token")
}

// ParseJWTNoExpiry parses a JWT token using the provided secret
// but **skips expiration validation**. It returns the token claims if valid.
func ParseJWTNoExpiry(tokenString, secret string) (map[string]interface{}, error) {
	// Parse without verifying exp claim
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Ensure the signing method is what we expect (HMAC)
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected signing method")
		}
		return []byte(secret), nil
	}, jwt.WithoutClaimsValidation()) // disables exp/nbf checks

	if err != nil {
		return nil, err
	}

	// Validate token signature
	if !token.Valid {
		return nil, errors.New("invalid token signature")
	}

	// Extract claims
	if claims, ok := token.Claims.(jwt.MapClaims); ok {
		return claims, nil
	}

	return nil, errors.New("invalid token claims")
}

// GenerateJWTNoExpiry creates a JWT token with no expiration time.
func GenerateJWTNoExpiry(claims map[string]interface{}, secret string) (string, error) {
	token := jwt.New(jwt.SigningMethodHS256)
	c := jwt.MapClaims{}
	for k, v := range claims {
		c[k] = v
	}
	// Optionally include iat for creation time
	c["iat"] = time.Now().Unix()

	token.Claims = c
	return token.SignedString([]byte(secret))
}

// GenerateTOTPSecret returns a randomly generated Base32-encoded string suitable for
// provisioning a TOTP authenticator. We use 160 bits of entropy (20 bytes) to align with
// RFC 4226 recommendations and strip padding for broader authenticator compatibility.
func GenerateTOTPSecret() (string, error) {
	const secretBytes = 20
	buf := make([]byte, secretBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	return strings.ToUpper(secret), nil
}

// ValidateTOTP validates a 6-digit Time-based One-Time Password against a shared secret.
// The secret must be a base32 encoded string. A +/-1 time-step window is permitted
// to account for minor clock skew between client and server.
func ValidateTOTP(secret, code string) bool {
	secret = strings.TrimSpace(secret)
	code = strings.TrimSpace(code)
	if secret == "" || code == "" {
		return false
	}

	key, err := decodeBase32(secret)
	if err != nil {
		return false
	}

	now := time.Now().UTC().Unix() / 30
	for _, offset := range []int64{0, -1, 1} {
		value := generateTOTP(key, now+offset)
		if subtle.ConstantTimeCompare([]byte(code), []byte(value)) == 1 {
			return true
		}
	}
	return false
}

func decodeBase32(secret string) ([]byte, error) {
	normalized := strings.ToUpper(strings.ReplaceAll(secret, " ", ""))
	// Try no padding first, then fallback to standard encoding with padding.
	decoder := base32.StdEncoding.WithPadding(base32.NoPadding)
	key, err := decoder.DecodeString(normalized)
	if err == nil {
		return key, nil
	}
	return base32.StdEncoding.DecodeString(normalized)
}

func generateTOTP(key []byte, counter int64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], uint64(counter))

	h := hmac.New(sha256.New, key)
	_, _ = h.Write(buf[:])
	sum := h.Sum(nil)
	if len(sum) < 20 {
		return ""
	}
	offset := sum[len(sum)-1] & 0x0F
	binCode := (int(sum[offset])&0x7f)<<24 |
		(int(sum[offset+1])&0xff)<<16 |
		(int(sum[offset+2])&0xff)<<8 |
		(int(sum[offset+3]) & 0xff)

	otp := binCode % 1000000
	return fmt.Sprintf("%06d", otp)
}
