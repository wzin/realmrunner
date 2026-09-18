package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPropertiesMissingFile(t *testing.T) {
	props, err := ReadProperties(filepath.Join(t.TempDir(), "server.properties"))
	if err != nil {
		t.Fatalf("ReadProperties: %v", err)
	}
	if len(props) != 0 {
		t.Errorf("got %v, want an empty map", props)
	}
}

func TestReadProperties(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.properties")
	os.WriteFile(path, []byte("#Minecraft server properties\n\nserver-port=25565\nmotd=Hello World\nmalformed-line\n"), 0644)

	props, err := ReadProperties(path)
	if err != nil {
		t.Fatalf("ReadProperties: %v", err)
	}
	if props["server-port"] != "25565" {
		t.Errorf("server-port = %q", props["server-port"])
	}
	if props["motd"] != "Hello World" {
		t.Errorf("motd = %q", props["motd"])
	}
	if len(props) != 2 {
		t.Errorf("got %d properties, want 2", len(props))
	}
}

// Updating must not disturb comments or properties the operator set by hand.
func TestUpdatePropertiesPreservesTheRest(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.properties")
	original := "#Minecraft server properties\n#Sat Sep 12 10:00:00 UTC 2026\nmotd=My Realm\nserver-port=25565\ndifficulty=hard\n"
	os.WriteFile(path, []byte(original), 0644)

	err := UpdateProperties(path, map[string]string{
		"server-port":   "30001",
		"enable-rcon":   "true",
		"rcon.port":     "35565",
		"rcon.password": "hunter2",
	})
	if err != nil {
		t.Fatalf("UpdateProperties: %v", err)
	}

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.HasPrefix(content, "#Minecraft server properties\n#Sat Sep 12") {
		t.Errorf("comments were not preserved:\n%s", content)
	}

	props, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if props["motd"] != "My Realm" || props["difficulty"] != "hard" {
		t.Errorf("operator settings were lost: %v", props)
	}
	if props["server-port"] != "30001" {
		t.Errorf("server-port = %q, want 30001", props["server-port"])
	}
	if props["enable-rcon"] != "true" || props["rcon.port"] != "35565" || props["rcon.password"] != "hunter2" {
		t.Errorf("rcon settings were not applied: %v", props)
	}

	// A key must never be duplicated.
	if strings.Count(content, "server-port=") != 1 {
		t.Errorf("server-port appears more than once:\n%s", content)
	}
}

func TestUpdatePropertiesCreatesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.properties")

	if err := UpdateProperties(path, map[string]string{"server-port": "25565"}); err != nil {
		t.Fatalf("UpdateProperties: %v", err)
	}

	props, err := ReadProperties(path)
	if err != nil {
		t.Fatal(err)
	}
	if props["server-port"] != "25565" {
		t.Errorf("got %v", props)
	}
}

func TestUpdatePropertiesNoOp(t *testing.T) {
	path := filepath.Join(t.TempDir(), "server.properties")
	if err := UpdateProperties(path, nil); err != nil {
		t.Fatalf("UpdateProperties: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("an empty update should not create the file")
	}
}
