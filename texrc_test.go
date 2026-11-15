package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseTexrc(t *testing.T) {
	content := "# Windows\r\n" +
		"\r\n" +
		"  editor :  vim .  \r\n" +
		"   # indented comment\n" +
		"queue: php artisan queue:work\n" +
		"shell:\n" +
		"api server.v2: npm run dev # not a comment\n" +
		"\tlast: tail -f log" // No trailing newline.

	windows, err := parseTexrc(strings.NewReader(content))
	if err != nil {
		t.Fatalf("parseTexrc: %v", err)
	}

	want := []window{
		{name: "editor", command: "vim ."},
		{name: "queue", command: "php artisan queue:work"},
		{name: "shell", command: ""},
		{name: "api server.v2", command: "npm run dev # not a comment"},
		{name: "last", command: "tail -f log"},
	}

	if !reflect.DeepEqual(windows, want) {
		t.Errorf("windows = %#v\nwant %#v", windows, want)
	}
}

func TestParseTexrcErrors(t *testing.T) {
	tests := []struct {
		content string
		want    string
	}{
		{"editor vim .\n", ".texrc:1: expected 'name: command'"},
		{"# comment\n\n  : vim .\n", ".texrc:3: window name is empty"},
		{"a: one\nb:\na: two\n", ".texrc:3: duplicate window 'a'"},
		{"a: one\r\nbroken\r\n", ".texrc:2: expected 'name: command'"},
		{"# only comments\n\n", ".texrc defines no windows"},
		{"", ".texrc defines no windows"},
	}

	for _, test := range tests {
		_, err := parseTexrc(strings.NewReader(test.content))
		if err == nil || err.Error() != test.want {
			t.Errorf("parseTexrc(%q) error = %v, want %q", test.content, err, test.want)
		}
	}
}

func TestReadTexrc(t *testing.T) {
	t.Run("missing", func(t *testing.T) {
		_, found, err := readTexrc(t.TempDir())
		if found || err != nil {
			t.Errorf("found = %v, err = %v; want false, nil", found, err)
		}
	})

	t.Run("broken symlink is missing", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Symlink("nowhere", filepath.Join(directory, ".texrc")); err != nil {
			t.Fatal(err)
		}

		_, found, err := readTexrc(directory)
		if found || err != nil {
			t.Errorf("found = %v, err = %v; want false, nil", found, err)
		}
	})

	t.Run("directory", func(t *testing.T) {
		directory := t.TempDir()
		if err := os.Mkdir(filepath.Join(directory, ".texrc"), 0o755); err != nil {
			t.Fatal(err)
		}

		_, found, err := readTexrc(directory)
		if !found || err != errCannotReadTexrc {
			t.Errorf("found = %v, err = %v; want true, %v", found, err, errCannotReadTexrc)
		}
	})

	t.Run("valid", func(t *testing.T) {
		directory := t.TempDir()
		writeFile(t, directory, ".texrc", "one: vim\n")

		windows, found, err := readTexrc(directory)
		if !found || err != nil || len(windows) != 1 {
			t.Errorf("windows = %v, found = %v, err = %v", windows, found, err)
		}
	})
}

func writeFile(t *testing.T, directory, name, content string) {
	t.Helper()

	if err := os.WriteFile(filepath.Join(directory, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
