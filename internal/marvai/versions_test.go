package marvai

import (
	"testing"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		name         string
		version      string
		wantMajor    int
		wantMinor    int
		wantPatch    int
		wantPreRel   string
		wantError    bool
		errorMessage string
	}{
		// Valid semantic versions
		{
			name:       "basic semantic version",
			version:    "1.2.3",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "",
			wantError:  false,
		},
		{
			name:       "version with v prefix",
			version:    "v1.2.3",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "",
			wantError:  false,
		},
		{
			name:       "version with pre-release",
			version:    "1.2.3-beta",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "beta",
			wantError:  false,
		},
		{
			name:       "version with pre-release and v prefix",
			version:    "v1.2.3-alpha.1",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "alpha.1",
			wantError:  false,
		},
		{
			name:       "version with build metadata",
			version:    "1.2.3+build.1",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "",
			wantError:  false,
		},
		{
			name:       "version with pre-release and build metadata",
			version:    "1.2.3-beta+build.1",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "beta",
			wantError:  false,
		},
		{
			name:       "major version only",
			version:    "1",
			wantMajor:  1,
			wantMinor:  0,
			wantPatch:  0,
			wantPreRel: "",
			wantError:  false,
		},
		{
			name:       "major.minor version",
			version:    "1.2",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  0,
			wantPreRel: "",
			wantError:  false,
		},
		{
			name:       "zero versions",
			version:    "0.0.0",
			wantMajor:  0,
			wantMinor:  0,
			wantPatch:  0,
			wantPreRel: "",
			wantError:  false,
		},
		{
			name:       "large version numbers",
			version:    "10.20.30",
			wantMajor:  10,
			wantMinor:  20,
			wantPatch:  30,
			wantPreRel: "",
			wantError:  false,
		},
		{
			name:       "version with hyphen in pre-release",
			version:    "1.2.3-beta-1",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "beta-1",
			wantError:  false,
		},
		{
			name:       "version with dots in pre-release",
			version:    "1.2.3-beta.1.2",
			wantMajor:  1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "beta.1.2",
			wantError:  false,
		},

		// Invalid versions
		{
			name:         "empty version",
			version:      "",
			wantError:    true,
			errorMessage: "empty version string",
		},
		{
			name:         "invalid major version",
			version:      "a.1.2",
			wantError:    true,
			errorMessage: "invalid major version",
		},
		{
			name:         "invalid minor version",
			version:      "1.a.2",
			wantError:    true,
			errorMessage: "invalid minor version",
		},
		{
			name:         "invalid patch version",
			version:      "1.2.a",
			wantError:    true,
			errorMessage: "invalid patch version",
		},
		{
			name:         "no version numbers",
			version:      "abc",
			wantError:    true,
			errorMessage: "invalid major version",
		},
		{
			name:         "version with only dots",
			version:      "...",
			wantError:    true,
			errorMessage: "invalid major version",
		},
		{
			name:       "negative version",
			version:    "-1.2.3",
			wantMajor:  -1,
			wantMinor:  2,
			wantPatch:  3,
			wantPreRel: "",
			wantError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			major, minor, patch, preRel, err := parseVersion(tt.version)

			if tt.wantError {
				if err == nil {
					t.Errorf("parseVersion(%q) expected error containing %q, got nil", tt.version, tt.errorMessage)
				} else if tt.errorMessage != "" && !contains(err.Error(), tt.errorMessage) {
					t.Errorf("parseVersion(%q) expected error containing %q, got %q", tt.version, tt.errorMessage, err.Error())
				}
				return
			}

			if err != nil {
				t.Errorf("parseVersion(%q) unexpected error: %v", tt.version, err)
				return
			}

			if major != tt.wantMajor {
				t.Errorf("parseVersion(%q) major = %d, want %d", tt.version, major, tt.wantMajor)
			}
			if minor != tt.wantMinor {
				t.Errorf("parseVersion(%q) minor = %d, want %d", tt.version, minor, tt.wantMinor)
			}
			if patch != tt.wantPatch {
				t.Errorf("parseVersion(%q) patch = %d, want %d", tt.version, patch, tt.wantPatch)
			}
			if preRel != tt.wantPreRel {
				t.Errorf("parseVersion(%q) preRelease = %q, want %q", tt.version, preRel, tt.wantPreRel)
			}
		})
	}
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		v1       string
		v2       string
		expected int
	}{
		// Equal versions
		{"equal versions", "1.2.3", "1.2.3", 0},
		{"equal with v prefix", "v1.2.3", "1.2.3", 0},
		{"equal zero versions", "0.0.0", "0.0.0", 0},
		{"equal single digit", "1", "1.0.0", 0},
		{"equal two digit", "1.2", "1.2.0", 0},

		// Major version differences
		{"major version greater", "2.0.0", "1.9.9", 1},
		{"major version lesser", "1.0.0", "2.0.0", -1},
		{"major version zero vs one", "0.1.0", "1.0.0", -1},

		// Minor version differences
		{"minor version greater", "1.2.0", "1.1.9", 1},
		{"minor version lesser", "1.1.0", "1.2.0", -1},
		{"minor version zero vs one", "1.0.0", "1.1.0", -1},

		// Patch version differences
		{"patch version greater", "1.2.3", "1.2.2", 1},
		{"patch version lesser", "1.2.2", "1.2.3", -1},
		{"patch version zero vs one", "1.2.0", "1.2.1", -1},

		// Pre-release versions
		{"release vs pre-release", "1.2.3", "1.2.3-beta", 1},
		{"pre-release vs release", "1.2.3-beta", "1.2.3", -1},
		{"pre-release vs pre-release alpha", "1.2.3-beta", "1.2.3-alpha", 1},
		{"pre-release vs pre-release beta", "1.2.3-alpha", "1.2.3-beta", -1},
		{"pre-release equal", "1.2.3-beta", "1.2.3-beta", 0},
		{"pre-release version numbers", "1.2.3-beta.1", "1.2.3-beta.2", -1},

		// Mixed scenarios
		{"different major with pre-release", "2.0.0-alpha", "1.9.9", 1},
		{"same base different pre-release", "1.2.3-alpha", "1.2.3-beta", -1},
		{"complex pre-release", "1.2.3-beta.1.2", "1.2.3-beta.1.3", -1},

		// Large version numbers
		{"large versions", "10.20.30", "10.20.29", 1},
		{"large major", "100.0.0", "99.99.99", 1},

		// Invalid versions (should fall back to string comparison)
		{"invalid v1", "invalid", "1.2.3", 1},
		{"invalid v2", "1.2.3", "invalid", -1},
		{"both invalid equal", "invalid", "invalid", 0},
		{"both invalid different", "invalid1", "invalid2", -1},

		// Edge cases
		{"empty strings", "", "", 0},
		{"one empty", "", "1.2.3", -1},
		{"other empty", "1.2.3", "", 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareVersions(tt.v1, tt.v2)
			if result != tt.expected {
				t.Errorf("compareVersions(%q, %q) = %d, want %d", tt.v1, tt.v2, result, tt.expected)
			}
		})
	}
}

func TestIsVersionUpToDate(t *testing.T) {
	tests := []struct {
		name          string
		localVersion  string
		remoteVersion string
		expected      bool
	}{
		// Up-to-date scenarios (local >= remote)
		{"exact match", "1.2.3", "1.2.3", true},
		{"local newer patch", "1.2.3", "1.2.2", true},
		{"local newer minor", "1.2.0", "1.1.9", true},
		{"local newer major", "2.0.0", "1.9.9", true},
		{"local release vs remote pre-release", "1.2.3", "1.2.3-beta", true},
		{"local newer with pre-release", "1.2.4-alpha", "1.2.3", true},
		{"same pre-release", "1.2.3-beta", "1.2.3-beta", true},
		{"local newer pre-release", "1.2.3-beta", "1.2.3-alpha", true},

		// Outdated scenarios (local < remote)
		{"local older patch", "1.2.2", "1.2.3", false},
		{"local older minor", "1.1.9", "1.2.0", false},
		{"local older major", "1.9.9", "2.0.0", false},
		{"local pre-release vs remote release", "1.2.3-beta", "1.2.3", false},
		{"local older with pre-release", "1.2.3", "1.2.4-alpha", false},
		{"local older pre-release", "1.2.3-alpha", "1.2.3-beta", false},
		{"local older pre-release version", "1.2.3-beta.1", "1.2.3-beta.2", false},

		// Edge cases
		{"empty local version", "", "1.2.3", false},
		{"empty remote version", "1.2.3", "", true},
		{"both empty", "", "", true},
		{"invalid local version", "invalid", "1.2.3", true},
		{"invalid remote version", "1.2.3", "invalid", false},
		{"both invalid same", "invalid", "invalid", true},
		{"both invalid different", "invalid1", "invalid2", false},

		// Version prefix handling
		{"v prefix local", "v1.2.3", "1.2.3", true},
		{"v prefix remote", "1.2.3", "v1.2.3", true},
		{"v prefix both", "v1.2.3", "v1.2.3", true},
		{"v prefix local newer", "v1.2.4", "1.2.3", true},
		{"v prefix local older", "v1.2.2", "1.2.3", false},

		// Complex scenarios
		{"major upgrade available", "1.9.9", "2.0.0", false},
		{"minor upgrade available", "1.2.9", "1.3.0", false},
		{"patch upgrade available", "1.2.3", "1.2.4", false},
		{"pre-release to release upgrade", "1.2.3-rc.1", "1.2.3", false},
		{"beta to stable upgrade", "1.2.3-beta", "1.2.3", false},
		{"alpha to beta upgrade", "1.2.3-alpha", "1.2.3-beta", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isVersionUpToDate(tt.localVersion, tt.remoteVersion)
			if result != tt.expected {
				t.Errorf("isVersionUpToDate(%q, %q) = %t, want %t", tt.localVersion, tt.remoteVersion, result, tt.expected)
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(substr) > 0 && findInString(s, substr)))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
