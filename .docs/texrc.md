# `.texrc`

A `.texrc` file in the directory where you run `tex` defines that project's windows. When it exists, it **replaces** the built-in layout completely: no `editor`, `opencode`, `terminal`, or auto-detected windows are created unless the file lists them.

## Example

```text
# Windows open in this order. The first one is selected on attach.
editor: vim .
ai: opencode
shell:
queue: php artisan queue:work
api server: npm run dev
```

## Format

One window per line, written as `name: command`.

- Blank lines are ignored.
- Lines whose first non-space character is `#` are comments.
- The line is split on the **first** `:`. Everything after it is the command, so `php artisan queue:work` works.
- Whitespace around the name and the command is trimmed.
- An empty command (`shell:`) opens a plain interactive shell.
- A `#` after the start of a line is part of the command, not a comment.
- Names may contain spaces and dots. They cannot contain `:`, and each name must be unique.
- Windows open in file order. The first window is selected when tex attaches.
- Every window starts in the project directory.

Commands are typed into the window's shell exactly as written, so shell aliases, functions, `&&`, and environment prefixes (`FOO=1 npm run dev`) all work.

## Lookup

tex only reads `.texrc` from the exact current directory. It does not search parent directories, the Git root, or any global location.

## Applying changes

If the session is already running, `tex` attaches to it and ignores `.texrc`. To apply edits:

```bash
tex stop && tex
```

## Errors

tex validates the whole file before creating a session. Any of these errors stops tex without creating a session:

| Problem | Message |
| --- | --- |
| A non-comment line has no `:` | `tex: .texrc:3: expected 'name: command'` |
| The window name is empty | `tex: .texrc:3: window name is empty` |
| A window name repeats | `tex: .texrc:5: duplicate window 'editor'` |
| The file contains no windows | `tex: .texrc defines no windows` |
| `.texrc` is not a readable file | `tex: cannot read .texrc` |

## Dependencies

tex does not check that the programs in `.texrc` commands are installed. A missing program shows up as `command not found` in its window. Only `tmux` is required.

## Security

tex runs `.texrc` commands without asking. Before running `tex` in a repository you did not write, read its `.texrc`, just as you would read its `package.json` scripts.
