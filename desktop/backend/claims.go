package backend

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

type claimInput struct {
	ItemID            string `json:"itemId"`
	DescriptionAnswer string `json:"descriptionAnswer"`
	LossAnswer        string `json:"lossAnswer"`
	Contact           string `json:"contact"`
}

type claimReceipt struct {
	ID                string `json:"id"`
	ItemID            string `json:"itemId"`
	Status            string `json:"status"`
	Reason            string `json:"reason,omitempty"`
	DecidedAt         string `json:"decidedAt,omitempty"`
	ReviewRequestedAt string `json:"reviewRequestedAt,omitempty"`
}

type claimSummary struct {
	claimReceipt
	Category     string `json:"category"`
	PublicName   string `json:"publicName"`
	HoldingPoint string `json:"holdingPoint"`
	CreatedAt    string `json:"createdAt"`
}

func (a *API) submitClaim(w http.ResponseWriter, r *http.Request, actor *Account) {
	var input claimInput
	if !readBody(w, r, &input) {
		return
	}
	input.ItemID = strings.TrimSpace(input.ItemID)
	input.DescriptionAnswer = strings.TrimSpace(input.DescriptionAnswer)
	input.LossAnswer = strings.TrimSpace(input.LossAnswer)
	input.Contact = strings.TrimSpace(input.Contact)
	var fields []string
	if input.ItemID == "" || len(input.ItemID) > 100 {
		fields = append(fields, "itemId")
	}
	if input.DescriptionAnswer == "" || utf8.RuneCountInString(input.DescriptionAnswer) > 2000 {
		fields = append(fields, "descriptionAnswer")
	}
	if input.LossAnswer == "" || utf8.RuneCountInString(input.LossAnswer) > 2000 {
		fields = append(fields, "lossAnswer")
	}
	if utf8.RuneCountInString(input.Contact) > 200 || strings.IndexFunc(input.Contact, unicode.IsControl) >= 0 {
		fields = append(fields, "contact")
	}
	if len(fields) > 0 {
		reply(w, 400, map[string]any{"code": "invalidFields", "fields": fields})
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !submissionPattern.MatchString(key) {
		fail(w, 400, "submissionKeyRequired")
		return
	}
	fingerprint := hash("claim-create:" + encode(input))
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	if replayClaim(w, tx, actor.ID, key, fingerprint) {
		return
	}
	var status, existingClaimID string
	var approved bool
	err = tx.QueryRow(`SELECT status,
		EXISTS(SELECT 1 FROM claims WHERE item_id=items.id AND status='Approved'),
		coalesce((SELECT id FROM claims WHERE item_id=items.id AND claimant_id=? AND status IN ('Submitted','Approved') LIMIT 1),'')
		FROM items WHERE id=?`, actor.ID, input.ItemID).Scan(&status, &approved, &existingClaimID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "notFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	switch {
	case status != "Open":
		fail(w, 409, "itemNotOpen")
		return
	case existingClaimID != "":
		reply(w, 409, map[string]any{"code": "claimAlreadyActive", "claimId": existingClaimID})
		return
	case approved:
		fail(w, 409, "claimAlreadyApproved")
		return
	}
	claim := claimReceipt{ID: "CLC-" + rand.Text(), ItemID: input.ItemID, Status: "Submitted"}
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.Exec(`INSERT INTO claims(id,item_id,claimant_id,description_answer,loss_answer,contact,status,created_at)
		VALUES (?,?,?,?,?,?,?,?)`, claim.ID, claim.ItemID, actor.ID, input.DescriptionAnswer, input.LossAnswer, input.Contact, claim.Status, timestamp)
	if err == nil {
		_, err = tx.Exec("INSERT INTO claim_history(claim_id,actor_id,action,timestamp,values_json) VALUES(?,?,'submitted',?,?)",
			claim.ID, actor.ID, timestamp, encode(claim))
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO claim_submissions VALUES(?,?,?,?,?)", actor.ID, key, fingerprint, claim.ID, encode(claim))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 201, map[string]any{"claim": claim, "replayed": false})
}

func replayClaim(w http.ResponseWriter, tx *sql.Tx, accountID, key, fingerprint string) bool {
	var previousHash, response string
	err := tx.QueryRow("SELECT request_hash,response_json FROM claim_submissions WHERE account_id=? AND submission_key=?", accountID, key).Scan(&previousHash, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return false
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return true
	}
	if previousHash != fingerprint {
		fail(w, 409, "submissionChanged")
		return true
	}
	var receipt claimReceipt
	if err := json.Unmarshal([]byte(response), &receipt); err != nil {
		fail(w, 503, "unavailable")
		return true
	}
	reply(w, 200, map[string]any{"claim": receipt, "replayed": true})
	return true
}

const claimSummaryColumns = `c.id,c.item_id,c.status,i.category,i.public_name,i.holding_point,c.created_at`

func (summary *claimSummary) columns() []any {
	return []any{&summary.ID, &summary.ItemID, &summary.Status, &summary.Category, &summary.PublicName, &summary.HoldingPoint, &summary.CreatedAt}
}

func (a *API) listClaims(w http.ResponseWriter, r *http.Request, actor *Account) {
	page, valid := recordPage(r)
	if !valid {
		fail(w, 400, "invalidFilters")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT `+claimSummaryColumns+`,coalesce(q.requested_at,'')
		FROM claims c JOIN items i ON i.id=c.item_id LEFT JOIN claim_review_requests q ON q.claim_id=c.id
		WHERE c.claimant_id=? OR ?='admin' ORDER BY (?='admin' AND q.claim_id IS NOT NULL) DESC,c.created_at DESC,c.id LIMIT ? OFFSET ?`,
		actor.ID, actor.Role, actor.Role, recordPageSize+1, (page-1)*recordPageSize)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	claims := make([]claimSummary, 0, recordPageSize+1)
	for rows.Next() {
		var claim claimSummary
		if err := rows.Scan(append(claim.columns(), &claim.ReviewRequestedAt)...); err != nil {
			fail(w, 503, "unavailable")
			return
		}
		claims = append(claims, claim)
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"claims": claims[:min(len(claims), recordPageSize)], "hasMore": len(claims) > recordPageSize})
}

type claimDetail struct {
	claimSummary
	DescriptionAnswer string `json:"descriptionAnswer"`
	LossAnswer        string `json:"lossAnswer"`
	Contact           string `json:"contact"`
	ReopenedAt        string `json:"reopenedAt,omitempty"`
	OwnClaim          bool   `json:"ownClaim"`
	ItemStatus        string `json:"itemStatus"`
}

func loadClaimDetail(db queryer, id string, actor *Account) (claimDetail, error) {
	var claim claimDetail
	err := db.QueryRow(`SELECT `+claimSummaryColumns+`,c.description_answer,c.loss_answer,c.contact,c.reason,
		coalesce(d.decided_at,''),coalesce(q.requested_at,''),coalesce(o.reopened_at,''),c.claimant_id=?,i.status
		FROM claims c JOIN items i ON i.id=c.item_id LEFT JOIN claim_decisions d ON d.claim_id=c.id
		LEFT JOIN claim_review_requests q ON q.claim_id=c.id LEFT JOIN claim_reopenings o ON o.claim_id=c.id
		WHERE c.id=? AND (c.claimant_id=? OR ?='admin')`, actor.ID, id, actor.ID, actor.Role).
		Scan(append(claim.columns(), &claim.DescriptionAnswer, &claim.LossAnswer, &claim.Contact, &claim.Reason,
			&claim.DecidedAt, &claim.ReviewRequestedAt, &claim.ReopenedAt, &claim.OwnClaim, &claim.ItemStatus)...)
	return claim, err
}

func (a *API) readClaim(w http.ResponseWriter, r *http.Request, id string, actor *Account) {
	claim, err := loadClaimDetail(a.DB, id, actor)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "claimNotFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"claim": claim})
}
