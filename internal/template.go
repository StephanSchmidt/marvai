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

// RenderTemplate renders a Handlebars template with the given variables
func RenderTemplate(template string, values map[string]string) (string, error) {
	RegisterHelpers()
	
	result, err := raymond.Render(template, values)
	if err != nil {
		return "", fmt.Errorf("error rendering template: %w", err)
	}
	return result, nil
}