package marvai

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"

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


