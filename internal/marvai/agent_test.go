package marvai

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
)

func TestIsValidCliBinary(t *testing.T) {
	tests := []struct {
		name           string
		setupFS        func(afero.Fs) string
		expectedResult bool
		description    string
	}{
		{
			name: "valid executable binary",
			setupFS: func(fs afero.Fs) string {
				path := "/usr/bin/test-cli"
				// Create a regular file with executable permissions
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'test'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: true,
			description:    "Should return true for valid executable binary",
		},
		{
			name: "file does not exist",
			setupFS: func(fs afero.Fs) string {
				return "/nonexistent/path/binary"
			},
			expectedResult: false,
			description:    "Should return false when file doesn't exist",
		},
		{
			name: "file is not executable",
			setupFS: func(fs afero.Fs) string {
				path := "/usr/bin/non-executable"
				// Create a regular file without executable permissions
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'test'"), 0644)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: false,
			description:    "Should return false for non-executable file",
		},
		{
			name: "path contains path traversal",
			setupFS: func(fs afero.Fs) string {
				// Create a valid executable file
				validPath := "/usr/bin/valid-cli"
				err := afero.WriteFile(fs, validPath, []byte("#!/bin/bash\necho 'test'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				// Return a path with .. traversal (this will be cleaned by filepath.Clean)
				return "/usr/bin/../bin/valid-cli"
			},
			expectedResult: true,
			description:    "Should return true for paths that resolve to valid locations after cleaning",
		},
		{
			name: "binary in dangerous directory /tmp",
			setupFS: func(fs afero.Fs) string {
				path := "/tmp/malicious-binary"
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'malicious'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: false,
			description:    "Should return false for binaries in /tmp/",
		},
		{
			name: "binary in dangerous directory /var/tmp",
			setupFS: func(fs afero.Fs) string {
				path := "/var/tmp/malicious-binary"
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'malicious'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: false,
			description:    "Should return false for binaries in /var/tmp/",
		},
		{
			name: "binary in dangerous directory /dev/shm",
			setupFS: func(fs afero.Fs) string {
				path := "/dev/shm/malicious-binary"
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'malicious'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: false,
			description:    "Should return false for binaries in /dev/shm/",
		},
		{
			name: "valid binary in user home directory",
			setupFS: func(fs afero.Fs) string {
				path := "/home/user/.local/bin/cli-tool"
				// Create directory structure
				err := fs.MkdirAll(filepath.Dir(path), 0755)
				if err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				// Create executable file
				err = afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'user tool'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: true,
			description:    "Should return true for valid binary in user directory",
		},
		{
			name: "valid binary in /usr/local/bin",
			setupFS: func(fs afero.Fs) string {
				path := "/usr/local/bin/custom-cli"
				// Create directory structure
				err := fs.MkdirAll(filepath.Dir(path), 0755)
				if err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				// Create executable file
				err = afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'custom tool'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: true,
			description:    "Should return true for valid binary in /usr/local/bin",
		},
		{
			name: "path with complex traversal attempt",
			setupFS: func(fs afero.Fs) string {
				// Create a valid executable file
				validPath := "/usr/bin/valid-cli"
				err := afero.WriteFile(fs, validPath, []byte("#!/bin/bash\necho 'test'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				// Return a path with complex traversal (this will be cleaned by filepath.Clean)
				return "/usr/bin/../../usr/bin/valid-cli"
			},
			expectedResult: true,
			description:    "Should return true for paths that resolve to valid locations after cleaning",
		},
		{
			name: "path with literal .. in filename",
			setupFS: func(fs afero.Fs) string {
				// Create a file with literal ".." in the filename
				path := "/usr/bin/cli..tool"
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'test'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: false,
			description:    "Should return false for paths containing '..' even as part of filename",
		},
		{
			name: "file exists but is directory",
			setupFS: func(fs afero.Fs) string {
				path := "/usr/bin/directory-not-file"
				// Create a directory instead of file
				err := fs.MkdirAll(path, 0755)
				if err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				return path
			},
			expectedResult: false,
			description:    "Should return false when path points to directory",
		},
		{
			name: "binary with minimal executable permissions",
			setupFS: func(fs afero.Fs) string {
				path := "/usr/bin/minimal-exec"
				// Create file with minimal executable permissions (owner execute only)
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'minimal'"), 0100)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: true,
			description:    "Should return true for file with minimal executable permissions",
		},
		{
			name: "binary with group/other execute permissions",
			setupFS: func(fs afero.Fs) string {
				path := "/usr/bin/group-exec"
				// Create file with group/other executable permissions
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'group'"), 0011)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: true,
			description:    "Should return true for file with group/other execute permissions",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new in-memory filesystem for each test
			fs := afero.NewMemMapFs()

			// Set up the filesystem and get the path to test
			testPath := tt.setupFS(fs)

			// Call the function under test
			result := isValidCliBinary(fs, testPath)

			// Check the result
			if result != tt.expectedResult {
				t.Errorf("isValidCliBinary(%q) = %v, want %v", testPath, result, tt.expectedResult)
			}

			t.Logf("✅ %s", tt.description)
		})
	}
}

func TestIsValidCliBinaryEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		setupFS        func(afero.Fs) string
		expectedResult bool
		description    string
	}{
		{
			name: "empty path",
			setupFS: func(fs afero.Fs) string {
				return ""
			},
			expectedResult: false,
			description:    "Should return false for empty path",
		},
		{
			name: "root path",
			setupFS: func(fs afero.Fs) string {
				return "/"
			},
			expectedResult: false,
			description:    "Should return false for root path",
		},
		{
			name: "path with trailing slash",
			setupFS: func(fs afero.Fs) string {
				path := "/usr/bin/test-cli"
				err := afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'test'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path + "/"
			},
			expectedResult: true,
			description:    "Should return true for valid file even with trailing slash (filepath.Clean normalizes it)",
		},
		{
			name: "relative path",
			setupFS: func(fs afero.Fs) string {
				path := "bin/relative-cli"
				// Create directory structure
				err := fs.MkdirAll(filepath.Dir(path), 0755)
				if err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				// Create executable file
				err = afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'relative'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: true,
			description:    "Should return true for valid relative path",
		},
		{
			name: "path in tmp subdirectory should be allowed",
			setupFS: func(fs afero.Fs) string {
				path := "/home/user/tmp/safe-binary"
				// Create directory structure
				err := fs.MkdirAll(filepath.Dir(path), 0755)
				if err != nil {
					t.Fatalf("Failed to create directory: %v", err)
				}
				// Create executable file
				err = afero.WriteFile(fs, path, []byte("#!/bin/bash\necho 'safe'"), 0755)
				if err != nil {
					t.Fatalf("Failed to create test file: %v", err)
				}
				return path
			},
			expectedResult: true,
			description:    "Should return true for path containing 'tmp' but not starting with dangerous directories",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new in-memory filesystem for each test
			fs := afero.NewMemMapFs()

			// Set up the filesystem and get the path to test
			testPath := tt.setupFS(fs)

			// Call the function under test
			result := isValidCliBinary(fs, testPath)

			// Check the result
			if result != tt.expectedResult {
				t.Errorf("isValidCliBinary(%q) = %v, want %v", testPath, result, tt.expectedResult)
			}

			t.Logf("✅ %s", tt.description)
		})
	}
}

// TestIsValidCliBinaryFileSystemErrors tests error handling for filesystem operations
func TestIsValidCliBinaryFileSystemErrors(t *testing.T) {
	// Create a filesystem that will simulate stat errors
	fs := afero.NewMemMapFs()

	// Test with a path that doesn't exist
	result := isValidCliBinary(fs, "/nonexistent/binary")
	if result != false {
		t.Errorf("isValidCliBinary() should return false for non-existent file, got %v", result)
	}

	t.Log("✅ Should handle filesystem errors gracefully")
}
