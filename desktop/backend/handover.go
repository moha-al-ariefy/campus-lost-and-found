package backend

import (
	"database/sql"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

const campusMinuteLayout = "2006-01-02T15:04"

type releaseItem struct {
	ClaimID      string `json:"claimId"`
	ItemID       string `json:"itemId"`
	Category     string `json:"category"`
	PublicName   string `json:"publicName"`
	Location     string `json:"location"`
	HoldingPoint string `json:"holdingPoint"`
}

const releaseItemColumns = "c.id,i.id,i.category,i.public_name,i.location,i.holding_point"

func (item *releaseItem) columns() []any {
	return []any{&item.ClaimID, &item.ItemID, &item.Category, &item.PublicName, &item.Location, &item.HoldingPoint}
}

func (a *API) listPendingHandovers(w http.ResponseWriter, r *http.Request, actor *Account) {
	if actor.Role != "staff" {
		fail(w, 403, "forbidden")
		return
	}
	page, valid := recordPage(r)
	if !valid {
		fail(w, 400, "invalidFilters")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT `+releaseItemColumns+`
		FROM claims c JOIN items i ON i.id=c.item_id
		WHERE c.status='Approved' AND i.status='Open' AND i.holding_point IN (SELECT value FROM json_each(?))
		AND NOT EXISTS(SELECT 1 FROM handovers h WHERE h.claim_id=c.id)
		ORDER BY c.created_at,c.id LIMIT ? OFFSET ?`, encode(actor.HoldingPoints), recordPageSize+1, (page-1)*recordPageSize)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := make([]releaseItem, 0, recordPageSize+1)
	for rows.Next() {
		var item releaseItem
		if err := rows.Scan(item.columns()...); err != nil {
			fail(w, 503, "unavailable")
			return
		}
		items = append(items, item)
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"items": items[:min(len(items), recordPageSize)], "hasMore": len(items) > recordPageSize})
}

type releaseTarget struct {
	releaseItem
	claimantID  string
	claimStatus string
	itemStatus  string
	handedOver  bool
}

func readReleaseTarget(w http.ResponseWriter, db queryer, id string, actor *Account) (releaseTarget, bool) {
	var target releaseTarget
	err := db.QueryRow(`SELECT `+releaseItemColumns+`,c.claimant_id,c.status,i.status,
		EXISTS(SELECT 1 FROM handovers WHERE claim_id=c.id)
		FROM claims c JOIN items i ON i.id=c.item_id WHERE c.id=?`, id).
		Scan(append(target.columns(), &target.claimantID, &target.claimStatus, &target.itemStatus, &target.handedOver)...)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, 404, "claimNotFound")
	case err != nil:
		fail(w, 503, "unavailable")
	case !slices.Contains(actor.HoldingPoints, target.HoldingPoint):
		fail(w, 403, "handoverWrongPoint")
	default:
		return target, true
	}
	return releaseTarget{}, false
}

func (target releaseTarget) eligible(w http.ResponseWriter) bool {
	switch {
	case target.handedOver:
		fail(w, 409, "handoverAlreadyRecorded")
	case target.claimStatus != "Approved":
		fail(w, 409, "handoverClaimNotApproved")
	case target.itemStatus != "Open":
		fail(w, 409, "itemNotOpen")
	default:
		return true
	}
	return false
}

type handoverInput struct {
	ReleasedAt           string `json:"releasedAt"`
	Verification         string `json:"verification"`
	ReceiptReference     string `json:"receiptReference"`
	SignedAcknowledgment bool   `json:"signedAcknowledgment"`
}

type handoverReceipt struct {
	ClaimID string `json:"claimId"`
	ItemID  string `json:"itemId"`
	Status  string `json:"status"`
	handoverInput
}

func (a *API) handover(w http.ResponseWriter, r *http.Request, id string, actor *Account) {
	if actor.Role != "staff" {
		fail(w, 403, "forbidden")
		return
	}
	if r.Method == "GET" {
		tx, err := a.DB.BeginTx(r.Context(), &sql.TxOptions{ReadOnly: true})
		if err != nil {
			fail(w, 503, "unavailable")
			return
		}
		defer tx.Rollback()
		target, allowed := readReleaseTarget(w, tx, id, actor)
		if !allowed || !target.eligible(w) {
			return
		}
		var claimantName string
		if err := tx.QueryRow("SELECT name FROM accounts WHERE id=?", target.claimantID).Scan(&claimantName); err != nil {
			fail(w, 503, "unavailable")
			return
		}
		reply(w, 200, map[string]any{"item": target.releaseItem, "claimantName": claimantName})
		return
	}
	var input handoverInput
	if !readBody(w, r, &input) {
		return
	}
	input.Verification = strings.TrimSpace(input.Verification)
	input.ReceiptReference = strings.TrimSpace(input.ReceiptReference)
	releasedAt, err := time.ParseInLocation(campusMinuteLayout, input.ReleasedAt, time.FixedZone("Asia/Dubai", 4*60*60))
	var fields []string
	if err != nil || releasedAt.Format(campusMinuteLayout) != input.ReleasedAt || releasedAt.After(time.Now()) {
		fields = append(fields, "releasedAt")
	}
	if input.Verification == "" || utf8.RuneCountInString(input.Verification) > 1000 {
		fields = append(fields, "verification")
	}
	if input.ReceiptReference == "" || utf8.RuneCountInString(input.ReceiptReference) > 200 {
		fields = append(fields, "receiptReference")
	}
	if !input.SignedAcknowledgment {
		fields = append(fields, "signedAcknowledgment")
	}
	if len(fields) != 0 {
		reply(w, 400, map[string]any{"code": "invalidFields", "fields": fields})
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !submissionPattern.MatchString(key) {
		fail(w, 400, "submissionKeyRequired")
		return
	}

	fingerprint := hash(encode([]any{"handover", id, input}))
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	target, allowed := readReleaseTarget(w, tx, id, actor)
	if !allowed {
		return
	}
	var previousHash string
	var receipt handoverReceipt
	err = tx.QueryRow(`SELECT request_hash,claim_id,item_id,released_at,verification,receipt_reference,signed_acknowledgment
		FROM handovers WHERE releasing_account_id=? AND submission_key=?`, actor.ID, key).
		Scan(&previousHash, &receipt.ClaimID, &receipt.ItemID, &receipt.ReleasedAt, &receipt.Verification, &receipt.ReceiptReference, &receipt.SignedAcknowledgment)
	switch {
	case err == nil:
		if previousHash != fingerprint {
			fail(w, 409, "submissionChanged")
			return
		}
		receipt.Status = "Returned"
		reply(w, 200, map[string]any{"handover": receipt, "replayed": true})
		return
	case !errors.Is(err, sql.ErrNoRows):
		fail(w, 503, "unavailable")
		return
	}
	if !target.eligible(w) {
		return
	}

	result, err := tx.Exec(`UPDATE items SET status='Returned' WHERE id=? AND status='Open' AND holding_point=?
		AND EXISTS(SELECT 1 FROM claims WHERE id=? AND item_id=items.id AND status='Approved')
		AND NOT EXISTS(SELECT 1 FROM dispositions WHERE item_id=items.id)`, target.ItemID, target.HoldingPoint, id)
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
		fail(w, 409, "handoverConflict")
		return
	}
	input.ReleasedAt = releasedAt.UTC().Format(time.RFC3339)
	_, err = tx.Exec(`INSERT INTO handovers(claim_id,item_id,claimant_id,releasing_account_id,released_at,verification,
		receipt_reference,signed_acknowledgment,submission_key,request_hash) VALUES(?,?,?,?,?,?,?,?,?,?)`,
		id, target.ItemID, target.claimantID, actor.ID, input.ReleasedAt, input.Verification, input.ReceiptReference, input.SignedAcknowledgment, key, fingerprint)
	if err == nil {
		_, err = tx.Exec(`INSERT INTO dispositions(item_id,kind,occurred_at,account_id,handover_claim_id) VALUES(?,'Returned',?,?,?)`,
			target.ItemID, input.ReleasedAt, actor.ID, id)
	}
	if err == nil {
		event := encode(map[string]string{"before": "Open", "after": "Returned", "claimId": id, "releasedAt": input.ReleasedAt})
		_, err = tx.Exec(`INSERT INTO history(record_id,action,account_id,timestamp,values_json) VALUES(?,'returned',?,?,?)`,
			target.ItemID, actor.ID, time.Now().UTC().Format(time.RFC3339Nano), event)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	receipt = handoverReceipt{ClaimID: id, ItemID: target.ItemID, Status: "Returned", handoverInput: input}
	reply(w, 200, map[string]any{"handover": receipt, "replayed": false})
}
