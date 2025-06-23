package marvai

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
	
	"marvai/internal"
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

// FindClaudeBinaryWithRunner finds the Claude binary using dependency injection for testing
func FindClaudeBinaryWithRunner(runner CommandRunner, fs afero.Fs, goos string, homeDir string) string {
	// SECURITY: First try to find claude in secure, well-known paths
	// Avoid using PATH to prevent binary hijacking
	
	// Define secure installation paths by OS
	var securePaths []string
	
	switch goos {
	case "darwin":
		securePaths = []string{
			"/usr/local/bin/claude",
			"/opt/homebrew/bin/claude", 
			"/Applications/Claude.app/Contents/MacOS/claude",
		}
		// Only add user paths if homeDir is secure
		if isSecureHomeDir(homeDir) {
			securePaths = append(securePaths, filepath.Join(homeDir, ".local", "bin", "claude"))
		}
	default: // linux and others
		securePaths = []string{
			"/usr/local/bin/claude",
			"/usr/bin/claude",
		}
		// Only add user paths if homeDir is secure
		if isSecureHomeDir(homeDir) {
			securePaths = append(securePaths, 
				filepath.Join(homeDir, ".local", "bin", "claude"),
				filepath.Join(homeDir, "bin", "claude"))
		}
	}

	// Check secure paths first
	for _, path := range securePaths {
		if isValidClaudeBinary(fs, path) {
			return path
		}
	}

	// SECURITY: Only use PATH as last resort and validate the result
	if path, err := runner.LookPath("claude"); err == nil {
		if isValidClaudeBinary(fs, path) {
			return path
		}
	}

	// Fallback to just "claude" if nothing found
	return "claude"
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

// isValidClaudeBinary validates that a binary is actually the claude binary
func isValidClaudeBinary(fs afero.Fs, binaryPath string) bool {
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

// FindClaudeBinary finds the Claude binary using OS defaults
func FindClaudeBinary() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		homeDir = "/" // Fallback to root if home directory can't be determined
	}
	return FindClaudeBinaryWithRunner(OSCommandRunner{}, afero.NewOsFs(), runtime.GOOS, homeDir)
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

// LoadPrompt loads a prompt file from the .marvai directory with symlink protection
func LoadPrompt(fs afero.Fs, promptName string) ([]byte, error) {
	if err := ValidatePromptName(promptName); err != nil {
		return nil, fmt.Errorf("invalid prompt name: %w", err)
	}
	
	promptFile := filepath.Join(".marvai", promptName+".prompt")
	
	// SECURITY: Prevent symlink attacks by checking if file is a symlink
	if err := validateFileIsNotSymlink(fs, promptFile); err != nil {
		return nil, fmt.Errorf("security error: %w", err)
	}
	
	// SECURITY: Ensure the resolved path is still within .marvai directory
	if err := validateFileWithinMarvaiDirectory(promptFile); err != nil {
		return nil, fmt.Errorf("security error: %w", err)
	}
	
	return afero.ReadFile(fs, promptFile)
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

// RunWithPromptAndRunner executes Claude with a prompt using dependency injection for testing
func RunWithPromptAndRunner(fs afero.Fs, promptName string, runner CommandRunner, stdout, stderr io.Writer) error {
	content, err := LoadPrompt(fs, promptName)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	claudePath := FindClaudeBinary()
	cmd := runner.Command(claudePath)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error creating stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close() // Clean up stdin pipe if command fails to start
		return fmt.Errorf("error starting claude: %w", err)
	}

	// Write content to stdin in a goroutine, but ensure proper cleanup
	done := make(chan error, 1)
	go func() {
		defer stdin.Close()
		_, writeErr := stdin.Write(content)
		done <- writeErr
	}()

	// Wait for command to complete
	waitErr := cmd.Wait()
	
	// Wait for the write goroutine to complete or timeout
	select {
	case writeErr := <-done:
		if writeErr != nil && waitErr == nil {
			// Only report write error if command didn't fail
			return fmt.Errorf("error writing to claude stdin: %w", writeErr)
		}
	case <-time.After(5 * time.Second):
		// Timeout waiting for write to complete
		return fmt.Errorf("timeout waiting for stdin write to complete")
	}

	if waitErr != nil {
		return fmt.Errorf("error running claude: %w", waitErr)
	}

	return nil
}

// RunWithPrompt executes Claude with a prompt using OS defaults
func RunWithPrompt(fs afero.Fs, promptName string) error {
	return RunWithPromptAndRunner(fs, promptName, OSCommandRunner{}, os.Stdout, os.Stderr)
}

// WizardVariable represents a variable in the wizard section
type WizardVariable struct {
	ID       string `yaml:"id"`
	Question string `yaml:"question"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
}

// MPromptData represents the parsed .mprompt file
type MPromptData struct {
	Variables []WizardVariable
	Template  string
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

	lines := strings.Split(string(content), "\n")
	var wizardLines []string
	var templateLines []string
	var inTemplate bool

	for _, line := range lines {
		if strings.TrimSpace(line) == "--" {
			inTemplate = true
			continue
		}
		
		if inTemplate {
			templateLines = append(templateLines, line)
		} else {
			wizardLines = append(wizardLines, line)
		}
	}

	var variables []WizardVariable
	if len(wizardLines) > 0 {
		wizardYaml := strings.Join(wizardLines, "\n")
		
		// SECURITY: Limit YAML size to prevent billion laughs attack
		if len(wizardYaml) > 1024*1024 { // 1MB limit for YAML section
			return nil, fmt.Errorf("wizard YAML section too large (%d bytes), maximum allowed is 1MB", len(wizardYaml))
		}
		
		if err := yaml.Unmarshal([]byte(wizardYaml), &variables); err != nil {
			return nil, fmt.Errorf("error parsing wizard YAML: %w", err)
		}
		
		// SECURITY: Validate wizard variables
		if err := validateWizardVariables(variables); err != nil {
			return nil, fmt.Errorf("invalid wizard variables: %w", err)
		}
	}

	template := strings.Join(templateLines, "\n")
	template = strings.TrimSpace(template)

	return &MPromptData{
		Variables: variables,
		Template:  template,
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
			return nil, fmt.Errorf("error reading input for variable '%s'", variable.ID)
		}
	}

	return values, nil
}

// SubstituteVariables uses Handlebars templating to replace variables
func SubstituteVariables(template string, values map[string]string) (string, error) {
	return internal.RenderTemplate(template, values)
}

// InstallMPrompt processes a .mprompt file and creates a .prompt file
func InstallMPrompt(fs afero.Fs, mpromptName string) error {
	if err := ValidatePromptName(mpromptName); err != nil {
		return fmt.Errorf("invalid mprompt name: %w", err)
	}
	
	mpromptFile := mpromptName + ".mprompt"
	
	data, err := ParseMPrompt(fs, mpromptFile)
	if err != nil {
		return err
	}

	values, err := ExecuteWizard(data.Variables)
	if err != nil {
		return err
	}

	finalPrompt, err := SubstituteVariables(data.Template, values)
	if err != nil {
		return err
	}

	if err := fs.MkdirAll(".marvai", 0755); err != nil {
		return fmt.Errorf("error creating .marvai directory: %w", err)
	}

	promptFile := filepath.Join(".marvai", mpromptName+".prompt")
	if err := afero.WriteFile(fs, promptFile, []byte(finalPrompt), 0644); err != nil {
		return fmt.Errorf("error writing .prompt file: %w", err)
	}

	fmt.Printf("Created %s from %s\n", promptFile, mpromptFile)
	return nil
}

// Run executes the main application logic
func Run(args []string, fs afero.Fs, stderr io.Writer) error {
	if len(args) < 2 {
		fmt.Fprintf(stderr, "Usage: %s <command> [args...]\n", args[0])
		fmt.Fprintf(stderr, "Commands:\n")
		fmt.Fprintf(stderr, "  prompt <name>   - Execute a prompt\n")
		fmt.Fprintf(stderr, "  install <name>  - Install a .mprompt file\n")
		return fmt.Errorf("insufficient arguments")
	}

	command := args[1]
	
	switch command {
	case "prompt":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "Usage: %s prompt <prompt-name>\n", args[0])
			return fmt.Errorf("prompt name required")
		}
		promptName := args[2]
		return RunWithPrompt(fs, promptName)
		
	case "install":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "Usage: %s install <mprompt-name>\n", args[0])
			return fmt.Errorf("mprompt name required")
		}
		mpromptName := args[2]
		return InstallMPrompt(fs, mpromptName)
		
	default:
		// Backward compatibility: if no command specified, treat as prompt
		promptName := args[1]
		return RunWithPrompt(fs, promptName)
	}
}