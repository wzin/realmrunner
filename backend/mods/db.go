package mods

import (
	"database/sql"
	"time"
)

type InstalledMod struct {
	ID          string    `json:"id"`
	ServerID    string    `json:"server_id"`
	ModrinthID  string    `json:"modrinth_id"`
	Name        string    `json:"name"`
	Version     string    `json:"version"`
	Filename    string    `json:"filename"`
	Loader      string    `json:"loader"`
	InstalledAt time.Time `json:"installed_at"`
}

func InitModsTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS installed_mods (
		id TEXT PRIMARY KEY,
		server_id TEXT NOT NULL,
		modrinth_id TEXT NOT NULL,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		filename TEXT NOT NULL,
		loader TEXT NOT NULL,
		installed_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (server_id) REFERENCES servers(id) ON DELETE CASCADE
	);
	`
	_, err := db.Exec(schema)
	return err
}

func ListInstalledMods(db *sql.DB, serverID string) ([]InstalledMod, error) {
	rows, err := db.Query(
		"SELECT id, server_id, modrinth_id, name, version, filename, loader, installed_at FROM installed_mods WHERE server_id = ? ORDER BY name",
		serverID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var mods []InstalledMod
	for rows.Next() {
		var m InstalledMod
		if err := rows.Scan(&m.ID, &m.ServerID, &m.ModrinthID, &m.Name, &m.Version, &m.Filename, &m.Loader, &m.InstalledAt); err != nil {
			return nil, err
		}
		mods = append(mods, m)
	}
	return mods, nil
}

func InsertInstalledMod(db *sql.DB, m *InstalledMod) error {
	_, err := db.Exec(
		"INSERT INTO installed_mods (id, server_id, modrinth_id, name, version, filename, loader, installed_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		m.ID, m.ServerID, m.ModrinthID, m.Name, m.Version, m.Filename, m.Loader, m.InstalledAt,
	)
	return err
}

func GetInstalledMod(db *sql.DB, id string) (*InstalledMod, error) {
	var m InstalledMod
	err := db.QueryRow(
		"SELECT id, server_id, modrinth_id, name, version, filename, loader, installed_at FROM installed_mods WHERE id = ?",
		id,
	).Scan(&m.ID, &m.ServerID, &m.ModrinthID, &m.Name, &m.Version, &m.Filename, &m.Loader, &m.InstalledAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func DeleteInstalledMod(db *sql.DB, id string) error {
	_, err := db.Exec("DELETE FROM installed_mods WHERE id = ?", id)
	return err
}
