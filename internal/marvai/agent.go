package marvai

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/afero"
)

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