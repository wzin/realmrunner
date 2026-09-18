package rcon

import (
	"reflect"
	"testing"
)

func TestParsePlayerList(t *testing.T) {
	tests := []struct {
		name     string
		response string
		want     PlayerList
	}{
		{
			name:     "modern format",
			response: "There are 2 of a max of 20 players online: Alice, Bob",
			want:     PlayerList{Online: 2, Max: 20, Names: []string{"Alice", "Bob"}},
		},
		{
			name:     "legacy format",
			response: "There are 1/20 players online: Alice",
			want:     PlayerList{Online: 1, Max: 20, Names: []string{"Alice"}},
		},
		{
			name:     "empty server",
			response: "There are 0 of a max of 20 players online:",
			want:     PlayerList{Online: 0, Max: 20, Names: []string{}},
		},
		{
			name:     "colour codes are stripped",
			response: "§fThere are §a3§f of a max of §a20§f players online: Alice, Bob, Carol",
			want:     PlayerList{Online: 3, Max: 20, Names: []string{"Alice", "Bob", "Carol"}},
		},
		{
			name:     "annotated names",
			response: "There are 1 of a max of 20 players online: Alice (survival)",
			want:     PlayerList{Online: 1, Max: 20, Names: []string{"Alice"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePlayerList(tt.response)
			if err != nil {
				t.Fatalf("ParsePlayerList: %v", err)
			}
			if !reflect.DeepEqual(*got, tt.want) {
				t.Errorf("got %+v, want %+v", *got, tt.want)
			}
		})
	}
}

func TestParsePlayerListRejectsGarbage(t *testing.T) {
	for _, response := range []string{"", "Unknown command", "no players here"} {
		if _, err := ParsePlayerList(response); err == nil {
			t.Errorf("ParsePlayerList(%q) succeeded, want an error", response)
		}
	}
}
