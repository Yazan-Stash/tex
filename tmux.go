package main

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

type tmux struct {
	path string
}

// target matches the session named exactly session. A bare name would also
// match by prefix, so `foo` could resolve to a running `foobar`.
func target(session string) string {
	return "=" + session
}

func (t tmux) run(args ...string) (string, error) {
	var stderr bytes.Buffer

	cmd := exec.Command(t.path, args...)
	cmd.Stderr = &stderr

	output, err := cmd.Output()
	if err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}

		return "", fmt.Errorf("tmux %s: %s", args[0], message)
	}

	return strings.TrimSpace(string(output)), nil
}

func (t tmux) hasSession(session string) bool {
	_, err := t.run("has-session", "-t", target(session))

	return err == nil
}

func (t tmux) killSession(session string) error {
	_, err := t.run("kill-session", "-t", target(session))

	return err
}

// createSession builds a detached session with windows in order and selects
// the first one. A partially built session is removed on failure.
func (t tmux) createSession(session, directory string, windows []window) (err error) {
	info, err := t.run("new-session", "-d", "-P", "-F", "#{session_id} #{window_id}",
		"-s", session, "-n", windows[0].name, "-c", directory)
	if err != nil {
		return err
	}

	sessionID, firstWindowID, _ := strings.Cut(info, " ")

	defer func() {
		if err != nil {
			_, _ = t.run("kill-session", "-t", sessionID)
		}
	}()

	// Windows are targeted by ID so names may contain spaces, dots, or colons.
	for index, w := range windows {
		windowID := firstWindowID

		if index > 0 {
			windowID, err = t.run("new-window", "-d", "-P", "-F", "#{window_id}",
				"-t", sessionID+":", "-n", w.name, "-c", directory)
			if err != nil {
				return err
			}
		}

		if w.command == "" {
			continue
		}

		if _, err = t.run("send-keys", "-t", windowID, "-l", "--", w.command); err != nil {
			return err
		}

		if _, err = t.run("send-keys", "-t", windowID, "Enter"); err != nil {
			return err
		}
	}

	_, err = t.run("select-window", "-t", firstWindowID)

	return err
}

// attach replaces tex with `tmux attach-session`. It only returns on failure.
func (t tmux) attach(session string) error {
	err := syscall.Exec(t.path, []string{"tmux", "attach-session", "-t", target(session)}, os.Environ())

	return fmt.Errorf("cannot attach to session '%s': %w", session, err)
}
