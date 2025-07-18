package marvai

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/afero"
)

// ListInstalledPrompts scans the .marvai directory for .mprompt files and displays them
func ListInstalledPrompts(fs afero.Fs) error {
	// Check if .marvai directory exists
	exists, err := afero.DirExists(fs, ".marvai")
	if err != nil {
		return fmt.Errorf("error checking .marvai directory: %w", err)
	}

	if !exists {
		fmt.Println("No .marvai directory found. Run 'install' command to install prompts first.")
		return nil
	}

	files, err := afero.ReadDir(fs, ".marvai")
	if err != nil {
		return fmt.Errorf("error reading .marvai directory: %w", err)
	}

	var promptFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".mprompt") {
			// Extract the name without .mprompt extension
			name := strings.TrimSuffix(file.Name(), ".mprompt")
			promptFiles = append(promptFiles, name)
		}
	}

	if len(promptFiles) == 0 {
		fmt.Println("No installed prompts found in .marvai directory")
		return nil
	}

	if len(promptFiles) == 1 {
		fmt.Printf("Found %d installed prompt(s):\n", len(promptFiles))
	} else {
		fmt.Printf("Found %d installed prompt(s):\n", len(promptFiles))
	}
	for _, name := range promptFiles {
		// Check if .var file exists to show configuration status
		varFile := filepath.Join(".marvai", name+".var")
		varExists, _ := afero.Exists(fs, varFile)

		// Get version information from the .mprompt file
		mpromptFile := filepath.Join(".marvai", name+".mprompt")
		promptName, description, author, version := getInstalledMPromptInfo(fs, mpromptFile)

		// Use frontmatter name if available, otherwise use filename
		displayName := promptName
		if displayName == "" {
			displayName = name
		}

		// Build the display line
		line := displayName

		if version != "" {
			line += fmt.Sprintf(" v%s", version)
		}

		if description != "" {
			line += fmt.Sprintf(" - %s", description)
		}

		if author != "" {
			line += fmt.Sprintf(" (by %s)", author)
		}

		if varExists {
			line += " (configured)"
		}

		fmt.Printf("  %s\n", line)
	}

	return nil
}
