package internal

import (
	"fmt"
	"strings"
	"sync"

	"github.com/aymerick/raymond"
)

var (
	helpersRegistered bool
	helpersMutex      sync.Mutex
)

// RegisterHelpers registers custom Handlebars helpers (only once)
func RegisterHelpers() {
	helpersMutex.Lock()
	defer helpersMutex.Unlock()

	if helpersRegistered {
		return
	}

	// Register helpful custom helpers
	raymond.RegisterHelper("split", func(str string, separator string) []string {
		if str == "" {
			return []string{}
		}
		parts := strings.Split(str, separator)
		var result []string
		for _, part := range parts {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				result = append(result, trimmed)
			}
		}
		return result
	})

	helpersRegistered = true
}

// RenderTemplate renders a Handlebars template with the given variables with security controls
func RenderTemplate(template string, values map[string]string) (string, error) {
	// SECURITY: Validate template before rendering
	if err := validateTemplate(template); err != nil {
		return "", fmt.Errorf("template security validation failed: %w", err)
	}

	// SECURITY: Sanitize template values
	sanitizedValues := sanitizeTemplateValues(values)

	RegisterHelpers()

	result, err := raymond.Render(template, sanitizedValues)
	if err != nil {
		return "", fmt.Errorf("error rendering template: %w", err)
	}

	// SECURITY: Validate output size to prevent memory exhaustion
	if len(result) > 10*1024*1024 { // 10MB limit
		return "", fmt.Errorf("template output too large (%d bytes), possible DoS attempt", len(result))
	}

	return result, nil
}

// validateTemplate performs security validation on template content
func validateTemplate(template string) error {
	// SECURITY: Check template size to prevent memory exhaustion
	if len(template) > 1024*1024 { // 1MB limit
		return fmt.Errorf("template too large (%d bytes), maximum allowed is 1MB", len(template))
	}

	// SECURITY: Check for deeply nested constructs that could cause DoS
	nestedLevel := 0
	maxNested := 50 // Reasonable limit

	for i := 0; i < len(template); i++ {
		// Ensure we have enough characters left for the pattern
		if i+2 < len(template) && template[i:i+3] == "{{#" {
			nestedLevel++
			if nestedLevel > maxNested {
				return fmt.Errorf("template has too many nested constructs (%d), maximum allowed is %d",
					nestedLevel, maxNested)
			}
		} else if i+2 < len(template) && template[i:i+3] == "{{/" {
			// Prevent negative nesting levels
			if nestedLevel > 0 {
				nestedLevel--
			}
		}
	}

	// SECURITY: Block dangerous helpers and patterns
	dangerousPatterns := []string{
		"{{>",         // Block partials
		"constructor", // Block constructor access
		"__proto__",   // Block prototype access
		"prototype",   // Block prototype access
		"toString",    // Block toString access (potential info leak)
	}

	for _, pattern := range dangerousPatterns {
		if strings.Contains(template, pattern) {
			return fmt.Errorf("template contains dangerous pattern: %q", pattern)
		}
	}

	return nil
}

// sanitizeTemplateValues sanitizes user input values to prevent injection
func sanitizeTemplateValues(values map[string]string) map[string]string {
	sanitized := make(map[string]string)

	for key, value := range values {
		// SECURITY: Validate variable names
		if !isValidVariableName(key) {
			continue // Skip dangerous variable names
		}

		// SECURITY: Limit value size
		if len(value) > 100*1024 { // 100KB per value
			value = value[:100*1024] + "...[truncated]"
		}

		// SECURITY: Remove or escape potentially dangerous characters
		value = sanitizeValue(value)

		sanitized[key] = value
	}

	return sanitized
}

// isValidVariableName checks if a variable name is safe
func isValidVariableName(name string) bool {
	if name == "" {
		return false
	}

	// SECURITY: Block dangerous variable names
	dangerousNames := []string{
		"__proto__", "constructor", "prototype", "toString", "valueOf",
	}

	for _, dangerous := range dangerousNames {
		if name == dangerous {
			return false
		}
	}

	// SECURITY: Only allow alphanumeric, underscore, and hyphen
	for _, r := range name {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') && r != '_' && r != '-' {
			return false
		}
	}

	return true
}

// sanitizeValue removes or escapes potentially dangerous content from values
func sanitizeValue(value string) string {
	// SECURITY: Remove null bytes
	value = strings.ReplaceAll(value, "\x00", "")

	// SECURITY: Remove other control characters except common whitespace
	var result strings.Builder
	for _, r := range value {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' || r >= 32 {
			result.WriteRune(r)
		}
	}

	return result.String()
}
