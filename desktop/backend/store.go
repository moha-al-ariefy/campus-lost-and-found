package backend

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/scrypt"
	_ "modernc.org/sqlite"
)

var Categories = []string{"electronics", "keys", "cards", "bags", "clothing", "books", "other"}
var HoldingPoints = []string{"reception", "student-affairs"}

type Account struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Role          string   `json:"role"`
	HoldingPoints []string `json:"holdingPoints"`
}

func OpenStore(path string) (*sql.DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// One connection serializes local writes. The transaction still owns every invariant.
	db.SetMaxOpenConns(1)
	_, err = db.Exec(`
        PRAGMA journal_mode=WAL;
        PRAGMA synchronous=FULL;
        PRAGMA foreign_keys=ON;
        PRAGMA busy_timeout=5000;
        CREATE TABLE IF NOT EXISTS accounts (
          id TEXT PRIMARY KEY, name TEXT NOT NULL, login TEXT UNIQUE NOT NULL,
          role TEXT NOT NULL CHECK(role IN ('campus','staff','admin')),
          holding_points TEXT NOT NULL, salt TEXT NOT NULL, password_hash TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS sessions (
          token_hash TEXT PRIMARY KEY, account_id TEXT NOT NULL REFERENCES accounts(id), expires_at INTEGER NOT NULL
        );
        CREATE TABLE IF NOT EXISTS items (
          id TEXT PRIMARY KEY, category TEXT NOT NULL, location TEXT NOT NULL,
          date_found TEXT NOT NULL, holding_point TEXT NOT NULL,
          status TEXT NOT NULL DEFAULT 'Open', creator_id TEXT NOT NULL REFERENCES accounts(id),
          private_data TEXT NOT NULL, created_at TEXT NOT NULL
        );
        CREATE INDEX IF NOT EXISTS directory ON items(status, date_found DESC);
        CREATE TABLE IF NOT EXISTS item_photos (
          item_id TEXT PRIMARY KEY REFERENCES items(id),
          mime TEXT NOT NULL CHECK(mime IN ('image/jpeg','image/png')),
          content BLOB NOT NULL CHECK(length(content) BETWEEN 1 AND 1000000)
        );
        CREATE TABLE IF NOT EXISTS history (
          sequence INTEGER PRIMARY KEY AUTOINCREMENT, record_id TEXT NOT NULL REFERENCES items(id),
          action TEXT NOT NULL, account_id TEXT NOT NULL REFERENCES accounts(id),
          timestamp TEXT NOT NULL, values_json TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS submissions (
          account_id TEXT NOT NULL REFERENCES accounts(id), submission_key TEXT NOT NULL,
          request_hash TEXT NOT NULL, item_id TEXT NOT NULL REFERENCES items(id),
          PRIMARY KEY(account_id, submission_key)
        );
        CREATE TRIGGER IF NOT EXISTS history_no_update BEFORE UPDATE ON history
          BEGIN SELECT RAISE(ABORT, 'history_immutable'); END;
        CREATE TRIGGER IF NOT EXISTS history_no_delete BEFORE DELETE ON history
          BEGIN SELECT RAISE(ABORT, 'history_immutable'); END;
    `)
	if err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type executor interface {
	Exec(string, ...any) (sql.Result, error)
}

func CreateAccount(db executor, id, name, login, role string, points []string, password string) error {
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return err
	}
	salt := hex.EncodeToString(saltBytes)
	key, err := derivePassword(password, salt)
	if err != nil {
		return err
	}
	if points == nil {
		points = []string{}
	}
	encodedPoints, _ := json.Marshal(points)
	_, err = db.Exec("INSERT INTO accounts VALUES (?,?,?,?,?,?,?)", id, name, login, role, string(encodedPoints), salt, hex.EncodeToString(key))
	return err
}

func derivePassword(password, salt string) ([]byte, error) {
	if strongerSalt, ok := strings.CutPrefix(salt, "s2:"); ok {
		return scrypt.Key([]byte(password), []byte(strongerSalt), 32768, 8, 3, 64)
	}
	return scrypt.Key([]byte(password), []byte(salt), 16384, 8, 1, 64)
}

func SeedAccounts(db *sql.DB, password string) error {
	return SeedTestAccounts(db, [4]string{password, password, password, password})
}

// SeedTestAccounts provisions the staging roster in student, reception, affairs, admin order.
func SeedTestAccounts(db *sql.DB, passwords [4]string) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	accounts := []struct {
		id, name, role string
		points         []string
	}{
		{"student", "Campus user", "campus", nil},
		{"reception", "Reception staff", "staff", []string{"reception"}},
		{"affairs", "Student Affairs staff", "staff", []string{"student-affairs"}},
		{"admin", "Administrator", "admin", nil},
	}
	for index, account := range accounts {
		if err := CreateAccount(tx, account.id, account.name, account.id, account.role, account.points, passwords[index]); err != nil {
			return err
		}
	}
	return tx.Commit()
}
