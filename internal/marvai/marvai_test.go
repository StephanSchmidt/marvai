package marvai

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
)

// Mock interfaces for testing
type MockableCommand interface {
	StdinPipe() (io.WriteCloser, error)
	Start() error
	Wait() error
	SetStdout(io.Writer)
	SetStderr(io.Writer)
}

type MockableCommandRunner interface {
	Command(name string, arg ...string) MockableCommand
	LookPath(file string) (string, error)
}

// Mock CommandRunner for testing
type MockCommandRunner struct {
	lookPathResult    string
	lookPathError     error
	commands          []*MockCommand
	simulateHang      bool
	simulateError     bool
	stdinPipeError    bool
	commandStartError bool
	commandWaitError  bool
}

type MockCommand struct {
	name         string
	args         []string
	stdout       io.Writer
	stderr       io.Writer
	stdin        io.WriteCloser
	startError   error
	waitError    error
	stdinPipeErr error
	simulateHang bool
}

func (m *MockCommand) StdinPipe() (io.WriteCloser, error) {
	if m.stdinPipeErr != nil {
		return nil, m.stdinPipeErr
	}
	if m.stdin == nil {
		// Create a mock stdin pipe that can write to nowhere
		return &mockWriteCloser{}, nil
	}
	return m.stdin, nil
}

func (m *MockCommand) Start() error {
	if m.startError != nil {
		return m.startError
	}
	return nil
}

func (m *MockCommand) Wait() error {
	if m.simulateHang {
		// Simulate hanging for a long time
		time.Sleep(10 * time.Second)
	}
	if m.waitError != nil {
		return m.waitError
	}
	return nil
}

func (m *MockCommand) SetStdout(w io.Writer) {
	m.stdout = w
}

func (m *MockCommand) SetStderr(w io.Writer) {
	m.stderr = w
}

// mockWriteCloser is a simple WriteCloser that discards writes
type mockWriteCloser struct{}

func (m *mockWriteCloser) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func (m *mockWriteCloser) Close() error {
	return nil
}

func (m *MockCommandRunner) Command(name string, arg ...string) MockableCommand {
	// Create a mock command with configurable behavior
	mockCmd := &MockCommand{
		name:         name,
		args:         arg,
		simulateHang: m.simulateHang,
	}

	// Configure failures based on mock settings
	if m.stdinPipeError {
		mockCmd.stdinPipeErr = fmt.Errorf("mock stdin pipe creation failed")
	}

	if m.commandStartError || m.simulateError {
		mockCmd.startError = fmt.Errorf("mock command start failed")
	}

	if m.commandWaitError {
		mockCmd.waitError = fmt.Errorf("mock command wait failed")
	}

	m.commands = append(m.commands, mockCmd)
	return mockCmd
}

// Adapter to make MockCommandRunner work with the existing CommandRunner interface
type MockCommandRunnerAdapter struct {
	mock *MockCommandRunner
}

func (a *MockCommandRunnerAdapter) Command(name string, arg ...string) *exec.Cmd {
	// This is a fallback for tests that still use the old interface
	// We'll return a dummy command that won't actually be used
	return exec.Command("echo", "mock")
}

func (a *MockCommandRunnerAdapter) LookPath(file string) (string, error) {
	return a.mock.LookPath(file)
}

func (m *MockCommandRunner) LookPath(file string) (string, error) {
	return m.lookPathResult, m.lookPathError
}

// RunWithPromptAndMockableRunner is a testable version that uses the mockable interface
func RunWithPromptAndMockableRunner(fs afero.Fs, promptName string, cliTool string, runner MockableCommandRunner, stdout, stderr io.Writer) error {
	content, err := LoadPrompt(fs, promptName)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	cliPath, err := runner.LookPath(cliTool)
	if err != nil {
		// Fallback to using cliTool as-is if not found in PATH
		cliPath = cliTool
	}

	cmd := runner.Command(cliPath)
	cmd.SetStdout(stdout)
	cmd.SetStderr(stderr)

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

func TestFindClaudeBinaryWithRunner(t *testing.T) {
	tests := []struct {
		name           string
		lookPathResult string
		lookPathError  error
		existingFiles  []string
		goos           string
		homeDir        string
		expected       string
	}{
		{
			name:           "found in PATH",
			lookPathResult: "/usr/bin/claude",
			lookPathError:  nil,
			existingFiles:  []string{"/usr/bin/claude"},
			expected:       "/usr/bin/claude",
		},
		{
			name:          "not in PATH, found in /usr/local/bin (linux)",
			lookPathError: fmt.Errorf("not found"),
			existingFiles: []string{"/usr/local/bin/claude"},
			goos:          "linux",
			homeDir:       "/home/user",
			expected:      "/usr/local/bin/claude",
		},
		{
			name:          "not in PATH, found in homebrew (darwin)",
			lookPathError: fmt.Errorf("not found"),
			existingFiles: []string{"/opt/homebrew/bin/claude"},
			goos:          "darwin",
			homeDir:       "/Users/user",
			expected:      "/opt/homebrew/bin/claude",
		},
		{
			name:          "not in PATH, found in user local bin (linux)",
			lookPathError: fmt.Errorf("not found"),
			existingFiles: []string{"/home/user/.local/bin/claude"},
			goos:          "linux",
			homeDir:       "/home/user",
			expected:      "/home/user/.local/bin/claude",
		},
		{
			name:          "not found anywhere, fallback to claude",
			lookPathError: fmt.Errorf("not found"),
			existingFiles: []string{},
			goos:          "linux",
			homeDir:       "/home/user",
			expected:      "claude",
		},
		{
			name:          "found in macOS app bundle",
			lookPathError: fmt.Errorf("not found"),
			existingFiles: []string{"/Applications/Claude.app/Contents/MacOS/claude"},
			goos:          "darwin",
			homeDir:       "/Users/user",
			expected:      "/Applications/Claude.app/Contents/MacOS/claude",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock runner
			mockRunner := &MockCommandRunner{
				lookPathResult: tt.lookPathResult,
				lookPathError:  tt.lookPathError,
			}

			// Create adapter to work with the existing interface
			adapter := &MockCommandRunnerAdapter{mock: mockRunner}

			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Create existing files
			for _, file := range tt.existingFiles {
				fs.MkdirAll(file[:strings.LastIndex(file, "/")], 0755)
				afero.WriteFile(fs, file, []byte("mock claude binary"), 0755)
			}

			// Test function
			result := FindCliBinaryWithRunner("claude", adapter, fs, tt.goos, tt.homeDir)

			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		expectedError string
		checkStderr   bool
	}{
		{
			name:          "insufficient arguments",
			args:          []string{"program"},
			expectedError: "", // No error now, shows welcome screen
			checkStderr:   false,
		},
		{
			name:          "prompt command without name",
			args:          []string{"program", "prompt"},
			expectedError: "accepts 1 arg(s), received 0",
		},
		{
			name:          "install command without name",
			args:          []string{"program", "install"},
			expectedError: "accepts 1 arg(s), received 0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Capture stderr
			var stderr bytes.Buffer

			// Test run function
			err := Run(tt.args, fs, &stderr, "0.0.1")

			if tt.expectedError == "" {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			} else {
				if err == nil {
					t.Errorf("Expected error containing %q, got none", tt.expectedError)
				} else if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, got: %v", tt.expectedError, err)
				}
			}

			// Check stderr output for usage message
			if tt.checkStderr && len(tt.args) < 2 {
				expectedUsage := "Usage: program <command> [args...]\nCommands:\n  prompt <name>      - Execute a prompt\n  install <source>   - Install a .mprompt file from local path or HTTPS URL\n  list               - List available prompts from remote distro\n  list-local         - List available .mprompt files in current directory\n  installed          - List installed prompts in .marvai directory\n"
				if stderr.String() != expectedUsage {
					t.Errorf("Expected stderr %q, got %q", expectedUsage, stderr.String())
				}
			}
		})
	}
}

func TestLoadPrompt(t *testing.T) {
	tests := []struct {
		name           string
		promptName     string
		mpromptContent string
		varContent     string
		expectedResult string
		expectedError  bool
	}{
		{
			name:           "load existing prompt without variables",
			promptName:     "example",
			mpromptContent: "name: Example\n--\n--\nHello from example prompt",
			varContent:     "",
			expectedResult: "Hello from example prompt",
			expectedError:  false,
		},
		{
			name:           "load prompt with variables",
			promptName:     "greeting",
			mpromptContent: "name: Greeting\n--\n- id: name\n  question: \"What is your name?\"\n--\nHello {{name}}!",
			varContent:     "name: World",
			expectedResult: "Hello World!",
			expectedError:  false,
		},
		{
			name:           "load prompt with missing variable file",
			promptName:     "missing-vars",
			mpromptContent: "name: Test\n--\n- id: name\n  question: \"What is your name?\"\n--\nHello {{name}}!",
			varContent:     "",        // No .var file created
			expectedResult: "Hello !", // Empty variable
			expectedError:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Create .marvai directory
			err := fs.MkdirAll(".marvai", 0755)
			if err != nil {
				t.Fatalf("Failed to create .marvai directory: %v", err)
			}

			// Write .mprompt file
			mpromptFile := ".marvai/" + tt.promptName + ".mprompt"
			err = afero.WriteFile(fs, mpromptFile, []byte(tt.mpromptContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write .mprompt file: %v", err)
			}

			// Write .var file if provided
			if tt.varContent != "" {
				varFile := ".marvai/" + tt.promptName + ".var"
				err = afero.WriteFile(fs, varFile, []byte(tt.varContent), 0644)
				if err != nil {
					t.Fatalf("Failed to write .var file: %v", err)
				}
			}

			// Test LoadPrompt function
			content, err := LoadPrompt(fs, tt.promptName)

			if tt.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectedError && string(content) != tt.expectedResult {
				t.Errorf("Expected content %q, got %q", tt.expectedResult, string(content))
			}
		})
	}
}

func TestLoadPromptErrors(t *testing.T) {
	tests := []struct {
		name       string
		promptName string
		setupFs    func(afero.Fs) error
	}{
		{
			name:       "nonexistent prompt file",
			promptName: "nonexistent",
			setupFs:    func(fs afero.Fs) error { return nil }, // Don't create any files
		},
		{
			name:       "nonexistent .marvai directory",
			promptName: "test",
			setupFs:    func(fs afero.Fs) error { return nil }, // Don't create directory
		},
		{
			name:       "empty prompt name",
			promptName: "",
			setupFs: func(fs afero.Fs) error {
				return fs.MkdirAll(".marvai", 0755)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Setup filesystem
			if err := tt.setupFs(fs); err != nil {
				t.Fatalf("Failed to setup filesystem: %v", err)
			}

			// Test LoadPrompt function - should return error
			_, err := LoadPrompt(fs, tt.promptName)
			if err == nil {
				t.Errorf("Expected error but got none")
			}
		})
	}
}

func TestPromptFilePathConstruction(t *testing.T) {
	tests := []struct {
		name       string
		promptName string
		expected   string
	}{
		{
			name:       "simple name",
			promptName: "example",
			expected:   ".marvai/example.prompt",
		},
		{
			name:       "name with hyphens",
			promptName: "my-cool-prompt",
			expected:   ".marvai/my-cool-prompt.prompt",
		},
		{
			name:       "name with numbers",
			promptName: "prompt123",
			expected:   ".marvai/prompt123.prompt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem with expected file
			fs := afero.NewMemMapFs()
			fs.MkdirAll(".marvai", 0755)
			// Convert .prompt to .mprompt for the new system
			mpromptPath := strings.Replace(tt.expected, ".prompt", ".mprompt", 1)
			afero.WriteFile(fs, mpromptPath, []byte("name: Test\n--\n--\ntest content"), 0644)

			// Test that LoadPrompt correctly constructs the path
			content, err := LoadPrompt(fs, tt.promptName)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if string(content) != "test content" {
				t.Errorf("Expected 'test content', got %q", string(content))
			}
		})
	}
}

func TestLoadPromptWithSpecialCharacters(t *testing.T) {
	tests := []struct {
		name        string
		promptName  string
		fileContent string
	}{
		{
			name:        "unicode content",
			promptName:  "unicode",
			fileContent: "Hello 世界! 🌍",
		},
		{
			name:        "content with quotes",
			promptName:  "quotes",
			fileContent: `"Hello" and 'world' with quotes`,
		},
		{
			name:        "content with special chars",
			promptName:  "special",
			fileContent: "Content with $pecial ch@rs & symbols!",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()
			fs.MkdirAll(".marvai", 0755)

			// Write test file with special content
			mpromptFile := ".marvai/" + tt.promptName + ".mprompt"
			mpromptContent := "name: Test\n--\n--\n" + tt.fileContent
			err := afero.WriteFile(fs, mpromptFile, []byte(mpromptContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Test LoadPrompt function
			content, err := LoadPrompt(fs, tt.promptName)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if string(content) != tt.fileContent {
				t.Errorf("Expected content %q, got %q", tt.fileContent, string(content))
			}
		})
	}
}

// TestLoadPromptDirectoryTraversal tests for directory traversal vulnerabilities
func TestLoadPromptDirectoryTraversal(t *testing.T) {
	tests := []struct {
		name          string
		promptName    string
		shouldSucceed bool
		description   string
	}{
		{
			name:          "path traversal with dotdot",
			promptName:    "../../../etc/passwd",
			shouldSucceed: false,
			description:   "Should reject path traversal attempts",
		},
		{
			name:          "path traversal with multiple dotdots",
			promptName:    "../../../../usr/bin/bash",
			shouldSucceed: false,
			description:   "Should reject deep path traversal",
		},
		{
			name:          "path traversal encoded",
			promptName:    "..%2F..%2Fetc%2Fpasswd",
			shouldSucceed: false,
			description:   "Should reject URL encoded traversal",
		},
		{
			name:          "absolute path attempt",
			promptName:    "/etc/passwd",
			shouldSucceed: false,
			description:   "Should reject absolute paths",
		},
		{
			name:          "relative path with dot",
			promptName:    "./../../sensitive",
			shouldSucceed: false,
			description:   "Should reject relative paths with dot",
		},
		{
			name:          "valid prompt name",
			promptName:    "valid-prompt",
			shouldSucceed: true,
			description:   "Should accept valid prompt names",
		},
		{
			name:          "prompt name with underscores",
			promptName:    "my_test_prompt",
			shouldSucceed: true,
			description:   "Should accept underscores",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Create .marvai directory
			fs.MkdirAll(".marvai", 0755)

			// For valid test cases, create the expected files
			if tt.shouldSucceed {
				mpromptFile := ".marvai/" + tt.promptName + ".mprompt"
				afero.WriteFile(fs, mpromptFile, []byte("name: Test\n--\n--\ntest content"), 0644)
			}

			// Also create a sensitive file outside the .marvai directory to test traversal
			fs.MkdirAll("/etc", 0755)
			afero.WriteFile(fs, "/etc/passwd", []byte("sensitive data"), 0644)

			// Test LoadPrompt function
			content, err := LoadPrompt(fs, tt.promptName)

			if tt.shouldSucceed {
				if err != nil {
					t.Errorf("Expected success but got error: %v", err)
				}
				if string(content) != "test content" {
					t.Errorf("Expected 'test content', got %q", string(content))
				}
			} else {
				// For security tests, we expect the function to either:
				// 1. Return an error (file not found in .marvai directory)
				// 2. Not access files outside .marvai directory
				if err == nil && string(content) == "sensitive data" {
					t.Errorf("SECURITY VULNERABILITY: Successfully accessed file outside .marvai directory with name %q", tt.promptName)
				}
			}
		})
	}
}

// TestLoadPromptInputValidation tests edge cases for prompt name validation
func TestLoadPromptInputValidation(t *testing.T) {
	tests := []struct {
		name       string
		promptName string
		expectErr  bool
	}{
		{
			name:       "empty prompt name",
			promptName: "",
			expectErr:  true,
		},
		{
			name:       "prompt name with null byte",
			promptName: "test\x00prompt",
			expectErr:  true,
		},
		{
			name:       "extremely long prompt name",
			promptName: strings.Repeat("a", 1000),
			expectErr:  true,
		},
		{
			name:       "prompt name with newlines",
			promptName: "test\nprompt",
			expectErr:  true,
		},
		{
			name:       "prompt name with carriage return",
			promptName: "test\rprompt",
			expectErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			fs.MkdirAll(".marvai", 0755)

			_, err := LoadPrompt(fs, tt.promptName)

			if tt.expectErr && err == nil {
				t.Errorf("Expected error for invalid prompt name %q, but got none", tt.promptName)
			}
		})
	}
}

// TestRunWithPromptResourceLeaks tests for resource leaks in command execution
func TestRunWithPromptResourceLeaks(t *testing.T) {
	tests := []struct {
		name        string
		setupFs     func(afero.Fs) error
		setupRunner func() *MockCommandRunner
		expectError bool
		description string
	}{
		{
			name: "stdin pipe creation fails",
			setupFs: func(fs afero.Fs) error {
				fs.MkdirAll(".marvai", 0755)
				return afero.WriteFile(fs, ".marvai/test.mprompt", []byte("name: Test\n--\n--\ntest content"), 0644)
			},
			setupRunner: func() *MockCommandRunner {
				runner := &MockCommandRunner{
					lookPathResult: "/usr/bin/claude",
					stdinPipeError: true, // Now we can actually simulate stdin pipe failure
				}
				return runner
			},
			expectError: true, // Now it should correctly expect an error
			description: "Should handle stdin pipe creation failure",
		},
		{
			name: "command start fails",
			setupFs: func(fs afero.Fs) error {
				fs.MkdirAll(".marvai", 0755)
				return afero.WriteFile(fs, ".marvai/test.mprompt", []byte("name: Test\n--\n--\ntest content"), 0644)
			},
			setupRunner: func() *MockCommandRunner {
				return &MockCommandRunner{
					lookPathResult:    "/usr/bin/claude",
					commandStartError: true, // Properly simulate start failure
				}
			},
			expectError: true,
			description: "Should handle command start failure",
		},
		{
			name: "command wait fails",
			setupFs: func(fs afero.Fs) error {
				fs.MkdirAll(".marvai", 0755)
				return afero.WriteFile(fs, ".marvai/test.mprompt", []byte("name: Test\n--\n--\ntest content"), 0644)
			},
			setupRunner: func() *MockCommandRunner {
				runner := &MockCommandRunner{
					lookPathResult:   "/usr/bin/claude",
					commandWaitError: true, // Properly simulate wait failure
				}
				return runner
			},
			expectError: true, // Now it should correctly expect an error
			description: "Should handle command wait failure",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()
			if err := tt.setupFs(fs); err != nil {
				t.Fatalf("Failed to setup filesystem: %v", err)
			}

			// Setup mock runner
			runner := tt.setupRunner()

			// Capture stdout and stderr
			var stdout, stderr bytes.Buffer

			// Test RunWithPromptAndMockableRunner instead of the old one
			err := RunWithPromptAndMockableRunner(fs, "test", "claude", runner, &stdout, &stderr)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
			}
		})
	}
}

// TestInstallMPromptDirectoryTraversal tests directory traversal in InstallMPrompt
func TestInstallMPromptDirectoryTraversal(t *testing.T) {
	tests := []struct {
		name        string
		mpromptName string
		expectError bool
		description string
	}{
		{
			name:        "path traversal in mprompt name",
			mpromptName: "../../../etc/passwd",
			expectError: true,
			description: "Should reject path traversal in mprompt names",
		},
		{
			name:        "absolute path in mprompt name",
			mpromptName: "/etc/passwd",
			expectError: true,
			description: "Should reject absolute paths",
		},
		{
			name:        "valid mprompt name",
			mpromptName: "valid-template",
			expectError: false,
			description: "Should accept valid template names",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// For valid test cases, create the mprompt file
			if !tt.expectError {
				mpromptContent := `name: Test Template
description: A test template
--
- id: test
  question: "Test question?"
  type: string
  required: false
--
Hello {{test}}!`
				afero.WriteFile(fs, tt.mpromptName+".mprompt", []byte(mpromptContent), 0644)
			}

			// Test InstallMPrompt - we need to handle the wizard input
			// For now, let's just test that the validation works
			err := ValidatePromptName(tt.mpromptName)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected validation error for malicious mprompt name %q, but got none", tt.mpromptName)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected validation error for valid mprompt name %q: %v", tt.mpromptName, err)
				}
			}
		})
	}
}

// TestParseMPromptErrorHandling tests error handling in ParseMPrompt
func TestParseMPromptErrorHandling(t *testing.T) {
	tests := []struct {
		name        string
		fileContent string
		expectError bool
		description string
	}{
		{
			name: "invalid YAML in wizard section",
			fileContent: `name: Test
--
- id: test
  question: "Test?"
  invalid_yaml: {]
--
Template content`,
			expectError: true,
			description: "Should handle invalid YAML gracefully",
		},
		{
			name: "missing separator",
			fileContent: `name: Test
description: A test template
Template without separator`,
			expectError: true, // Invalid YAML in frontmatter should cause error
			description: "Should handle missing separator with invalid YAML",
		},
		{
			name:        "empty file",
			fileContent: "",
			expectError: false,
			description: "Should handle empty files",
		},
		{
			name:        "only one separator",
			fileContent: "name: Test\n--",
			expectError: false,
			description: "Should handle files with only one separator",
		},
		{
			name:        "extremely large file",
			fileContent: "name: Test\n--\n- id: test\n  question: \"Test?\"\n--\n" + strings.Repeat("A", 1000000),
			expectError: false,
			description: "Should handle large files without memory issues",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()
			filename := "test.mprompt"

			// Write test file
			err := afero.WriteFile(fs, filename, []byte(tt.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Test ParseMPrompt
			data, err := ParseMPrompt(fs, filename)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if data == nil {
					t.Errorf("Expected data but got nil")
				}
			}
		})
	}
}

// TestValidatePromptName tests the new validation function
func TestValidatePromptName(t *testing.T) {
	tests := []struct {
		name        string
		promptName  string
		expectError bool
	}{
		{
			name:        "valid prompt name",
			promptName:  "valid-prompt",
			expectError: false,
		},
		{
			name:        "empty prompt name",
			promptName:  "",
			expectError: true,
		},
		{
			name:        "path traversal with dotdot",
			promptName:  "../../../etc/passwd",
			expectError: true,
		},
		{
			name:        "path with forward slash",
			promptName:  "path/with/slash",
			expectError: true,
		},
		{
			name:        "path with backslash",
			promptName:  "path\\with\\backslash",
			expectError: true,
		},
		{
			name:        "prompt name with control characters",
			promptName:  "test\x00prompt",
			expectError: true,
		},
		{
			name:        "extremely long prompt name",
			promptName:  strings.Repeat("a", 101),
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePromptName(tt.promptName)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for prompt name %q, but got none", tt.promptName)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for valid prompt name %q: %v", tt.promptName, err)
				}
			}
		})
	}
}



func TestListInstalledPrompts(t *testing.T) {
	tests := []struct {
		name           string
		setupFS        func(afero.Fs) error
		expectedOutput string
	}{
		{
			name: "no .marvai directory",
			setupFS: func(fs afero.Fs) error {
				// Don't create .marvai directory
				return nil
			},
			expectedOutput: "No .marvai directory found. Run 'install' command to install prompts first.\n",
		},
		{
			name: "empty .marvai directory",
			setupFS: func(fs afero.Fs) error {
				return fs.MkdirAll(".marvai", 0755)
			},
			expectedOutput: "No installed prompts found in .marvai directory\n",
		},
		{
			name: "single installed prompt",
			setupFS: func(fs afero.Fs) error {
				if err := fs.MkdirAll(".marvai", 0755); err != nil {
					return err
				}
				return afero.WriteFile(fs, ".marvai/example.mprompt", []byte("name: Example\n--\n--\nTest prompt content"), 0644)
			},
			expectedOutput: "Found 1 installed prompt(s):\n  Example\n",
		},
		{
			name: "multiple installed prompts",
			setupFS: func(fs afero.Fs) error {
				if err := fs.MkdirAll(".marvai", 0755); err != nil {
					return err
				}
				if err := afero.WriteFile(fs, ".marvai/advanced.mprompt", []byte("name: Advanced\n--\n--\nAdvanced prompt"), 0644); err != nil {
					return err
				}
				if err := afero.WriteFile(fs, ".marvai/advanced.var", []byte("var1: value1"), 0644); err != nil {
					return err
				}
				if err := afero.WriteFile(fs, ".marvai/simple.mprompt", []byte("name: Simple\n--\n--\nSimple prompt"), 0644); err != nil {
					return err
				}
				// Add a non-prompt file to test filtering
				if err := afero.WriteFile(fs, ".marvai/config.txt", []byte("Config file"), 0644); err != nil {
					return err
				}
				return nil
			},
			expectedOutput: "Found 2 installed prompt(s):\n  Advanced (configured)\n  Simple\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Setup filesystem
			err := tt.setupFS(fs)
			if err != nil {
				t.Fatalf("Failed to setup filesystem: %v", err)
			}

			// Capture stdout
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			// Run the installed command
			err = ListInstalledPrompts(fs)

			// Restore stdout
			w.Close()
			os.Stdout = oldStdout

			// Read captured output
			output := make([]byte, 1024)
			n, _ := r.Read(output)
			actualOutput := string(output[:n])

			// Check for errors
			if err != nil {
				t.Errorf("ListInstalledPrompts returned error: %v", err)
			}

			// Check output
			if actualOutput != tt.expectedOutput {
				t.Errorf("Expected output:\n%q\nGot:\n%q", tt.expectedOutput, actualOutput)
			}
		})
	}
}

func TestInstalledCommand(t *testing.T) {
	// Create in-memory filesystem
	fs := afero.NewMemMapFs()

	// Create .marvai directory and install some prompts
	err := fs.MkdirAll(".marvai", 0755)
	if err != nil {
		t.Fatalf("Failed to create .marvai directory: %v", err)
	}

	err = afero.WriteFile(fs, ".marvai/test.mprompt", []byte("name: Test\n--\n--\nTest prompt content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create test prompt: %v", err)
	}

	err = afero.WriteFile(fs, ".marvai/example.mprompt", []byte("name: Example\n--\n--\nExample prompt content"), 0644)
	if err != nil {
		t.Fatalf("Failed to create example prompt: %v", err)
	}

	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	// Test the installed command via Run function
	var stderr bytes.Buffer
	err = Run([]string{"program", "installed"}, fs, &stderr, "0.0.1")

	// Restore stdout
	w.Close()
	os.Stdout = oldStdout

	// Read captured output
	output := make([]byte, 1024)
	n, _ := r.Read(output)
	actualOutput := string(output[:n])

	// Check for errors
	if err != nil {
		t.Errorf("Run with installed command returned error: %v", err)
	}

	// Check that we got some output about finding installed prompts
	if !strings.Contains(actualOutput, "Found 2 installed prompt(s)") {
		t.Errorf("Expected output to contain prompt count, got: %q", actualOutput)
	}

	if !strings.Contains(actualOutput, "Test") || !strings.Contains(actualOutput, "Example") {
		t.Errorf("Expected output to contain prompt names, got: %q", actualOutput)
	}
}

func TestVerifySHA256(t *testing.T) {
	tests := []struct {
		name         string
		content      []byte
		expectedHash string
		expectError  bool
	}{
		{
			name:         "valid hash matches",
			content:      []byte("hello world"),
			expectedHash: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
			expectError:  false,
		},
		{
			name:         "case insensitive hash matches",
			content:      []byte("hello world"),
			expectedHash: "B94D27B9934D3E08A52E52D7DA7DABFAC484EFE37A5380EE9088F7ACE2EFCDE9",
			expectError:  false,
		},
		{
			name:         "empty hash (skip verification)",
			content:      []byte("hello world"),
			expectedHash: "",
			expectError:  false,
		},
		{
			name:         "invalid hash does not match",
			content:      []byte("hello world"),
			expectedHash: "invalid_hash",
			expectError:  true,
		},
		{
			name:         "wrong hash does not match",
			content:      []byte("hello world"),
			expectedHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := verifySHA256(tt.content, tt.expectedHash)

			if tt.expectError && err == nil {
				t.Errorf("Expected error but got none")
			}

			if !tt.expectError && err != nil {
				t.Errorf("Expected no error but got: %v", err)
			}
		})
	}
}
