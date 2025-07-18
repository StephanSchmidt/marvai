package marvai

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

// ListRemotePrompts fetches and displays available prompts from the remote distro
func ListRemotePrompts(fs afero.Fs, repo string) error {
	// Fetch remote prompts
	prompts, err := fetchRemotePrompts(repo)
	if err != nil {
		// Exit immediately with the error message
		fmt.Printf("%s\n", err.Error())
		os.Exit(1)
	}

	if len(prompts) == 0 {
		fmt.Println("No remote prompts found")
		return nil
	}

	if len(prompts) == 1 {
		fmt.Printf("✨ Found %d prompt available:\n", len(prompts))
	} else {
		fmt.Printf("✨ Found %d prompts available:\n", len(prompts))
	}
	for _, entry := range prompts {
		// Check local installation status
		isInstalled, isUpToDate, _ := checkLocalPromptInstallation(fs, entry.Name, entry.Version)

		// Build the display line with prefix
		var prefix string
		if isInstalled {
			if isUpToDate {
				prefix = "* "
			} else {
				prefix = "+ "
			}
		}

		line := fmt.Sprintf("%s%s", prefix, entry.Name)

		if entry.Version != "" {
			line += fmt.Sprintf(" v%s", entry.Version)
		}

		if entry.Description != "" {
			line += fmt.Sprintf(" - %s", entry.Description)
		}

		if entry.Author != "" {
			line += fmt.Sprintf(" (by %s)", entry.Author)
		}

		line += fmt.Sprintf(" [%s]", entry.File)

		fmt.Println(line)
	}

	return nil
}

// fetchRemotePrompts fetches and parses the PROMPTS file from the remote distro
func fetchRemotePrompts(repoStr string) ([]PromptEntry, error) {

	var repo string
	if strings.TrimSpace(repoStr) == "" {
		repo = "marvai"
	} else {
		repo = repoStr
	}

	promptsURL := fmt.Sprintf("https://registry.marvai.dev/dist/%s/PROMPTS", repo)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make request to fetch prompts
	resp, err := client.Get(promptsURL)
	if err != nil {
		return nil, fmt.Errorf("repo %s can't be read from %s", repo, promptsURL)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repo %s can't be read, status code: %d", repo, resp.StatusCode)
	}

	// Read response with size limit
	const maxSize = 1024 * 1024 // 1MB limit for prompts list
	limitReader := io.LimitReader(resp.Body, maxSize+1)
	content, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("repo %s can't be read", repo)
	}

	// Check size limit
	if len(content) > maxSize {
		return nil, fmt.Errorf("repo %s can't be read", repo)
	}

	// Parse prompt entries separated by --
	promptsText := string(content)
	entryTexts := strings.Split(promptsText, "--")

	// Parse each entry as YAML
	var promptEntries []PromptEntry
	var skippedEntries int
	for i, entryText := range entryTexts {
		trimmed := strings.TrimSpace(entryText)
		if trimmed == "" {
			continue
		}

		var entry PromptEntry
		if err := yaml.Unmarshal([]byte(trimmed), &entry); err != nil {
			// Log warning for invalid entries but don't fail completely
			fmt.Printf("Warning: Failed to parse prompt entry %d: %v\n", i+1, err)
			skippedEntries++
			continue
		}

		// Validate required fields
		if entry.Name != "" && entry.File != "" {
			promptEntries = append(promptEntries, entry)
		} else {
			fmt.Printf("Warning: Prompt entry %d missing required fields (name: %q, file: %q)\n",
				i+1, entry.Name, entry.File)
			skippedEntries++
		}
	}

	if skippedEntries > 0 {
		fmt.Printf("Warning: Skipped %d invalid prompt entries\n", skippedEntries)
	}

	return promptEntries, nil
}
