# AGENTS.md

## Project Overview

`tex` is a small Bash command that creates or attaches to a tmux development session for the exact directory from which it is invoked. It is intended to be available globally through this symlink:

```text
/usr/local/bin/tex -> /Users/yazanstash/personal/Code/tex/tex
```

Keep the command generic. Do not add assumptions tied to one application or repository.

## Command Interface

```text
tex        Create the current directory's session, or attach if it exists.
tex stop   Stop the current directory's session.
```

The project directory is the exact current working directory, not the Git root. The session name is the directory basename with tmux-special characters replaced by underscores. Basename collisions are accepted behavior.

Attachment intentionally uses plain `tmux attach-session`. Do not switch clients or force nested tmux sessions. Running `tex` from inside tmux may therefore fail with tmux's normal nested-session warning.

## Project Layout File

If `.texrc` exists in the exact current directory, it fully replaces the built-in layout below. No auto-detection runs. Its format, parsing rules and error messages are specified in [`.docs/texrc.md`](.docs/texrc.md); keep that file in sync with the parser.

Key rules for `.texrc`:

- One `name: command` per line, split on the first `:`. Comment lines start with `#`, and an empty command opens a plain shell.
- The whole file is validated before tmux starts. Invalid lines, empty or duplicate names, an empty file, or an unreadable file fail without creating a session.
- Commands are not dependency-checked. Only `tmux` is required.
- The first window in the file is selected before attachment.
- An existing session is attached unchanged. Edits apply after `tex stop && tex`.
- There is no parent-directory or global lookup, and no trust prompt.

## Session Layout

Without a `.texrc`, every new session contains these windows:

| Window | Command |
| --- | --- |
| `editor` | `vim .` |
| `opencode` | `opencode` |
| `terminal` | Interactive shell only |

Conditional windows are created in this order:

| Condition | Window | Command |
| --- | --- | --- |
| Current directory is in a Git worktree | `lazygit` | `lazygit` |
| `package.json` defines `scripts.watch` | `watch` | `npm run watch` |
| No `watch`, but `package.json` defines `scripts.dev` | `dev` | `npm run dev` |
| An `artisan` file exists and `composer.json` declares `laravel/pail` | `logs` | `php artisan pail` |
| An `artisan` file exists without `laravel/pail` | `logs` | `tail -f storage/logs/laravel.log` |
| An `artisan` file exists and `composer.json` declares `laravel/horizon` | `horizon` | `php artisan horizon` |

`watch` always takes precedence over `dev`; never run both. Composer packages are detected in both `require` and `require-dev`.

## Implementation Constraints

- Keep the script compatible with Bash and retain `set -euo pipefail`.
- Quote paths and tmux targets. Project directories may contain spaces or punctuation.
- Validate applicable command dependencies of the built-in layout before creating a session.
- Both the built-in layout and `.texrc` build the same ordered window list. A single loop creates the windows, targeting them by tmux window ID so names may contain spaces or dots, and sends commands with `send-keys -l`.
- Keep the script compatible with macOS `/bin/bash` 3.2: no associative arrays, and never expand a possibly empty array under `set -u`.
- Keep the failure trap that removes a partially constructed session.
- Disable partial-session cleanup before the final attach so a non-interactive or nested attach failure does not destroy a successfully created session.
- Select the first window in the list before attachment (`editor` for the built-in layout).
- Prefer small inline detection blocks. `.texrc` is the only configuration file; do not add others or additional runtime files.
- Do not modify or replace the `/usr/local/bin/tex` symlink unless the installation path changes explicitly.

## Verification

There is no automated test suite. Every behavior change must be checked programmatically.

At minimum, run the syntax check:

```bash
bash -n tex
```

For session behavior, use uniquely named temporary directories and invoke the installed `tex` command from each directory. In a non-interactive agent shell, the final attach is expected to report `open terminal failed: not a terminal`; the completed session should remain available for inspection.

Inspect windows and commands with:

```bash
tmux list-windows -t SESSION -F '#{window_name}:#{pane_current_command}:#{pane_current_path}'
tmux capture-pane -p -t SESSION:WINDOW
```

Always stop temporary sessions after verification:

```bash
tex stop
```

When changing detection logic, cover each affected branch, including precedence cases such as a `package.json` containing both `watch` and `dev`.

When changing `.texrc` handling, cover:

- A valid file with comments, blank lines, a colon inside a command, an empty command, and names containing spaces or dots. Confirm window order and that the first window is active.
- A `.texrc` inside a Git repository with `package.json` scripts. Only the file's windows should appear.
- CRLF line endings and a missing trailing newline.
- Each error case. Confirm exit status 1, the expected message, and that no session was created.
