**----- NOT SAFE FOR PRODUCTION -----**

# MarvAI

<img src="https://raw.githubusercontent.com/marvai-dev/marvai/main/marvai-demo.gif" alt="Demo Marvai" width="70%">

A powerful CLI tool that changes how you work with AI prompts in your development workflow.
Think *NPM for prompts* - discover, install, and execute battle-tested AI prompts from the MarvAI 
distribution. Transform your development process with ready-to-use prompts 
that automate code reviews, security scans, refactoring, and more.

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
- **Execute prompts seamlessly**: Run AI prompts with Claude Code integration

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
...
```

## Commands

### `marvai install <source>`

Install a `.mprompt` file by running its wizard and generating configuration files.

```bash
> marvai install helloworld
```

This will:
1. Read the `.mprompt` file from local path or marvai registry
2. Execute the wizard, prompting for variable values
3. Generate `.marvai/<name>.mprompt` and `.marvai/<name>.var` files

Install from a different repository:

```bash
> marvai install @otherrepo/otherprompt
```

### `marvai prompt <name>`

Execute a previously installed prompt with Claude Code.

```bash
$ marvai prompt example
```

This will run the templated prompt through Claude Code.

### `marvai list`

List available `.mprompt` files in the current directory.

```bash
$ marvai list-local
Found 2 .mprompt file(s):
  Advanced Example - An advanced example (by Stephan Schmidt)
  Example v0.4 - An example (by Stephan Schmidt)
```

### `marvai installed`

List installed prompts in the `.marvai` directory.

```bash
$ marvai installed
Found 1 installed prompt(s):
  Example v0.4 - An example (by Stephan Schmidt) (configured)
```


# Features For Prompt Developers

- **Interactive Wizards**: Define variables with questions to prompt users for input
- **Templates**: Use powerful templating with `{{variablename}}` placeholders
- **Claude Integration**: Execute generated prompts directly with Claude Code

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
  question: "What should I say?"
  type: string
  required: true
--
Say {{hi}}
```

## Complete Example

1. Create `example.mprompt`:
   ```yaml
   name: Example
   description: An example prompt
   author: Your Name
   version: 1.0
   --
   - id: hi
     question: "What should I say?"
     type: string
     required: true
   --
   Say {{hi}}
   ```

2. Install the prompt:
   ```bash
   $ marvai install example
   What should I say? hello
   Installed example from example.mprompt
   ```

3. Execute the prompt:
   ```bash
   $ marvai prompt example
   ...
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
  question: "What's your name?"
  type: string
  required: true
- id: items
  question: "Enter comma-separated items:"
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
├── example.mprompt          # possible prompt template for the project
└── .marvai/
    ├── example.mprompt      # Installed template
    └── example.var          # Variable values
```