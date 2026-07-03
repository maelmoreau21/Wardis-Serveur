package validation_test

import (
	"testing"
	"wardis-server/internal/validation"
)

func TestIsUUID(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"ca000000-0000-0000-0000-000000000001", true},
		{"00000000-0000-0000-0000-000000000000", true},
		{"invalid-uuid", false},
		{"ca000000-0000-0000-0000-00000000000g", false}, // non-hex
		{"", false},
	}

	for _, tc := range tests {
		got := validation.IsUUID(tc.input)
		if got != tc.expected {
			t.Errorf("IsUUID(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestIsEmail(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"admin@wardis.com", true},
		{"test.user+tag@domain.co.uk", true},
		{"invalidemail", false},
		{"@domain.com", false},
		{"user@", false},
		{"", false},
	}

	for _, tc := range tests {
		got := validation.IsEmail(tc.input)
		if got != tc.expected {
			t.Errorf("IsEmail(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestIsAlphanumeric(t *testing.T) {
	tests := []struct {
		input    string
		min, max int
		expected bool
	}{
		{"CARD123", 3, 10, true},
		{"CARD_123", 3, 10, true},
		{"CARD-123", 3, 10, true},
		{"CA", 3, 10, false}, // too short
		{"VERYLONGCARDNUMBER", 3, 10, false}, // too long
		{"CARD@123", 3, 10, false}, // invalid char
		{"", 0, 10, true},
	}

	for _, tc := range tests {
		got := validation.IsAlphanumeric(tc.input, tc.min, tc.max)
		if got != tc.expected {
			t.Errorf("IsAlphanumeric(%q, %d, %d) = %v; want %v", tc.input, tc.min, tc.max, got, tc.expected)
		}
	}
}

func TestIsRTSPURL(t *testing.T) {
	tests := []struct {
		input    string
		expected bool
	}{
		{"rtsp://example.com/live", true},
		{"rtsps://example.com:8554/live/stream1", true},
		{"rtsp://192.168.1.50/stream", true},
		{"http://example.com/live", false},
		{"rtsp://", false},
		{"", false},
	}

	for _, tc := range tests {
		got := validation.IsRTSPURL(tc.input)
		if got != tc.expected {
			t.Errorf("IsRTSPURL(%q) = %v; want %v", tc.input, got, tc.expected)
		}
	}
}

func TestIsName(t *testing.T) {
	tests := []struct {
		input    string
		min, max int
		expected bool
	}{
		{"Main Entrance", 3, 50, true},
		{"Server Room Door", 3, 50, true},
		{"<script>alert(1)</script>", 3, 50, false}, // XSS
		{"", 1, 10, false},
	}

	for _, tc := range tests {
		got := validation.IsName(tc.input, tc.min, tc.max)
		if got != tc.expected {
			t.Errorf("IsName(%q, %d, %d) = %v; want %v", tc.input, tc.min, tc.max, got, tc.expected)
		}
	}
}
