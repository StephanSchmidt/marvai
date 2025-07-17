
<img src="https://raw.githubusercontent.com/marvai-dev/marvai/main/marvai-logo-square.png" alt="Marvai" width="50%">

# MarvAI

**----- NOT SAFE FOR PRODUCTION -----**

A powerful CLI tool that changes how you work with AI prompts in your development workflow.
Think *NPM for prompts* - discover, install, and execute battle-tested AI prompts from the MarvAI 
distribution. Transform your development process with ready-to-use prompts 
that automate code reviews, security scans, refactoring, and more.

**Supported AI Tools:**
- **Claude Code** (default) - The official Claude CLI
- **Gemini** - Google's Gemini CLI
- **Codex** - OpenAI's Codex CLI

**Why MarvAI?**
- 🚀 **Instant productivity**: Skip writing prompts from scratch - use proven and optimized templates
- 📦 **Ecosystem-driven**: Tap into a growing library of community-contributed prompts
- 🔧 **Zero configuration**: Install and run prompts with a single command
- 🎯 **Purpose-built**: Designed specifically for developers who want AI automation without the hassle - **AI that works for you**

Examples:
* Security Scanning
* Bug finding
* Dependency upgrades
* Programming language upgrades 
* Architecture checks and recommendations
* Scaling checks

automated with Claude Code prompts.

## Features 

- **Find prompts for your problems**: Find a prompt in the Marvai repository
- **Manage your prompt library**: Install, update, and organize prompts locally
- **Execute prompts seamlessly**: Run AI prompts with Claude Code, Gemini, or Codex integration

<img src="https://raw.githubusercontent.com/marvai-dev/marvai/main/marvai-demo.gif" alt="Demo Marvai" width="70%">

## Install

With Brew:
```bash
brew install marvai-dev/marvai/marvai
```

With Go:
```bash
go install github.com/marvai-dev/marvai@latest
```

## 'Hello World' Example

```bash
$ marvai list 
...
$ marvai install helloworld
# will ask you the language
For what language? Go
$ marvai prompt helloworld 
# calls claude code with the hello world prompt and generate the hello world
# in the specified language

# Or use with different AI tools
$ marvai --cli gemini prompt helloworld
$ marvai --cli codex prompt helloworld
```

## Commands

### `marvai install <source>`

Install a `.mprompt` file by running its wizard and generating configuration files.

**Note:** Prompts can only be installed in git repositories for security reasons.

```bash
> marvai install helloworld
```

This will:
1. Read the `.mprompt` file from the marvai registry (remote)
2. Execute the wizard, prompting for variable values
3. Generate `.marvai/<name>.mprompt` and `.marvai/<name>.var` files

Install from a different repository:

```bash
> marvai install otherrepo/otherprompt
```

### `marvai prompt <name>`

Execute a previously installed prompt with Claude Code.

```bash
$ marvai prompt example
```

This will run the templated prompt through Claude Code. You can also specify a different AI tool:

```bash
$ marvai --cli gemini prompt example
$ marvai --cli codex prompt example
```

### `marvai list [repo]`

List available prompts from the remote registry.

```bash
$ marvai list
✨ Found 3 prompts available:
  Hello World v1.0 - A simple hello world example (by Stephan Schmidt) [hello.mprompt]
  Security Audit v2.1 - Security analysis prompt (by Stephan Schmidt) [security.mprompt]
```

List prompts from a specific repository:

```bash
$ marvai list otherrepo
```

### `marvai installed`

List installed prompts in the `.marvai` directory.

```bash
$ marvai installed
Found 1 installed prompt:
  Example v0.4 - An example (by Stephan Schmidt) (configured)
```

### `marvai version`

Show version information.

```bash
$ marvai version
marvai version 1.0.0
```


# Features For Prompt Developers

- **Interactive Wizards**: Define variables with questions to prompt users for input
- **Templates**: Use powerful templating with `{{variablename}}` placeholders
- **AI CLI Integration**: Execute generated prompts directly with Claude Code, Gemini, or Codex

## .mprompt File Format

A `.mprompt` file contains three sections separated by `--`:

1. **Frontmatter**: YAML metadata (name, version, description, author)
2. **Wizard Section**: YAML configuration for interactive variables
3. **Template Section**: The prompt template syntax with variable placeholders

### Example

```yaml
name: Example
description: An example prompt
author: Your Name
version: 1.0
--
- id: hi
  description: "Hi prompt"
  type: string
  required: true
--
Say {{hi}}
```

## Complete Example

1. Install a prompt from the registry:
   ```bash
   $ marvai install example
   What should I say? hello
   Installed example from remote registry
   ```

2. Execute the prompt:
   ```bash
   $ marvai prompt example
   # This will run the templated prompt through Claude Code
   ```

3. Or use with different AI tools:
   ```bash
   $ marvai --cli gemini prompt example
   $ marvai --cli codex prompt example
   ```

The generated files will be:
- `.marvai/example.mprompt` (the template)
- `.marvai/example.var` (variable values)

When executed, the templated content will be:
```
Say hello
```

## Templating

The template section supports:

- **Variables**: `{{variablename}}`
- **Conditionals**: `{{#if condition}}...{{/if}}`
- **Loops**: `{{#each items}}...{{/each}}`
- **Helpers**: Built-in and custom helpers

### Advanced Example

```yaml
name: Advanced Example
description: An advanced example with loops and conditionals
author: Your Name
version: 1.0
--
- id: name
  questio n: "What's your name?"
  type: string
  required: true
- id: items
  quest ion: "Enter comma-separated items:"
  type: string
  required: false
--
Hello {{name}}!

{{#if items}}
Here are your items:
{{#each (split items ",")}}
- {{this}}
{{/each}}
{{else}}
No items provided.
{{/if}}
```

## Variable Types

- `string`: Text input
- `required`: Boolean flag to make the variable mandatory

## Directory Structure

```
your-project/
└── .marvai/
    ├── example.mprompt      # Installed template
    └── example.var          # Variable values
```

**Note:** All prompts are installed from the remote registry into the `.marvai` directory.