package source

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/afero"
)

// SourceHandler defines the interface for loading .mprompt files from different sources
type SourceHandler interface {
	// LoadContent loads the .mprompt file content from the source
	LoadContent(source string) ([]byte, error)
	
	// CanHandle returns true if this handler can process the given source
	CanHandle(source string) bool
	
	// GetDisplayName returns a human-readable name for the source (for logging/errors)
	GetDisplayName(source string) string
}

// FileHandler handles local file sources
type FileHandler struct {
	fs afero.Fs
}

// NewFileHandler creates a new file handler
func NewFileHandler(fs afero.Fs) *FileHandler {
	return &FileHandler{fs: fs}
}

// CanHandle returns true for non-URL sources (local files)
func (h *FileHandler) CanHandle(source string) bool {
	// If it's not a URL, we assume it's a local file
	_, err := url.Parse(source)
	if err != nil {
		return true // Parse error likely means it's not a URL
	}
	
	// Check if it has a scheme (http/https)
	parsed, err := url.Parse(source)
	if err != nil {
		return true
	}
	
	return parsed.Scheme == ""
}

// LoadContent loads content from a local file
func (h *FileHandler) LoadContent(source string) ([]byte, error) {
	// Add .mprompt extension if not present
	filename := source
	if !strings.HasSuffix(filename, ".mprompt") {
		filename += ".mprompt"
	}
	
	content, err := afero.ReadFile(h.fs, filename)
	if err != nil {
		return nil, fmt.Errorf("error reading local file %s: %w", filename, err)
	}
	
	return content, nil
}

// GetDisplayName returns the filename for display
func (h *FileHandler) GetDisplayName(source string) string {
	filename := source
	if !strings.HasSuffix(filename, ".mprompt") {
		filename += ".mprompt"
	}
	return filename
}

// HTTPSHandler handles HTTPS URL sources
type HTTPSHandler struct {
	client  *http.Client
	timeout time.Duration
}

// NewHTTPSHandler creates a new HTTPS handler with optional timeout
func NewHTTPSHandler(timeout time.Duration) *HTTPSHandler {
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}
	
	return &HTTPSHandler{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// CanHandle returns true for HTTPS URLs
func (h *HTTPSHandler) CanHandle(source string) bool {
	parsed, err := url.Parse(source)
	if err != nil {
		return false
	}
	
	return parsed.Scheme == "https"
}

// LoadContent downloads content from an HTTPS URL
func (h *HTTPSHandler) LoadContent(source string) ([]byte, error) {
	// Validate URL
	parsed, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}
	
	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("only HTTPS URLs are supported for security, got: %s", parsed.Scheme)
	}
	
	// Make HTTP request
	resp, err := h.client.Get(source)
	if err != nil {
		return nil, fmt.Errorf("error downloading from %s: %w", source, err)
	}
	defer resp.Body.Close()
	
	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d when downloading from %s", resp.StatusCode, source)
	}
	
	// Check content type (optional, but helpful)
	contentType := resp.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "text/") && !strings.Contains(contentType, "application/octet-stream") {
		// Warning but not error - some servers don't set proper content types
		fmt.Printf("Warning: Unexpected content type %s for .mprompt file\n", contentType)
	}
	
	// Read response body with size limit
	const maxSize = 10 * 1024 * 1024 // 10MB limit matching ParseMPrompt
	limitReader := io.LimitReader(resp.Body, maxSize+1)
	content, err := io.ReadAll(limitReader)
	if err != nil {
		return nil, fmt.Errorf("error reading response from %s: %w", source, err)
	}
	
	// Check size limit
	if len(content) > maxSize {
		return nil, fmt.Errorf("downloaded file too large (%d bytes), maximum allowed is %d bytes", len(content), maxSize)
	}
	
	return content, nil
}

// GetDisplayName returns the URL for display
func (h *HTTPSHandler) GetDisplayName(source string) string {
	return source
}

// SourceManager manages multiple source handlers
type SourceManager struct {
	handlers []SourceHandler
}

// NewSourceManager creates a new source manager with default handlers
func NewSourceManager(fs afero.Fs) *SourceManager {
	return &SourceManager{
		handlers: []SourceHandler{
			NewHTTPSHandler(30 * time.Second),
			NewFileHandler(fs), // File handler should be last (fallback)
		},
	}
}

// AddHandler adds a custom handler to the manager
func (sm *SourceManager) AddHandler(handler SourceHandler) {
	sm.handlers = append([]SourceHandler{handler}, sm.handlers...) // Prepend to give priority
}

// LoadContent attempts to load content using the appropriate handler
func (sm *SourceManager) LoadContent(source string) ([]byte, string, error) {
	for _, handler := range sm.handlers {
		if handler.CanHandle(source) {
			content, err := handler.LoadContent(source)
			if err != nil {
				return nil, "", err
			}
			return content, handler.GetDisplayName(source), nil
		}
	}
	
	return nil, "", fmt.Errorf("no handler found for source: %s", source)
}