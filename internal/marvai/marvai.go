package marvai

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/aymerick/raymond"
	"github.com/spf13/afero"
	"gopkg.in/yaml.v3"
)

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

// FindClaudeBinaryWithRunner finds the Claude binary using dependency injection for testing
func FindClaudeBinaryWithRunner(runner CommandRunner, fs afero.Fs, goos string, homeDir string) string {
	// Try to find claude in PATH first
	if path, err := runner.LookPath("claude"); err == nil {
		return path
	}

	// Common installation paths by OS
	var candidatePaths []string
	
	switch goos {
	case "darwin":
		candidatePaths = []string{
			"/usr/local/bin/claude",
			"/opt/homebrew/bin/claude",
			filepath.Join(homeDir, ".local", "bin", "claude"),
			"/Applications/Claude.app/Contents/MacOS/claude",
		}
	default: // linux and others
		candidatePaths = []string{
			"/usr/local/bin/claude",
			"/usr/bin/claude",
			filepath.Join(homeDir, ".local", "bin", "claude"),
			filepath.Join(homeDir, "bin", "claude"),
		}
	}

	// Check each candidate path
	for _, path := range candidatePaths {
		if _, err := fs.Stat(path); err == nil {
			return path
		}
	}

	// Fallback to just "claude" if nothing found
	return "claude"
}

// FindClaudeBinary finds the Claude binary using OS defaults
func FindClaudeBinary() string {
	homeDir, _ := os.UserHomeDir()
	return FindClaudeBinaryWithRunner(OSCommandRunner{}, afero.NewOsFs(), runtime.GOOS, homeDir)
}

// LoadPrompt loads a prompt file from the .marvai directory
func LoadPrompt(fs afero.Fs, promptName string) ([]byte, error) {
	promptFile := filepath.Join(".marvai", promptName+".prompt")
	return afero.ReadFile(fs, promptFile)
}

// RunWithPromptAndRunner executes Claude with a prompt using dependency injection for testing
func RunWithPromptAndRunner(fs afero.Fs, promptName string, runner CommandRunner, stdout, stderr io.Writer) error {
	content, err := LoadPrompt(fs, promptName)
	if err != nil {
		return fmt.Errorf("error reading file: %w", err)
	}

	claudePath := FindClaudeBinary()
	cmd := runner.Command(claudePath)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error creating stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error starting claude: %w", err)
	}

	go func() {
		defer stdin.Close()
		stdin.Write(content)
	}()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("error running claude: %w", err)
	}

	return nil
}

// RunWithPrompt executes Claude with a prompt using OS defaults
func RunWithPrompt(fs afero.Fs, promptName string) error {
	return RunWithPromptAndRunner(fs, promptName, OSCommandRunner{}, os.Stdout, os.Stderr)
}

// WizardVariable represents a variable in the wizard section
type WizardVariable struct {
	ID       string `yaml:"id"`
	Question string `yaml:"question"`
	Type     string `yaml:"type"`
	Required bool   `yaml:"required"`
}

// MPromptData represents the parsed .mprompt file
type MPromptData struct {
	Variables []WizardVariable
	Template  string
}

// ParseMPrompt parses a .mprompt file and separates wizard and template sections
func ParseMPrompt(fs afero.Fs, filename string) (*MPromptData, error) {
	content, err := afero.ReadFile(fs, filename)
	if err != nil {
		return nil, fmt.Errorf("error reading .mprompt file: %w", err)
	}

	lines := strings.Split(string(content), "\n")
	var wizardLines []string
	var templateLines []string
	var inTemplate bool

	for _, line := range lines {
		if strings.TrimSpace(line) == "--" {
			inTemplate = true
			continue
		}
		
		if inTemplate {
			templateLines = append(templateLines, line)
		} else {
			wizardLines = append(wizardLines, line)
		}
	}

	var variables []WizardVariable
	if len(wizardLines) > 0 {
		wizardYaml := strings.Join(wizardLines, "\n")
		if err := yaml.Unmarshal([]byte(wizardYaml), &variables); err != nil {
			return nil, fmt.Errorf("error parsing wizard YAML: %w", err)
		}
	}

	template := strings.Join(templateLines, "\n")
	template = strings.TrimSpace(template)

	return &MPromptData{
		Variables: variables,
		Template:  template,
	}, nil
}

// ExecuteWizard prompts the user for variable values
func ExecuteWizard(variables []WizardVariable) (map[string]string, error) {
	values := make(map[string]string)
	scanner := bufio.NewScanner(os.Stdin)

	for _, variable := range variables {
		fmt.Print(variable.Question + " ")
		if scanner.Scan() {
			response := strings.TrimSpace(scanner.Text())
			if variable.Required && response == "" {
				return nil, fmt.Errorf("variable '%s' is required", variable.ID)
			}
			values[variable.ID] = response
		} else {
			return nil, fmt.Errorf("error reading input for variable '%s'", variable.ID)
		}
	}

	return values, nil
}

// SubstituteVariables uses Handlebars templating to replace variables
func SubstituteVariables(template string, values map[string]string) (string, error) {
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

	result, err := raymond.Render(template, values)
	if err != nil {
		return "", fmt.Errorf("error rendering template: %w", err)
	}
	return result, nil
}

// InstallMPrompt processes a .mprompt file and creates a .prompt file
func InstallMPrompt(fs afero.Fs, mpromptName string) error {
	mpromptFile := mpromptName + ".mprompt"
	
	data, err := ParseMPrompt(fs, mpromptFile)
	if err != nil {
		return err
	}

	values, err := ExecuteWizard(data.Variables)
	if err != nil {
		return err
	}

	finalPrompt, err := SubstituteVariables(data.Template, values)
	if err != nil {
		return err
	}

	if err := fs.MkdirAll(".marvai", 0755); err != nil {
		return fmt.Errorf("error creating .marvai directory: %w", err)
	}

	promptFile := filepath.Join(".marvai", mpromptName+".prompt")
	if err := afero.WriteFile(fs, promptFile, []byte(finalPrompt), 0644); err != nil {
		return fmt.Errorf("error writing .prompt file: %w", err)
	}

	fmt.Printf("Created %s from %s\n", promptFile, mpromptFile)
	return nil
}

// Run executes the main application logic
func Run(args []string, fs afero.Fs, stderr io.Writer) error {
	if len(args) < 2 {
		fmt.Fprintf(stderr, "Usage: %s <command> [args...]\n", args[0])
		fmt.Fprintf(stderr, "Commands:\n")
		fmt.Fprintf(stderr, "  prompt <name>   - Execute a prompt\n")
		fmt.Fprintf(stderr, "  install <name>  - Install a .mprompt file\n")
		return fmt.Errorf("insufficient arguments")
	}

	command := args[1]
	
	switch command {
	case "prompt":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "Usage: %s prompt <prompt-name>\n", args[0])
			return fmt.Errorf("prompt name required")
		}
		promptName := args[2]
		return RunWithPrompt(fs, promptName)
		
	case "install":
		if len(args) < 3 {
			fmt.Fprintf(stderr, "Usage: %s install <mprompt-name>\n", args[0])
			return fmt.Errorf("mprompt name required")
		}
		mpromptName := args[2]
		return InstallMPrompt(fs, mpromptName)
		
	default:
		// Backward compatibility: if no command specified, treat as prompt
		promptName := args[1]
		return RunWithPrompt(fs, promptName)
	}
}