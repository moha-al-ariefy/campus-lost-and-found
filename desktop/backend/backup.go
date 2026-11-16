package backend

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func BackupStore(db *sql.DB, destination string) error {
	if !filepath.IsAbs(destination) {
		return errors.New("backup destination must be an absolute path")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		return errors.New("backup destination must be a new file in an existing directory")
	}
	temporary, err := os.CreateTemp(filepath.Dir(destination), ".campus-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(temporary.Name())
	if err := temporary.Close(); err != nil {
		return err
	}
	if _, err := db.Exec("VACUUM INTO ?", temporary.Name()); err != nil {
		return fmt.Errorf("write SQLite snapshot: %w", err)
	}
	if err := checkSnapshot(temporary.Name()); err != nil {
		return fmt.Errorf("verify SQLite snapshot: %w", err)
	}
	snapshot, err := os.OpenFile(temporary.Name(), os.O_RDWR, 0)
	if err != nil {
		return err
	}
	syncErr := snapshot.Sync()
	closeErr := snapshot.Close()
	if err := errors.Join(syncErr, closeErr); err != nil {
		return err
	}
	// Publish the finished snapshot without replacing an existing backup.
	return os.Link(temporary.Name(), destination)
}

func checkSnapshot(path string) error {
	snapshot, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer snapshot.Close()
	var integrity string
	if err := snapshot.QueryRow("PRAGMA quick_check").Scan(&integrity); err != nil {
		return err
	}
	if integrity != "ok" {
		return errors.New("snapshot failed SQLite quick_check")
	}
	rows, err := snapshot.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		return errors.New("snapshot contains a broken record reference")
	}
	return rows.Err()
}
