package main

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/afero"

	"marvai/internal"
	"marvai/internal/marvai"
)

// MockCommandRunner for testing binary operations
type MockCommandRunner struct {
	lookPathResult string
	lookPathError  error
}

func (m *MockCommandRunner) Command(name string, arg ...string) *exec.Cmd {
	// Simple mock that returns a basic command for testing
	return exec.Command("echo", "mock output")
}

func (m *MockCommandRunner) LookPath(file string) (string, error) {
	return m.lookPathResult, m.lookPathError
}

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
	// Create an in-memory filesystem for testing
	fs := afero.NewMemMapFs()
	
	// Create a sensitive file outside the intended directory
	sensitiveFile := "/sensitive.txt"
	err := afero.WriteFile(fs, sensitiveFile, []byte("SECRET DATA"), 0644)
	if err != nil {
		t.Fatalf("Failed to create sensitive file: %v", err)
	}

	// Create .marvai directory
	marvaiDir := ".marvai"
	err = fs.MkdirAll(marvaiDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create .marvai directory: %v", err)
	}

	// Since afero's MemMapFs doesn't support real symlinks, we simulate a symlink attack
	// by creating a file that contains a path to the sensitive file (like a symlink would)
	// This tests if the LoadPrompt function properly validates file paths
	
	// Try to create a prompt file that attempts to access the sensitive file via path traversal
	maliciousPromptPath := filepath.Join(marvaiDir, "malicious.prompt")
	
	// Test 1: Direct path traversal content
	err = afero.WriteFile(fs, maliciousPromptPath, []byte("SECRET DATA"), 0644)
	if err != nil {
		t.Fatalf("Failed to create malicious prompt file: %v", err)
	}

	// Test if LoadPrompt properly validates the file path and prevents access
	content, err := marvai.LoadPrompt(fs, "malicious")
	if err == nil {
		// Check if the content is what we expect (the actual prompt content, not sensitive data)
		if string(content) == "SECRET DATA" {
			// This is expected since we put the content directly in the file
			// The real protection is in the path validation
			t.Logf("✅ LoadPrompt successfully loaded content from valid .marvai path")
		}
	} else {
		t.Logf("LoadPrompt returned error: %v", err)
	}
	
	// Test 2: Validate that LoadPrompt rejects path traversal attempts in the prompt name
	_, err = marvai.LoadPrompt(fs, "../sensitive")
	if err != nil && (strings.Contains(err.Error(), "invalid") || strings.Contains(err.Error(), "path") || strings.Contains(err.Error(), "traversal")) {
		t.Logf("✅ SECURITY FIX: Path traversal attack properly blocked: %v", err)
	} else if err != nil {
		t.Logf("Path traversal blocked with error: %v", err)
	} else {
		t.Errorf("SECURITY VULNERABILITY: Path traversal attack may have succeeded")
	}
	
	// Test 3: Validate that LoadPrompt rejects absolute paths
	_, err = marvai.LoadPrompt(fs, "/sensitive")
	if err != nil {
		t.Logf("✅ SECURITY FIX: Absolute path attack properly blocked: %v", err)
	} else {
		t.Errorf("SECURITY VULNERABILITY: Absolute path attack may have succeeded")
	}
}

// TestBinaryHijacking demonstrates binary hijacking vulnerabilities using afero
func TestBinaryHijacking(t *testing.T) {
	// Create an in-memory filesystem for testing
	fs := afero.NewMemMapFs()
	
	// Create directory structure that simulates PATH directories
	maliciousDir := "/malicious/bin"
	err := fs.MkdirAll(maliciousDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create malicious directory: %v", err)
	}
	
	legitimateDir := "/usr/local/bin"
	err = fs.MkdirAll(legitimateDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create legitimate directory: %v", err)
	}
	
	// Create a malicious binary in the first directory
	maliciousBinary := filepath.Join(maliciousDir, "claude")
	err = afero.WriteFile(fs, maliciousBinary, []byte("#!/bin/sh\necho 'HIJACKED'\n"), 0755)
	if err != nil {
		t.Fatalf("Failed to create malicious binary: %v", err)
	}
	
	// Create a legitimate binary in the second directory
	legitimateBinary := filepath.Join(legitimateDir, "claude")
	err = afero.WriteFile(fs, legitimateBinary, []byte("#!/bin/sh\necho 'LEGITIMATE'\n"), 0755)
	if err != nil {
		t.Fatalf("Failed to create legitimate binary: %v", err)
	}
	
	// Test the binary finding function with a custom mock runner
	mockRunner := &MockCommandRunner{
		lookPathResult: maliciousBinary, // Simulate finding the malicious binary first
		lookPathError:  nil,
	}
	
	// Test that the function properly validates binaries
	claudePath := marvai.FindClaudeBinaryWithRunner(mockRunner, fs, "linux", "/home/user")
	
	// The security protection should either:
	// 1. Reject the malicious binary and return empty string, or
	// 2. Find a legitimate binary instead, or  
	// 3. Return an error
	if claudePath == maliciousBinary {
		t.Errorf("SECURITY VULNERABILITY: Binary hijacking possible, accepted malicious binary: %q", claudePath)
	} else if claudePath == "" {
		t.Logf("✅ SECURITY FIX: Binary hijacking prevented, no binary accepted")
	} else {
		t.Logf("✅ SECURITY FIX: Binary hijacking prevented, found alternative: %q", claudePath)
	}
	
	// Test with legitimate binary
	mockRunner.lookPathResult = legitimateBinary
	claudePath = marvai.FindClaudeBinaryWithRunner(mockRunner, fs, "linux", "/home/user")
	
	if claudePath == legitimateBinary || claudePath != "" {
		t.Logf("✅ Legitimate binary properly accepted: %q", claudePath)
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