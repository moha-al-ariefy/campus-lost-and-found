package backend

import (
	"crypto/rand"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"modernc.org/sqlite"
)

func init() {
	// SQLite's built-in lower() only folds ASCII.
	if err := sqlite.RegisterDeterministicScalarFunction("claf_match_attribute", 2, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		found, _ := args[0].(string)
		lost, _ := args[1].(string)
		found, lost = strings.TrimSpace(found), strings.TrimSpace(lost)
		return found != "" && strings.EqualFold(found, lost), nil
	}); err != nil {
		panic(err)
	}
}

type LostReportInput struct {
	Category    string `json:"category"`
	Description string `json:"description"`
	DateLost    string `json:"dateLost"`
	Color       string `json:"color"`
	Brand       string `json:"brand"`
	Building    string `json:"building"`
	Contact     string `json:"contact"`
	Assisted    bool   `json:"assisted"`
}

type LostReport struct {
	LostReportInput
	ID         string `json:"id"`
	ReporterID string `json:"reporterId"`
	CreatedAt  string `json:"createdAt"`
	PhotoURL   string `json:"photoUrl,omitempty"`
}

func validateLostReport(input *LostReportInput) []string {
	fields := []string{}
	if !slices.Contains(Categories, input.Category) {
		fields = append(fields, "category")
	}
	if !validDate(input.DateLost) {
		fields = append(fields, "dateLost")
	}
	for _, field := range []struct {
		name  string
		text  *string
		limit int
	}{
		{"description", &input.Description, 2000}, {"color", &input.Color, 500},
		{"brand", &input.Brand, 500}, {"building", &input.Building, 500}, {"contact", &input.Contact, 200},
	} {
		*field.text = strings.TrimSpace(*field.text)
		if utf8.RuneCountInString(*field.text) > field.limit || (field.name == "description" && *field.text == "") ||
			(field.name == "contact" && strings.IndexFunc(*field.text, unicode.IsControl) >= 0) {
			fields = append(fields, field.name)
		}
	}
	return fields
}

func (a *API) createLostReport(w http.ResponseWriter, r *http.Request, actor *Account) {
	key := r.Header.Get("Idempotency-Key")
	if !submissionPattern.MatchString(key) {
		fail(w, 400, "submissionKeyRequired")
		return
	}
	release, ok := a.takePhotoUpload(w, r)
	if !ok {
		return
	}
	defer release()
	var input LostReportInput
	photoBytes, ok := readPhotoSubmission(w, r, &input)
	if !ok {
		return
	}
	if input.Assisted && actor.Role == "campus" {
		fail(w, 403, "forbidden")
		return
	}
	if fields := validateLostReport(&input); len(fields) > 0 {
		reply(w, 400, map[string]any{"code": "invalidFields", "fields": fields})
		return
	}
	fingerprint := intakeFingerprint(input, photoBytes)
	if replayLostReport(w, a.DB, actor, key, fingerprint) {
		return
	}
	var photo itemPhoto
	if photoBytes != nil {
		if !a.waitForPhotoDecoder(w, r) {
			return
		}
		defer func() { <-a.photoDecoder }()
		if replayLostReport(w, a.DB, actor, key, fingerprint) {
			return
		}
		var err error
		photo, err = reducePhoto(photoBytes)
		if errors.Is(err, errPhotoDimensions) {
			fail(w, 400, "photoDimensions")
			return
		}
		if err != nil {
			fail(w, 400, "invalidPhoto")
			return
		}
	}
	tx, err := a.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	if replayLostReport(w, tx, actor, key, fingerprint) {
		return
	}
	report := LostReport{LostReportInput: input, ID: "CLR-" + rand.Text(), ReporterID: actor.ID, CreatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	_, err = tx.Exec(`INSERT INTO lost_reports(id,reporter_id,category,date_lost,description,color,brand,building,contact,assisted,created_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`, report.ID, actor.ID, input.Category, input.DateLost, input.Description,
		input.Color, input.Brand, input.Building, input.Contact, input.Assisted, report.CreatedAt)
	if err == nil && photoBytes != nil {
		_, err = tx.Exec("INSERT INTO lost_report_photos VALUES(?,?,?)", report.ID, photo.mime, photo.content)
		report.PhotoURL = "/api/reports/" + report.ID + "/photo"
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO lost_report_history VALUES(?,?,?,'created',?)", report.ID, actor.ID, report.CreatedAt, encode(report))
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO lost_report_submissions VALUES(?,?,?,?)", actor.ID, key, fingerprint, report.ID)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 201, map[string]any{"report": report, "replayed": false})
}

func replayLostReport(w http.ResponseWriter, db queryer, actor *Account, key, fingerprint string) bool {
	var id, previousHash string
	err := db.QueryRow("SELECT report_id,request_hash FROM lost_report_submissions WHERE account_id=? AND submission_key=?", actor.ID, key).Scan(&id, &previousHash)
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
	report, err := loadLostReport(db, id, actor)
	if err != nil {
		fail(w, 503, "unavailable")
		return true
	}
	reply(w, 200, map[string]any{"report": report, "replayed": true})
	return true
}

func loadLostReport(db queryer, id string, actor *Account) (LostReport, error) {
	var report LostReport
	var hasPhoto bool
	err := db.QueryRow(`SELECT id,reporter_id,category,description,date_lost,color,brand,building,contact,assisted,created_at,
		EXISTS(SELECT 1 FROM lost_report_photos WHERE report_id=lost_reports.id)
		FROM lost_reports WHERE id=? AND (reporter_id=? OR ?='admin')`, id, actor.ID, actor.Role).
		Scan(&report.ID, &report.ReporterID, &report.Category, &report.Description, &report.DateLost, &report.Color,
			&report.Brand, &report.Building, &report.Contact, &report.Assisted, &report.CreatedAt, &hasPhoto)
	if hasPhoto {
		report.PhotoURL = "/api/reports/" + report.ID + "/photo"
	}
	return report, err
}

const recordPageSize = 20

func recordPage(r *http.Request) (int, bool) {
	if !r.URL.Query().Has("page") {
		return 1, true
	}
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	return page, err == nil && page >= 1 && page <= 10000
}

func (a *API) listLostReports(w http.ResponseWriter, r *http.Request, actor *Account) {
	page, valid := recordPage(r)
	owner := r.URL.Query().Get("account")
	if !valid || len(owner) > 100 {
		fail(w, 400, "invalidFilters")
		return
	}
	if owner != "" && actor.Role != "admin" {
		fail(w, 403, "forbidden")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT id,reporter_id,category,date_lost,assisted FROM lost_reports
		WHERE (reporter_id=? OR ?='admin') AND (?='' OR reporter_id=?) ORDER BY created_at DESC,id
		LIMIT ? OFFSET ?`, actor.ID, actor.Role, owner, owner, recordPageSize+1, (page-1)*recordPageSize)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	reports := []map[string]any{}
	for rows.Next() {
		var id, reporter, category, date string
		var assisted bool
		if err := rows.Scan(&id, &reporter, &category, &date, &assisted); err != nil {
			fail(w, 503, "unavailable")
			return
		}
		reports = append(reports, map[string]any{"id": id, "reporterId": reporter, "category": category, "dateLost": date, "assisted": assisted})
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"reports": reports[:min(len(reports), recordPageSize)], "hasMore": len(reports) > recordPageSize})
}

func (a *API) readLostReport(w http.ResponseWriter, r *http.Request, actor *Account) {
	id, action, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/reports/"), "/")
	if action == "history" {
		a.readLostReportHistory(w, r, id, actor)
		return
	}
	report, err := loadLostReport(a.DB, id, actor)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "notFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	switch action {
	case "":
		reply(w, 200, map[string]any{"report": report})
	case "photo":
		var photo itemPhoto
		err := a.DB.QueryRow("SELECT mime,content FROM lost_report_photos WHERE report_id=?", report.ID).Scan(&photo.mime, &photo.content)
		if errors.Is(err, sql.ErrNoRows) {
			fail(w, 404, "notFound")
			return
		}
		if err != nil {
			fail(w, 503, "unavailable")
			return
		}
		w.Header().Set("Content-Type", photo.mime)
		w.Header().Set("Content-Length", strconv.Itoa(len(photo.content)))
		_, _ = w.Write(photo.content)
	case "matches":
		a.suggestLostReportMatches(w, r, report)
	default:
		fail(w, 404, "notFound")
	}
}

func (a *API) readLostReportHistory(w http.ResponseWriter, r *http.Request, id string, actor *Account) {
	if actor.Role != "admin" {
		fail(w, 403, "forbidden")
		return
	}
	var action, timestamp, name, values sql.NullString
	var sequence sql.NullInt64
	err := a.DB.QueryRowContext(r.Context(), `SELECT h.rowid,h.action,h.timestamp,a.name,h.values_json
		FROM lost_reports r LEFT JOIN lost_report_history h ON h.report_id=r.id
		LEFT JOIN accounts a ON a.id=h.actor_id WHERE r.id=?`, id).Scan(&sequence, &action, &timestamp, &name, &values)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "notFound")
		return
	}
	if err != nil || !sequence.Valid || !action.Valid || !timestamp.Valid || !name.Valid || !json.Valid([]byte(values.String)) {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"entries": []map[string]any{{
		"sequence": sequence.Int64, "action": action.String, "timestamp": timestamp.String, "actor": name.String,
		"recordId": id, "details": json.RawMessage(values.String),
	}}})
}

func (a *API) suggestLostReportMatches(w http.ResponseWriter, r *http.Request, report LostReport) {
	page, valid := recordPage(r)
	if !valid {
		fail(w, 400, "invalidFilters")
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `SELECT id,category,public_name,location,date_found,holding_point,
		 (claf_match_attribute(json_extract(private_data,'$.color'),?)
		 + claf_match_attribute(json_extract(private_data,'$.brand'),?)
		 + claf_match_attribute(json_extract(private_data,'$.building'),?)) AS score
		 FROM items WHERE category=? AND status='Open' ORDER BY score DESC,id LIMIT ? OFFSET ?`,
		report.Color, report.Brand, report.Building,
		report.Category, recordPageSize+1, (page-1)*recordPageSize)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	matches := []map[string]any{}
	for rows.Next() {
		var id, category, name, location, date, point string
		var score int
		if err := rows.Scan(&id, &category, &name, &location, &date, &point, &score); err != nil {
			fail(w, 503, "unavailable")
			return
		}
		matches = append(matches, map[string]any{"id": id, "category": category, "publicName": name, "location": location,
			"dateFound": date, "holdingPoint": point, "status": "Open", "score": score})
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"items": matches[:min(len(matches), recordPageSize)], "hasMore": len(matches) > recordPageSize})
}
