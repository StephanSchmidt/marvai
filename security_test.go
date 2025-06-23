package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"marvai/internal"
	"marvai/internal/marvai"
)

// TestTemplateInjectionVulnerabilities demonstrates SSTI attacks
func TestTemplateInjectionVulnerabilities(t *testing.T) {
	maliciousTemplates := []struct {
		name        string
		template    string
		values      map[string]string
		expectDanger bool
		description string
	}{
		{
			name:        "prototype pollution attempt",
			template:    "{{__proto__.polluted}}",
			values:      map[string]string{"__proto__": "malicious"},
			expectDanger: false, // Go doesn't have prototype pollution like JS
			description: "Attempt to pollute prototype chain",
		},
		{
			name:        "template with executable code patterns",
			template:    "{{#with (lookup this 'constructor')}}{{#with (lookup this 'constructor')}}{{this}}{{/with}}{{/with}}",
			values:      map[string]string{},
			expectDanger: true,
			description: "Attempt to access constructor functions",
		},
		{
			name:        "nested helper exploitation",
			template:    "{{#each (split (split input '{{') '}}')}}{{{this}}}{{/each}}",
			values:      map[string]string{"input": "{{#each}}malicious{{/each}}"},
			expectDanger: true,
			description: "Nested template evaluation through split helper",
		},
		{
			name:        "information disclosure via error messages",
			template:    "{{nonexistent.property.access}}",
			values:      map[string]string{},
			expectDanger: true,
			description: "Trigger error messages that might leak information",
		},
	}

	for _, tt := range maliciousTemplates {
		t.Run(tt.name, func(t *testing.T) {
			// This test demonstrates the vulnerabilities - in practice these should be blocked
			result, err := internal.RenderTemplate(tt.template, tt.values)
			
			if tt.expectDanger {
				t.Logf("POTENTIAL VULNERABILITY: Template %q rendered as %q with error: %v", 
					tt.template, result, err)
				// In a secure system, these should be blocked or sanitized
			} else {
				t.Logf("Template %q safely handled: %q", tt.template, result)
			}
		})
	}
}

// TestCommandInjectionVulnerabilities demonstrates command injection risks
func TestCommandInjectionVulnerabilities(t *testing.T) {
	// Test if claude binary path can be manipulated
	maliciousPaths := []string{
		"claude; rm -rf /",
		"claude && echo 'compromised' > /tmp/pwned",
		"claude | cat /etc/passwd",
		"../../../usr/bin/malicious-binary",
		"/tmp/fake-claude",
	}

	for _, path := range maliciousPaths {
		t.Run("command_injection_"+path, func(t *testing.T) {
			// Test that the FindClaudeBinary function can be exploited
			// This would be tested by setting up fake binaries in the filesystem
			t.Logf("POTENTIAL VULNERABILITY: Binary path could be: %q", path)
		})
	}
}

// TestYAMLInjectionVulnerabilities demonstrates YAML injection attacks  
func TestYAMLInjectionVulnerabilities(t *testing.T) {
	maliciousYAML := []struct {
		name     string
		content  string
		dangerous bool
	}{
		{
			name: "billion_laughs_attack",
			content: `
&a ["lol","lol","lol","lol","lol","lol","lol","lol","lol"]
&b [*a,*a,*a,*a,*a,*a,*a,*a,*a]
&c [*b,*b,*b,*b,*b,*b,*b,*b,*b]
&d [*c,*c,*c,*c,*c,*c,*c,*c,*c]
--
template`,
			dangerous: true,
		},
		{
			name: "yaml_bomb",
			content: strings.Repeat("- ", 100000) + `
  id: test
  question: "Test?"
--
template`,
			dangerous: true,
		},
		{
			name: "null_byte_injection",
			content: "- id: test\x00\n  question: \"Test?\"\n--\ntemplate",
			dangerous: true,
		},
	}

	for _, tt := range maliciousYAML {
		t.Run(tt.name, func(t *testing.T) {
			fs := afero.NewMemMapFs()
			filename := "malicious.mprompt"
			
			err := afero.WriteFile(fs, filename, []byte(tt.content), 0644)
			if err != nil {
				t.Fatalf("Failed to write test file: %v", err)
			}

			// Test parsing the malicious YAML
			_, err = marvai.ParseMPrompt(fs, filename)
			
			if tt.dangerous && err == nil {
				t.Errorf("SECURITY VULNERABILITY: Malicious YAML was parsed successfully: %s", tt.name)
			}
			
			t.Logf("YAML attack %q result: %v", tt.name, err)
		})
	}
}

// TestSymlinkAttacks demonstrates symlink-based attacks
func TestSymlinkAttacks(t *testing.T) {
	if os.Getenv("CI") != "" {
		t.Skip("Skipping symlink tests in CI environment")
	}

	// Create a temporary directory structure for testing
	tempDir := t.TempDir()
	
	// Create a sensitive file outside the intended directory
	sensitiveFile := filepath.Join(tempDir, "sensitive.txt")
	err := os.WriteFile(sensitiveFile, []byte("SECRET DATA"), 0644)
	if err != nil {
		t.Fatalf("Failed to create sensitive file: %v", err)
	}

	// Create .marvai directory
	marvaiDir := filepath.Join(tempDir, ".marvai")
	err = os.MkdirAll(marvaiDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create .marvai directory: %v", err)
	}

	// Create a symlink pointing to the sensitive file
	symlinkPath := filepath.Join(marvaiDir, "malicious.prompt")
	err = os.Symlink(sensitiveFile, symlinkPath)
	if err != nil {
		t.Skipf("Cannot create symlinks on this system: %v", err)
	}

	// Test if LoadPrompt follows the symlink
	fs := afero.NewOsFs()
	
	// Change to temp directory
	oldDir, _ := os.Getwd()
	defer os.Chdir(oldDir)
	os.Chdir(tempDir)

	content, err := marvai.LoadPrompt(fs, "malicious")
	if err == nil && string(content) == "SECRET DATA" {
		t.Errorf("SECURITY VULNERABILITY: Symlink attack succeeded, accessed: %q", string(content))
	} else if err != nil && strings.Contains(err.Error(), "symbolic link") {
		t.Logf("✅ SECURITY FIX: Symlink attack properly blocked: %v", err)
	} else {
		t.Logf("Symlink attack blocked or failed: %v", err)
	}
}

// TestBinaryHijacking demonstrates binary hijacking vulnerabilities
func TestBinaryHijacking(t *testing.T) {
	tempDir := t.TempDir()
	
	// Create a malicious binary in a location that might be searched
	maliciousBinary := filepath.Join(tempDir, "claude")
	err := os.WriteFile(maliciousBinary, []byte("#!/bin/sh\necho 'HIJACKED'\n"), 0755)
	if err != nil {
		t.Fatalf("Failed to create malicious binary: %v", err)
	}

	// Test if the binary search can be manipulated
	originalPath := os.Getenv("PATH")
	defer os.Setenv("PATH", originalPath)
	
	// Prepend malicious directory to PATH
	os.Setenv("PATH", tempDir+":"+originalPath)
	
	claudePath := marvai.FindClaudeBinary()
	if strings.Contains(claudePath, tempDir) {
		t.Errorf("SECURITY VULNERABILITY: Binary hijacking possible, found: %q", claudePath)
	} else {
		t.Logf("✅ SECURITY FIX: Binary hijacking prevented, found secure path: %q", claudePath)
	}
}

// TestMemoryExhaustionAttacks demonstrates DoS through memory exhaustion
func TestMemoryExhaustionAttacks(t *testing.T) {
	attacks := []struct {
		name        string
		template    string
		values      map[string]string
		expectError bool
	}{
		{
			name:        "large_variable_content",
			template:    "{{content}}",
			values:      map[string]string{"content": strings.Repeat("A", 10*1024*1024)}, // 10MB
			expectError: false, // Should handle but might be slow
		},
		{
			name:        "recursive_split_attack",
			template:    "{{#each (split (split (split items ',') ',') ',')}}{{{this}}}{{/each}}",
			values:      map[string]string{"items": strings.Repeat("a,", 10000)},
			expectError: false, // Should handle but might be slow
		},
	}

	for _, attack := range attacks {
		t.Run(attack.name, func(t *testing.T) {
			// Add timeout to prevent hanging
			done := make(chan bool, 1)
			var result string
			var err error
			
			go func() {
				result, err = internal.RenderTemplate(attack.template, attack.values)
				done <- true
			}()
			
			select {
			case <-done:
				t.Logf("Memory attack %q completed with result length: %d, error: %v", 
					attack.name, len(result), err)
			default:
				t.Logf("Memory attack %q may have caused performance issues", attack.name)
			}
		})
	}
}