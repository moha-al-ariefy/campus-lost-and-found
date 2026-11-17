package backend

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

const checkNotRecorded = "Not recorded"

type verificationChecks struct {
	DetailsCheck  string `json:"detailsCheck"`
	IdentityCheck string `json:"identityCheck"`
}

func (checks verificationChecks) bothPassed() bool {
	return checks.DetailsCheck == "Passed" && checks.IdentityCheck == "Passed"
}

type claimDecisionInput struct {
	Outcome    string `json:"outcome"`
	Reason     string `json:"reason"`
	ReopenedAt string `json:"reopenedAt,omitempty"`
	verificationChecks
}

func loadClaimChecks(db queryer, id string) (verificationChecks, string, error) {
	var checks verificationChecks
	var reviewer string
	err := db.QueryRow(`SELECT d.details_check,d.identity_check,a.name FROM claim_decisions d
		JOIN accounts a ON a.id=d.decided_by WHERE d.claim_id=?`, id).
		Scan(&checks.DetailsCheck, &checks.IdentityCheck, &reviewer)
	if errors.Is(err, sql.ErrNoRows) {
		return verificationChecks{DetailsCheck: checkNotRecorded, IdentityCheck: checkNotRecorded}, "", nil
	}
	return checks, reviewer, err
}

func (a *API) claimEvidence(w http.ResponseWriter, r *http.Request, id string, actor *Account) {
	if actor.Role != "admin" {
		fail(w, 403, "forbidden")
		return
	}
	tx, err := a.DB.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	claim, err := loadClaimDetail(tx, id, actor)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "claimNotFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	item, err := loadItem(tx, claim.ItemID, actor)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	checks, reviewer, err := loadClaimChecks(tx, id)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"claim": claim, "item": item, "checks": checks, "reviewer": reviewer})
}

func validVerificationCheck(value string) bool {
	return value == "Passed" || value == "Failed" || value == checkNotRecorded
}

func (a *API) decideClaim(w http.ResponseWriter, r *http.Request, id string, actor *Account) {
	if actor.Role != "admin" {
		fail(w, 403, "forbidden")
		return
	}
	var decision claimDecisionInput
	if !readBody(w, r, &decision) {
		return
	}
	decision.Reason = strings.TrimSpace(decision.Reason)
	if decision.DetailsCheck == "" {
		decision.DetailsCheck = checkNotRecorded
	}
	if decision.IdentityCheck == "" {
		decision.IdentityCheck = checkNotRecorded
	}
	if !validVerificationCheck(decision.DetailsCheck) || !validVerificationCheck(decision.IdentityCheck) {
		fail(w, 400, "invalidClaimChecks")
		return
	}
	var nextStatus string
	switch decision.Outcome {
	case "approve":
		nextStatus = "Approved"
		decision.Reason = ""
	case "reject":
		nextStatus = "Rejected"
		if decision.Reason == "" || utf8.RuneCountInString(decision.Reason) > 1000 {
			fail(w, 400, "claimReasonRequired")
			return
		}
	default:
		fail(w, 400, "invalidClaimDecision")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !submissionPattern.MatchString(key) {
		fail(w, 400, "submissionKeyRequired")
		return
	}
	fingerprint := hash(encode([]any{"claim-decision", id, decision}))
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	if replayClaim(w, tx, actor.ID, key, fingerprint) {
		return
	}

	var currentStatus, itemStatus, itemID, reopenedAt string
	var alreadyApproved bool
	err = tx.QueryRow(`SELECT c.status,i.status,c.item_id,
		EXISTS(SELECT 1 FROM claims approved WHERE approved.item_id=c.item_id AND approved.status='Approved'),
		coalesce(o.reopened_at,'')
		FROM claims c JOIN items i ON i.id=c.item_id LEFT JOIN claim_reopenings o ON o.claim_id=c.id WHERE c.id=?`, id).
		Scan(&currentStatus, &itemStatus, &itemID, &alreadyApproved, &reopenedAt)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "claimNotFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	if currentStatus != "Submitted" {
		fail(w, 409, "claimNotSubmitted")
		return
	}
	if decision.ReopenedAt != reopenedAt {
		fail(w, 409, "claimDecisionConflict")
		return
	}
	if nextStatus == "Approved" {
		switch {
		case !decision.bothPassed():
			fail(w, 400, "claimChecksRequired")
			return
		case itemStatus != "Open":
			fail(w, 409, "itemNotOpen")
			return
		case alreadyApproved:
			fail(w, 409, "claimAlreadyApproved")
			return
		}
	}
	result, err := tx.Exec(`UPDATE claims SET status=?,reason=? WHERE id=? AND status='Submitted'
		AND (?='Rejected' OR (EXISTS(SELECT 1 FROM items WHERE id=claims.item_id AND status='Open')
		AND NOT EXISTS(SELECT 1 FROM claims sibling WHERE sibling.item_id=claims.item_id AND sibling.status='Approved')))`,
		nextStatus, decision.Reason, id, nextStatus)
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
		fail(w, 409, "claimDecisionConflict")
		return
	}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec(`INSERT INTO claim_decisions(claim_id,decided_by,decided_at,details_check,identity_check) VALUES(?,?,?,?,?)`,
		id, actor.ID, timestamp, decision.DetailsCheck, decision.IdentityCheck)
	if err == nil {
		event := map[string]any{"before": currentStatus, "after": nextStatus, "itemId": itemID, "reason": decision.Reason, "checks": decision.verificationChecks}
		_, err = tx.Exec("INSERT INTO claim_history(claim_id,actor_id,action,timestamp,values_json) VALUES(?,?,'decided',?,?)", id, actor.ID, timestamp, encode(event))
	}
	var receipt claimReceipt
	if err == nil {
		// SD-1 reads the decision and both checks back before giving a confirmation.
		var savedChecks verificationChecks
		var reviewer string
		err = tx.QueryRow(`SELECT c.id,c.item_id,c.status,c.reason,d.decided_at,d.decided_by,d.details_check,d.identity_check
			FROM claims c JOIN claim_decisions d ON c.id=d.claim_id WHERE c.id=?`, id).
			Scan(&receipt.ID, &receipt.ItemID, &receipt.Status, &receipt.Reason, &receipt.DecidedAt,
				&reviewer, &savedChecks.DetailsCheck, &savedChecks.IdentityCheck)
		if err == nil && (receipt.Status != nextStatus || receipt.ItemID != itemID || receipt.Reason != decision.Reason ||
			receipt.DecidedAt != timestamp || reviewer != actor.ID || savedChecks != decision.verificationChecks) {
			err = errors.New("claim decision read-back differs from the pending decision")
		}
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO claim_submissions VALUES(?,?,?,?,?)", actor.ID, key, fingerprint, id, encode(receipt))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"claim": receipt, "replayed": false})
}
