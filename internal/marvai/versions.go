package marvai

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/afero"
)

// checkLocalPromptInstallation checks if a prompt is locally installed and compares versions
// Returns: isInstalled, isUpToDate, localVersion
func checkLocalPromptInstallation(fs afero.Fs, promptName, remoteVersion string) (bool, bool, string) {
	// Check if .mprompt file exists
	mpromptFile := filepath.Join(".marvai", promptName+".mprompt")
	if exists, err := afero.Exists(fs, mpromptFile); err != nil || !exists {
		return false, false, ""
	}

	// Get local version info
	_, _, _, localVersion := getInstalledMPromptInfo(fs, mpromptFile)

	// Compare versions
	isUpToDate := localVersion == remoteVersion

	return true, isUpToDate, localVersion
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