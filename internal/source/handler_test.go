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
		{"https://github.com/owner/repo", false},
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
		{"https://github.com/owner/repo", true},
		{"https://github.com/owner/repo/blob/main/test.mprompt", true},
		{"https://example.com/test.mprompt", false},
		{"http://github.com/owner/repo", false},
		{"ftp://github.com/owner/repo", false},
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

	url := "https://github.com/owner/repo"
	result := handler.GetDisplayName(url)
	if result != url {
		t.Errorf("HTTPSHandler.GetDisplayName(%q) = %q, expected %q", url, result, url)
	}
}

func TestHTTPSHandler_transformToRawURL(t *testing.T) {
	handler := NewHTTPSHandler(10 * time.Second)

	tests := []struct {
		githubURL string
		expected  string
		shouldErr bool
	}{
		{
			"https://github.com/StephanSchmidt/greatprompt",
			"https://raw.githubusercontent.com/StephanSchmidt/greatprompt/main/greatprompt.mprompt",
			false,
		},
		{
			"https://github.com/owner/repo/blob/main/test.mprompt",
			"https://raw.githubusercontent.com/owner/repo/main/test.mprompt",
			false,
		},
		{
			"https://github.com/owner/repo/blob/develop/prompts/myprompt",
			"https://raw.githubusercontent.com/owner/repo/develop/prompts/myprompt.mprompt",
			false,
		},
		{
			"https://github.com/owner/repo/custom/path",
			"https://raw.githubusercontent.com/owner/repo/main/custom/path.mprompt",
			false,
		},
		{
			"https://github.com/owner",
			"",
			true,
		},
		{
			"invalid-url",
			"",
			true,
		},
	}

	for _, test := range tests {
		result, err := handler.transformToRawURL(test.githubURL)
		
		if test.shouldErr && err == nil {
			t.Errorf("transformToRawURL(%q) should have returned an error", test.githubURL)
		}
		
		if !test.shouldErr && err != nil {
			t.Errorf("transformToRawURL(%q) returned unexpected error: %v", test.githubURL, err)
		}
		
		if !test.shouldErr && result != test.expected {
			t.Errorf("transformToRawURL(%q) = %q, expected %q", test.githubURL, result, test.expected)
		}
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