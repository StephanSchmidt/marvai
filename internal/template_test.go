package internal

import (
	"strings"
	"testing"
)

func TestRenderTemplate(t *testing.T) {
	tests := []struct {
		name     string
		template string
		values   map[string]string
		expected string
	}{
		{
			name:     "simple variable substitution",
			template: "Hello {{name}}!",
			values:   map[string]string{"name": "World"},
			expected: "Hello World!",
		},
		{
			name:     "multiple variables",
			template: "{{greeting}} {{name}}, you have {{count}} messages",
			values: map[string]string{
				"greeting": "Hello",
				"name":     "Alice",
				"count":    "5",
			},
			expected: "Hello Alice, you have 5 messages",
		},
		{
			name:     "conditional with if helper",
			template: "{{#if show}}This is visible{{/if}}",
			values:   map[string]string{"show": "true"},
			expected: "This is visible",
		},
		{
			name:     "conditional with else",
			template: "{{#if show}}Visible{{else}}Hidden{{/if}}",
			values:   map[string]string{"show": ""},
			expected: "Hidden",
		},
		{
			name:     "split helper with comma-separated values",
			template: "{{#each (split items \",\")}}Item: {{this}}\n{{/each}}",
			values:   map[string]string{"items": "apple, banana, orange"},
			expected: "Item: apple\nItem: banana\nItem: orange\n",
		},
		{
			name:     "split helper with empty string",
			template: "{{#each (split items \",\")}}Item: {{this}}\n{{/each}}",
			values:   map[string]string{"items": ""},
			expected: "",
		},
		{
			name:     "split helper with whitespace trimming",
			template: "{{#each (split items \",\")}}{{this}}|{{/each}}",
			values:   map[string]string{"items": " first , second , third "},
			expected: "first|second|third|",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.values)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestRenderTemplateErrors(t *testing.T) {
	tests := []struct {
		name     string
		template string
		values   map[string]string
	}{
		{
			name:     "invalid template syntax",
			template: "{{#if unclosed",
			values:   map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RenderTemplate(tt.template, tt.values)
			if err == nil {
				t.Error("Expected error but got none")
			}
		})
	}
}

func TestRegisterHelpers(t *testing.T) {
	// Test that helpers are registered correctly
	RegisterHelpers()

	// Test split helper directly through a template
	template := "{{#each (split \"a,b,c\" \",\")}}{{this}}-{{/each}}"
	values := map[string]string{}

	result, err := RenderTemplate(template, values)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}

	expected := "a-b-c-"
	if result != expected {
		t.Errorf("Expected %q, got %q", expected, result)
	}
}

// TestRenderTemplateSecurityIssues tests for template security vulnerabilities
func TestRenderTemplateSecurityIssues(t *testing.T) {
	tests := []struct {
		name        string
		template    string
		values      map[string]string
		expectError bool
		description string
	}{
		{
			name:        "deeply nested template",
			template:    strings.Repeat("{{#if true}}", 51) + "deep" + strings.Repeat("{{/if}}", 51),
			values:      map[string]string{},
			expectError: true,
			description: "Should reject deeply nested templates that exceed nesting limit",
		},
		{
			name:        "recursive template attempt",
			template:    "{{> recursive}}",
			values:      map[string]string{},
			expectError: true,
			description: "Should reject recursive template references",
		},
		{
			name:        "malicious variable names",
			template:    "{{__proto__}} {{constructor}} {{toString}}",
			values:      map[string]string{"__proto__": "proto", "constructor": "ctor", "toString": "str"},
			expectError: true,
			description: "Should reject templates with dangerous variable names",
		},
		{
			name:        "extremely long variable content",
			template:    "{{content}}",
			values:      map[string]string{"content": strings.Repeat("A", 1000000)},
			expectError: false,
			description: "Should handle large variable content",
		},
		{
			name:        "template with null bytes",
			template:    "Hello {{name\x00}}",
			values:      map[string]string{"name": "test"},
			expectError: false, // Handlebars library doesn't reject null bytes, which is okay
			description: "Should handle null bytes in templates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.values)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none for template: %q", tt.template)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				// For non-error cases, just check that we got some result
				if len(result) == 0 && len(tt.template) > 0 {
					t.Errorf("Expected non-empty result for template: %q", tt.template)
				}
			}
		})
	}
}

// TestSplitHelperEdgeCases tests edge cases for the split helper
func TestSplitHelperEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		template string
		values   map[string]string
		expected string
	}{
		{
			name:     "split with very long separator",
			template: "{{#each (split items \"=====\")}}{{this}}|{{/each}}",
			values:   map[string]string{"items": "a=====b=====c"},
			expected: "a|b|c|",
		},
		{
			name:     "split with special regex characters",
			template: "{{#each (split items \".*\")}}{{this}}|{{/each}}",
			values:   map[string]string{"items": "a.*b.*c"},
			expected: "a|b|c|",
		},
		{
			name:     "split with unicode separator",
			template: "{{#each (split items \"🌟\")}}{{this}}|{{/each}}",
			values:   map[string]string{"items": "hello🌟world🌟test"},
			expected: "hello|world|test|",
		},
		{
			name:     "split with null byte in string",
			template: "{{#each (split items \",\")}}{{this}}|{{/each}}",
			values:   map[string]string{"items": "a\x00,b,c"},
			expected: "a|b|c|",
		},
		{
			name:     "split extremely long input",
			template: "{{#each (split items \",\")}}x{{/each}}",
			values:   map[string]string{"items": strings.Repeat("a,", 10000) + "b"},
			expected: strings.Repeat("x", 10001),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := RenderTemplate(tt.template, tt.values)
			if err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}
