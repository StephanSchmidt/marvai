package main

import (
	"bytes"
	"testing"

	"github.com/spf13/afero"

	"marvai/internal/marvai"
)

// TestMainIntegration tests the main function integration without actually running Claude
func TestMainIntegration(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		expectError bool
	}{
		{
			name:        "insufficient arguments",
			args:        []string{"program"},
			expectError: true,
		},
		{
			name:        "command validation",
			args:        []string{"program", "prompt"},
			expectError: true, // Should fail because no prompt name provided
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()

			// Capture stderr
			var stderr bytes.Buffer

			// Test the main Run function from internal package
			err := marvai.Run(tt.args, fs, &stderr)

			if tt.expectError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}