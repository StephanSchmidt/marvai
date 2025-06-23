package internal

import (
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