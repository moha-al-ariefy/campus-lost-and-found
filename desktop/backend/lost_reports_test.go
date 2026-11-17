package backend

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"
)

func TestLostReportOwnerAndStaffAssistance(t *testing.T) {
	api, db := fixture(t)
	student := login(t, api, "student")
	staff := login(t, api, "reception")
	admin := login(t, api, "admin")
	for _, invalid := range []LostReportInput{
		{Description: "Cable", DateLost: "2026-11-10"}, {Category: "electronics", DateLost: "2026-11-10"},
		{Category: "electronics", Description: "Cable"},
	} {
		requireCode(t, call(api, "POST", "/api/reports", invalid, student, "report-required-fields"), http.StatusBadRequest)
	}
	input := LostReportInput{Category: "electronics", Description: "Cable with a private mark", DateLost: "2026-11-10", Contact: "test@example.test", Assisted: true}
	const key = "report-owner-retry-key"
	requireCode(t, call(api, "POST", "/api/reports", input, student, key), http.StatusForbidden)
	created := call(api, "POST", "/api/reports", input, staff, key)
	requireCode(t, created, http.StatusCreated)
	detail := contents(t, created)["report"].(map[string]any)
	if detail["assisted"] != true || detail["reporterId"] != "reception" || detail["contact"] != input.Contact {
		t.Fatalf("visitor report did not keep staff ownership and provided contact: %v", detail)
	}
	path := "/api/reports/" + detail["id"].(string)
	requireCode(t, call(api, "POST", "/api/reports", input, staff, key), http.StatusOK)
	input.Description = "Changed description"
	requireCode(t, call(api, "POST", "/api/reports", input, staff, key), http.StatusConflict)
	for _, suffix := range []string{"", "/photo", "/matches"} {
		requireCode(t, call(api, "GET", path+suffix, nil, student, ""), http.StatusNotFound)
	}
	for _, viewer := range []string{admin, staff} {
		requireCode(t, call(api, "GET", path, nil, viewer, ""), http.StatusOK)
	}
	if len(contents(t, call(api, "GET", "/api/reports", nil, student, ""))["reports"].([]any)) != 0 ||
		len(contents(t, call(api, "GET", "/api/reports?account=reception", nil, admin, ""))["reports"].([]any)) != 1 ||
		count(t, db, "lost_report_history") != 1 || count(t, db, "lost_report_submissions") != 1 {
		t.Fatal("report listing, history or retry cardinality is incorrect")
	}
}

func TestLostReportPhotoAndHistoryRollbackTogether(t *testing.T) {
	api, db := fixture(t)
	student := login(t, api, "student")
	photo := jpegFixture(t, 123)
	uploadReport := func() *httptest.ResponseRecorder {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		if err := writer.WriteField("item", encode(LostReportInput{Category: "keys", Description: "Private fixture", DateLost: "2026-11-10"})); err != nil {
			t.Fatal(err)
		}
		part, err := writer.CreateFormFile("photo", "fixture.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(photo); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest("POST", "http://localhost/api/reports", &body)
		request.Header.Set("Content-Type", writer.FormDataContentType())
		request.Header.Set("Cookie", student)
		request.Header.Set("X-Campus-Request", "1")
		request.Header.Set("Idempotency-Key", "report-atomic-photo-key")
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		return response
	}
	if _, err := db.Exec(`CREATE TRIGGER fixture_report_history BEFORE INSERT ON lost_report_history BEGIN SELECT RAISE(ABORT,'fixture'); END;`); err != nil {
		t.Fatal(err)
	}
	requireCode(t, uploadReport(), http.StatusServiceUnavailable)
	for _, name := range []string{"lost_reports", "lost_report_photos", "lost_report_history", "lost_report_submissions"} {
		if count(t, db, name) != 0 {
			t.Fatal("partial report saved in", name)
		}
	}
	if _, err := db.Exec("DROP TRIGGER fixture_report_history"); err != nil {
		t.Fatal(err)
	}
	created := uploadReport()
	requireCode(t, created, http.StatusCreated)
	photoURL := contents(t, created)["report"].(map[string]any)["photoUrl"].(string)
	requireCode(t, call(api, "GET", photoURL, nil, student, ""), http.StatusOK)
	requireCode(t, call(api, "GET", photoURL, nil, login(t, api, "reception"), ""), http.StatusNotFound)
	requireCode(t, uploadReport(), http.StatusOK)
}

func TestLostReportMatchesOrderAndPublicProjection(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	student := login(t, api, "student")
	report := call(api, "POST", "/api/reports", LostReportInput{Category: "electronics", Description: "Fixture", DateLost: "2026-11-10", Color: " BLACK ", Brand: "ÉXAMPLE", Building: "Main"}, student, "report-matches-order")
	requireCode(t, report, http.StatusCreated)
	id := contents(t, report)["report"].(map[string]any)["id"].(string)
	matching := sample()
	matching.Brand = "éxample"
	full := contents(t, call(api, "POST", "/api/items", matching, staff, "matches-all-attributes"))["item"].(map[string]any)["id"].(string)
	matching.Brand, matching.Building = "", "Other"
	var ties []string
	for _, key := range []string{"matches-one-attribute-a", "matches-one-attribute-b"} {
		ties = append(ties, contents(t, call(api, "POST", "/api/items", matching, staff, key))["item"].(map[string]any)["id"].(string))
	}
	sort.Strings(ties)
	excluded := contents(t, call(api, "POST", "/api/items", matching, staff, "matches-withdrawn-item"))["item"].(map[string]any)["id"].(string)
	if _, err := db.Exec("UPDATE items SET status='Withdrawn' WHERE id=?", excluded); err != nil {
		t.Fatal(err)
	}
	path := "/api/reports/" + id + "/matches"
	results := call(api, "GET", path, nil, student, "")
	requireCode(t, results, http.StatusOK)
	items := contents(t, results)["items"].([]any)
	want := []string{full, ties[0], ties[1]}
	if len(items) != len(want) {
		t.Fatalf("unexpected matching result count: %d", len(items))
	}
	for i, value := range items {
		item := value.(map[string]any)
		if item["id"] != want[i] || item["description"] != nil || item["photoUrl"] != nil || item["color"] != nil {
			t.Fatal("incorrect suggestion order or exposed private fields")
		}
	}
	if items[0].(map[string]any)["score"] != float64(3) {
		t.Fatal("Unicode case or surrounding spaces affected the match score")
	}
	if count(t, db, "items") != 4 || count(t, db, "lost_report_history") != 1 {
		t.Fatal("matching wrote business records")
	}
}
