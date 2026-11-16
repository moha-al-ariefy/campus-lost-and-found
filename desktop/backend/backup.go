package backend

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
)

// BackupFile reads the existing store without opening it for schema changes.
func BackupFile(source, destination string) error {
	if !filepath.IsAbs(source) {
		return errors.New("backup source must be an absolute path")
	}
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("locate backup source: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("backup source must be a nonempty campus database file")
	}
	path := url.URL{Path: filepath.ToSlash(source)}
	db, err := sql.Open("sqlite", "file:"+path.EscapedPath()+"?mode=ro&_pragma=busy_timeout(5000)")
	if err != nil {
		return fmt.Errorf("open backup source: %w", err)
	}
	defer db.Close()
	var tables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_schema
		WHERE type='table' AND name IN ('accounts','items','history')`).Scan(&tables); err != nil {
		return fmt.Errorf("read backup source schema: %w", err)
	}
	if tables != 3 {
		return errors.New("backup source is missing the campus database tables")
	}
	return BackupStore(db, destination)
}

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
