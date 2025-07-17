package marvai

import (
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

// UpdatePrompt checks for new version of a prompt and updates it safely
func UpdatePrompt(fs afero.Fs, promptName string) error {
	// Validate prompt name
	if err := ValidatePromptName(promptName); err != nil {
		return fmt.Errorf("invalid prompt name: %w", err)
	}
	
	// Check if prompt is installed
	mpromptFile := filepath.Join(".marvai", promptName+".mprompt")
	varFile := filepath.Join(".marvai", promptName+".var")
	
	mpromptExists, err := afero.Exists(fs, mpromptFile)
	if err != nil {
		return fmt.Errorf("error checking if prompt is installed: %w", err)
	}
	
	if !mpromptExists {
		return fmt.Errorf("prompt '%s' is not installed. Use 'marvai install %s' to install it first", promptName, promptName)
	}
	
	// Get current installed version
	currentVersion := getInstalledPromptVersion(fs, mpromptFile)
	
	fmt.Printf("Checking for updates to prompt '%s'...\n", promptName)
	
	// Fetch remote prompts to get latest version
	prompts, err := fetchRemotePrompts("")
	if err != nil {
		return fmt.Errorf("error fetching remote prompts: %w", err)
	}
	
	// Find the prompt entry
	promptEntry, err := findPromptByName(prompts, promptName)
	if err != nil {
		return fmt.Errorf("prompt '%s' not found in remote registry: %w", promptName, err)
	}
	
	// Compare versions
	if currentVersion != "" && isVersionUpToDate(currentVersion, promptEntry.Version) {
		fmt.Printf("Prompt '%s' is already up to date (v%s)\n", promptName, currentVersion)
		return nil
	}
	
	fmt.Printf("New version available: v%s", promptEntry.Version)
	if currentVersion != "" {
		fmt.Printf(" (current: v%s)", currentVersion)
	}
	fmt.Println()
	
	// Ask user for confirmation
	fmt.Printf("Do you want to update '%s' to version %s? (yes/no) ", promptName, promptEntry.Version)
	var response string
	fmt.Scanln(&response)
	
	if strings.ToLower(strings.TrimSpace(response)) != "yes" {
		fmt.Println("Update cancelled.")
		return nil
	}
	
	// Backup existing .var file
	var existingValues map[string]string
	varExists, err := afero.Exists(fs, varFile)
	if err != nil {
		return fmt.Errorf("error checking .var file: %w", err)
	}
	
	if varExists {
		existingValues, err = loadVarFile(fs, varFile)
		if err != nil {
			fmt.Printf("Warning: Could not load existing .var file: %v\n", err)
			existingValues = make(map[string]string)
		}
	} else {
		existingValues = make(map[string]string)
	}
	
	// Backup current .mprompt file
	backupMpromptFile := mpromptFile + ".backup"
	if err := copyFileAfero(fs, mpromptFile, backupMpromptFile); err != nil {
		return fmt.Errorf("error backing up .mprompt file: %w", err)
	}
	
	// Download new version
	promptURL := fmt.Sprintf("https://registry.marvai.dev/dist/marvai/%s", promptEntry.File)
	
	client := &http.Client{
		Timeout: 30 * time.Second,
	}
	
	resp, err := client.Get(promptURL)
	if err != nil {
		return fmt.Errorf("error downloading new version: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status code: %d", resp.StatusCode)
	}
	
	// Read new content
	const maxSize = 10 * 1024 * 1024 // 10MB limit
	limitReader := io.LimitReader(resp.Body, maxSize+1)
	newContent, err := io.ReadAll(limitReader)
	if err != nil {
		return fmt.Errorf("error reading new version: %w", err)
	}
	
	if len(newContent) > maxSize {
		return fmt.Errorf("new version too large (%d bytes)", len(newContent))
	}
	
	// Parse new content
	newData, err := ParseMPromptContent(newContent, fmt.Sprintf("remote-%s", promptName))
	if err != nil {
		return fmt.Errorf("error parsing new version: %w", err)
	}
	
	// Verify SHA256 hash
	if err := verifySHA256([]byte(newData.Template), promptEntry.SHA256); err != nil {
		return fmt.Errorf("SHA256 verification failed: %w", err)
	}
	
	// Install new version
	updatedContent, err := injectSourceIntoMPrompt(newContent, "distro")
	if err != nil {
		return fmt.Errorf("error injecting source: %w", err)
	}
	
	if err := afero.WriteFile(fs, mpromptFile, updatedContent, 0644); err != nil {
		// Restore backup on failure
		copyFileAfero(fs, backupMpromptFile, mpromptFile)
		fs.Remove(backupMpromptFile)
		return fmt.Errorf("error installing new version: %w", err)
	}
	
	// Run wizard with prefilled values if there are variables
	if len(newData.Variables) > 0 {
		fmt.Printf("\nRunning configuration wizard for updated prompt '%s'...\n", promptName)
		fmt.Println("You can press Enter to keep existing values or type new ones.")
		
		newValues, err := ExecuteWizardWithPrefills(newData.Variables, existingValues)
		if err != nil {
			fmt.Printf("Warning: Configuration wizard failed: %v\n", err)
			
			// Ask if user wants to rollback
			fmt.Print("Do you want to rollback to the previous version? (yes/no) ")
			var rollbackResponse string
			fmt.Scanln(&rollbackResponse)
			
			if strings.ToLower(strings.TrimSpace(rollbackResponse)) == "yes" {
				// Restore backup
				if err := copyFileAfero(fs, backupMpromptFile, mpromptFile); err != nil {
					fmt.Printf("Error: Could not rollback: %v\n", err)
				} else {
					fmt.Printf("Successfully rolled back prompt '%s' to previous version.\n", promptName)
				}
				fs.Remove(backupMpromptFile)
				return fmt.Errorf("update rolled back due to wizard failure")
			}
			
			// Keep new version but warn about configuration
			fmt.Printf("Prompt '%s' updated but may need manual configuration.\n", promptName)
		} else {
			// Save new configuration
			if err := saveVarFile(fs, varFile, newValues); err != nil {
				fmt.Printf("Warning: Could not save new configuration: %v\n", err)
			}
		}
	}
	
	// Clean up backup
	fs.Remove(backupMpromptFile)
	
	fmt.Printf("Successfully updated prompt '%s' to version %s\n", promptName, promptEntry.Version)
	return nil
}

// copyFileAfero copies a file using afero filesystem
func copyFileAfero(fs afero.Fs, src, dst string) error {
	srcFile, err := fs.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	
	dstFile, err := fs.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	
	_, err = io.Copy(dstFile, srcFile)
	return err
}

// loadVarFile loads variables from a .var file
func loadVarFile(fs afero.Fs, filePath string) (map[string]string, error) {
	content, err := afero.ReadFile(fs, filePath)
	if err != nil {
		return nil, err
	}
	
	var values map[string]string
	if err := yaml.Unmarshal(content, &values); err != nil {
		return nil, err
	}
	
	return values, nil
}

// saveVarFile saves variables to a .var file
func saveVarFile(fs afero.Fs, filePath string, values map[string]string) error {
	data, err := yaml.Marshal(values)
	if err != nil {
		return err
	}
	
	return afero.WriteFile(fs, filePath, data, 0644)
}