package marvai

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"testing"

	"github.com/spf13/afero"
)

// Mock CommandRunner for testing
type MockCommandRunner struct {
	lookPathResult string
	lookPathError  error
	commands       []*MockCommand
}

type MockCommand struct {
	name   string
	args   []string
	stdout io.Writer
	stderr io.Writer
	stdin  io.WriteCloser
}

func (m *MockCommand) StdinPipe() (io.WriteCloser, error) {
	if m.stdin == nil {
		return nil, fmt.Errorf("stdin pipe error")
	}
	return m.stdin, nil
}

func (m *MockCommand) Start() error {
	return nil
}

func (m *MockCommand) Wait() error {
	return nil
}

func (m *MockCommandRunner) Command(name string, arg ...string) *exec.Cmd {
	mockCmd := &MockCommand{
		name: name,
		args: arg,
	}
	m.commands = append(m.commands, mockCmd)
	
	// Create a real exec.Cmd but replace its methods for testing
	cmd := &exec.Cmd{
		Path: name,
		Args: append([]string{name}, arg...),
	}
	return cmd
}

func (m *MockCommandRunner) LookPath(file string) (string, error) {
	return m.lookPathResult, m.lookPathError
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

			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Create existing files
			for _, file := range tt.existingFiles {
				fs.MkdirAll(file[:strings.LastIndex(file, "/")], 0755)
				afero.WriteFile(fs, file, []byte("mock claude binary"), 0755)
			}

			// Test function
			result := FindClaudeBinaryWithRunner(mockRunner, fs, tt.goos, tt.homeDir)

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
			expectedError: "insufficient arguments",
			checkStderr:   true,
		},
		{
			name:          "prompt command without name",
			args:          []string{"program", "prompt"},
			expectedError: "prompt name required",
		},
		{
			name:          "install command without name",
			args:          []string{"program", "install"},
			expectedError: "mprompt name required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Capture stderr
			var stderr bytes.Buffer

			// Test run function
			err := Run(tt.args, fs, &stderr)

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
				expectedUsage := "Usage: program <command> [args...]\nCommands:\n  prompt <name>   - Execute a prompt\n  install <name>  - Install a .mprompt file\n"
				if stderr.String() != expectedUsage {
					t.Errorf("Expected stderr %q, got %q", expectedUsage, stderr.String())
				}
			}
		})
	}
}

func TestLoadPrompt(t *testing.T) {
	tests := []struct {
		name          string
		promptName    string
		fileContent   string
		expectedError bool
	}{
		{
			name:          "load existing prompt",
			promptName:    "example",
			fileContent:   "Hello from example prompt",
			expectedError: false,
		},
		{
			name:          "load prompt with spaces in name",
			promptName:    "my-prompt",
			fileContent:   "This is a test prompt with content",
			expectedError: false,
		},
		{
			name:          "load empty prompt",
			promptName:    "empty",
			fileContent:   "",
			expectedError: false,
		},
		{
			name:          "load multiline prompt",
			promptName:    "multiline",
			fileContent:   "Line 1\nLine 2\nLine 3",
			expectedError: false,
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

			// Write test file
			promptFile := ".marvai/" + tt.promptName + ".prompt"
			err = afero.WriteFile(fs, promptFile, []byte(tt.fileContent), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Test LoadPrompt function
			content, err := LoadPrompt(fs, tt.promptName)

			if tt.expectedError && err == nil {
				t.Errorf("Expected error but got none")
			}
			if !tt.expectedError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if !tt.expectedError && string(content) != tt.fileContent {
				t.Errorf("Expected content %q, got %q", tt.fileContent, string(content))
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
			afero.WriteFile(fs, tt.expected, []byte("test content"), 0644)

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
			promptFile := ".marvai/" + tt.promptName + ".prompt"
			err := afero.WriteFile(fs, promptFile, []byte(tt.fileContent), 0644)
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