# Marvai

<img src="https://raw.githubusercontent.com/StephanSchmidt/marvai/main/marvai-demo.gif" alt="Demo Marvai" width="70%">

**!Not safe for production!**

A CLI tool for managing and executing prompt templates with interactive wizards.

## Install

```bash
go install github.com/StephanSchmidt/marvai@latest
```

## Features

- **Interactive Wizards**: Define variables with questions to prompt users for input
- **Templates**: Use powerful templating with `{{variablename}}` placeholders
- **Claude Integration**: Execute generated prompts directly with Claude Code

## 'Hello World' Example

```bash
marvai install hello
# will ask you the language
For what language? Go
marvai prompt hello 
# calls claude code with the hello world prompt and generate the hello world
# in the specified language
...
```

## Commands

### `marvai install <source>`

Install a `.mprompt` file by running its wizard and generating configuration files.

```bash
marvai install helloworldprompt
# or from Github
marvai install https://github.com/StephanSchmidt/helloworldprompt
```

This will:
1. Read the `.mprompt` file from local path or Github URL
2. Execute the wizard, prompting for variable values
3. Generate `.marvai/<name>.mprompt` and `.marvai/<name>.var` files

### `marvai prompt <name>`

Execute a previously installed prompt with Claude Code.

```bash
marvai prompt example
```

This will run the templated prompt through Claude Code.

### `marvai list`

List available `.mprompt` files in the current directory.

```bash
$ marvai list
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