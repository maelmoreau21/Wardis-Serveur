package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"math"
	"net/url"
	"time"
)

// GenerateSecret generates a random 20-byte base32 secret (160 bits)
func GenerateSecret() (string, error) {
	bytes := make([]byte, 20)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(bytes), nil
}

// GetProvisioningURI returns the otpauth URI for QR code generation
func GetProvisioningURI(username, secret string) string {
	return fmt.Sprintf("otpauth://totp/Wardis:%s?secret=%s&issuer=Wardis", url.PathEscape(username), secret)
}

// ValidateTOTP validates a 6-digit TOTP code against a base32 secret, allowing +/- 1 time interval tolerance
func ValidateTOTP(secret string, code string) bool {
	if len(code) != 6 {
		return false
	}

	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(secret)
	if err != nil {
		// Fallback to padded decoding
		key, err = base32.StdEncoding.DecodeString(secret)
		if err != nil {
			return false
		}
	}

	currentTime := time.Now().Unix()
	interval := currentTime / 30

	// Check current, previous, and next interval for network/clock drift
	for i := int64(-1); i <= 1; i++ {
		counter := uint64(interval + i)
		expectedCode := fmt.Sprintf("%06d", generateHOTP(key, counter, 6))
		if expectedCode == code {
			return true
		}
	}

	return false
}

// generateHOTP generates an HOTP value for a given key and counter
func generateHOTP(key []byte, counter uint64, digits int) int {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, counter)

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	// Dynamic truncation
	offset := sum[len(sum)-1] & 0xf
	binaryVal := int64(sum[offset]&0x7f)<<24 |
		int64(sum[offset+1]&0xff)<<16 |
		int64(sum[offset+2]&0xff)<<8 |
		int64(sum[offset+3]&0xff)

	otp := binaryVal % int64(math.Pow10(digits))
	return int(otp)
}
