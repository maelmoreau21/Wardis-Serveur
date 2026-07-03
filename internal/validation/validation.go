package validation

import (
	"net/mail"
	"net/url"
	"regexp"
	"strings"
)

var (
	uuidRegex   = regexp.MustCompile(`^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$`)
	alphanumRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	rtspRegex   = regexp.MustCompile(`^rtsp(s)?://[a-zA-Z0-9.-]+(:[0-9]+)?(/.*)?$`)
)

// IsUUID checks if the given string is a valid UUIDv4
func IsUUID(s string) bool {
	return uuidRegex.MatchString(s)
}

// IsEmail checks if the email is structured correctly
func IsEmail(email string) bool {
	if email == "" || len(email) > 254 {
		return false
	}
	_, err := mail.ParseAddress(email)
	return err == nil
}

// IsAlphanumeric checks if the string contains only alphanumeric characters, underscores or dashes, and is within bounds
func IsAlphanumeric(s string, min, max int) bool {
	length := len(s)
	if length < min || length > max {
		return false
	}
	if length == 0 && min == 0 {
		return true
	}
	return alphanumRegex.MatchString(s)
}

// IsRTSPURL checks if the string is a valid RTSP stream URL
func IsRTSPURL(s string) bool {
	if s == "" || len(s) > 255 {
		return false
	}
	if !rtspRegex.MatchString(s) {
		return false
	}
	_, err := url.Parse(s)
	return err == nil
}

// IsName checks if a name is valid (non-empty, safe length, clean characters)
func IsName(s string, min, max int) bool {
	trimmed := strings.TrimSpace(s)
	length := len(trimmed)
	if length < min || length > max {
		return false
	}
	// Avoid basic control characters/XSS vectors
	return !strings.ContainsAny(trimmed, "<>;\"'`")
}
