package backend

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

type custodyTransfer struct {
	From string `json:"fromHoldingPoint"`
	To   string `json:"holdingPoint"`
}

type itemCorrection struct {
	Intake
	Revision int64 `json:"revision"`
}

func intakePrivateData(data Intake) string {
	return encode(map[string]string{"description": data.Description, "identifyingDetails": data.IdentifyingDetails,
		"finder": data.Finder, "receivedAt": data.ReceivedAt, "color": data.Color, "brand": data.Brand, "building": data.Building})
}

type nonReturnDisposition struct {
	Kind            string `json:"kind"`
	Date            string `json:"date"`
	DecisionBasis   string `json:"decisionBasis"`
	PoliceReference string `json:"policeReference"`
}

type ownerContactAttempt struct {
	AttemptedAt string `json:"attemptedAt"`
	Method      string `json:"method"`
	Outcome     string `json:"outcome"`
}

type itemActivityReceipt struct {
	ItemID       string `json:"itemId"`
	HoldingPoint string `json:"holdingPoint"`
	Status       string `json:"status"`
	Sequence     int64  `json:"sequence"`
}

func (a *API) recordItemActivity(w http.ResponseWriter, r *http.Request, id, operation string, actor *Account) {
	if operation != "custody" && operation != "disposition" && operation != "contact-attempt" && operation != "correction" {
		fail(w, 404, "notFound")
		return
	}
	if actor.Role != "admin" && (actor.Role != "staff" || (operation != "custody" && operation != "correction")) {
		fail(w, 403, "forbidden")
		return
	}
	var transfer custodyTransfer
	var disposition nonReturnDisposition
	var contact ownerContactAttempt
	var correction itemCorrection
	var action string
	var input any
	var fields []string
	switch operation {
	case "correction":
		if !readBody(w, r, &correction) {
			return
		}
		fields = validate(&correction.Intake)
		action, input = "corrected", correction
	case "custody":
		if !readBody(w, r, &transfer) {
			return
		}
		if !slices.Contains(HoldingPoints, transfer.From) || !slices.Contains(HoldingPoints, transfer.To) || transfer.From == transfer.To {
			fields = append(fields, "holdingPoint")
		}
		action, input = "transferred", transfer
	case "disposition":
		if !readBody(w, r, &disposition) {
			return
		}
		disposition.DecisionBasis = strings.TrimSpace(disposition.DecisionBasis)
		disposition.PoliceReference = strings.TrimSpace(disposition.PoliceReference)
		if !slices.Contains([]string{"Handed to Police", "Donated", "Disposed"}, disposition.Kind) {
			fields = append(fields, "kind")
		}
		if !validDate(disposition.Date) {
			fields = append(fields, "date")
		}
		if disposition.DecisionBasis == "" || utf8.RuneCountInString(disposition.DecisionBasis) > 1000 {
			fields = append(fields, "decisionBasis")
		}
		if disposition.Kind == "Handed to Police" {
			if disposition.PoliceReference == "" || utf8.RuneCountInString(disposition.PoliceReference) > 200 {
				fields = append(fields, "policeReference")
			}
		} else if disposition.PoliceReference != "" {
			fields = append(fields, "policeReference")
		}
		action, input = "disposed", disposition
	case "contact-attempt":
		if !readBody(w, r, &contact) {
			return
		}
		contact.Method = strings.TrimSpace(contact.Method)
		contact.Outcome = strings.TrimSpace(contact.Outcome)
		occurred, err := time.ParseInLocation(campusMinuteLayout, contact.AttemptedAt, time.FixedZone("Asia/Dubai", 4*60*60))
		if err != nil || occurred.Format(campusMinuteLayout) != contact.AttemptedAt || occurred.After(time.Now()) {
			fields = append(fields, "attemptedAt")
		}
		if contact.Method == "" || utf8.RuneCountInString(contact.Method) > 100 {
			fields = append(fields, "method")
		}
		if contact.Outcome == "" || utf8.RuneCountInString(contact.Outcome) > 1000 {
			fields = append(fields, "outcome")
		}
		contact.AttemptedAt = occurred.UTC().Format(time.RFC3339)
		action, input = "contactAttempt", contact
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
	fingerprint := hash(encode([]any{operation, id, input}))
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	if replayItemActivity(w, tx, actor.ID, key, fingerprint) {
		return
	}
	receipt := itemActivityReceipt{ItemID: id}
	var approvedClaim bool
	var creatorID string
	err = tx.QueryRow(`SELECT holding_point,status,creator_id,EXISTS(SELECT 1 FROM claims WHERE item_id=items.id AND status='Approved')
		FROM items WHERE id=?`, id).Scan(&receipt.HoldingPoint, &receipt.Status, &creatorID, &approvedClaim)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, 404, "notFound")
		return
	case err != nil:
		fail(w, 503, "unavailable")
		return
	case actor.Role == "staff" && !slices.Contains(actor.HoldingPoints, receipt.HoldingPoint):
		fail(w, 403, "custodyWrongPoint")
		return
	case operation == "correction" && actor.Role == "staff" && creatorID != actor.ID:
		fail(w, 403, "forbidden")
		return
	case operation != "contact-attempt" && receipt.Status != "Open":
		fail(w, 409, "itemNotOpen")
		return
	case operation == "disposition" && approvedClaim:
		fail(w, 409, "dispositionHasApprovedClaim")
		return
	case operation == "custody" && receipt.HoldingPoint != transfer.From:
		fail(w, 409, "custodyChanged")
		return
	}

	var change sql.Result
	switch operation {
	case "correction":
		var previous Intake
		var privateJSON string
		var revision int64
		err = tx.QueryRow(`SELECT category,public_name,location,date_found,holding_point,private_data,
			coalesce((SELECT max(sequence) FROM history WHERE record_id=items.id AND action='corrected'),0)
			FROM items WHERE id=?`, id).
			Scan(&previous.Category, &previous.PublicName, &previous.Location, &previous.DateFound, &previous.HoldingPoint, &privateJSON, &revision)
		if err != nil || json.Unmarshal([]byte(privateJSON), &previous) != nil {
			fail(w, 503, "unavailable")
			return
		}
		if correction.Revision != revision || correction.HoldingPoint != previous.HoldingPoint {
			fail(w, 409, "itemActivityConflict")
			return
		}
		change, err = tx.Exec(`UPDATE items SET category=?,public_name=?,location=?,date_found=?,private_data=?
			WHERE id=? AND status='Open' AND holding_point=?`, correction.Category, correction.PublicName,
			correction.Location, correction.DateFound, intakePrivateData(correction.Intake), id, previous.HoldingPoint)
		input = map[string]Intake{"before": previous, "after": correction.Intake}
	case "custody":
		change, err = tx.Exec("UPDATE items SET holding_point=? WHERE id=? AND status='Open' AND holding_point=?", transfer.To, id, transfer.From)
		receipt.HoldingPoint = transfer.To
	case "disposition":
		change, err = tx.Exec(`UPDATE items SET status=? WHERE id=? AND status='Open'
			AND NOT EXISTS(SELECT 1 FROM claims WHERE item_id=items.id AND status='Approved')
			AND NOT EXISTS(SELECT 1 FROM dispositions WHERE item_id=items.id)`, disposition.Kind, id)
		receipt.Status = disposition.Kind
	}
	if err == nil && change != nil {
		var changed int64
		changed, err = change.RowsAffected()
		if err == nil && changed != 1 {
			fail(w, 409, "itemActivityConflict")
			return
		}
	}
	if err == nil && operation == "disposition" {
		_, err = tx.Exec(`INSERT INTO dispositions(item_id,kind,occurred_at,account_id,decision_basis,police_reference) VALUES(?,?,?,?,?,?)`,
			id, disposition.Kind, disposition.Date, actor.ID, disposition.DecisionBasis, disposition.PoliceReference)
	}
	if err == nil {
		var event sql.Result
		event, err = tx.Exec("INSERT INTO history(record_id,action,account_id,timestamp,values_json) VALUES(?,?,?,?,?)",
			id, action, actor.ID, time.Now().UTC().Format(time.RFC3339Nano), encode(input))
		if err == nil {
			receipt.Sequence, err = event.LastInsertId()
		}
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO item_activity_submissions(account_id,submission_key,request_hash,response_json) VALUES(?,?,?,?)",
			actor.ID, key, fingerprint, encode(receipt))
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"activity": receipt, "replayed": false})
}

func replayItemActivity(w http.ResponseWriter, tx *sql.Tx, actorID, key, fingerprint string) bool {
	var previousHash, response string
	err := tx.QueryRow("SELECT request_hash,response_json FROM item_activity_submissions WHERE account_id=? AND submission_key=?", actorID, key).Scan(&previousHash, &response)
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
	var receipt itemActivityReceipt
	if err := json.Unmarshal([]byte(response), &receipt); err != nil {
		fail(w, 503, "unavailable")
		return true
	}
	reply(w, 200, map[string]any{"activity": receipt, "replayed": true})
	return true
}
