package backup

import (
	"archive/tar"
	"compress/gzip"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Backup struct {
	ID        string    `json:"id"`
	ServerID  string    `json:"server_id"`
	Filename  string    `json:"filename"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

func InitBackupTable(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS backups (
		id TEXT PRIMARY KEY,
		server_id TEXT NOT NULL,
		filename TEXT NOT NULL,
		size_bytes INTEGER DEFAULT 0,
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err := db.Exec(schema)
	return err
}

func CreateBackup(db *sql.DB, dataDir, serverID string) (*Backup, error) {
	serverDir := filepath.Join(dataDir, "servers", serverID)
	if _, err := os.Stat(serverDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("server directory not found")
	}

	backupDir := filepath.Join(dataDir, "backups", serverID)
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create backup directory: %w", err)
	}

	backupID := uuid.New().String()
	timestamp := time.Now().Format("2006-01-02_15-04-05")
	filename := fmt.Sprintf("backup_%s.tar.gz", timestamp)
	backupPath := filepath.Join(backupDir, filename)

	// Create tar.gz
	file, err := os.Create(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create backup file: %w", err)
	}
	defer file.Close()

	gw := gzip.NewWriter(file)
	defer gw.Close()

	tw := tar.NewWriter(gw)
	defer tw.Close()

	err = filepath.Walk(serverDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		// Skip the logs directory to save space
		rel, _ := filepath.Rel(serverDir, path)
		if strings.HasPrefix(rel, "logs") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = rel

		if err := tw.WriteHeader(header); err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()

		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		os.Remove(backupPath)
		return nil, fmt.Errorf("failed to create backup: %w", err)
	}

	// Close writers to flush
	tw.Close()
	gw.Close()
	file.Close()

	// Get file size
	stat, _ := os.Stat(backupPath)
	sizeBytes := stat.Size()

	backup := &Backup{
		ID:        backupID,
		ServerID:  serverID,
		Filename:  filename,
		SizeBytes: sizeBytes,
		CreatedAt: time.Now(),
	}

	query := `INSERT INTO backups (id, server_id, filename, size_bytes, created_at) VALUES (?, ?, ?, ?, ?)`
	if _, err := db.Exec(query, backup.ID, backup.ServerID, backup.Filename, backup.SizeBytes, backup.CreatedAt); err != nil {
		return nil, err
	}

	log.Printf("Created backup %s for server %s (%.2f MB)", filename, serverID, float64(sizeBytes)/(1024*1024))
	return backup, nil
}

func ListBackups(db *sql.DB, serverID string) ([]Backup, error) {
	query := `SELECT id, server_id, filename, size_bytes, created_at FROM backups WHERE server_id = ? ORDER BY created_at DESC`
	rows, err := db.Query(query, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var backups []Backup
	for rows.Next() {
		var b Backup
		if err := rows.Scan(&b.ID, &b.ServerID, &b.Filename, &b.SizeBytes, &b.CreatedAt); err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, nil
}

func RestoreBackup(db *sql.DB, dataDir, serverID, backupID string) error {
	// Get backup record
	var filename string
	err := db.QueryRow(`SELECT filename FROM backups WHERE id = ? AND server_id = ?`, backupID, serverID).Scan(&filename)
	if err != nil {
		return fmt.Errorf("backup not found")
	}

	backupPath := filepath.Join(dataDir, "backups", serverID, filename)
	serverDir := filepath.Join(dataDir, "servers", serverID)

	// Clear server directory (except logs)
	entries, _ := os.ReadDir(serverDir)
	for _, e := range entries {
		if e.Name() == "logs" {
			continue
		}
		os.RemoveAll(filepath.Join(serverDir, e.Name()))
	}

	// Extract backup
	file, err := os.Open(backupPath)
	if err != nil {
		return fmt.Errorf("backup file not found: %w", err)
	}
	defer file.Close()

	gr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("invalid backup file: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		target := filepath.Join(serverDir, header.Name)

		// Security: ensure target is within serverDir
		absTarget, _ := filepath.Abs(target)
		absServer, _ := filepath.Abs(serverDir)
		if !strings.HasPrefix(absTarget, absServer) {
			continue
		}

		switch header.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0755)
			f, err := os.Create(target)
			if err != nil {
				continue
			}
			io.Copy(f, tr)
			f.Close()
		}
	}

	log.Printf("Restored backup %s for server %s", filename, serverID)
	return nil
}

func DeleteBackup(db *sql.DB, dataDir, serverID, backupID string) error {
	var filename string
	err := db.QueryRow(`SELECT filename FROM backups WHERE id = ? AND server_id = ?`, backupID, serverID).Scan(&filename)
	if err != nil {
		return fmt.Errorf("backup not found")
	}

	backupPath := filepath.Join(dataDir, "backups", serverID, filename)
	os.Remove(backupPath)

	db.Exec(`DELETE FROM backups WHERE id = ?`, backupID)
	return nil
}
