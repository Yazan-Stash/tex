package main

// Integration tests build tex and run it against a real, isolated tmux
// server. They skip when tmux is not installed. `make test` runs them in
// Docker.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var texBinary string

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

func runTests(m *testing.M) int {
	if _, err := exec.LookPath("tmux"); err != nil {
		return m.Run()
	}

	directory, err := os.MkdirTemp("", "tex-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	defer os.RemoveAll(directory)

	texBinary = filepath.Join(directory, "tex")

	build := exec.Command("go", "build", "-o", texBinary, ".")
	build.Stdout = os.Stderr
	build.Stderr = os.Stderr

	if err := build.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "building tex:", err)
		return 1
	}

	return m.Run()
}

// allStubs are the commands the built-in layout can run. Each stub records
// its invocation in the stub log instead of doing anything.
var allStubs = []string{"vim", "opencode", "lazygit", "npm", "php", "tail"}

type sandbox struct {
	t       *testing.T
	root    string
	env     []string
	stubLog string
}

type result struct {
	code   int
	stdout string
	stderr string
}

func newSandbox(t *testing.T, stubs ...string) *sandbox {
	t.Helper()

	if texBinary == "" {
		t.Skip("tmux is not installed")
	}

	// Kept short: tmux socket paths are limited to about 100 bytes.
	socketDirectory, err := os.MkdirTemp("", "tmux")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(socketDirectory) })

	root := t.TempDir()
	stubDirectory := filepath.Join(root, "bin")
	home := filepath.Join(root, "home")

	for _, directory := range []string{stubDirectory, home} {
		if err := os.Mkdir(directory, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// tmux starts login shells, and /etc/profile may reset PATH.
	profile := "PATH=" + stubDirectory + ":$PATH\n"
	if err := os.WriteFile(filepath.Join(home, ".profile"), []byte(profile), 0o644); err != nil {
		t.Fatal(err)
	}

	stub := "#!/bin/sh\nprintf '%s\\n' \"$(basename \"$0\") $*\" >> \"$TEX_STUB_LOG\"\n"
	for _, name := range stubs {
		if err := os.WriteFile(filepath.Join(stubDirectory, name), []byte(stub), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	s := &sandbox{
		t:       t,
		root:    root,
		stubLog: filepath.Join(root, "stub.log"),
	}
	s.env = []string{
		"PATH=" + stubDirectory + ":/usr/bin:/bin",
		"HOME=" + home,
		"SHELL=/bin/sh",
		"TERM=xterm",
		"TMUX_TMPDIR=" + socketDirectory,
		"TEX_STUB_LOG=" + s.stubLog,
	}

	t.Cleanup(func() { _ = s.command("tmux", "kill-server").Run() })

	return s
}

func (s *sandbox) command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	cmd.Env = s.env

	return cmd
}

// project creates a project directory with the given files.
func (s *sandbox) project(name string, files map[string]string) string {
	s.t.Helper()

	directory := filepath.Join(s.root, name)
	if err := os.Mkdir(directory, 0o755); err != nil {
		s.t.Fatal(err)
	}

	for file, content := range files {
		writeFile(s.t, directory, file, content)
	}

	return directory
}

func (s *sandbox) gitInit(directory string) {
	s.t.Helper()

	if output, err := s.command("git", "init", "-q", directory).CombinedOutput(); err != nil {
		s.t.Fatalf("git init: %v: %s", err, output)
	}
}

func (s *sandbox) tex(directory string, args ...string) result {
	s.t.Helper()

	var stdout, stderr bytes.Buffer

	cmd := s.command(texBinary, args...)
	cmd.Dir = directory
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	var exitErr *exec.ExitError
	if err != nil && !errors.As(err, &exitErr) {
		s.t.Fatalf("running tex: %v", err)
	}

	return result{code: cmd.ProcessState.ExitCode(), stdout: stdout.String(), stderr: stderr.String()}
}

// texCreate runs tex where creating the session should succeed. With no
// terminal, the final attach fails but the session stays.
func (s *sandbox) texCreate(directory string) {
	s.t.Helper()

	r := s.tex(directory)
	if !strings.Contains(r.stderr, "not a terminal") {
		s.t.Fatalf("tex: expected only the attach to fail, got code %d, stdout %q, stderr %q", r.code, r.stdout, r.stderr)
	}
}

func (s *sandbox) hasSession(session string) bool {
	return s.command("tmux", "has-session", "-t", "="+session).Run() == nil
}

// windows lists "name active" for each window, in order.
func (s *sandbox) windows(session string) []string {
	s.t.Helper()

	output, err := s.command("tmux", "list-windows", "-t", "="+session, "-F", "#{window_name} #{window_active}").Output()
	if err != nil {
		s.t.Fatalf("list-windows %s: %v", session, err)
	}

	return strings.Split(strings.TrimSpace(string(output)), "\n")
}

// waitForStubCalls waits until every wanted invocation is in the stub log.
func (s *sandbox) waitForStubCalls(want ...string) {
	s.t.Helper()

	deadline := time.Now().Add(10 * time.Second)

	for {
		data, _ := os.ReadFile(s.stubLog)
		calls := map[string]bool{}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			calls[strings.TrimSpace(line)] = true
		}

		missing := []string{}
		for _, call := range want {
			if !calls[call] {
				missing = append(missing, call)
			}
		}

		if len(missing) == 0 {
			return
		}

		if time.Now().After(deadline) {
			s.t.Fatalf("commands never ran: %q\nstub log:\n%s", missing, data)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func assertWindows(t *testing.T, got []string, want ...string) {
	t.Helper()

	if !reflect.DeepEqual(got, want) {
		t.Errorf("windows = %q\nwant      %q", got, want)
	}
}

func TestBuiltInLayoutWithEveryWindow(t *testing.T) {
	s := newSandbox(t, allStubs...)
	directory := s.project("my.app dir", map[string]string{
		"package.json":  `{"scripts": {"dev": "vite", "watch": "vite build --watch"}}`,
		"artisan":       "",
		"composer.json": `{"require": {"laravel/horizon": "^5"}, "require-dev": {"laravel/pail": "^1"}}`,
	})
	s.gitInit(directory)

	s.texCreate(directory)

	assertWindows(t, s.windows("my_app_dir"),
		"editor 1", "opencode 0", "terminal 0", "lazygit 0", "watch 0", "logs 0", "horizon 0")
	s.waitForStubCalls("vim .", "opencode", "lazygit", "npm run watch", "php artisan pail", "php artisan horizon")
}

func TestBuiltInLayoutWithoutGit(t *testing.T) {
	s := newSandbox(t, allStubs...)
	directory := s.project("plain", map[string]string{
		"package.json": `{"scripts": {"dev": "vite"}}`,
		"artisan":      "",
	})

	s.texCreate(directory)

	assertWindows(t, s.windows("plain"), "editor 1", "opencode 0", "terminal 0", "dev 0", "logs 0")
	s.waitForStubCalls("vim .", "opencode", "npm run dev", "tail -f storage/logs/laravel.log")
}

func TestMissingDependencyCreatesNoSession(t *testing.T) {
	s := newSandbox(t, "vim")
	directory := s.project("deps", nil)

	r := s.tex(directory)

	if r.code != 1 || r.stderr != "tex: opencode is required.\n" {
		t.Errorf("got code %d, stderr %q", r.code, r.stderr)
	}

	if s.hasSession("deps") {
		t.Error("a session was created")
	}
}

func TestTexrcReplacesBuiltInLayout(t *testing.T) {
	s := newSandbox(t, allStubs...)
	directory := s.project("custom", map[string]string{
		"package.json": `{"scripts": {"watch": "vite build --watch"}}`,
		".texrc": "# Windows open in this order.\r\n" +
			"\r\n" +
			"main editor: vim .\r\n" +
			"  # indented comment\r\n" +
			"queue: php artisan queue:work\r\n" +
			"shell:\r\n" +
			"v1.2 api: npm run dev\r\n" +
			"notes: vim notes.md # shell comment",
	})
	s.gitInit(directory)

	s.texCreate(directory)

	assertWindows(t, s.windows("custom"), "main editor 1", "queue 0", "shell 0", "v1.2 api 0", "notes 0")
	s.waitForStubCalls("vim .", "php artisan queue:work", "npm run dev", "vim notes.md")
}

func TestTexrcErrorsCreateNoSession(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"missing colon", "editor: vim .\nbroken\n", "tex: .texrc:2: expected 'name: command'\n"},
		{"empty name", ": vim .\n", "tex: .texrc:1: window name is empty\n"},
		{"duplicate name", "a:\nb:\n  a : vim\n", "tex: .texrc:3: duplicate window 'a'\n"},
		{"no windows", "# nothing\n\n", "tex: .texrc defines no windows\n"},
		{"empty file", "", "tex: .texrc defines no windows\n"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := newSandbox(t)
			directory := s.project("broken", map[string]string{".texrc": test.content})

			r := s.tex(directory)

			if r.code != 1 || r.stderr != test.want {
				t.Errorf("got code %d, stderr %q; want 1, %q", r.code, r.stderr, test.want)
			}

			if s.hasSession("broken") {
				t.Error("a session was created")
			}
		})
	}

	t.Run("unreadable", func(t *testing.T) {
		s := newSandbox(t)
		directory := s.project("broken", nil)
		if err := os.Mkdir(filepath.Join(directory, ".texrc"), 0o755); err != nil {
			t.Fatal(err)
		}

		r := s.tex(directory)

		if r.code != 1 || r.stderr != "tex: cannot read .texrc\n" {
			t.Errorf("got code %d, stderr %q", r.code, r.stderr)
		}

		if s.hasSession("broken") {
			t.Error("a session was created")
		}
	})
}

func TestExistingSessionIsAttachedUnchanged(t *testing.T) {
	s := newSandbox(t)
	directory := s.project("reuse", map[string]string{".texrc": "one:\n"})
	s.texCreate(directory)

	writeFile(t, directory, ".texrc", "one:\ntwo:\n")
	r := s.tex(directory)

	if r.stdout != "Attaching to existing session 'reuse'...\n" {
		t.Errorf("stdout = %q", r.stdout)
	}

	assertWindows(t, s.windows("reuse"), "one 1")
}

func TestStop(t *testing.T) {
	s := newSandbox(t)
	directory := s.project("stopme", map[string]string{".texrc": "one:\n"})
	s.texCreate(directory)

	if r := s.tex(directory, "stop"); r.code != 0 || r.stdout != "Session 'stopme' stopped.\n" {
		t.Errorf("first stop: code %d, stdout %q", r.code, r.stdout)
	}

	if s.hasSession("stopme") {
		t.Error("session still running")
	}

	if r := s.tex(directory, "stop"); r.code != 0 || r.stdout != "No session 'stopme' running.\n" {
		t.Errorf("second stop: code %d, stdout %q", r.code, r.stdout)
	}
}

func TestSessionNamesMatchExactly(t *testing.T) {
	s := newSandbox(t)
	if output, err := s.command("tmux", "new-session", "-d", "-s", "foobar").CombinedOutput(); err != nil {
		t.Fatalf("new-session: %v: %s", err, output)
	}

	directory := s.project("foo", map[string]string{".texrc": "one:\n"})

	if r := s.tex(directory, "stop"); r.stdout != "No session 'foo' running.\n" {
		t.Errorf("stop: stdout %q", r.stdout)
	}

	if !s.hasSession("foobar") {
		t.Fatal("tex stop in foo killed foobar")
	}

	s.texCreate(directory)

	if !s.hasSession("foo") {
		t.Error("tex in foo attached to foobar instead of creating foo")
	}
}

func TestCommandsAreTypedLiterally(t *testing.T) {
	s := newSandbox(t, "vim")
	directory := s.project("literal", map[string]string{".texrc": "keys: vim -n Enter C-c\ndash: -t x --help\n"})

	s.texCreate(directory)

	s.waitForStubCalls("vim -n Enter C-c")

	deadline := time.Now().Add(10 * time.Second)
	for {
		pane, _ := s.command("tmux", "capture-pane", "-p", "-t", "=literal:dash").Output()
		if strings.Contains(string(pane), "-t x --help") {
			return
		}

		if time.Now().After(deadline) {
			t.Fatalf("dash window never received its command; pane:\n%s", pane)
		}

		time.Sleep(50 * time.Millisecond)
	}
}

func TestUsage(t *testing.T) {
	s := newSandbox(t)
	directory := s.project("usage", nil)

	for _, args := range [][]string{{"bogus"}, {"stop", "extra"}} {
		if r := s.tex(directory, args...); r.code != 1 || r.stderr != "Usage: tex [stop]\n" {
			t.Errorf("tex %v: code %d, stderr %q", args, r.code, r.stderr)
		}
	}
}
