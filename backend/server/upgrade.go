package server

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/wzin/realmrunner/backup"
)

// upgradeVerifyTimeout is how long an upgraded server gets to come up before
// the upgrade is considered failed and rolled back.
const upgradeVerifyTimeout = 5 * time.Minute

// UpgradeResult describes what an upgrade did, including whether it had to roll
// back and why.
type UpgradeResult struct {
	Version    string `json:"version"`
	Flavor     string `json:"flavor"`
	BackupID   string `json:"backup_id,omitempty"`
	RolledBack bool   `json:"rolled_back"`
	Message    string `json:"message"`
}

// UpgradeServer changes a server's version or flavor, taking a backup first and
// putting everything back if the new version fails to start.
//
// The sequence is: check a suitable Java runtime exists, back the world up,
// swap the jar, then start the server and wait for it to answer on its control
// port. Any failure restores the previous jar and version.
func (m *Manager) UpgradeServer(id, version, flavor string) (*UpgradeResult, error) {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return nil, err
	}

	// A sleeping server is not running either: its port is merely held open.
	if srv.Status != StatusStopped && srv.Status != StatusCrashed && srv.Status != StatusSleeping {
		return nil, fmt.Errorf("server must be stopped to upgrade")
	}

	if flavor == "" {
		flavor = srv.Flavor
	}

	// A version the image cannot run would fail at start; say so before
	// touching anything.
	if err := preflightJava(version); err != nil {
		return nil, err
	}

	provider, ok := m.registry.GetProvider(flavor)
	if !ok {
		return nil, fmt.Errorf("unknown server flavor: %s", flavor)
	}

	result := &UpgradeResult{Version: version, Flavor: flavor}

	// Back up the world before changing anything. A failure here stops the
	// upgrade: an unprotected upgrade is exactly what we are trying to avoid.
	snapshot, err := backup.CreateBackup(m.db, m.config.DataDir, id)
	if err != nil {
		return nil, fmt.Errorf("failed to back up the server before upgrading: %w", err)
	}
	result.BackupID = snapshot.ID
	log.Printf("Upgrade of %s: created backup %s", id, snapshot.ID)

	serverDir := m.getServerDir(id)
	jarPath := filepath.Join(serverDir, "server.jar")
	previousJar := filepath.Join(serverDir, "server.jar.previous")

	// Keep the old jar so a failed upgrade can be undone without unpacking the
	// backup.
	os.Remove(previousJar)
	jarExisted := false
	if _, err := os.Stat(jarPath); err == nil {
		if err := os.Rename(jarPath, previousJar); err != nil {
			return nil, fmt.Errorf("failed to set the current jar aside: %w", err)
		}
		jarExisted = true
	}

	rollback := func(reason string) (*UpgradeResult, error) {
		os.Remove(jarPath)
		if jarExisted {
			os.Rename(previousJar, jarPath)
		}
		UpdateServerVersion(m.db, id, srv.Version, srv.Flavor)
		SetServerReady(m.db, id, jarExisted)
		UpdateServerStatus(m.db, id, m.restingStatus(id))
		RecordExit(m.db, id, 0, reason)

		result.RolledBack = true
		result.Version = srv.Version
		result.Flavor = srv.Flavor
		result.Message = reason
		log.Printf("Upgrade of %s rolled back: %s", id, reason)
		return result, fmt.Errorf("%s", reason)
	}

	if err := provider.DownloadServer(serverDir, version); err != nil {
		return rollback(fmt.Sprintf("Download of %s %s failed: %v. The previous version was restored.", flavor, version, err))
	}

	if err := UpdateServerVersion(m.db, id, version, flavor); err != nil {
		return rollback(fmt.Sprintf("Could not record the new version: %v. The previous version was restored.", err))
	}
	SetServerReady(m.db, id, true)

	// Verify the new version actually runs.
	if err := m.StartServer(id); err != nil {
		return rollback(fmt.Sprintf("%s %s failed to start: %v. The previous version was restored.", flavor, version, err))
	}

	if err := m.waitUntilReady(id, upgradeVerifyTimeout); err != nil {
		// Leave nothing running on a failed upgrade.
		m.ForceStopServer(id)
		return rollback(fmt.Sprintf("%s %s did not finish starting: %v. The previous version was restored; the world backup %s is untouched.", flavor, version, err, snapshot.ID))
	}

	os.Remove(previousJar)
	result.Message = fmt.Sprintf("Upgraded to %s %s and verified it starts. It is running now; backup %s was taken beforehand.", flavor, version, snapshot.ID)
	log.Printf("Upgrade of %s to %s %s succeeded", id, flavor, version)
	return result, nil
}

// waitUntilReady blocks until the server answers on its control port, it
// crashes, or the timeout expires.
func (m *Manager) waitUntilReady(id string, timeout time.Duration) error {
	srv, err := GetServer(m.db, id)
	if err != nil {
		return err
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		current, err := GetServer(m.db, id)
		if err != nil {
			return err
		}
		if current.Status == StatusCrashed || current.Status == StatusStopped {
			if current.LastError != "" {
				return fmt.Errorf("%s", current.LastError)
			}
			return fmt.Errorf("the server stopped while starting")
		}

		if err := m.waitForRCON(srv, 5*time.Second); err == nil {
			return nil
		}
	}

	return fmt.Errorf("timed out after %s", timeout)
}
