package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// lookPathFunc reports where a command is installed, like exec.LookPath.
type lookPathFunc func(file string) (string, error)

// defaultLayout returns the built-in windows for directory, after checking
// that every command they run is installed.
func defaultLayout(directory string, lookPath lookPathFunc) ([]window, error) {
	required := []string{"vim", "opencode"}
	windows := []window{
		{name: "editor", command: "vim ."},
		{name: "opencode", command: "opencode"},
		{name: "terminal"},
	}

	if inGitWorktree(directory, lookPath) {
		required = append(required, "lazygit")
		windows = append(windows, window{name: "lazygit", command: "lazygit"})
	}

	script, err := packageScript(directory)
	if err != nil {
		return nil, err
	}

	if script != "" {
		required = append(required, "npm")
		windows = append(windows, window{name: script, command: "npm run " + script})
	}

	if isFile(filepath.Join(directory, "artisan")) {
		required = append(required, "php")
		packages := composerPackages(directory)

		if packages["laravel/pail"] {
			windows = append(windows, window{name: "logs", command: "php artisan pail"})
		} else {
			required = append(required, "tail")
			windows = append(windows, window{name: "logs", command: "tail -f storage/logs/laravel.log"})
		}

		if packages["laravel/horizon"] {
			windows = append(windows, window{name: "horizon", command: "php artisan horizon"})
		}
	}

	for _, name := range required {
		if _, err := lookPath(name); err != nil {
			return nil, fmt.Errorf("%s is required.", name)
		}
	}

	return windows, nil
}

func inGitWorktree(directory string, lookPath lookPathFunc) bool {
	git, err := lookPath("git")
	if err != nil {
		return false
	}

	output, err := exec.Command(git, "-C", directory, "rev-parse", "--is-inside-work-tree").Output()

	return err == nil && strings.TrimSpace(string(output)) == "true"
}

// packageScript returns "watch" or "dev" when package.json defines that
// script. watch always takes precedence.
func packageScript(directory string) (string, error) {
	path := filepath.Join(directory, "package.json")
	if !isFile(path) {
		return "", nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", errors.New("unable to parse package.json.")
	}

	var manifest any
	if err := json.Unmarshal(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")), &manifest); err != nil {
		return "", errors.New("unable to parse package.json.")
	}

	scripts := objectField(manifest, "scripts")

	for _, script := range []string{"watch", "dev"} {
		if _, ok := scripts[script]; ok {
			return script, nil
		}
	}

	return "", nil
}

// composerPackages returns the packages composer.json declares in require
// and require-dev. A missing or invalid composer.json declares none.
func composerPackages(directory string) map[string]bool {
	packages := map[string]bool{}

	data, err := os.ReadFile(filepath.Join(directory, "composer.json"))
	if err != nil {
		return packages
	}

	var manifest any
	if json.Unmarshal(data, &manifest) != nil {
		return packages
	}

	for _, section := range []string{"require", "require-dev"} {
		for name, version := range objectField(manifest, section) {
			if version != nil {
				packages[name] = true
			}
		}
	}

	return packages
}

// objectField returns value[key] when both are JSON objects, and nil otherwise.
func objectField(value any, key string) map[string]any {
	object, _ := value.(map[string]any)
	field, _ := object[key].(map[string]any)

	return field
}

func isFile(path string) bool {
	info, err := os.Stat(path)

	return err == nil && info.Mode().IsRegular()
}
