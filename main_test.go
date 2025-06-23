package main

import (
	"bytes"
	"testing"

	"github.com/spf13/afero"

	"marvai/internal/marvai"
)

// TestMainIntegration tests the main function integration
func TestMainIntegration(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		promptExists  bool
		promptContent string
		expectError   bool
	}{
		{
			name:        "insufficient arguments",
			args:        []string{"program"},
			expectError: true,
		},
		{
			name:          "valid prompt execution",
			args:          []string{"program", "test"},
			promptExists:  true,
			promptContent: "test prompt content",
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create in-memory filesystem
			fs := afero.NewMemMapFs()
			
			if tt.promptExists {
				fs.MkdirAll(".marvai", 0755)
				afero.WriteFile(fs, ".marvai/"+tt.args[1]+".prompt", []byte(tt.promptContent), 0644)
			}

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