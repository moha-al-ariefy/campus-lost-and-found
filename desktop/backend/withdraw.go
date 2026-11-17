package backend

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

func (a *API) withdrawItem(w http.ResponseWriter, r *http.Request, id string, actor *Account) {
	if actor.Role != "admin" {
		fail(w, 403, "forbidden")
		return
	}
	var input struct {
		Reason string `json:"reason"`
	}
	if !readBody(w, r, &input) {
		return
	}
	reason := strings.TrimSpace(input.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > 1000 {
		reply(w, 400, map[string]any{"code": "invalidFields", "fields": []string{"reason"}})
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !submissionPattern.MatchString(key) {
		fail(w, 400, "submissionKeyRequired")
		return
	}

	fingerprint := hash(encode([]string{"withdraw", id, reason}))
	tx, err := a.DB.Begin()
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()

	var previousHash string
	err = tx.QueryRow("SELECT request_hash FROM submissions WHERE account_id=? AND submission_key=?", actor.ID, key).Scan(&previousHash)
	if err == nil {
		if previousHash != fingerprint {
			fail(w, 409, "submissionChanged")
			return
		}
		item, err := loadItem(tx, id, actor)
		if err != nil {
			fail(w, 503, "unavailable")
			return
		}
		reply(w, 200, map[string]any{"item": item, "replayed": true})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		fail(w, 503, "unavailable")
		return
	}
	var status string
	var approved bool
	err = tx.QueryRow(`SELECT status, EXISTS(SELECT 1 FROM claims WHERE item_id=items.id AND status='Approved') FROM items WHERE id=?`, id).Scan(&status, &approved)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "notFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	if status != "Open" {
		fail(w, 409, "itemNotOpen")
		return
	}
	// M3 Q-13 leaves withdrawal with an approved claim unresolved.
	if approved {
		fail(w, 409, "itemHasApprovedClaim")
		return
	}
	result, err := tx.Exec(`UPDATE items SET status='Withdrawn' WHERE id=? AND status='Open'
		AND NOT EXISTS(SELECT 1 FROM claims WHERE item_id=items.id AND status='Approved')`, id)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	changed, err := result.RowsAffected()
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	if changed != 1 {
		fail(w, 409, "itemNotOpen")
		return
	}
	event := encode(map[string]string{"before": "Open", "after": "Withdrawn", "reason": reason})
	_, err = tx.Exec("INSERT INTO history(record_id,action,account_id,timestamp,values_json) VALUES(?,?,?,?,?)",
		id, "withdrawn", actor.ID, time.Now().UTC().Format(time.RFC3339Nano), event)
	if err == nil {
		_, err = tx.Exec("INSERT INTO submissions VALUES(?,?,?,?)", actor.ID, key, fingerprint, id)
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	item, err := loadItem(tx, id, actor)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"item": item, "replayed": false})
}
