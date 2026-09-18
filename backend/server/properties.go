package server

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"
)

// ReadProperties parses a server.properties file. A missing file is not an
// error: it simply has no properties yet.
func ReadProperties(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	defer file.Close()

	props := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		props[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return props, nil
}

// UpdateProperties sets the given keys in a server.properties file, leaving
// every other line - including comments and the operator's own edits - intact.
func UpdateProperties(path string, updates map[string]string) error {
	if len(updates) == 0 {
		return nil
	}

	var lines []string
	if data, err := os.ReadFile(path); err == nil {
		lines = strings.Split(strings.TrimRight(string(data), "\n"), "\n")
	} else if !os.IsNotExist(err) {
		return err
	}

	remaining := make(map[string]string, len(updates))
	for k, v := range updates {
		remaining[k] = v
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, _, found := strings.Cut(trimmed, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		if value, ok := remaining[key]; ok {
			lines[i] = fmt.Sprintf("%s=%s", key, value)
			delete(remaining, key)
		}
	}

	// Append the keys that were not already present, in a stable order.
	keys := make([]string, 0, len(remaining))
	for k := range remaining {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		lines = append(lines, fmt.Sprintf("%s=%s", k, remaining[k]))
	}

	output := strings.Join(lines, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}
	return os.WriteFile(path, []byte(output), 0644)
}
