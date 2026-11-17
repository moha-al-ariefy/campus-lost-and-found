package backend

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type reviewRequestInput struct {
	DecidedAt string `json:"decidedAt"`
}

func (a *API) requestClaimReview(w http.ResponseWriter, r *http.Request, id string, actor *Account) {
	var input reviewRequestInput
	if !readBody(w, r, &input) {
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !submissionPattern.MatchString(key) {
		fail(w, 400, "submissionKeyRequired")
		return
	}
	fingerprint := hash(encode([]string{"claim-review-request", id, input.DecidedAt}))
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	if replayClaim(w, tx, actor.ID, key, fingerprint) {
		return
	}
	var claim claimReceipt
	claim.ID = id
	err = tx.QueryRow(`SELECT c.item_id,c.status,c.reason,coalesce(d.decided_at,''),coalesce(q.requested_at,'')
		FROM claims c LEFT JOIN claim_decisions d ON d.claim_id=c.id LEFT JOIN claim_review_requests q ON q.claim_id=c.id
		WHERE c.id=? AND c.claimant_id=?`, id, actor.ID).
		Scan(&claim.ItemID, &claim.Status, &claim.Reason, &claim.DecidedAt, &claim.ReviewRequestedAt)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "claimNotFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	if claim.Status != "Rejected" {
		fail(w, 409, "claimNotRejected")
		return
	}
	if input.DecidedAt == "" || input.DecidedAt != claim.DecidedAt {
		fail(w, 409, "claimDecisionConflict")
		return
	}
	alreadyRequested := claim.ReviewRequestedAt != ""
	if !alreadyRequested {
		claim.ReviewRequestedAt = time.Now().UTC().Format(time.RFC3339Nano)
		_, err = tx.Exec("INSERT INTO claim_review_requests(claim_id,requested_at) VALUES(?,?)", id, claim.ReviewRequestedAt)
		if err == nil {
			_, err = tx.Exec("INSERT INTO claim_history(claim_id,actor_id,action,timestamp,values_json) VALUES(?,?,'reviewRequested',?,?)",
				id, actor.ID, claim.ReviewRequestedAt, encode(map[string]string{"itemId": claim.ItemID, "decidedAt": claim.DecidedAt}))
		}
	}
	finishClaimReview(w, tx, actor.ID, key, fingerprint, claim, alreadyRequested, err)
}

type reopenClaimInput struct {
	reviewRequestInput
	Reason string `json:"reason"`
}

func (a *API) reopenClaim(w http.ResponseWriter, r *http.Request, id string, actor *Account) {
	if actor.Role != "admin" {
		fail(w, 403, "forbidden")
		return
	}
	var input reopenClaimInput
	if !readBody(w, r, &input) {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.Reason == "" || utf8.RuneCountInString(input.Reason) > 1000 {
		fail(w, 400, "reopeningReasonRequired")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !submissionPattern.MatchString(key) {
		fail(w, 400, "submissionKeyRequired")
		return
	}
	fingerprint := hash(encode([]any{"claim-reopen", id, input}))
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	if replayClaim(w, tx, actor.ID, key, fingerprint) {
		return
	}

	var itemID, claimStatus, itemStatus, decidedAt, requestedAt string
	var hasApproval, otherActiveClaim bool
	err = tx.QueryRow(`SELECT c.item_id,c.status,i.status,coalesce(d.decided_at,''),coalesce(q.requested_at,''),
		EXISTS(SELECT 1 FROM claims WHERE item_id=c.item_id AND status='Approved'),
		EXISTS(SELECT 1 FROM claims WHERE item_id=c.item_id AND claimant_id=c.claimant_id AND id!=c.id AND status IN ('Submitted','Approved'))
		FROM claims c JOIN items i ON i.id=c.item_id LEFT JOIN claim_decisions d ON d.claim_id=c.id
		LEFT JOIN claim_review_requests q ON q.claim_id=c.id WHERE c.id=?`, id).
		Scan(&itemID, &claimStatus, &itemStatus, &decidedAt, &requestedAt, &hasApproval, &otherActiveClaim)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "claimNotFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	switch {
	case claimStatus != "Rejected":
		fail(w, 409, "claimNotRejected")
		return
	case input.DecidedAt == "" || input.DecidedAt != decidedAt:
		fail(w, 409, "claimDecisionConflict")
		return
	case itemStatus != "Open":
		fail(w, 409, "itemNotOpen")
		return
	case hasApproval:
		fail(w, 409, "claimAlreadyApproved")
		return
	case otherActiveClaim:
		fail(w, 409, "claimantHasOtherActiveClaim")
		return
	}

	result, err := tx.Exec(`UPDATE claims SET status='Submitted',reason='' WHERE id=? AND status='Rejected'
		AND EXISTS(SELECT 1 FROM items WHERE id=claims.item_id AND status='Open')
		AND NOT EXISTS(SELECT 1 FROM claims other WHERE other.item_id=claims.item_id AND other.status='Approved')
		AND NOT EXISTS(SELECT 1 FROM claims other WHERE other.item_id=claims.item_id AND other.claimant_id=claims.claimant_id
			AND other.id!=claims.id AND other.status='Submitted')`, id)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	rows, err := result.RowsAffected()
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	if rows != 1 {
		fail(w, 409, "claimDecisionConflict")
		return
	}
	// Previous decisions stay in history. Reset the current checks for a fresh review.
	_, err = tx.Exec("DELETE FROM claim_decisions WHERE claim_id=?", id)
	if err == nil {
		_, err = tx.Exec("DELETE FROM claim_review_requests WHERE claim_id=?", id)
	}
	reopenedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO claim_reopenings(claim_id,reopened_at) VALUES(?,?)
			ON CONFLICT(claim_id) DO UPDATE SET reopened_at=excluded.reopened_at`, id, reopenedAt)
	}
	if err == nil {
		event := map[string]string{"before": "Rejected", "after": "Submitted", "itemId": itemID, "reason": input.Reason, "decidedAt": decidedAt}
		if requestedAt != "" {
			event["requestedAt"] = requestedAt
		}
		_, err = tx.Exec("INSERT INTO claim_history(claim_id,actor_id,action,timestamp,values_json) VALUES(?,?,'reopened',?,?)",
			id, actor.ID, reopenedAt, encode(event))
	}
	finishClaimReview(w, tx, actor.ID, key, fingerprint, claimReceipt{ID: id, ItemID: itemID, Status: "Submitted"}, false, err)
}

func finishClaimReview(w http.ResponseWriter, tx *sql.Tx, actorID, key, fingerprint string, claim claimReceipt, replayed bool, err error) {
	if err == nil {
		_, err = tx.Exec("INSERT INTO claim_submissions VALUES(?,?,?,?,?)", actorID, key, fingerprint, claim.ID, encode(claim))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"claim": claim, "replayed": replayed})
}
