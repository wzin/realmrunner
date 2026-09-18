package rcon

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// PlayerList is the parsed result of the server's "list" command.
type PlayerList struct {
	Online int      `json:"online"`
	Max    int      `json:"max"`
	Names  []string `json:"names"`
}

var (
	// "There are 2 of a max of 20 players online: Alice, Bob" (modern) and
	// "There are 2/20 players online: Alice, Bob" (older servers).
	listCountRe = regexp.MustCompile(`(?i)there are (\d+)\s*(?:of a max(?:imum)? of|/)\s*(\d+)`)
	colorCodeRe = regexp.MustCompile("§.")
)

// ParsePlayerList reads the response of the "list" command.
func ParsePlayerList(response string) (*PlayerList, error) {
	clean := strings.TrimSpace(colorCodeRe.ReplaceAllString(response, ""))

	match := listCountRe.FindStringSubmatch(clean)
	if match == nil {
		return nil, fmt.Errorf("rcon: unrecognised player list response: %q", clean)
	}

	online, err := strconv.Atoi(match[1])
	if err != nil {
		return nil, fmt.Errorf("rcon: invalid online count: %w", err)
	}
	max, err := strconv.Atoi(match[2])
	if err != nil {
		return nil, fmt.Errorf("rcon: invalid max count: %w", err)
	}

	list := &PlayerList{Online: online, Max: max, Names: []string{}}

	// Names follow the first colon after the counts.
	if idx := strings.Index(clean, ":"); idx >= 0 {
		for _, name := range strings.Split(clean[idx+1:], ",") {
			name = strings.TrimSpace(name)
			// Some servers annotate names, e.g. "Alice (survival)".
			if space := strings.Index(name, " "); space > 0 {
				name = name[:space]
			}
			if name != "" {
				list.Names = append(list.Names, name)
			}
		}
	}

	return list, nil
}
