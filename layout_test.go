package main

import (
	"errors"
	"os/exec"
	"reflect"
	"testing"
)

// installed returns a lookPath that finds the named commands, and finds git
// only where it is really installed.
func installed(names ...string) lookPathFunc {
	return func(file string) (string, error) {
		if file == "git" {
			return exec.LookPath("git")
		}

		for _, name := range names {
			if name == file {
				return "/stub/" + name, nil
			}
		}

		return "", errors.New("not found")
	}
}

var everything = installed("vim", "opencode", "lazygit", "npm", "php", "tail")

func windowNames(windows []window) []string {
	names := []string{}
	for _, w := range windows {
		names = append(names, w.name)
	}

	return names
}

func TestDefaultLayoutBase(t *testing.T) {
	windows, err := defaultLayout(t.TempDir(), everything)
	if err != nil {
		t.Fatal(err)
	}

	want := []window{
		{name: "editor", command: "vim ."},
		{name: "opencode", command: "opencode"},
		{name: "terminal"},
	}

	if !reflect.DeepEqual(windows, want) {
		t.Errorf("windows = %#v, want %#v", windows, want)
	}
}

func TestDefaultLayoutGit(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git is not installed")
	}

	directory := t.TempDir()
	if output, err := exec.Command(git, "init", "-q", directory).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}

	windows, err := defaultLayout(directory, everything)
	if err != nil {
		t.Fatal(err)
	}

	if got := windowNames(windows); !reflect.DeepEqual(got, []string{"editor", "opencode", "terminal", "lazygit"}) {
		t.Errorf("windows = %v", got)
	}

	if _, err := defaultLayout(directory, installed("vim", "opencode")); err == nil || err.Error() != "lazygit is required." {
		t.Errorf("without lazygit: err = %v", err)
	}
}

func TestPackageScript(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{"watch beats dev", `{"scripts": {"dev": "vite", "watch": "vite build -w"}}`, "watch"},
		{"dev only", `{"scripts": {"dev": "vite"}}`, "dev"},
		{"neither", `{"scripts": {"build": "vite build"}}`, ""},
		{"null script value", `{"scripts": {"dev": null}}`, "dev"},
		{"no scripts", `{"name": "app"}`, ""},
		{"null scripts", `{"scripts": null}`, ""},
		{"top-level array", `[]`, ""},
		{"byte order mark", "\xef\xbb\xbf{\"scripts\": {\"dev\": \"vite\"}}", "dev"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeFile(t, directory, "package.json", test.content)

			got, err := packageScript(directory)
			if err != nil || got != test.want {
				t.Errorf("packageScript = %q, %v; want %q", got, err, test.want)
			}
		})
	}

	t.Run("invalid json", func(t *testing.T) {
		directory := t.TempDir()
		writeFile(t, directory, "package.json", `{"scripts":`)

		if _, err := packageScript(directory); err == nil || err.Error() != "unable to parse package.json." {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("npm is required", func(t *testing.T) {
		directory := t.TempDir()
		writeFile(t, directory, "package.json", `{"scripts": {"dev": "vite"}}`)

		if _, err := defaultLayout(directory, installed("vim", "opencode")); err == nil || err.Error() != "npm is required." {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("no script needs no npm", func(t *testing.T) {
		directory := t.TempDir()
		writeFile(t, directory, "package.json", `{}`)

		if _, err := defaultLayout(directory, installed("vim", "opencode")); err != nil {
			t.Errorf("err = %v", err)
		}
	})
}

func TestLaravelWindows(t *testing.T) {
	tests := []struct {
		name     string
		composer string // Empty means no composer.json.
		want     []window
	}{
		{
			name: "no composer.json",
			want: []window{{name: "logs", command: "tail -f storage/logs/laravel.log"}},
		},
		{
			name:     "invalid composer.json",
			composer: `{`,
			want:     []window{{name: "logs", command: "tail -f storage/logs/laravel.log"}},
		},
		{
			name:     "pail in require-dev",
			composer: `{"require-dev": {"laravel/pail": "^1.0"}}`,
			want:     []window{{name: "logs", command: "php artisan pail"}},
		},
		{
			name:     "pail and horizon in require",
			composer: `{"require": {"laravel/pail": "^1.0", "laravel/horizon": "^5.0"}}`,
			want: []window{
				{name: "logs", command: "php artisan pail"},
				{name: "horizon", command: "php artisan horizon"},
			},
		},
		{
			name:     "horizon without pail",
			composer: `{"require": {"laravel/horizon": "^5.0"}}`,
			want: []window{
				{name: "logs", command: "tail -f storage/logs/laravel.log"},
				{name: "horizon", command: "php artisan horizon"},
			},
		},
		{
			name:     "require as a list",
			composer: `{"require": ["laravel/pail"]}`,
			want:     []window{{name: "logs", command: "tail -f storage/logs/laravel.log"}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			writeFile(t, directory, "artisan", "")
			if test.composer != "" {
				writeFile(t, directory, "composer.json", test.composer)
			}

			windows, err := defaultLayout(directory, everything)
			if err != nil {
				t.Fatal(err)
			}

			if got := windows[3:]; !reflect.DeepEqual(got, test.want) {
				t.Errorf("windows = %#v, want %#v", got, test.want)
			}
		})
	}

	t.Run("php is required", func(t *testing.T) {
		directory := t.TempDir()
		writeFile(t, directory, "artisan", "")

		if _, err := defaultLayout(directory, installed("vim", "opencode", "tail")); err == nil || err.Error() != "php is required." {
			t.Errorf("err = %v", err)
		}
	})
}

func TestDependencyOrder(t *testing.T) {
	directory := t.TempDir()
	writeFile(t, directory, "artisan", "")
	writeFile(t, directory, "package.json", `{"scripts": {"dev": "vite"}}`)

	if _, err := defaultLayout(directory, installed()); err == nil || err.Error() != "vim is required." {
		t.Errorf("err = %v", err)
	}

	if _, err := defaultLayout(directory, installed("vim", "opencode", "php")); err == nil || err.Error() != "npm is required." {
		t.Errorf("err = %v", err)
	}
}

func TestSessionName(t *testing.T) {
	tests := map[string]string{
		"/work/my-app_2":     "my-app_2",
		"/work/my.app dir":   "my_app_dir",
		"/work/a:b$c":        "a_b_c",
		"/work/café":         "café",
		"/":                  "_",
		"/work/nested/inner": "inner",
	}

	for directory, want := range tests {
		if got := sessionName(directory); got != want {
			t.Errorf("sessionName(%q) = %q, want %q", directory, got, want)
		}
	}
}
