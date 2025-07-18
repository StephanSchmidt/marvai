package marvai

import (
	"os/exec"

	"github.com/spf13/afero"
)

// isGitRepository checks if the current directory is a valid git repository
// by verifying both the presence of .git and that git commands work
func isGitRepository(fs afero.Fs, runner CommandRunner) bool {
	// First check if .git exists (could be file for worktrees or directory)
	gitPath := ".git"
	if exists, err := afero.Exists(fs, gitPath); err != nil || !exists {
		return false
	}

	// Check if git is available
	if _, err := runner.LookPath("git"); err != nil {
		return false
	}

	// Try to run git rev-parse --git-dir to verify it's a valid git repo
	cmd := runner.Command("git", "rev-parse", "--git-dir")
	if err := cmd.Run(); err != nil {
		return false
	}

	// Additional check: try to get the current branch or commit
	cmd = runner.Command("git", "rev-parse", "--verify", "HEAD")
	if err := cmd.Run(); err != nil {
		// This might fail for a fresh repo with no commits, so check if we're in a git repo another way
		cmd = runner.Command("git", "status", "--porcelain")
		if err := cmd.Run(); err != nil {
			return false
		}
	}

	return true
}

// CommandRunner interface for abstracting command execution
type CommandRunner interface {
	Command(name string, arg ...string) *exec.Cmd
	LookPath(file string) (string, error)
}

// OSCommandRunner implements CommandRunner using real OS commands
type OSCommandRunner struct{}

func (o OSCommandRunner) Command(name string, arg ...string) *exec.Cmd {
	return exec.Command(name, arg...)
}

func (o OSCommandRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}
