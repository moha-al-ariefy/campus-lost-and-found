package backend

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/scrypt"
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
        CREATE TABLE IF NOT EXISTS holding_period_settings (
          id INTEGER PRIMARY KEY CHECK(id=1), days INTEGER NOT NULL CHECK(days>0)
        );
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
        CREATE INDEX IF NOT EXISTS item_history_by_record ON history(record_id,sequence);
        CREATE TABLE IF NOT EXISTS item_activity_submissions (
          account_id TEXT NOT NULL REFERENCES accounts(id), submission_key TEXT NOT NULL,
          request_hash TEXT NOT NULL, response_json TEXT NOT NULL,
          PRIMARY KEY(account_id,submission_key)
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
        CREATE TABLE IF NOT EXISTS lost_reports (
          id TEXT PRIMARY KEY, reporter_id TEXT NOT NULL REFERENCES accounts(id),
          category TEXT NOT NULL, date_lost TEXT NOT NULL, description TEXT NOT NULL,
          color TEXT NOT NULL, brand TEXT NOT NULL, building TEXT NOT NULL, contact TEXT NOT NULL,
          assisted INTEGER NOT NULL CHECK(assisted IN (0,1)), created_at TEXT NOT NULL
        );
        CREATE INDEX IF NOT EXISTS lost_report_owner ON lost_reports(reporter_id,created_at DESC,id);
        CREATE TABLE IF NOT EXISTS lost_report_photos (
          report_id TEXT PRIMARY KEY REFERENCES lost_reports(id),
          mime TEXT NOT NULL CHECK(mime IN ('image/jpeg','image/png')),
          content BLOB NOT NULL CHECK(length(content) BETWEEN 1 AND 1000000)
        );
        CREATE TABLE IF NOT EXISTS lost_report_history (
          report_id TEXT PRIMARY KEY REFERENCES lost_reports(id), actor_id TEXT NOT NULL REFERENCES accounts(id),
          timestamp TEXT NOT NULL, action TEXT NOT NULL CHECK(action='created'), values_json TEXT NOT NULL
        );
        CREATE TRIGGER IF NOT EXISTS report_history_no_update BEFORE UPDATE ON lost_report_history
          BEGIN SELECT RAISE(ABORT, 'history_immutable'); END;
        CREATE TRIGGER IF NOT EXISTS report_history_no_delete BEFORE DELETE ON lost_report_history
          BEGIN SELECT RAISE(ABORT, 'history_immutable'); END;
        CREATE TABLE IF NOT EXISTS lost_report_submissions (
          account_id TEXT NOT NULL REFERENCES accounts(id), submission_key TEXT NOT NULL,
          request_hash TEXT NOT NULL, report_id TEXT NOT NULL REFERENCES lost_reports(id),
          PRIMARY KEY(account_id, submission_key)
        );
        CREATE TABLE IF NOT EXISTS claims (
          id TEXT PRIMARY KEY, item_id TEXT NOT NULL REFERENCES items(id),
          claimant_id TEXT NOT NULL REFERENCES accounts(id), description_answer TEXT NOT NULL,
          loss_answer TEXT NOT NULL, contact TEXT NOT NULL,
          status TEXT NOT NULL CHECK(status IN ('Submitted','Approved','Rejected')),
          created_at TEXT NOT NULL, reason TEXT NOT NULL DEFAULT ''
        );
        CREATE UNIQUE INDEX IF NOT EXISTS claims_one_active_per_claimant
          ON claims(item_id,claimant_id) WHERE status IN ('Submitted','Approved');
        CREATE UNIQUE INDEX IF NOT EXISTS claims_one_approval ON claims(item_id) WHERE status='Approved';
        CREATE INDEX IF NOT EXISTS claims_by_owner ON claims(claimant_id,created_at DESC,id);
        CREATE TABLE IF NOT EXISTS handovers (
          claim_id TEXT PRIMARY KEY REFERENCES claims(id), item_id TEXT NOT NULL UNIQUE REFERENCES items(id),
          claimant_id TEXT NOT NULL REFERENCES accounts(id), releasing_account_id TEXT NOT NULL REFERENCES accounts(id),
          released_at TEXT NOT NULL, verification TEXT NOT NULL CHECK(length(verification)>0),
          receipt_reference TEXT NOT NULL CHECK(length(receipt_reference)>0),
          signed_acknowledgment INTEGER NOT NULL CHECK(signed_acknowledgment=1),
          submission_key TEXT NOT NULL, request_hash TEXT NOT NULL, UNIQUE(releasing_account_id,submission_key)
        );
        CREATE TABLE IF NOT EXISTS dispositions (
          item_id TEXT PRIMARY KEY REFERENCES items(id),
          kind TEXT NOT NULL CHECK(kind IN ('Returned','Handed to Police','Donated','Disposed')),
          occurred_at TEXT NOT NULL, account_id TEXT NOT NULL REFERENCES accounts(id),
          handover_claim_id TEXT UNIQUE REFERENCES handovers(claim_id),
          decision_basis TEXT NOT NULL DEFAULT '', police_reference TEXT NOT NULL DEFAULT '',
          CHECK((kind='Returned') = (handover_claim_id IS NOT NULL))
        );
        CREATE TABLE IF NOT EXISTS claim_decisions (
          claim_id TEXT PRIMARY KEY REFERENCES claims(id), decided_by TEXT NOT NULL REFERENCES accounts(id),
          decided_at TEXT NOT NULL,
          details_check TEXT NOT NULL CHECK(details_check IN ('Passed','Failed','Not recorded')),
          identity_check TEXT NOT NULL CHECK(identity_check IN ('Passed','Failed','Not recorded'))
        );
        CREATE TABLE IF NOT EXISTS claim_review_requests (
          claim_id TEXT PRIMARY KEY REFERENCES claims(id), requested_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS claim_reopenings (
          claim_id TEXT PRIMARY KEY REFERENCES claims(id), reopened_at TEXT NOT NULL
        );
        CREATE TABLE IF NOT EXISTS claim_history (
          sequence INTEGER PRIMARY KEY AUTOINCREMENT, claim_id TEXT NOT NULL REFERENCES claims(id),
          actor_id TEXT NOT NULL REFERENCES accounts(id), action TEXT NOT NULL, timestamp TEXT NOT NULL, values_json TEXT NOT NULL
        );
        CREATE INDEX IF NOT EXISTS claim_history_by_claim ON claim_history(claim_id,sequence);
        CREATE INDEX IF NOT EXISTS claims_by_item ON claims(item_id);
        CREATE TRIGGER IF NOT EXISTS claim_history_no_update BEFORE UPDATE ON claim_history
          BEGIN SELECT RAISE(ABORT, 'history_immutable'); END;
        CREATE TRIGGER IF NOT EXISTS claim_history_no_delete BEFORE DELETE ON claim_history
          BEGIN SELECT RAISE(ABORT, 'history_immutable'); END;
        CREATE TABLE IF NOT EXISTS claim_submissions (
          account_id TEXT NOT NULL REFERENCES accounts(id), submission_key TEXT NOT NULL, request_hash TEXT NOT NULL,
          claim_id TEXT NOT NULL REFERENCES claims(id), response_json TEXT NOT NULL,
          PRIMARY KEY(account_id, submission_key)
        );
    `)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := ensureItemPublicName(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("add public item names: %w", err)
	}
	return db, nil
}

func ensureItemPublicName(db *sql.DB) error {
	const columnExists = `SELECT EXISTS(SELECT 1 FROM pragma_table_info('items') WHERE name='public_name')`
	var exists bool
	if err := db.QueryRow(columnExists).Scan(&exists); err != nil || exists {
		return err
	}
	_, err := db.Exec(`ALTER TABLE items ADD COLUMN public_name TEXT NOT NULL DEFAULT '' CHECK(length(public_name) <= 80)`)
	if err != nil {
		// Another process may have opened the old database at the same time.
		if check := db.QueryRow(columnExists).Scan(&exists); check == nil && exists {
			return nil
		}
	}
	return err
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
