package backend

import (
	"fmt"
	"net/http"
	"path/filepath"
	"reflect"
	"testing"
)

func createActivityItem(t *testing.T, api *API, auth, key string) string {
	t.Helper()
	result := call(api, "POST", "/api/items", sample(), auth, key)
	requireCode(t, result, http.StatusCreated)
	return contents(t, result)["item"].(map[string]any)["id"].(string)
}

func TestCustodyTransferAndContactHistory(t *testing.T) {
	api, db := fixture(t)
	reception, affairs := login(t, api, "reception"), login(t, api, "affairs")
	admin, student := login(t, api, "admin"), login(t, api, "student")
	id := createActivityItem(t, api, reception, "activity-intake-transfer")
	path := "/api/items/" + id
	claimID := submitEvidenceFixture(t, api, id, student, "activity-transfer-claim")
	claimPath := "/api/claims/" + claimID
	approvedReleaseClaim(t, api, claimPath, admin, "activity-transfer-approval")
	transfer := custodyTransfer{From: "reception", To: "student-affairs"}
	requireCode(t, call(api, "POST", path+"/custody", transfer, affairs, "activity-transfer-denied"), http.StatusForbidden)
	bad := custodyTransfer{From: "reception", To: "unknown"}
	requireCode(t, call(api, "POST", path+"/custody", bad, reception, "activity-transfer-invalid"), http.StatusBadRequest)
	const transferKey = "activity-transfer-success"
	if _, err := db.Exec(`CREATE TRIGGER fixture_failed_transfer BEFORE INSERT ON history WHEN NEW.action='transferred'
		BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", path+"/custody", transfer, reception, transferKey), http.StatusServiceUnavailable)
	if count(t, db, "items WHERE holding_point='reception'") != 1 || count(t, db, "item_activity_submissions") != 0 {
		t.Fatal("failed transfer changed the record or left a receipt")
	}
	if _, err := db.Exec("DROP TRIGGER fixture_failed_transfer"); err != nil {
		t.Fatal(err)
	}
	saved := call(api, "POST", path+"/custody", transfer, reception, transferKey)
	requireCode(t, saved, http.StatusOK)
	replayed := contents(t, call(api, "POST", path+"/custody", transfer, reception, transferKey))
	if replayed["replayed"] != true || !reflect.DeepEqual(replayed["activity"], contents(t, saved)["activity"]) {
		t.Fatal("transfer retry lost its confirmation after the item moved")
	}
	requireCode(t, call(api, "POST", path+"/custody", custodyTransfer{From: "student-affairs", To: "reception"}, reception, transferKey), http.StatusConflict)
	requireCode(t, call(api, "POST", path+"/custody", transfer, admin, "activity-transfer-stale"), http.StatusConflict)
	requireCode(t, call(api, "GET", claimPath+"/handover", nil, reception, ""), http.StatusForbidden)
	requireCode(t, call(api, "GET", claimPath+"/handover", nil, affairs, ""), http.StatusOK)
	publicItem := contents(t, call(api, "GET", path, nil, student, ""))["item"].(map[string]any)
	if publicItem["holdingPoint"] != "student-affairs" || publicItem["status"] != "Open" {
		t.Fatal("transfer did not change the public holding point on the same item")
	}
	contact := ownerContactAttempt{AttemptedAt: "2026-11-11T13:40", Method: "Phone call", Outcome: "No answer."}
	requireCode(t, call(api, "POST", path+"/contact-attempt", contact, affairs, "activity-contact-denied"), http.StatusForbidden)
	requireCode(t, call(api, "POST", path+"/contact-attempt", contact, admin, "activity-contact-saved"), http.StatusOK)
	requireCode(t, call(api, "POST", path+"/contact-attempt", contact, admin, "activity-contact-saved"), http.StatusOK)
	requireCode(t, call(api, "GET", path+"/history", nil, affairs, ""), http.StatusForbidden)
	requireCode(t, call(api, "POST", claimPath+"/handover", handoverFixture(), affairs, "activity-transfer-release"), http.StatusOK)
	history := contents(t, call(api, "GET", path+"/history", nil, admin, ""))["entries"].([]any)
	expected := []string{"created", "submitted", "decided", "transferred", "contactAttempt", "returned"}
	if len(history) != len(expected) {
		t.Fatalf("incomplete item history: %v", history)
	}
	// Consecutive writes can share a clock tick. The chronology test checks the tie-break.
	detailsByAction := make(map[string]map[string]any, len(history))
	for _, event := range history {
		entry := event.(map[string]any)
		detailsByAction[entry["action"].(string)] = entry["details"].(map[string]any)
	}
	for _, action := range expected {
		if _, present := detailsByAction[action]; !present {
			t.Fatalf("missing %s from item history", action)
		}
	}
	recorded, handover := detailsByAction["contactAttempt"], detailsByAction["returned"]
	if recorded["attemptedAt"] != "2026-11-11T09:40:00Z" || recorded["outcome"] != contact.Outcome ||
		handover["receiptReference"] != handoverFixture().ReceiptReference {
		t.Fatal("history lost contact or release details")
	}
	requireCode(t, call(api, "POST", path+"/custody", custodyTransfer{From: "student-affairs", To: "reception"}, admin, "activity-transfer-final"), http.StatusConflict)
}

func TestNonReturnDispositionsAndTransitions(t *testing.T) {
	api, db := fixture(t)
	admin, staff, student := login(t, api, "admin"), login(t, api, "reception"), login(t, api, "student")
	kinds := []string{"Handed to Police", "Donated", "Disposed"}
	disposition := func(kind string) nonReturnDisposition {
		input := nonReturnDisposition{Kind: kind, Date: "2026-11-11", DecisionBasis: "Recorded outcome for the fixture."}
		if kind == "Handed to Police" {
			input.PoliceReference = "POL-017"
		}
		return input
	}
	for index, kind := range append(append([]string{}, kinds...), "Returned") {
		item := createActivityItem(t, api, staff, fmt.Sprintf("activity-final-item-%d", index))
		path := "/api/items/" + item
		claim := submitEvidenceFixture(t, api, item, student, fmt.Sprintf("activity-final-claim-%d", index))
		claimPath := "/api/claims/" + claim
		requireCode(t, call(api, "POST", path+"/disposition", disposition("Returned"), admin, "activity-direct-return"), http.StatusBadRequest)
		if kind == "Returned" {
			approvedReleaseClaim(t, api, claimPath, admin, "activity-final-approve")
			for n, refused := range kinds {
				requireCode(t, call(api, "POST", path+"/disposition", disposition(refused), admin, fmt.Sprintf("activity-approved-close-%d", n)), http.StatusConflict)
			}
			requireCode(t, call(api, "POST", claimPath+"/handover", handoverFixture(), staff, "activity-final-return"), http.StatusOK)
		} else {
			input := disposition(kind)
			requireCode(t, call(api, "POST", path+"/disposition", input, staff, "activity-staff-close"), http.StatusForbidden)
			if kind == "Handed to Police" {
				missing := input
				missing.PoliceReference = ""
				requireCode(t, call(api, "POST", path+"/disposition", missing, admin, "activity-police-reference"), http.StatusBadRequest)
			}
			key := fmt.Sprintf("activity-non-return-save-%d", index)
			requireCode(t, call(api, "POST", path+"/disposition", input, admin, key), http.StatusOK)
			requireCode(t, call(api, "POST", path+"/disposition", input, admin, key), http.StatusOK)
			command := claimDecisionInput{Outcome: "approve", verificationChecks: verificationChecks{"Passed", "Passed"}}
			requireCode(t, call(api, "POST", claimPath+"/decision", command, admin, "activity-approve-closed"), http.StatusConflict)
			requireCode(t, call(api, "GET", claimPath, nil, student, ""), http.StatusOK)
		}
		for n, refused := range kinds {
			requireCode(t, call(api, "POST", path+"/disposition", disposition(refused), admin, fmt.Sprintf("activity-already-final-%d", n)), http.StatusConflict)
		}
		requireCode(t, call(api, "GET", path, nil, student, ""), http.StatusNotFound)
		final := contents(t, call(api, "GET", path, nil, admin, ""))["item"].(map[string]any)
		if final["status"] != kind {
			t.Fatalf("expected archived item status %s, got %v", kind, final["status"])
		}
	}
	if count(t, db, "dispositions") != 4 || count(t, db, "history WHERE action IN ('returned','disposed')") != 4 {
		t.Fatal("final outcomes and history must be unique")
	}
}

func TestDispositionAndApprovalRaceWithFailedSave(t *testing.T) {
	api, db := fixture(t)
	admin, staff, student := login(t, api, "admin"), login(t, api, "reception"), login(t, api, "student")
	item := createActivityItem(t, api, staff, "activity-intake-close-race")
	closePath := "/api/items/" + item + "/disposition"
	claim := submitEvidenceFixture(t, api, item, student, "activity-claim-close-race")
	command := nonReturnDisposition{Kind: "Donated", Date: "2026-11-11", DecisionBasis: "Fixture"}
	if _, err := db.Exec(`CREATE TRIGGER fixture_failed_disposition BEFORE INSERT ON history WHEN NEW.action='disposed'
		BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	const key = "activity-close-race-request"
	requireCode(t, call(api, "POST", closePath, command, admin, key), http.StatusServiceUnavailable)
	if count(t, db, "dispositions") != 0 || count(t, db, "items WHERE status='Open'") != 1 || count(t, db, "item_activity_submissions") != 0 {
		t.Fatal("failed disposition left a partial change")
	}
	if _, err := db.Exec("DROP TRIGGER fixture_failed_disposition"); err != nil {
		t.Fatal(err)
	}
	start, results := make(chan struct{}), make(chan int, 2)
	go func() {
		<-start
		results <- call(api, "POST", closePath, command, admin, key).Code
	}()
	go func() {
		<-start
		approval := claimDecisionInput{Outcome: "approve", verificationChecks: verificationChecks{"Passed", "Passed"}}
		results <- call(api, "POST", "/api/claims/"+claim+"/decision", approval, admin, "activity-approval-race").Code
	}()
	close(start)
	first, second := <-results, <-results
	if min(first, second) != http.StatusOK || max(first, second) != http.StatusConflict ||
		count(t, db, "dispositions")+count(t, db, "claims WHERE status='Approved'") != 1 {
		t.Fatalf("expected one successful and one denied command, got %d and %d", first, second)
	}
}

func TestHoldingPeriodConfigurationSurvivesRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "campus.db")
	db, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	if err := SeedAccounts(db, testPassword); err != nil {
		t.Fatal(err)
	}
	api := NewAPI(db)
	staff, admin, student := login(t, api, "reception"), login(t, api, "admin"), login(t, api, "student")
	itemPath := "/api/items/" + createActivityItem(t, api, staff, "activity-holding-period")
	endDate := func(auth string) any {
		return contents(t, call(api, "GET", itemPath, nil, auth, ""))["item"].(map[string]any)["holdingPeriodEnd"]
	}
	if endDate(admin) != nil {
		t.Fatal("new installation showed an assumed holding period")
	}
	for _, setting := range []struct{ days, end string }{{"60", "2027-01-10"}, {"30", "2026-12-11"}} {
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db, err = OpenStore(path)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("CAMPUS_HOLDING_PERIOD_DAYS", setting.days)
		if err := LoadHoldingPeriod(db); err != nil {
			t.Fatal(err)
		}
		api = NewAPI(db)
		if endDate(admin) != setting.end || endDate(student) != nil {
			t.Fatalf("unexpected holding period for %s days", setting.days)
		}
	}
}

func TestItemHistoryKeepsFractionalAndEqualTimestampOrder(t *testing.T) {
	api, db := fixture(t)
	staff, student, admin := login(t, api, "reception"), login(t, api, "student"), login(t, api, "admin")
	item := createActivityItem(t, api, staff, "activity-chronology-item")
	claim := submitEvidenceFixture(t, api, item, student, "activity-chronology-claim")
	// Fractional seconds in RFC3339Nano have variable width. The shorter fraction must sort first.
	for _, timestamp := range []string{"2025-12-01T04:00:00.1201Z", "2025-12-01T04:00:00.12Z", "2025-12-01T04:00:00.12Z"} {
		if _, err := db.Exec("INSERT INTO history(record_id,action,account_id,timestamp,values_json) VALUES(?,'contactAttempt','admin',?,'{}')", item, timestamp); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.Exec("INSERT INTO claim_history(claim_id,actor_id,action,timestamp,values_json) VALUES(?,'admin','reviewRequested','2025-12-01T04:00:00.12Z','{}')", claim); err != nil {
		t.Fatal(err)
	}
	path := "/api/items/" + item + "/history"
	result := call(api, "GET", path, nil, admin, "")
	requireCode(t, result, http.StatusOK)
	events := contents(t, result)["entries"].([]any)
	if len(events) != 6 {
		t.Fatalf("expected six related events but got %d", len(events))
	}
	want := []struct{ source, timestamp string }{
		{"claim", "2025-12-01T04:00:00.12Z"}, {"item", "2025-12-01T04:00:00.12Z"},
		{"item", "2025-12-01T04:00:00.12Z"}, {"item", "2025-12-01T04:00:00.1201Z"},
	}
	for index, expected := range want {
		entry := events[index].(map[string]any)
		if entry["source"] != expected.source || entry["timestamp"] != expected.timestamp {
			t.Fatalf("event %d is out of order: %v", index, entry)
		}
	}
	if events[1].(map[string]any)["sequence"].(float64) >= events[2].(map[string]any)["sequence"].(float64) {
		t.Fatal("identical timestamps were not ordered by sequence")
	}
	second := call(api, "GET", path, nil, admin, "")
	if !reflect.DeepEqual(contents(t, result)["entries"], contents(t, second)["entries"]) {
		t.Fatal("reading the same timeline returned a different order")
	}
}
