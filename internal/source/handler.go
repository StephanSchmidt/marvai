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

// HTTPSHandler handles GitHub URL sources
type HTTPSHandler struct {
	client  *http.Client
	timeout time.Duration
}

// NewHTTPSHandler creates a new GitHub URL handler with optional timeout
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

// CanHandle returns true for GitHub URLs
func (h *HTTPSHandler) CanHandle(source string) bool {
	parsed, err := url.Parse(source)
	if err != nil {
		return false
	}

	return parsed.Scheme == "https" && strings.HasPrefix(parsed.Host, "github.com")
}

// MarvaiHandler handles marvai.dev URL sources
type MarvaiHandler struct {
	client  *http.Client
	timeout time.Duration
}

// NewMarvaiHandler creates a new marvai.dev URL handler with optional timeout
func NewMarvaiHandler(timeout time.Duration) *MarvaiHandler {
	if timeout == 0 {
		timeout = 30 * time.Second // Default timeout
	}

	return &MarvaiHandler{
		client: &http.Client{
			Timeout: timeout,
		},
		timeout: timeout,
	}
}

// CanHandle returns true for marvai.dev URLs
func (h *MarvaiHandler) CanHandle(source string) bool {
	parsed, err := url.Parse(source)
	if err != nil {
		return false
	}

	return parsed.Scheme == "https" && strings.HasSuffix(parsed.Host, "marvai.dev")
}

// LoadContent downloads content from a marvai.dev URL
func (h *MarvaiHandler) LoadContent(source string) ([]byte, error) {
	// Validate URL
	parsed, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("only HTTPS URLs are supported for security, got: %s", parsed.Scheme)
	}

	if !strings.HasSuffix(parsed.Host, "marvai.dev") {
		return nil, fmt.Errorf("only marvai.dev URLs are supported, got: %s", parsed.Host)
	}

	// Make HTTP request directly to the URL
	resp, err := h.client.Get(source)
	if err != nil {
		return nil, fmt.Errorf("error downloading from %s: %w", source, err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d when downloading from %s", resp.StatusCode, source)
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
func (h *MarvaiHandler) GetDisplayName(source string) string {
	return source
}

// LoadContent downloads content from a GitHub URL
func (h *HTTPSHandler) LoadContent(source string) ([]byte, error) {
	// Validate URL
	parsed, err := url.Parse(source)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	if parsed.Scheme != "https" {
		return nil, fmt.Errorf("only HTTPS URLs are supported for security, got: %s", parsed.Scheme)
	}

	if !strings.HasPrefix(parsed.Host, "github.com") {
		return nil, fmt.Errorf("only GitHub URLs are supported, got: %s", parsed.Host)
	}

	// Transform GitHub URL to raw.githubusercontent.com
	rawURL, err := h.transformToRawURL(source)
	if err != nil {
		return nil, fmt.Errorf("failed to transform GitHub URL: %w", err)
	}

	// Make HTTP request using the raw URL
	resp, err := h.client.Get(rawURL)
	if err != nil {
		return nil, fmt.Errorf("error downloading from %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP error %d when downloading from %s", resp.StatusCode, rawURL)
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
		return nil, fmt.Errorf("error reading response from %s: %w", rawURL, err)
	}

	// Check size limit
	if len(content) > maxSize {
		return nil, fmt.Errorf("downloaded file too large (%d bytes), maximum allowed is %d bytes", len(content), maxSize)
	}

	return content, nil
}

// transformToRawURL converts a GitHub URL to raw.githubusercontent.com format
func (h *HTTPSHandler) transformToRawURL(githubURL string) (string, error) {
	parsed, err := url.Parse(githubURL)
	if err != nil {
		return "", fmt.Errorf("invalid GitHub URL: %w", err)
	}

	// Extract path components: /owner/repo/...
	pathParts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(pathParts) < 2 {
		return "", fmt.Errorf("invalid GitHub URL format: expected /owner/repo/..., got: %s", parsed.Path)
	}

	owner := pathParts[0]
	repo := pathParts[1]

	// Handle different GitHub URL formats
	var filePath string
	if len(pathParts) == 2 {
		// Simple format: https://github.com/owner/repo
		// Default to main branch and .mprompt file with repo name
		filePath = fmt.Sprintf("main/%s.mprompt", repo)
	} else if len(pathParts) >= 4 && pathParts[2] == "blob" {
		// Full format: https://github.com/owner/repo/blob/branch/path/to/file
		branch := pathParts[3]
		remainingPath := strings.Join(pathParts[4:], "/")
		if remainingPath == "" {
			// No file specified, use repo name with .mprompt extension
			remainingPath = fmt.Sprintf("%s.mprompt", repo)
		} else if !strings.HasSuffix(remainingPath, ".mprompt") {
			// Add .mprompt extension if not present
			remainingPath += ".mprompt"
		}
		filePath = fmt.Sprintf("%s/%s", branch, remainingPath)
	} else {
		// Other formats: treat remaining path as file path, default to main branch
		remainingPath := strings.Join(pathParts[2:], "/")
		if remainingPath == "" {
			remainingPath = fmt.Sprintf("%s.mprompt", repo)
		} else if !strings.HasSuffix(remainingPath, ".mprompt") {
			remainingPath += ".mprompt"
		}
		filePath = fmt.Sprintf("main/%s", remainingPath)
	}

	// Construct raw.githubusercontent.com URL
	rawURL := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", owner, repo, filePath)
	return rawURL, nil
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
			NewMarvaiHandler(30 * time.Second),
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
