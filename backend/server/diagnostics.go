package server

import (
	"regexp"
	"sort"
	"strings"
)

// Diagnostics summarises what a server's log says about connection quality, so
// a pattern of disconnects can be seen without reading thousands of lines.
type Diagnostics struct {
	Disconnects      []DisconnectSummary `json:"disconnects"`
	DisconnectEvents []DisconnectEvent   `json:"recent_disconnects"`
	Players          []PlayerSummary     `json:"players"`
	LagWarnings      int                 `json:"lag_warnings"`
	WorstLagMs       int                 `json:"worst_lag_ms"`
	Restarts         int                 `json:"restarts"`
	Notes            []string            `json:"notes"`
}

// DisconnectSummary counts one kind of disconnect.
type DisconnectSummary struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
	// Meaning explains, in plain words, what this reason indicates.
	Meaning string `json:"meaning"`
}

// DisconnectEvent is a single disconnect, newest last.
type DisconnectEvent struct {
	Time   string `json:"time"`
	Player string `json:"player"`
	Reason string `json:"reason"`
	IP     string `json:"ip,omitempty"`
}

// PlayerSummary counts how often one player dropped.
type PlayerSummary struct {
	Name        string   `json:"name"`
	Disconnects int      `json:"disconnects"`
	Logins      int      `json:"logins"`
	IPs         []string `json:"ips,omitempty"`
}

var (
	// "Ggocio lost connection: Disconnected" and
	// "unbanZEIY (/1.2.3.4:5678) lost connection: Timed out"
	disconnectRe = regexp.MustCompile(`^\[(\d{2}:\d{2}:\d{2})\].*?: ([A-Za-z0-9_]{1,16})(?: \((?:/)?([0-9a-fA-F:.]+):\d+\))?(?: \([0-9a-f-]{36}\))? lost connection: (.+)$`)
	// "Ggocio[/1.2.3.4:5678] logged in with entity id ..."
	loginRe = regexp.MustCompile(`^\[(\d{2}:\d{2}:\d{2})\].*?: ([A-Za-z0-9_]{1,16})\[/([0-9a-fA-F:.]+):\d+\] logged in`)
	// "Can't keep up! Is the server overloaded? Running 2051ms or 41 ticks behind"
	lagRe = regexp.MustCompile(`Can't keep up!.*Running (\d+)ms`)
)

// disconnectMeanings explains the reasons Minecraft reports.
var disconnectMeanings = map[string]string{
	"Disconnected":                        "The connection dropped without the client saying goodbye: a network path problem (home router, NAT or ISP) rather than the server rejecting anyone.",
	"Timed out":                           "The client stopped answering keep-alives for 30 seconds: either the connection broke or the server stalled long enough to miss them.",
	"You logged in from another location": "The same account connected again while the server still held the old session, which is what happens when a connection dies silently and the player reconnects.",
	"Server closed":                       "The server shut down while the player was online.",
	"Internal Exception":                  "The connection failed at the protocol level, which usually means the stream was cut mid-packet.",
}

// AnalyseLogs reads a server's log and summarises its connection history.
func AnalyseLogs(lines []string) *Diagnostics {
	diag := &Diagnostics{}

	reasons := map[string]int{}
	players := map[string]*PlayerSummary{}
	seenIPs := map[string]map[string]bool{}

	player := func(name string) *PlayerSummary {
		if _, ok := players[name]; !ok {
			players[name] = &PlayerSummary{Name: name}
			seenIPs[name] = map[string]bool{}
		}
		return players[name]
	}

	for _, line := range lines {
		if strings.Contains(line, "Starting minecraft server version") {
			diag.Restarts++
		}

		if match := lagRe.FindStringSubmatch(line); match != nil {
			diag.LagWarnings++
			if ms := atoi(match[1]); ms > diag.WorstLagMs {
				diag.WorstLagMs = ms
			}
		}

		if match := loginRe.FindStringSubmatch(line); match != nil {
			p := player(match[2])
			p.Logins++
			if ip := match[3]; ip != "" && !seenIPs[match[2]][ip] {
				seenIPs[match[2]][ip] = true
				p.IPs = append(p.IPs, ip)
			}
			continue
		}

		if match := disconnectRe.FindStringSubmatch(line); match != nil {
			reason := strings.TrimSpace(match[4])
			reasons[reason]++

			p := player(match[2])
			p.Disconnects++

			diag.DisconnectEvents = append(diag.DisconnectEvents, DisconnectEvent{
				Time:   match[1],
				Player: match[2],
				Reason: reason,
				IP:     match[3],
			})
		}
	}

	for reason, count := range reasons {
		diag.Disconnects = append(diag.Disconnects, DisconnectSummary{
			Reason:  reason,
			Count:   count,
			Meaning: disconnectMeanings[reason],
		})
	}
	sort.Slice(diag.Disconnects, func(i, j int) bool {
		return diag.Disconnects[i].Count > diag.Disconnects[j].Count
	})

	for _, p := range players {
		diag.Players = append(diag.Players, *p)
	}
	sort.Slice(diag.Players, func(i, j int) bool {
		return diag.Players[i].Disconnects > diag.Players[j].Disconnects
	})

	// Keep the tail of the event list: the recent pattern is what matters.
	if len(diag.DisconnectEvents) > 50 {
		diag.DisconnectEvents = diag.DisconnectEvents[len(diag.DisconnectEvents)-50:]
	}

	diag.Notes = buildNotes(diag)
	return diag
}

// buildNotes turns the counts into the observations an operator would make.
func buildNotes(diag *Diagnostics) []string {
	var notes []string

	// Several players sharing one address point at their side of the link.
	addresses := map[string][]string{}
	for _, p := range diag.Players {
		for _, ip := range p.IPs {
			addresses[ip] = append(addresses[ip], p.Name)
		}
	}
	for ip, names := range addresses {
		if len(names) >= 2 {
			sort.Strings(names)
			notes = append(notes,
				"These players share the address "+ip+": "+strings.Join(names, ", ")+
					". Repeated drops affecting only them point at that connection or its router rather than the server.")
		}
	}

	for _, d := range diag.Disconnects {
		switch d.Reason {
		case "You logged in from another location":
			if d.Count > 0 {
				notes = append(notes,
					"A session was replaced "+itoa(d.Count)+" time(s). That happens when a connection dies without the server noticing and the player reconnects before the old session times out.")
			}
		case "Timed out":
			if d.Count > 0 {
				notes = append(notes,
					itoa(d.Count)+" player(s) timed out. Compare these against the lag warnings below: if they line up, the server stalled; if not, the network dropped.")
			}
		}
	}

	if diag.LagWarnings > 0 {
		notes = append(notes,
			itoa(diag.LagWarnings)+" lag warning(s), worst "+itoa(diag.WorstLagMs)+"ms behind. Sustained stalls make clients time out; more heap and the tuned GC flags usually fix it.")
	}

	if diag.Restarts > 1 {
		notes = append(notes,
			"The server started "+itoa(diag.Restarts)+" times in this log. Every restart disconnects everyone who was online.")
	}

	return notes
}

func atoi(value string) int {
	result := 0
	for _, r := range value {
		if r < '0' || r > '9' {
			return result
		}
		result = result*10 + int(r-'0')
	}
	return result
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}
	return digits
}

// Diagnose reads a server's log and analyses it.
func (m *Manager) Diagnose(id string) (*Diagnostics, error) {
	lines, err := ReadHistoricalLogs(m.getServerDir(id))
	if err != nil {
		return nil, err
	}
	return AnalyseLogs(lines), nil
}
