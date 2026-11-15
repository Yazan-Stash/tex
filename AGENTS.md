# AGENTS.md

## Project Overview

`tex` is a small Go command that creates or attaches to a tmux development session for the exact directory from which it is invoked. `make build` compiles it to `./tex`, which is git-ignored and available globally through this symlink:

```text
/usr/local/bin/tex -> /Users/yazanstash/personal/Code/tex/tex
```

Keep the command generic. Do not add assumptions tied to one application or repository.

## Command Interface

```text
tex        Create the current directory's session, or attach if it exists.
tex stop   Stop the current directory's session.
```

The project directory is the exact physical current working directory, not the Git root. The session name is the directory basename with every character other than letters, digits, `_` and `-` replaced by underscores. Basename collisions are accepted behavior.

All tmux commands that target a session by name use the exact-match form `=name`. A bare name also matches by prefix, so `foo` would resolve to a running `foobar`.

Attachment intentionally replaces the process with plain `tmux attach-session`. Do not switch clients or force nested tmux sessions. Running `tex` from inside tmux may therefore fail with tmux's normal nested-session warning.

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

`watch` always takes precedence over `dev`; never run both. Composer packages are detected in both `require` and `require-dev`. Both files are parsed in Go, so detection needs neither `node` nor `php`. An invalid `package.json` is an error; an invalid `composer.json` declares no packages.

## Code Layout

| File | Responsibility |
| --- | --- |
| `main.go` | Arguments, session name, stop/attach/create flow, error output |
| `texrc.go` | `.texrc` reading and parsing |
| `layout.go` | Built-in layout detection and dependency checks |
| `tmux.go` | All tmux calls |
| `*_test.go` | Unit tests, plus `integration_test.go` against a real tmux server |

## Implementation Constraints

- Use only the Go standard library. The binary must stay a single static file (`CGO_ENABLED=0`).
- Never use a local Go toolchain. Build, vet and test only through `make`, which runs everything in Docker (see `Dockerfile`).
- Keep error messages and stdout text exactly as documented here and in `.docs/texrc.md`. Errors print as `tex: <message>` and exit 1.
- Validate applicable command dependencies of the built-in layout before creating a session.
- Both the built-in layout and `.texrc` build the same ordered `[]window`. One loop in `tmux.createSession` creates the windows, targeting them by tmux window ID so names may contain spaces or dots, and sends commands with `send-keys -l --`.
- Remove a partially constructed session when creation fails. Never remove it after creation succeeds, so a non-interactive or nested attach failure leaves the session running.
- Select the first window in the list before attachment (`editor` for the built-in layout).
- Prefer small inline detection logic. `.texrc` is the only configuration file; do not add others or additional runtime files.
- Do not modify or replace the `/usr/local/bin/tex` symlink unless the installation path changes explicitly.

## Verification

Every behavior change needs an automated test. Run everything in Docker:

```bash
make test    # gofmt check, go vet, unit tests, tmux integration tests (Linux)
make build   # cross-compiles ./tex for the host OS and architecture
```

Integration tests build `tex`, run it in temporary directories against an isolated tmux server (`TMUX_TMPDIR`), and replace `vim`, `opencode`, `lazygit`, `npm`, `php` and `tail` with stubs that log their arguments. Add a case there for any change to session creation, detection, or `.texrc` handling.

When changing detection logic, cover each affected branch, including precedence cases such as a `package.json` containing both `watch` and `dev`.

When changing `.texrc` handling, cover:

- A valid file with comments, blank lines, a colon inside a command, an empty command, and names containing spaces or dots. Confirm window order and that the first window is active.
- A `.texrc` inside a Git repository with `package.json` scripts. Only the file's windows should appear.
- CRLF line endings and a missing trailing newline.
- Each error case. Confirm exit status 1, the expected message, and that no session was created.

After `make build`, smoke-test the real binary on the host. Use uniquely named temporary directories so no existing session is touched. In a non-interactive agent shell, the final attach is expected to report `open terminal failed: not a terminal`; the completed session should remain available for inspection:

```bash
tmux list-windows -t '=SESSION' -F '#{window_name}:#{window_active}:#{pane_current_command}'
tmux capture-pane -p -t '=SESSION:WINDOW'
tex stop
```
