package marvai

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/afero"
)

// parseVersion parses a semantic version string and returns major, minor, patch, and pre-release info
func parseVersion(version string) (major, minor, patch int, preRelease string, err error) {
	if version == "" {
		return 0, 0, 0, "", fmt.Errorf("empty version string")
	}

	// Remove 'v' prefix if present
	version = strings.TrimPrefix(version, "v")

	// Regex pattern for semantic versioning: major.minor.patch[-prerelease][+buildmetadata]
	pattern := `^(\d+)\.(\d+)\.(\d+)(?:-([a-zA-Z0-9\-\.]+))?(?:\+([a-zA-Z0-9\-\.]+))?$`
	regex := regexp.MustCompile(pattern)
	
	matches := regex.FindStringSubmatch(version)
	if len(matches) < 4 {
		// Try simpler patterns
		if parts := strings.Split(version, "."); len(parts) >= 1 {
			major, err = strconv.Atoi(parts[0])
			if err != nil {
				return 0, 0, 0, "", fmt.Errorf("invalid major version: %s", parts[0])
			}
			if len(parts) >= 2 {
				minor, err = strconv.Atoi(parts[1])
				if err != nil {
					return 0, 0, 0, "", fmt.Errorf("invalid minor version: %s", parts[1])
				}
			}
			if len(parts) >= 3 {
				// Handle pre-release in patch version
				patchPart := parts[2]
				if hyphenIndex := strings.Index(patchPart, "-"); hyphenIndex >= 0 {
					preRelease = patchPart[hyphenIndex+1:]
					patchPart = patchPart[:hyphenIndex]
				}
				patch, err = strconv.Atoi(patchPart)
				if err != nil {
					return 0, 0, 0, "", fmt.Errorf("invalid patch version: %s", patchPart)
				}
			}
			return major, minor, patch, preRelease, nil
		}
		return 0, 0, 0, "", fmt.Errorf("invalid version format: %s", version)
	}

	major, err = strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid major version: %s", matches[1])
	}

	minor, err = strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid minor version: %s", matches[2])
	}

	patch, err = strconv.Atoi(matches[3])
	if err != nil {
		return 0, 0, 0, "", fmt.Errorf("invalid patch version: %s", matches[3])
	}

	if len(matches) > 4 {
		preRelease = matches[4]
	}

	return major, minor, patch, preRelease, nil
}

// compareVersions compares two semantic version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	if v1 == v2 {
		return 0
	}

	major1, minor1, patch1, preRelease1, err1 := parseVersion(v1)
	major2, minor2, patch2, preRelease2, err2 := parseVersion(v2)

	// If either version is invalid, fall back to string comparison
	if err1 != nil || err2 != nil {
		if v1 < v2 {
			return -1
		} else if v1 > v2 {
			return 1
		}
		return 0
	}

	// Compare major version
	if major1 != major2 {
		if major1 < major2 {
			return -1
		}
		return 1
	}

	// Compare minor version
	if minor1 != minor2 {
		if minor1 < minor2 {
			return -1
		}
		return 1
	}

	// Compare patch version
	if patch1 != patch2 {
		if patch1 < patch2 {
			return -1
		}
		return 1
	}

	// Compare pre-release versions
	// Pre-release versions have lower precedence than normal versions
	if preRelease1 == "" && preRelease2 != "" {
		return 1 // v1 is a release, v2 is pre-release
	}
	if preRelease1 != "" && preRelease2 == "" {
		return -1 // v1 is pre-release, v2 is a release
	}
	if preRelease1 != "" && preRelease2 != "" {
		// Both are pre-releases, compare lexicographically
		if preRelease1 < preRelease2 {
			return -1
		} else if preRelease1 > preRelease2 {
			return 1
		}
	}

	return 0
}

// isVersionUpToDate checks if localVersion is up-to-date compared to remoteVersion
// Returns true if local version is >= remote version
func isVersionUpToDate(localVersion, remoteVersion string) bool {
	comparison := compareVersions(localVersion, remoteVersion)
	return comparison >= 0
}

// checkLocalPromptInstallation checks if a prompt is locally installed and compares versions
// Returns: isInstalled, isUpToDate, localVersion
func checkLocalPromptInstallation(fs afero.Fs, promptName, remoteVersion string) (bool, bool, string) {
	// Check if .mprompt file exists
	mpromptFile := filepath.Join(".marvai", promptName+".mprompt")
	if exists, err := afero.Exists(fs, mpromptFile); err != nil || !exists {
		return false, false, ""
	}

	// Get local version info
	localVersion := getInstalledPromptVersion(fs, mpromptFile)

	// Compare versions using semantic version comparison
	isUpToDate := isVersionUpToDate(localVersion, remoteVersion)

	return true, isUpToDate, localVersion
}

// getInstalledPromptVersion extracts only the version from an installed .mprompt file
func getInstalledPromptVersion(fs afero.Fs, filename string) string {
	// Read file content directly since ParseMPrompt has security checks for path separators
	content, err := afero.ReadFile(fs, filename)
	if err != nil {
		return ""
	}

	data, err := ParseMPromptContent(content, filename)
	if err != nil {
		return ""
	}

	// Return version from frontmatter if available
	return data.Frontmatter.Version
}

// getInstalledMPromptInfo attempts to extract information from an installed .mprompt file including version
func getInstalledMPromptInfo(fs afero.Fs, filename string) (name, description, author, version string) {
	// Read file content directly since ParseMPrompt has security checks for path separators
	content, err := afero.ReadFile(fs, filename)
	if err != nil {
		return "", "", "", ""
	}

	data, err := ParseMPromptContent(content, filename)
	if err != nil {
		return "", "", "", ""
	}

	// Use frontmatter information if available
	if data.Frontmatter.Name != "" {
		name = data.Frontmatter.Name
	}
	if data.Frontmatter.Description != "" {
		description = data.Frontmatter.Description
	}
	if data.Frontmatter.Author != "" {
		author = data.Frontmatter.Author
	}
	if data.Frontmatter.Version != "" {
		version = data.Frontmatter.Version
	}

	// Fallback to old behavior if no frontmatter description
	if description == "" && len(data.Variables) > 0 {
		// Look for a description variable
		for _, variable := range data.Variables {
			if variable.ID == "description" {
				description = variable.Description
				break
			}
		}

		// Otherwise, show the first variable's description as a hint of what this prompt does
		if description == "" {
			description = fmt.Sprintf("Prompts for: %s", data.Variables[0].Description)
		}
	}

	return name, description, author, version
}

// ShowVersion displays the version information
func ShowVersion(fs afero.Fs, version string) error {
	fmt.Printf("marvai version %s\n", version)
	return nil
}