package marvai

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/afero"
)

// LogAction represents the type of action being logged
type LogAction string

const (
	LogActionInstallPrompt LogAction = "INSTALL_PROMPT"
	LogActionExecutePrompt LogAction = "EXECUTE_PROMPT"
)

// LogEntry represents a single log entry
type LogEntry struct {
	Timestamp   time.Time
	Action      LogAction
	PromptName  string
	Details     string
}

// LogToMarvaiLog writes a log entry to the marvai.log file in the .marvai directory
func LogToMarvaiLog(fs afero.Fs, action LogAction, promptName string, details string) error {
	// Get the .marvai directory path
	marvaiDir := ".marvai"
	
	// Ensure .marvai directory exists
	if err := fs.MkdirAll(marvaiDir, 0755); err != nil {
		return fmt.Errorf("failed to create .marvai directory: %w", err)
	}
	
	// Create log file path
	logPath := filepath.Join(marvaiDir, "marvai.log")
	
	// Open log file in append mode, create if not exists
	file, err := fs.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer file.Close()
	
	// Create log entry
	entry := LogEntry{
		Timestamp:  time.Now(),
		Action:     action,
		PromptName: promptName,
		Details:    details,
	}
	
	// Format log entry
	logLine := fmt.Sprintf("[%s] %s: %s - %s\n",
		entry.Timestamp.Format("2006-01-02 15:04:05"),
		string(entry.Action),
		entry.PromptName,
		entry.Details,
	)
	
	// Write to log file
	if _, err := file.WriteString(logLine); err != nil {
		return fmt.Errorf("failed to write to log file: %w", err)
	}
	
	return nil
}

// LogPromptInstall logs a prompt installation event
func LogPromptInstall(fs afero.Fs, promptName string, repo string, success bool) error {
	var details string
	if success {
		if repo != "" {
			details = fmt.Sprintf("Successfully installed from repo: %s", repo)
		} else {
			details = "Successfully installed from default repo"
		}
	} else {
		details = "Installation failed"
	}
	
	return LogToMarvaiLog(fs, LogActionInstallPrompt, promptName, details)
}

// LogPromptExecution logs a prompt execution event
func LogPromptExecution(fs afero.Fs, promptName string, cliTool string, success bool) error {
	var details string
	if success {
		details = fmt.Sprintf("Successfully executed with %s", cliTool)
	} else {
		details = fmt.Sprintf("Execution failed with %s", cliTool)
	}
	
	return LogToMarvaiLog(fs, LogActionExecutePrompt, promptName, details)
}