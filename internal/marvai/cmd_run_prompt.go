package marvai

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/spf13/afero"
)

// RunWithPrompt executes the specified CLI tool with a prompt using OS defaults
func RunWithPrompt(fs afero.Fs, promptName string, cliTool string) error {
	return RunWithPromptAndRunner(fs, promptName, cliTool, OSCommandRunner{}, os.Stdout, os.Stderr)
}

// RunWithPromptAndRunner executes the specified CLI tool with a prompt using dependency injection for testing
func RunWithPromptAndRunner(fs afero.Fs, promptName string, cliTool string, runner CommandRunner, stdout, stderr io.Writer) error {
	content, err := LoadPrompt(fs, promptName)
	if err != nil {
		// Log failed execution
		LogPromptExecution(fs, promptName, cliTool, false)
		return fmt.Errorf("error reading file: %w", err)
	}

	cliPath := FindCliBinary(cliTool)

	var cmd *exec.Cmd
	if cliTool == "codex" {
		// For codex, pass the prompt as a command-line argument
		cmd = runner.Command(cliPath, string(content))
		cmd.Stdout = stdout
		cmd.Stderr = stderr
		// For codex, just run the command directly since prompt is passed as argument
		err := cmd.Run()
		if err != nil {
			// Log failed execution
			LogPromptExecution(fs, promptName, cliTool, false)
			return err
		}
		// Log successful execution
		LogPromptExecution(fs, promptName, cliTool, true)
		return nil
	} else {
		// For claude and gemini, use stdin
		cmd = runner.Command(cliPath)
	}

	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// For claude and gemini, use stdin
	stdin, err := cmd.StdinPipe()
	if err != nil {
		// Log failed execution
		LogPromptExecution(fs, promptName, cliTool, false)
		return fmt.Errorf("error creating stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		stdin.Close() // Clean up stdin pipe if command fails to start
		// Log failed execution
		LogPromptExecution(fs, promptName, cliTool, false)
		return fmt.Errorf("error starting %s: %w", cliTool, err)
	}

	// Write content to stdin in a goroutine with proper synchronization
	done := make(chan error, 1)
	go func() {
		defer stdin.Close()
		_, writeErr := stdin.Write(content)
		if writeErr == nil {
			// Send /exit command to terminate CLI tool after processing the prompt
			// Note: This works for Claude, other tools may need different exit commands
			if cliTool == "claude" {
				_, writeErr = stdin.Write([]byte("\n/exit\n"))
			} else {
				// For other tools, just close stdin to signal end of input
				// Individual tools may require different exit strategies
			}
		}
		done <- writeErr
	}()

	// Wait for both the write goroutine and command to complete
	var writeErr error
	select {
	case writeErr = <-done:
		// Write completed, now wait for command
	case <-time.After(10 * time.Second):
		// Timeout waiting for write to complete
		// Log failed execution
		LogPromptExecution(fs, promptName, cliTool, false)
		return fmt.Errorf("timeout waiting for stdin write to complete")
	}

	// Wait for command to complete
	waitErr := cmd.Wait()

	// Return appropriate error
	if writeErr != nil && waitErr == nil {
		// Log failed execution
		LogPromptExecution(fs, promptName, cliTool, false)
		return fmt.Errorf("error writing to %s stdin: %w", cliTool, writeErr)
	}

	if waitErr != nil {
		// Log failed execution
		LogPromptExecution(fs, promptName, cliTool, false)
		return fmt.Errorf("error running %s: %w", cliTool, waitErr)
	}

	// Log successful execution
	LogPromptExecution(fs, promptName, cliTool, true)
	return nil
}