package marvai

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/afero"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/marvai-dev/marvai/internal"
	"github.com/marvai-dev/marvai/internal/source"
)

// CommandRunner interface for abstracting command execution
type CommandRunner interface {
	Command(name string, arg ...string) *exec.Cmd
	LookPath(file string) (string, error)
}

// OSCommandRunner implements CommandRunner using real OS commands
type OSCommandRunner struct{}

func (o OSCommandRunner) Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

func (o OSCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

// FindCliBinaryWithRunner finds the specified CLI binary using dependency injection for testing
func FindCliBinaryWithRunner(cliTool string, runner CommandRunner, fs afero.Fs, goos string, homeDir string) string {
	// SECURITY: First try to find the CLI tool in secure, well-known paths
	// Avoid using PATH to prevent binary hijacking

	// Define secure installation paths by OS
	var securePaths []string

	switch goos {
	case "darwin":
		securePaths = []string{
			"/usr/local/bin/" + cliTool,
			"/opt/homebrew/bin/" + cliTool,
		}
		if cliTool == "claude" {
			securePaths = append(securePaths, "/Applications/Claude.app/Contents/MacOS/claude")
		}
		// Only add user paths if homeDir is secure
		if isSecureHomeDir(homeDir) {
			securePaths = append(securePaths, filepath.Join(homeDir, ".local", "bin", cliTool))
		}
	default: // linux and others
		securePaths = []string{
			"/usr/local/bin/" + cliTool,
			"/usr/bin/" + cliTool,
		}
		// Only add user paths if homeDir is secure
		if isSecureHomeDir(homeDir) {
			securePaths = append(securePaths,
				filepath.Join(homeDir, ".local", "bin", cliTool),
				filepath.Join(homeDir, "bin", cliTool))
		}
	}

	// Check secure paths first
	for _, path := range securePaths {
		if isValidCliBinary(fs, path) {
			return path
		}
	}

	// SECURITY: Only use PATH as last resort and validate the result
	if path, err := runner.LookPath(cliTool); err == nil {
		if isValidCliBinary(fs, path) {
			return path
		}
	}

	// Fallback to just the tool name if nothing found
	return cliTool
}

// isSecureHomeDir validates that the home directory is secure
func isSecureHomeDir(homeDir string) bool {
	if homeDir == "" || homeDir == "/" {
		return false
	}

	// SECURITY: Reject suspicious home directories
	suspiciousPaths := []string{"/tmp", "/var/tmp", "/dev/shm"}
	for _, suspicious := range suspiciousPaths {
		if strings.HasPrefix(homeDir, suspicious) {
			return false
		}
	}

	return true
}

// isValidCliBinary validates that a binary is actually a valid CLI tool binary
func isValidCliBinary(fs afero.Fs, binaryPath string) bool {
	// Check if file exists and is executable
	fileInfo, err := fs.Stat(binaryPath)
	if err != nil {
		return false
	}

	// SECURITY: Ensure it's a regular file (not a symlink or device)
	if !fileInfo.Mode().IsRegular() {
		return false
	}

	// SECURITY: Check file permissions (should be executable)
	if fileInfo.Mode().Perm()&0111 == 0 {
		return false
	}

	// SECURITY: Validate the binary path doesn't contain suspicious patterns
	cleanPath := filepath.Clean(binaryPath)
	if strings.Contains(cleanPath, "..") {
		return false
	}

	// SECURITY: Reject paths in commonly writable directories
	dangerousDirs := []string{"/tmp/", "/var/tmp/", "/dev/shm/"}
	for _, dangerous := range dangerousDirs {
		if strings.HasPrefix(cleanPath, dangerous) {
			return false
		}
	}

	return true
}

// FindCliBinary finds the specified CLI binary using OS defaults
func FindCliBinary(cliTool string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/" // Fallback to root if home directory can't be determined
	}
	return FindCliBinaryWithRunner(cliTool, OSCommandRunner{}, afero.NewOsFs(), runtime.GOOS, homeDir)
}

// FindClaudeBinary finds the Claude binary using OS defaults (for backward compatibility)
func FindClaudeBinary() string {
	return FindCliBinary("claude")
}

// ValidatePromptName validates that a prompt name is safe to use
func ValidatePromptName(promptName string) error {
	if promptName == "" {
		return fmt.Errorf("prompt name cannot be empty")
	}

	if len(promptName) > 100 {
		return fmt.Errorf("prompt name too long (max 100 characters)")
	}

	// Check for path traversal attempts
	if strings.Contains(promptName, "..") {
		return fmt.Errorf("prompt name cannot contain '..'")
	}

	if strings.Contains(promptName, "/") {
		return fmt.Errorf("prompt name cannot contain '/'")
	}

	if strings.Contains(promptName, "\\") {
		return fmt.Errorf("prompt name cannot contain '\\'")
	}

	// Check for control characters
	for _, r := range promptName {
		if r < 32 || r == 127 {
			return fmt.Errorf("prompt name cannot contain control characters")
		}
	}

	return nil
}

// LoadPrompt loads and templates a prompt from .mprompt and .var files in the .marvai directory
func LoadPrompt(fs afero.Fs, promptName string) ([]byte, error) {
	if err := ValidatePromptName(promptName); err != nil {
		return nil, fmt.Errorf("invalid prompt name: %w", err)
	}

	mpromptFile := filepath.Join(".marvai", promptName+".mprompt")
	varFile := filepath.Join(".marvai", promptName+".var")

	// SECURITY: Prevent symlink attacks by checking if files are symlinks
	if err := validateFileIsNotSymlink(fs, mpromptFile); err != nil {
		return nil, fmt.Errorf("security error: %w", err)
	}
	if err := validateFileIsNotSymlink(fs, varFile); err != nil {
		return nil, fmt.Errorf("security error: %w", err)
	}

	// SECURITY: Ensure the resolved paths are still within .marvai directory
	if err := validateFileWithinMarvaiDirectory(mpromptFile); err != nil {
		return nil, fmt.Errorf("security error: %w", err)
	}
	if err := validateFileWithinMarvaiDirectory(varFile); err != nil {
		return nil, fmt.Errorf("security error: %w", err)
	}

	// Load and parse the .mprompt file
	content, err := afero.ReadFile(fs, mpromptFile)
	if err != nil {
		return nil, fmt.Errorf("error reading .mprompt file: %w", err)
	}

	data, err := ParseMPromptContent(content, mpromptFile)
	if err != nil {
		return nil, fmt.Errorf("error parsing .mprompt file: %w", err)
	}

	// Load variables from .var file if it exists
	var values map[string]string
	if varContent, err := afero.ReadFile(fs, varFile); err == nil {
		if err := yaml.Unmarshal(varContent, &values); err != nil {
			return nil, fmt.Errorf("error parsing .var file: %w", err)
		}
	} else {
		// No .var file exists, use empty values
		values = make(map[string]string)
	}

	// Template the prompt with the variables
	finalPrompt, err := SubstituteVariables(data.Template, values)
	if err != nil {
		return nil, fmt.Errorf("error templating prompt: %w", err)
	}

	return []byte(finalPrompt), nil
}

// validateFileIsNotSymlink checks if a file is a symbolic link
func validateFileIsNotSymlink(fs afero.Fs, filePath string) error {
	// Check if the filesystem supports Lstat (for symlink detection)
	if lstater, ok := fs.(afero.Lstater); ok {
		fileInfo, lstatCalled, err := lstater.LstatIfPossible(filePath)
		if err != nil {
			// File doesn't exist, which is fine for validation
			return nil
		}
		// Only check for symlinks if lstat was actually called
		if lstatCalled && fileInfo.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("file %q is a symbolic link, which is not allowed for security reasons", filePath)
		}
	}
	return nil
}

// validateFileWithinMarvaiDirectory ensures the file path resolves within .marvai
func validateFileWithinMarvaiDirectory(filePath string) error {
	// Clean the path to resolve any .. or . components
	cleanPath := filepath.Clean(filePath)

	// Ensure the path starts with .marvai/
	if !strings.HasPrefix(cleanPath, ".marvai/") && cleanPath != ".marvai" {
		return fmt.Errorf("file path %q is outside the allowed .marvai directory", cleanPath)
	}

	// Additional check: ensure no directory traversal even after cleaning
	if strings.Contains(cleanPath, "..") {
		return fmt.Errorf("file path %q contains directory traversal sequences", cleanPath)
	}

	return nil
}

// RunWithPromptAndRunner executes the specified CLI tool with a prompt using dependency injection for testing
func RunWithPromptAndRunner(fs afero.Fs, promptName string, cliTool string, runner CommandRunner, stdout, stderr io.Writer) error {
	content, err := LoadPrompt(fs, promptName)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	cliPath := FindCliBinary(cliTool)

	var cmd *exec.Cmd
	if cliTool == "codex" {
		// For codex, pass the prompt as a command-line argument
		cmd = runner.Command(cliPath, string(content))
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		// For codex, just run the command directly since prompt is passed as argument
		return cmd.Run()
	} else {
		// For claude and gemini, use stdin
		cmd = runner.Command(cliPath)
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// For claude and gemini, use stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error creating stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close() // Clean up stdin pipe if command fails to start
		return fmt.Errorf("error starting %s: %w", cliTool, err)
	}

	// Write content to stdin in a goroutine with proper synchronization
	done := make(chan error, 1)
	go func() {
		defer stdin.Close()
		_, writeErr := stdin.Write(content)
		if writeErr == nil {
			// Send /exit command to terminate CLI tool after processing the prompt
			// Note: This works for Claude, other tools may need different exit commands
			if cliTool == "claude" {
				_, writeErr = stdin.Write([]byte("\n/exit\n"))
			} else {
				// For other tools, just close stdin to signal end of input
				// Individual tools may require different exit strategies
			}
		}
		done <- writeErr
	}()

	// Wait for both the write goroutine and command to complete
	var writeErr error
	select {
	case writeErr = <-done:
		// Write completed, now wait for command
	case <-time.After(10 * time.Second):
		// Timeout waiting for write to complete
		return fmt.Errorf("timeout waiting for stdin write to complete")
	}

	// Wait for command to complete
	waitErr := cmd.Wait()

	// Return appropriate error
	if writeErr != nil && waitErr == nil {
		return fmt.Errorf("error writing to %s stdin: %w", cliTool, writeErr)
	}

	if waitErr != nil {
		return fmt.Errorf("error running %s: %w", cliTool, waitErr)
	}

	return nil
}

// RunWithPrompt executes the specified CLI tool with a prompt using OS defaults
func RunWithPrompt(fs afero.Fs, promptName string, cliTool string) error {
	return RunWithPromptAndRunner(fs, promptName, cliTool, OSCommandRunner{}, os.Stdout, os.Stderr)
}

// WizardVariable represents a variable in the wizard section
type WizardVariable struct {
	ID       string `yaml:"id"`
	Question string `yaml:"question"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
}

// MPromptFrontmatter represents the frontmatter section of a .mprompt file
type MPromptFrontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	Version     string `yaml:"version"`
	File        string `yaml:"file,omitempty"`
	Source      string `yaml:"source,omitempty"`
}

// PromptEntry represents an entry in the PROMPTS manifest file
type PromptEntry struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Author      string `yaml:"author"`
	Version     string `yaml:"version"`
	File        string `yaml:"file"`
	SHA256      string `yaml:"sha256,omitempty"`
}

// MPromptData represents the parsed .mprompt file
type MPromptData struct {
	Frontmatter MPromptFrontmatter
	Variables   []WizardVariable
	Template    string
}

// ParseMPrompt parses a .mprompt file and separates wizard and template sections with security controls
func ParseMPrompt(fs afero.Fs, filename string) (*MPromptData, error) {
	// SECURITY: Validate filename to prevent path traversal
	if err := validateSafeFilename(filename); err != nil {
		return nil, fmt.Errorf("unsafe filename: %w", err)
	}

	content, err := afero.ReadFile(fs, filename)
	if err != nil {
		return nil, fmt.Errorf("error reading .mprompt file: %w", err)
	}

	// SECURITY: Limit file size to prevent memory exhaustion
	if len(content) > 10*1024*1024 { // 10MB limit
		return nil, fmt.Errorf("mprompt file too large (%d bytes), maximum allowed is 10MB", len(content))
	}

	return ParseMPromptContent(content, filename)
}

// ParseMPromptContent parses .mprompt content directly (for use with source handlers)
// Format: frontmatter -- wizard variables -- template
func ParseMPromptContent(content []byte, displayName string) (*MPromptData, error) {
	// SECURITY: Limit file size to prevent memory exhaustion
	if len(content) > 10*1024*1024 { // 10MB limit
		return nil, fmt.Errorf("mprompt content too large (%d bytes), maximum allowed is 10MB", len(content))
	}

	lines := strings.Split(string(content), "\n")
	var frontmatterLines []string
	var wizardLines []string
	var templateLines []string

	section := 0 // 0=frontmatter, 1=wizard, 2=template

	for _, line := range lines {
		if strings.TrimSpace(line) == "--" {
			section++
			continue
		}

		switch section {
		case 0:
			frontmatterLines = append(frontmatterLines, line)
		case 1:
			wizardLines = append(wizardLines, line)
		case 2:
			templateLines = append(templateLines, line)
		default:
			// More than 2 separators - treat as part of template
			templateLines = append(templateLines, line)
		}
	}

	// Parse frontmatter
	var frontmatter MPromptFrontmatter
	if len(frontmatterLines) > 0 {
		frontmatterYaml := strings.Join(frontmatterLines, "\n")

		// SECURITY: Limit YAML size to prevent billion laughs attack
		if len(frontmatterYaml) > 1024*1024 { // 1MB limit for frontmatter section
			return nil, fmt.Errorf("frontmatter YAML section too large (%d bytes), maximum allowed is 1MB", len(frontmatterYaml))
		}

		if err := yaml.Unmarshal([]byte(frontmatterYaml), &frontmatter); err != nil {
			return nil, fmt.Errorf("error parsing frontmatter YAML from %s: %w", displayName, err)
		}
	}

	// Parse wizard variables
	var variables []WizardVariable
	if len(wizardLines) > 0 {
		wizardYaml := strings.Join(wizardLines, "\n")

		// SECURITY: Limit YAML size to prevent billion laughs attack
		if len(wizardYaml) > 1024*1024 { // 1MB limit for YAML section
			return nil, fmt.Errorf("wizard YAML section too large (%d bytes), maximum allowed is 1MB", len(wizardYaml))
		}

		if err := yaml.Unmarshal([]byte(wizardYaml), &variables); err != nil {
			return nil, fmt.Errorf("error parsing wizard YAML from %s: %w", displayName, err)
		}

		// SECURITY: Validate wizard variables
		if err := validateWizardVariables(variables); err != nil {
			return nil, fmt.Errorf("invalid wizard variables in %s: %w", displayName, err)
		}
	}

	// Parse template
	template := strings.Join(templateLines, "\n")
	template = strings.TrimSpace(template)

	return &MPromptData{
		Frontmatter: frontmatter,
		Variables:   variables,
		Template:    template,
	}, nil
}

// validateSafeFilename ensures the filename is safe
func validateSafeFilename(filename string) error {
	// SECURITY: Prevent directory traversal
	if strings.Contains(filename, "..") {
		return fmt.Errorf("filename contains directory traversal: %q", filename)
	}

	if strings.Contains(filename, "/") {
		return fmt.Errorf("filename contains path separator: %q", filename)
	}

	if len(filename) > 255 {
		return fmt.Errorf("filename too long: %d characters", len(filename))
	}

	return nil
}

// verifySHA256 compares the SHA256 hash of content against an expected hash
func verifySHA256(content []byte, expectedHash string) error {
	if expectedHash == "" {
		return nil // No hash provided, skip verification
	}

	// Calculate actual SHA256 hash
	hasher := sha256.New()
	hasher.Write(content)
	actualHash := hex.EncodeToString(hasher.Sum(nil))

	// Compare hashes (case-insensitive)
	if !strings.EqualFold(actualHash, expectedHash) {
		return fmt.Errorf("SHA256 verification failed: expected %s, got %s", expectedHash, actualHash)
	}

	return nil
}

// validateWizardVariables validates wizard variable definitions for security
func validateWizardVariables(variables []WizardVariable) error {
	if len(variables) > 100 { // Reasonable limit
		return fmt.Errorf("too many wizard variables (%d), maximum allowed is 100", len(variables))
	}

	for i, variable := range variables {
		// SECURITY: Validate variable ID
		if !isValidVariableNameLocal(variable.ID) {
			return fmt.Errorf("variable %d has invalid ID: %q", i, variable.ID)
		}

		// SECURITY: Limit question length
		if len(variable.Question) > 1000 {
			return fmt.Errorf("variable %d question too long: %d characters", i, len(variable.Question))
		}

		// SECURITY: Validate variable type
		if variable.Type != "" && variable.Type != "string" {
			return fmt.Errorf("variable %d has unsupported type: %q", i, variable.Type)
		}
	}

	return nil
}

// isValidVariableNameLocal checks if a variable name is safe (local copy)
func isValidVariableNameLocal(name string) bool {
	if name == "" {
		return false
	}

	// SECURITY: Block dangerous variable names
	dangerousNames := []string{
		"__proto__", "constructor", "prototype", "toString", "valueOf",
	}

	for _, dangerous := range dangerousNames {
		if name == dangerous {
			return false
		}
	}

	// SECURITY: Only allow alphanumeric, underscore, and hyphen
	for _, r := range name {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '_' || r == '-') {
			return false
		}
	}

	return true
}

// ExecuteWizard prompts the user for variable values
func ExecuteWizard(variables []WizardVariable) (map[string]string, error) {
	return ExecuteWizardWithReader(variables, os.Stdin)
}

// ExecuteWizardWithReader prompts the user for variable values using the provided reader
func ExecuteWizardWithReader(variables []WizardVariable, reader io.Reader) (map[string]string, error) {
	if reader == nil {
		return nil, fmt.Errorf("reader cannot be nil")
	}

	values := make(map[string]string)
	scanner := bufio.NewScanner(reader)

	for _, variable := range variables {
		fmt.Print(variable.Question + " ")
		if scanner.Scan() {
			response := strings.TrimSpace(scanner.Text())
			if variable.Required && response == "" {
				return nil, fmt.Errorf("variable '%s' is required", variable.ID)
			}
			values[variable.ID] = response
		} else {
			if err := scanner.Err(); err != nil {
				return nil, fmt.Errorf("error reading input for variable '%s': %w", variable.ID, err)
			}
			// Handle EOF case - treat as empty input
			if variable.Required {
				return nil, fmt.Errorf("variable '%s' is required but EOF encountered", variable.ID)
			}
			values[variable.ID] = ""
		}
	}

	return values, nil
}

// SubstituteVariables uses Handlebars templating to replace variables
func SubstituteVariables(template string, values map[string]string) (string, error) {
	return internal.RenderTemplate(template, values)
}

// InstallMPrompt processes a .mprompt file from any supported source and copies it to .marvai with wizard answers
func InstallMPrompt(fs afero.Fs, mpromptSource string) error {
	// Create source manager
	sourceManager := source.NewSourceManager(fs)

	// Load content from source (could be file or HTTPS URL)
	content, displayName, err := sourceManager.LoadContent(mpromptSource)
	if err != nil {
		return fmt.Errorf("failed to load mprompt from source: %w", err)
	}

	// Parse the content to extract wizard variables
	data, err := ParseMPromptContent(content, displayName)
	if err != nil {
		return err
	}

	// Determine the prompt name for validation and output file
	promptName := mpromptSource
	if strings.HasPrefix(mpromptSource, "https://") {
		// For URLs, extract filename from path
		if strings.Contains(mpromptSource, "/") {
			parts := strings.Split(mpromptSource, "/")
			promptName = parts[len(parts)-1]
			// Handle empty filename (URL ends with /)
			if promptName == "" && len(parts) > 1 {
				promptName = parts[len(parts)-2]
			}
		} else {
			promptName = "downloaded-prompt"
		}
		// If still empty, use default, but keep filenames with extensions
		if promptName == "" {
			promptName = "downloaded-prompt"
		}
	}

	// Remove .mprompt extension if present for validation
	if strings.HasSuffix(promptName, ".mprompt") {
		promptName = strings.TrimSuffix(promptName, ".mprompt")
	}

	if err := ValidatePromptName(promptName); err != nil {
		return fmt.Errorf("invalid prompt name derived from source: %w", err)
	}

	// Check if both .mprompt and .var files already exist
	mpromptFile := filepath.Join(".marvai", promptName+".mprompt")
	varFile := filepath.Join(".marvai", promptName+".var")

	mpromptExists, err := afero.Exists(fs, mpromptFile)
	if err != nil {
		return fmt.Errorf("error checking if .mprompt file exists: %w", err)
	}

	varExists, err := afero.Exists(fs, varFile)
	if err != nil {
		return fmt.Errorf("error checking if .var file exists: %w", err)
	}

	if mpromptExists || varExists {
		if mpromptExists && varExists {
			fmt.Printf("Prompt '%s' is already installed (both .mprompt and .var files exist)\n", promptName)
		} else if mpromptExists {
			fmt.Printf("Prompt '%s' is already installed (.mprompt file exists)\n", promptName)
		} else {
			fmt.Printf("Prompt '%s' is already installed (.var file exists)\n", promptName)
		}
		return nil
	}

	// Create .marvai directory
	if err := fs.MkdirAll(".marvai", 0755); err != nil {
		return fmt.Errorf("error creating .marvai directory: %w", err)
	}

	// Determine source type and inject it into the content
	var sourceType string
	if strings.HasPrefix(mpromptSource, "https://github.com/") {
		sourceType = "github"
	} else if strings.HasPrefix(mpromptSource, "https://") {
		sourceType = "distro"
	} else {
		sourceType = "local"
	}

	// Inject source information into the content
	updatedContent, err := injectSourceIntoMPrompt(content, sourceType)
	if err != nil {
		return fmt.Errorf("error injecting source into .mprompt content: %w", err)
	}

	// Copy .mprompt file to .marvai directory
	if err := afero.WriteFile(fs, mpromptFile, updatedContent, 0644); err != nil {
		return fmt.Errorf("error writing .mprompt file: %w", err)
	}

	// Run wizard and save answers to .var file
	if len(data.Variables) > 0 {
		values, err := ExecuteWizard(data.Variables)
		if err != nil {
			return err
		}

		// Save wizard answers as YAML
		varData, err := yaml.Marshal(values)
		if err != nil {
			return fmt.Errorf("error marshaling wizard answers: %w", err)
		}

		if err := afero.WriteFile(fs, varFile, varData, 0644); err != nil {
			return fmt.Errorf("error writing .var file: %w", err)
		}
		fmt.Printf("Installed %s with variables saved to %s\n", mpromptFile, varFile)
	} else {
		fmt.Printf("Installed %s (no variables to configure)\n", mpromptFile)
	}

	return nil
}

// ListMPromptFiles scans the current directory for .mprompt files and displays them
func ListMPromptFiles(fs afero.Fs) error {
	files, err := afero.ReadDir(fs, ".")
	if err != nil {
		return fmt.Errorf("error reading current directory: %w", err)
	}

	var mpromptFiles []string
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".mprompt") {
			mpromptFiles = append(mpromptFiles, file.Name())
		}
	}

	if len(mpromptFiles) == 0 {
		fmt.Println("No .mprompt files found in current directory")
		return nil
	}

	fmt.Printf("Found %d .mprompt file(s):\n", len(mpromptFiles))
	for _, file := range mpromptFiles {
		// Extract the name without .mprompt extension for filename display
		filename := strings.TrimSuffix(file, ".mprompt")

		// Get frontmatter information
		name, description, author, version := getMPromptInfo(fs, file)

		// Use frontmatter name if available, otherwise use filename
		displayName := name
		if displayName == "" {
			displayName = filename
		}

		// Build the display line
		line := fmt.Sprintf("  %s", displayName)

		if version != "" {
			line += fmt.Sprintf(" v%s", version)
		}

		if description != "" {
			line += fmt.Sprintf(" - %s", description)
		}

		if author != "" {
			line += fmt.Sprintf(" (by %s)", author)
		}

		fmt.Println(line)
	}

	return nil
}

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

	fmt.Printf("Found %d installed prompt(s):\n", len(promptFiles))
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
		line := fmt.Sprintf("  %s", displayName)

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

		fmt.Println(line)
	}

	return nil
}

// getMPromptInfo attempts to extract information from the .mprompt file
func getMPromptInfo(fs afero.Fs, filename string) (name, description, author, version string) {
	data, err := ParseMPrompt(fs, filename)
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
				description = variable.Question
				break
			}
		}

		// Otherwise, show the first variable's question as a hint of what this prompt does
		if description == "" {
			description = fmt.Sprintf("Prompts for: %s", data.Variables[0].Question)
		}
	}

	return name, description, author, version
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
				description = variable.Question
				break
			}
		}

		// Otherwise, show the first variable's question as a hint of what this prompt does
		if description == "" {
			description = fmt.Sprintf("Prompts for: %s", data.Variables[0].Question)
		}
	}

	return name, description, author, version
}

// fetchRemotePrompts fetches and parses the PROMPTS file from the remote distro
func fetchRemotePrompts() ([]PromptEntry, error) {
	const promptsURL = "https://distro.marvai.dev/PROMPTS"

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make request to fetch prompts
	resp, err := client.Get(promptsURL)
	if err != nil {
		return nil, fmt.Errorf("error fetching remote prompts: %w", err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d when fetching remote prompts", resp.StatusCode)
	}

	// Read response with size limit
	const maxSize = 1024 * 1024 // 1MB limit for prompts list
	limitReader := io.LimitReader(resp.Body, maxSize+1)
	content, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("error reading remote prompts response: %w", err)
	}

	// Check size limit
	if len(content) > maxSize {
		return nil, fmt.Errorf("remote prompts list too large (%d bytes), maximum allowed is %d bytes", len(content), maxSize)
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

// findPromptByName searches for a prompt by name in the list of prompt entries
func findPromptByName(prompts []PromptEntry, name string) (PromptEntry, error) {
	name = strings.ToLower(strings.TrimSpace(name))

	for _, entry := range prompts {
		// Check exact name match
		if strings.ToLower(entry.Name) == name {
			return entry, nil
		}

		// Check if description contains the name
		if strings.Contains(strings.ToLower(entry.Description), name) {
			return entry, nil
		}
	}

	return PromptEntry{}, fmt.Errorf("prompt '%s' not found in remote prompts", name)
}

// injectFilenameIntoMPrompt adds the filename to the frontmatter of a .mprompt file content
func injectFilenameIntoMPrompt(content []byte, filename string) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	var result []string
	section := 0 // 0=frontmatter, 1=wizard, 2=template
	frontmatterLines := []string{}

	i := 0
	// Collect frontmatter lines
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "--" {
			section++
			break
		}
		frontmatterLines = append(frontmatterLines, line)
		i++
	}

	// Parse existing frontmatter
	var frontmatter MPromptFrontmatter
	if len(frontmatterLines) > 0 {
		frontmatterYaml := strings.Join(frontmatterLines, "\n")
		if frontmatterYaml != "" {
			if err := yaml.Unmarshal([]byte(frontmatterYaml), &frontmatter); err != nil {
				return nil, fmt.Errorf("error parsing frontmatter YAML: %w", err)
			}
		}
	}

	// Add the filename to the frontmatter
	frontmatter.File = filename

	// Marshal the updated frontmatter
	updatedFrontmatter, err := yaml.Marshal(&frontmatter)
	if err != nil {
		return nil, fmt.Errorf("error marshaling updated frontmatter: %w", err)
	}

	// Build the result
	result = append(result, strings.TrimSpace(string(updatedFrontmatter)))

	// Add the rest of the content (from the first -- separator onwards)
	for i < len(lines) {
		result = append(result, lines[i])
		i++
	}

	return []byte(strings.Join(result, "\n")), nil
}

// injectSourceIntoMPrompt adds the source field to the frontmatter of a .mprompt file content
func injectSourceIntoMPrompt(content []byte, sourceType string) ([]byte, error) {
	lines := strings.Split(string(content), "\n")
	var result []string
	section := 0 // 0=frontmatter, 1=wizard, 2=template
	frontmatterLines := []string{}

	i := 0
	// Collect frontmatter lines
	for i < len(lines) {
		line := lines[i]
		if strings.TrimSpace(line) == "--" {
			section++
			break
		}
		frontmatterLines = append(frontmatterLines, line)
		i++
	}

	// Parse existing frontmatter
	var frontmatter MPromptFrontmatter
	if len(frontmatterLines) > 0 {
		frontmatterYaml := strings.Join(frontmatterLines, "\n")
		if frontmatterYaml != "" {
			if err := yaml.Unmarshal([]byte(frontmatterYaml), &frontmatter); err != nil {
				return nil, fmt.Errorf("error parsing frontmatter YAML: %w", err)
			}
		}
	}

	// Add the source to the frontmatter
	frontmatter.Source = sourceType

	// Marshal the updated frontmatter
	updatedFrontmatter, err := yaml.Marshal(&frontmatter)
	if err != nil {
		return nil, fmt.Errorf("error marshaling updated frontmatter: %w", err)
	}

	// Build the result
	result = append(result, strings.TrimSpace(string(updatedFrontmatter)))

	// Add the rest of the content (from the first -- separator onwards)
	for i < len(lines) {
		result = append(result, lines[i])
		i++
	}

	return []byte(strings.Join(result, "\n")), nil
}

// InstallMPromptByName fetches the PROMPTS file, finds a prompt by name, and installs it
func InstallMPromptByName(fs afero.Fs, promptName string) error {
	// Validate prompt name
	if err := ValidatePromptName(promptName); err != nil {
		return fmt.Errorf("invalid prompt name: %w", err)
	}

	// Fetch remote prompts
	prompts, err := fetchRemotePrompts()
	if err != nil {
		return fmt.Errorf("failed to fetch remote prompts: %w", err)
	}

	// Find the prompt entry by name
	promptEntry, err := findPromptByName(prompts, promptName)
	if err != nil {
		return err
	}

	// Download the actual .mprompt file
	promptURL := fmt.Sprintf("https://distro.marvai.dev/%s", promptEntry.File)

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Make request to fetch the .mprompt file
	resp, err := client.Get(promptURL)
	if err != nil {
		return fmt.Errorf("error downloading .mprompt file from %s: %w", promptURL, err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP error %d when downloading .mprompt file from %s", resp.StatusCode, promptURL)
	}

	// Read response with size limit
	const maxSize = 10 * 1024 * 1024 // 10MB limit for .mprompt files
	limitReader := io.LimitReader(resp.Body, maxSize+1)
	promptContent, err := io.ReadAll(limitReader)
	if err != nil {
		return fmt.Errorf("error reading .mprompt file response: %w", err)
	}

	// Check size limit
	if len(promptContent) > maxSize {
		return fmt.Errorf(".mprompt file too large (%d bytes), maximum allowed is %d bytes", len(promptContent), maxSize)
	}

	// Verify SHA256 hash if provided
	if err := verifySHA256(promptContent, promptEntry.SHA256); err != nil {
		return fmt.Errorf("SHA256 verification failed for %s: %w", promptURL, err)
	}

	// Parse the downloaded .mprompt file
	data, err := ParseMPromptContent(promptContent, fmt.Sprintf("remote-%s", promptName))
	if err != nil {
		return fmt.Errorf("failed to parse downloaded .mprompt file: %w", err)
	}

	// Use the frontmatter name if available, otherwise use the provided name
	finalName := promptName
	if data.Frontmatter.Name != "" {
		finalName = data.Frontmatter.Name
		// Validate the frontmatter name
		if err := ValidatePromptName(finalName); err != nil {
			// If frontmatter name is invalid, fall back to provided name
			finalName = promptName
		}
	}

	// Check if prompt is already installed
	mpromptFile := filepath.Join(".marvai", finalName+".mprompt")
	varFile := filepath.Join(".marvai", finalName+".var")

	mpromptExists, err := afero.Exists(fs, mpromptFile)
	if err != nil {
		return fmt.Errorf("error checking if .mprompt file exists: %w", err)
	}

	varExists, err := afero.Exists(fs, varFile)
	if err != nil {
		return fmt.Errorf("error checking if .var file exists: %w", err)
	}

	if mpromptExists || varExists {
		if mpromptExists && varExists {
			fmt.Printf("Prompt '%s' is already installed (both .mprompt and .var files exist)\n", finalName)
		} else if mpromptExists {
			fmt.Printf("Prompt '%s' is already installed (.mprompt file exists)\n", finalName)
		} else {
			fmt.Printf("Prompt '%s' is already installed (.var file exists)\n", finalName)
		}
		return nil
	}

	// Create .marvai directory
	if err := fs.MkdirAll(".marvai", 0755); err != nil {
		return fmt.Errorf("error creating .marvai directory: %w", err)
	}

	// Inject the filename from the PROMPTS file into the .mprompt content
	updatedContent, err := injectFilenameIntoMPrompt(promptContent, promptEntry.File)
	if err != nil {
		return fmt.Errorf("error injecting filename into .mprompt content: %w", err)
	}

	// Inject source information (distro for PROMPTS-based installs)
	updatedContent, err = injectSourceIntoMPrompt(updatedContent, "distro")
	if err != nil {
		return fmt.Errorf("error injecting source into .mprompt content: %w", err)
	}

	// Write .mprompt file with the updated content
	if err := afero.WriteFile(fs, mpromptFile, updatedContent, 0644); err != nil {
		return fmt.Errorf("error writing .mprompt file: %w", err)
	}

	// Run wizard and save answers to .var file
	if len(data.Variables) > 0 {
		values, err := ExecuteWizard(data.Variables)
		if err != nil {
			return err
		}

		// Save wizard answers as YAML
		varData, err := yaml.Marshal(values)
		if err != nil {
			return fmt.Errorf("error marshaling wizard answers: %w", err)
		}

		if err := afero.WriteFile(fs, varFile, varData, 0644); err != nil {
			return fmt.Errorf("error writing .var file: %w", err)
		}
		fmt.Printf("Installed %s with variables saved to %s\n", mpromptFile, varFile)
	} else {
		fmt.Printf("Installed %s (no variables to configure)\n", mpromptFile)
	}

	return nil
}

// ListRemotePrompts fetches and displays available prompts from the remote distro
func ListRemotePrompts(fs afero.Fs) error {
	// Fetch remote prompts
	prompts, err := fetchRemotePrompts()
	if err != nil {
		return err
	}

	if len(prompts) == 0 {
		fmt.Println("No remote prompts found")
		return nil
	}

	fmt.Printf("Found %d remote prompt(s):\n", len(prompts))
	for _, entry := range prompts {
		// Build the display line
		line := fmt.Sprintf("  %s", entry.Name)

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

// ShowVersion displays the version information
func ShowVersion(fs afero.Fs, version string) error {
	fmt.Printf("marvai version %s\n", version)
	return nil
}

// showWelcomeScreen displays a welcome message similar to Claude Code
func showWelcomeScreen(w io.Writer) {
	// ANSI color codes
	const (
		cyan   = "\033[36m"
		green  = "\033[32m"
		yellow = "\033[33m"
		reset  = "\033[0m"
		bold   = "\033[1m"
	)

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "unknown"
	}

	// Box width is 56 characters inside the borders
	const boxWidth = 56

	// Helper function to pad a line to exact width
	padLine := func(content string) string {
		if len(content) > boxWidth {
			return content[:boxWidth-3] + "..."
		}
		return content + strings.Repeat(" ", boxWidth-len(content))
	}

	// Define content lines
	line1 := " ✻ Welcome to Marvai!"
	line2 := "   Prompt templates for Claude Code & Gemini"
	line3 := "   Commands:"
	line4 := "     marvai install <source>  Install a prompt"
	line5 := "     marvai list              List available prompts"
	line6 := "     marvai prompt <name>     Execute a prompt"
	line7 := "     marvai --cli gemini <cmd>  Use Gemini instead"
	line8 := "   cwd: " + cwd

	fmt.Fprintf(w, "%s╭────────────────────────────────────────────────────────╮%s\n", cyan, reset)
	fmt.Fprintf(w, "%s│%s %s✻ Welcome to Marvai!%s%s%s│%s\n", cyan, reset, bold+green, reset, strings.Repeat(" ", boxWidth-len(line1)+2), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(""), cyan, reset)
	fmt.Fprintf(w, "%s│%s   %sPrompt templates for Claude Code & Gemini%s%s%s│%s\n", cyan, reset, yellow, reset, strings.Repeat(" ", boxWidth-len(line2)), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(""), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(line3), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(line4), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(line5), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(line6), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(line7), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(""), cyan, reset)
	fmt.Fprintf(w, "%s│%s%s%s│%s\n", cyan, reset, padLine(line8), cyan, reset)
	fmt.Fprintf(w, "%s╰────────────────────────────────────────────────────────╯%s\n", cyan, reset)
}

// Run executes the main application logic using Cobra for command-line parsing
func Run(args []string, fs afero.Fs, stderr io.Writer, version string) error {
	var cliTool string

	// Create root command
	rootCmd := &cobra.Command{
		Use:   "marvai",
		Short: "Prompt templates for Claude Code and other AI CLI tools",
		Long:  "Marvai is a CLI tool for managing and executing prompt templates with Claude Code, Gemini, and other AI CLI tools.",
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) == 0 {
				showWelcomeScreen(stderr)
				return
			}
			// Backward compatibility: if no subcommand specified, treat first arg as prompt name
			promptName := args[0]
			if err := RunWithPrompt(fs, promptName, cliTool); err != nil {
				fmt.Fprintf(stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	// Add global flag for CLI tool selection
	rootCmd.PersistentFlags().StringVar(&cliTool, "cli", "claude", "CLI tool to use (claude, gemini, codex)")

	// Add validation for CLI tool
	rootCmd.PersistentPreRunE = func(cmd *cobra.Command, args []string) error {
		if cliTool != "claude" && cliTool != "gemini" && cliTool != "codex" {
			return fmt.Errorf("invalid CLI tool '%s'. Available tools: claude, gemini, codex", cliTool)
		}
		return nil
	}

	// Create prompt command
	promptCmd := &cobra.Command{
		Use:   "prompt <prompt-name>",
		Short: "Execute a prompt template",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return RunWithPrompt(fs, args[0], cliTool)
		},
	}

	// Create install command
	installCmd := &cobra.Command{
		Use:   "install <source>",
		Short: "Install a prompt from a source",
		Long:  "Install a prompt from a local file, HTTPS URL, or prompt name from remote distro",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			mpromptSource := args[0]

			// Check if this looks like a URL or local file
			if strings.HasPrefix(mpromptSource, "https://") || strings.HasSuffix(mpromptSource, ".mprompt") || strings.Contains(mpromptSource, "/") {
				// Install from URL or local file
				return InstallMPrompt(fs, mpromptSource)
			} else {
				// Install by name from remote distro
				return InstallMPromptByName(fs, mpromptSource)
			}
		},
	}

	// Create list command
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available remote prompts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListRemotePrompts(fs)
		},
	}

	// Create list-local command
	listLocalCmd := &cobra.Command{
		Use:   "list-local",
		Short: "List local .mprompt files",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListMPromptFiles(fs)
		},
	}

	// Create installed command
	installedCmd := &cobra.Command{
		Use:   "installed",
		Short: "List installed prompts",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ListInstalledPrompts(fs)
		},
	}

	// Create create command
	createCmd := &cobra.Command{
		Use:   "create <filename>",
		Short: "Create a new .mprompt file",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return CreateMPrompt(fs, args[0])
		},
	}

	// Create version command
	versionCmd := &cobra.Command{
		Use:   "version",
		Short: "Show version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return ShowVersion(fs, version)
		},
	}

	// Add all commands to root
	rootCmd.AddCommand(promptCmd, installCmd, listCmd, listLocalCmd, installedCmd, createCmd, versionCmd)

	// Set up command line arguments
	rootCmd.SetArgs(args[1:]) // Skip program name
	rootCmd.SetErr(stderr)

	// Execute the command
	return rootCmd.Execute()
}

// CreateMPrompt creates a new .mprompt file with wizard-driven frontmatter collection
func CreateMPrompt(fs afero.Fs, filename string) error {
	// Check if user provided .mprompt extension (user error)
	if strings.HasSuffix(filename, ".mprompt") {
		return fmt.Errorf("filename should not include .mprompt extension, this is probably not what you wanted")
	}
	// Add .mprompt extension
	filename += ".mprompt"

	// Check if file already exists
	if exists, err := afero.Exists(fs, filename); err != nil {
		return fmt.Errorf("failed to check if file exists: %w", err)
	} else if exists {
		return fmt.Errorf("file %s already exists", filename)
	}

	fmt.Printf("Creating new mprompt file: %s\n\n", filename)

	// Collect frontmatter through wizard
	frontmatter := map[string]interface{}{}

	// Name
	fmt.Print("Enter prompt name: ")
	name, err := readUserInput()
	if err != nil {
		return fmt.Errorf("failed to read name: %w", err)
	}
	frontmatter["name"] = name

	// Description
	fmt.Print("Enter prompt description: ")
	description, err := readUserInput()
	if err != nil {
		return fmt.Errorf("failed to read description: %w", err)
	}
	frontmatter["description"] = description

	// Author
	fmt.Print("Enter author name: ")
	author, err := readUserInput()
	if err != nil {
		return fmt.Errorf("failed to read author: %w", err)
	}
	frontmatter["author"] = author

	// Version
	fmt.Print("Enter version (default: 1.0): ")
	version, err := readUserInput()
	if err != nil {
		return fmt.Errorf("failed to read version: %w", err)
	}
	if version == "" {
		version = "1.0"
	}
	frontmatter["version"] = version

	// Convert frontmatter to YAML
	frontmatterYAML, err := yaml.Marshal(frontmatter)
	if err != nil {
		return fmt.Errorf("failed to marshal frontmatter: %w", err)
	}

	// Create the mprompt file content
	content := fmt.Sprintf("%s--\n\n--\nEnter your prompt template here\n", string(frontmatterYAML))

	// Write the file
	err = afero.WriteFile(fs, filename, []byte(content), 0644)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("\n✓ Created %s successfully!\n", filename)
	fmt.Printf("You can now edit the file to add your prompt template.\n")
	fmt.Printf("To add wizard variables, edit the middle section between the '--' separators.\n")

	return nil
}

// readUserInput reads a line of input from the user
func readUserInput() (string, error) {
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.TrimSpace(scanner.Text()), nil
	}
	return "", scanner.Err()
}
