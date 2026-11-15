// Command tex creates or attaches to a tmux development session for the exact
// directory it is run from.
//
//	tex        Create the current directory's session, or attach if it exists.
//	tex stop   Stop the current directory's session.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"
)

const usage = "Usage: tex [stop]"

var errUsage = errors.New(usage)

// window is one tmux window. An empty command leaves an interactive shell.
type window struct {
	name    string
	command string
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	err := execute(args, stdout)

	switch {
	case err == nil:
		return 0
	case errors.Is(err, errUsage):
		fmt.Fprintln(stderr, usage)
	default:
		fmt.Fprintf(stderr, "tex: %v\n", err)
	}

	return 1
}

func execute(args []string, stdout io.Writer) error {
	tmuxPath, err := exec.LookPath("tmux")
	if err != nil {
		return errors.New("tmux is required.")
	}

	t := tmux{path: tmuxPath}

	directory, err := currentDirectory()
	if err != nil {
		return err
	}

	session := sessionName(directory)

	switch {
	case len(args) == 1 && args[0] == "stop":
		if t.killSession(session) == nil {
			fmt.Fprintf(stdout, "Session '%s' stopped.\n", session)
		} else {
			fmt.Fprintf(stdout, "No session '%s' running.\n", session)
		}

		return nil
	case len(args) != 0:
		return errUsage
	}

	if t.hasSession(session) {
		fmt.Fprintf(stdout, "Attaching to existing session '%s'...\n", session)

		return t.attach(session)
	}

	windows, err := loadLayout(directory)
	if err != nil {
		return err
	}

	if err := t.createSession(session, directory, windows); err != nil {
		return err
	}

	// A failed attach (no terminal, nested tmux) leaves the new session running.
	return t.attach(session)
}

// currentDirectory returns the physical working directory, like `pwd -P`.
func currentDirectory() (string, error) {
	directory, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("cannot determine current directory: %w", err)
	}

	resolved, err := filepath.EvalSymlinks(directory)
	if err != nil {
		return "", fmt.Errorf("cannot determine current directory: %w", err)
	}

	return resolved, nil
}

// sessionName is the directory basename with every character other than
// letters, digits, `_` and `-` replaced by `_`, so tmux never reads `.` or `:`
// in it as a target separator.
func sessionName(directory string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' {
			return r
		}

		return '_'
	}, filepath.Base(directory))
}

// loadLayout returns the windows from .texrc when it exists, and the built-in
// layout otherwise.
func loadLayout(directory string) ([]window, error) {
	windows, found, err := readTexrc(directory)
	if found || err != nil {
		return windows, err
	}

	return defaultLayout(directory, exec.LookPath)
}
