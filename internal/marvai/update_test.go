package marvai

import (
	"strings"
	"testing"

	"github.com/spf13/afero"
)

func TestExecuteWizardWithPrefills(t *testing.T) {
	tests := []struct {
		name           string
		variables      []WizardVariable
		prefillValues  map[string]string
		userInput      string
		expectedValues map[string]string
		expectedError  string
	}{
		{
			name: "use prefilled values when user presses enter",
			variables: []WizardVariable{
				{ID: "name", Description: "Enter your name", Required: true},
				{ID: "email", Description: "Enter your email", Required: false},
			},
			prefillValues: map[string]string{
				"name":  "John Doe",
				"email": "john@example.com",
			},
			userInput: "\n\n", // Press enter for both
			expectedValues: map[string]string{
				"name":  "John Doe",
				"email": "john@example.com",
			},
		},
		{
			name: "override prefilled values with new input",
			variables: []WizardVariable{
				{ID: "name", Description: "Enter your name", Required: true},
			},
			prefillValues: map[string]string{
				"name": "John Doe",
			},
			userInput: "Jane Smith\n",
			expectedValues: map[string]string{
				"name": "Jane Smith",
			},
		},
		{
			name: "required field with no prefill and empty input",
			variables: []WizardVariable{
				{ID: "name", Description: "Enter your name", Required: true},
			},
			prefillValues: map[string]string{},
			userInput:     "\n",
			expectedError: "variable 'name' is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a string reader for user input
			reader := strings.NewReader(tt.userInput)
			
			values, err := ExecuteWizardWithPrefilledReader(tt.variables, tt.prefillValues, reader)
			
			if tt.expectedError != "" {
				if err == nil {
					t.Errorf("Expected error containing %q, but got no error", tt.expectedError)
					return
				}
				if !strings.Contains(err.Error(), tt.expectedError) {
					t.Errorf("Expected error containing %q, but got %q", tt.expectedError, err.Error())
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			for key, expectedValue := range tt.expectedValues {
				if actualValue, exists := values[key]; !exists {
					t.Errorf("Expected key %q not found in result", key)
				} else if actualValue != expectedValue {
					t.Errorf("For key %q: expected %q, got %q", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestCopyFileAfero(t *testing.T) {
	tests := []struct {
		name          string
		setupFS       func(afero.Fs)
		srcFile       string
		dstFile       string
		expectedError bool
	}{
		{
			name: "copy existing file",
			setupFS: func(fs afero.Fs) {
				afero.WriteFile(fs, "source.txt", []byte("test content"), 0644)
			},
			srcFile: "source.txt",
			dstFile: "destination.txt",
		},
		{
			name: "copy non-existent file",
			setupFS: func(fs afero.Fs) {
				// Don't create source file
			},
			srcFile:       "nonexistent.txt",
			dstFile:       "destination.txt",
			expectedError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			tt.setupFS(fs)
			
			err := copyFileAfero(fs, tt.srcFile, tt.dstFile)
			
			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			// Verify destination file exists and has same content
			exists, err := afero.Exists(fs, tt.dstFile)
			if err != nil {
				t.Errorf("Error checking destination file: %v", err)
				return
			}
			if !exists {
				t.Errorf("Destination file %q was not created", tt.dstFile)
				return
			}
			
			// Compare contents
			srcContent, err := afero.ReadFile(fs, tt.srcFile)
			if err != nil {
				t.Errorf("Error reading source file: %v", err)
				return
			}
			
			dstContent, err := afero.ReadFile(fs, tt.dstFile)
			if err != nil {
				t.Errorf("Error reading destination file: %v", err)
				return
			}
			
			if string(srcContent) != string(dstContent) {
				t.Errorf("File contents don't match: src=%q, dst=%q", srcContent, dstContent)
			}
		})
	}
}

func TestSaveVarFile(t *testing.T) {
	tests := []struct {
		name          string
		values        map[string]string
		expectedError bool
	}{
		{
			name: "save valid values",
			values: map[string]string{
				"name":  "John Doe",
				"email": "john@example.com",
			},
		},
		{
			name:   "save empty values",
			values: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			testFile := "test.var"
			
			err := saveVarFile(fs, testFile, tt.values)
			
			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			// Verify file was created and content is correct
			exists, err := afero.Exists(fs, testFile)
			if err != nil {
				t.Errorf("Error checking file existence: %v", err)
				return
			}
			if !exists {
				t.Errorf("File %q was not created", testFile)
				return
			}
			
			// Load the file back and verify content
			loadedValues, err := loadVarFile(fs, testFile)
			if err != nil {
				t.Errorf("Error loading saved file: %v", err)
				return
			}
			
			if len(loadedValues) != len(tt.values) {
				t.Errorf("Expected %d values, got %d", len(tt.values), len(loadedValues))
				return
			}
			
			for key, expectedValue := range tt.values {
				if actualValue, exists := loadedValues[key]; !exists {
					t.Errorf("Expected key %q not found", key)
				} else if actualValue != expectedValue {
					t.Errorf("For key %q: expected %q, got %q", key, expectedValue, actualValue)
				}
			}
		})
	}
}

func TestLoadVarFile(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		expectedVars  map[string]string
		expectedError bool
	}{
		{
			name: "valid var file",
			fileContent: `name: John Doe
email: john@example.com
age: "25"`,
			expectedVars: map[string]string{
				"name":  "John Doe",
				"email": "john@example.com",
				"age":   "25",
			},
		},
		{
			name:          "invalid yaml",
			fileContent:   `name: John Doe\nemail: [invalid yaml`,
			expectedError: true,
		},
		{
			name:        "empty file",
			fileContent: "",
			expectedVars: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			testFile := "test.var"
			
			afero.WriteFile(fs, testFile, []byte(tt.fileContent), 0644)
			
			vars, err := loadVarFile(fs, testFile)
			
			if tt.expectedError {
				if err == nil {
					t.Errorf("Expected error, but got none")
				}
				return
			}
			
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}
			
			if len(vars) != len(tt.expectedVars) {
				t.Errorf("Expected %d variables, got %d", len(tt.expectedVars), len(vars))
				return
			}
			
			for key, expectedValue := range tt.expectedVars {
				if actualValue, exists := vars[key]; !exists {
					t.Errorf("Expected key %q not found", key)
				} else if actualValue != expectedValue {
					t.Errorf("For key %q: expected %q, got %q", key, expectedValue, actualValue)
				}
			}
		})
	}
}