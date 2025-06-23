# Marvai

A CLI tool for managing and executing prompt templates with interactive wizards.

## Features

- **Interactive Wizards**: Define variables with questions to prompt users for input
- **Handlebars Templates**: Use powerful Handlebars templating with `{{variablename}}` placeholders
- **Claude Integration**: Execute generated prompts directly with Claude Code

## Commands

### `marvai install <name>`

Install a `.mprompt` file by running its wizard and generating a `.prompt` file.

```bash
marvai install example
```

This will:
1. Read `example.mprompt` from the current directory
2. Execute the wizard, prompting for variable values
3. Generate `.marvai/example.prompt` with variables substituted

### `marvai prompt <name>`

Execute a previously installed prompt with Claude Code.

```bash
marvai prompt example
```

This will run the generated prompt file through Claude Code.

## .mprompt File Format

A `.mprompt` file contains two sections separated by `--`:

1. **Wizard Section**: YAML configuration for interactive variables
2. **Template Section**: The prompt template using Handlebars syntax with variable placeholders

### Example

```yaml
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
   Created .marvai/example.prompt from example.mprompt
   ```

3. Execute the prompt:
   ```bash
   $ marvai prompt example
   Hello! I'm Claude Code, ready to help you with your software engineering tasks.
   ```

The generated `.marvai/example.prompt` will contain:
```
Say hello
```

## Handlebars Templating

The template section supports:

- **Variables**: `{{variablename}}`
- **Conditionals**: `{{#if condition}}...{{/if}}`
- **Loops**: `{{#each items}}...{{/each}}`
- **Helpers**: Built-in and custom helpers

### Advanced Example

```yaml
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
├── example.mprompt          # Your prompt template
└── .marvai/
    └── example.prompt       # Generated prompt file
```