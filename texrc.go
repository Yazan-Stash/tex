package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The .texrc format is specified in .docs/texrc.md. Keep the two in sync.

var errCannotReadTexrc = errors.New("cannot read .texrc")

// readTexrc parses the .texrc in directory. found is false when the file does
// not exist, in which case the built-in layout applies.
func readTexrc(directory string) (windows []window, found bool, err error) {
	path := filepath.Join(directory, ".texrc")

	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}

	if err != nil || !info.Mode().IsRegular() {
		return nil, true, errCannotReadTexrc
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, true, errCannotReadTexrc
	}
	defer file.Close()

	windows, err = parseTexrc(file)

	return windows, true, err
}

// parseTexrc validates the whole file and returns its windows in order.
func parseTexrc(r io.Reader) ([]window, error) {
	reader := bufio.NewReader(r)
	seen := map[string]bool{}
	windows := []window{}
	lineNumber := 0

	for {
		line, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, errCannotReadTexrc
		}

		if line == "" && err != nil {
			break
		}

		lineNumber++
		line = trimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		name, command, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf(".texrc:%d: expected 'name: command'", lineNumber)
		}

		name = trimSpace(name)
		command = trimSpace(command)

		if name == "" {
			return nil, fmt.Errorf(".texrc:%d: window name is empty", lineNumber)
		}

		if seen[name] {
			return nil, fmt.Errorf(".texrc:%d: duplicate window '%s'", lineNumber, name)
		}

		seen[name] = true
		windows = append(windows, window{name: name, command: command})
	}

	if len(windows) == 0 {
		return nil, errors.New(".texrc defines no windows")
	}

	return windows, nil
}

// trimSpace trims ASCII whitespace, including the `\r` of CRLF line endings.
func trimSpace(value string) string {
	return strings.Trim(value, " \t\n\v\f\r")
}
