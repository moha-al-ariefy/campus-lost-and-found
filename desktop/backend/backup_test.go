package backend

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupFileRefusesMissingAndUninitializedSources(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "campus.db")
	destination := filepath.Join(directory, "snapshot.db")
	if err := BackupFile(source, destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected missing source error, got %v", err)
	}
	if _, err := os.Stat(source); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("backup created a database at the missing source path")
	}
	if err := os.WriteFile(source, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if err := BackupFile(source, destination); err == nil {
		t.Fatal("backup accepted an empty source file")
	}
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("CREATE TABLE unrelated (id INTEGER)"); err != nil {
		t.Fatal(err)
	}
	if err := BackupFile(source, destination); err == nil {
		t.Fatal("backup accepted a database without the campus tables")
	}
	if _, err := os.Stat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("invalid source left a published snapshot")
	}
}

func TestBackupFileLeavesExistingSchemaUntouched(t *testing.T) {
	directory := t.TempDir()
	source := filepath.Join(directory, "campus #source.db")
	db, err := sql.Open("sqlite", source)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE accounts (id TEXT PRIMARY KEY);
		CREATE TABLE items (id TEXT PRIMARY KEY);
		CREATE TABLE history (id TEXT);
		INSERT INTO items VALUES ('CLF-backup-fixture');`); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(directory, "snapshot.db")
	if err := BackupFile(source, destination); err != nil {
		t.Fatal(err)
	}
	if count(t, db, "sqlite_schema WHERE type='table'") != 3 {
		t.Fatal("backup changed the source schema")
	}
	snapshot, err := sql.Open("sqlite", destination)
	if err != nil {
		t.Fatal(err)
	}
	defer snapshot.Close()
	var id string
	if err := snapshot.QueryRow("SELECT id FROM items").Scan(&id); err != nil || id != "CLF-backup-fixture" {
		t.Fatalf("snapshot lost the original record: id=%q err=%v", id, err)
	}
}
