package source

import (
	"testing"
	"time"

	"github.com/spf13/afero"
)

func TestFileHandler_CanHandle(t *testing.T) {
	fs := afero.NewMemMapFs()
	handler := NewFileHandler(fs)

	tests := []struct {
		source   string
		expected bool
	}{
		{"example", true},
		{"example.mprompt", true},
		{"path/to/file", true},
		{"https://example.com/test.mprompt", false},
		{"http://example.com/test.mprompt", false},
	}

	for _, test := range tests {
		result := handler.CanHandle(test.source)
		if result != test.expected {
			t.Errorf("FileHandler.CanHandle(%q) = %v, expected %v", test.source, result, test.expected)
		}
	}
}

func TestHTTPSHandler_CanHandle(t *testing.T) {
	handler := NewHTTPSHandler(10 * time.Second)

	tests := []struct {
		source   string
		expected bool
	}{
		{"example", false},
		{"example.mprompt", false},
		{"https://example.com/test.mprompt", true},
		{"http://example.com/test.mprompt", false},
		{"ftp://example.com/test.mprompt", false},
	}

	for _, test := range tests {
		result := handler.CanHandle(test.source)
		if result != test.expected {
			t.Errorf("HTTPSHandler.CanHandle(%q) = %v, expected %v", test.source, result, test.expected)
		}
	}
}

func TestFileHandler_LoadContent(t *testing.T) {
	fs := afero.NewMemMapFs()
	handler := NewFileHandler(fs)

	// Create test file
	testContent := `- id: test
  question: "Test?"
  type: string
--
Test template {{test}}`

	err := afero.WriteFile(fs, "test.mprompt", []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test loading with .mprompt extension
	content, err := handler.LoadContent("test.mprompt")
	if err != nil {
		t.Fatalf("Failed to load content: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Content mismatch. Got: %q, Expected: %q", string(content), testContent)
	}

	// Test loading without .mprompt extension
	content, err = handler.LoadContent("test")
	if err != nil {
		t.Fatalf("Failed to load content without extension: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Content mismatch. Got: %q, Expected: %q", string(content), testContent)
	}
}

func TestFileHandler_GetDisplayName(t *testing.T) {
	fs := afero.NewMemMapFs()
	handler := NewFileHandler(fs)

	tests := []struct {
		source   string
		expected string
	}{
		{"test", "test.mprompt"},
		{"test.mprompt", "test.mprompt"},
		{"path/to/test", "path/to/test.mprompt"},
	}

	for _, test := range tests {
		result := handler.GetDisplayName(test.source)
		if result != test.expected {
			t.Errorf("FileHandler.GetDisplayName(%q) = %q, expected %q", test.source, result, test.expected)
		}
	}
}

func TestHTTPSHandler_GetDisplayName(t *testing.T) {
	handler := NewHTTPSHandler(10 * time.Second)

	url := "https://example.com/test.mprompt"
	result := handler.GetDisplayName(url)
	if result != url {
		t.Errorf("HTTPSHandler.GetDisplayName(%q) = %q, expected %q", url, result, url)
	}
}

func TestSourceManager_LoadContent(t *testing.T) {
	fs := afero.NewMemMapFs()
	manager := NewSourceManager(fs)

	// Create test file
	testContent := `- id: test
  question: "Test?"
  type: string
--
Test template {{test}}`

	err := afero.WriteFile(fs, "test.mprompt", []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test loading local file
	content, displayName, err := manager.LoadContent("test")
	if err != nil {
		t.Fatalf("Failed to load local file: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Content mismatch. Got: %q, Expected: %q", string(content), testContent)
	}

	if displayName != "test.mprompt" {
		t.Errorf("Display name mismatch. Got: %q, Expected: %q", displayName, "test.mprompt")
	}
}