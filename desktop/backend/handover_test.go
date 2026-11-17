package backend

import (
	"net/http"
	"reflect"
	"testing"
)

func approvedReleaseClaim(t *testing.T, api *API, path, admin, key string) {
	t.Helper()
	approval := claimDecisionInput{Outcome: "approve", verificationChecks: verificationChecks{DetailsCheck: "Passed", IdentityCheck: "Passed"}}
	requireCode(t, call(api, "POST", path+"/decision", approval, admin, key), http.StatusOK)
}

func handoverFixture() handoverInput {
	return handoverInput{ReleasedAt: "2026-11-11T13:40", Verification: "Compared the item and confirmed the claimant in person.",
		ReceiptReference: "REC-017", SignedAcknowledgment: true}
}

func TestHandoverPermissionAndAtomicReturn(t *testing.T) {
	api, db := fixture(t)
	staff, student := login(t, api, "reception"), login(t, api, "student")
	affairs, admin := login(t, api, "affairs"), login(t, api, "admin")
	intake := call(api, "POST", "/api/items", sample(), staff, "release-item-fixture")
	requireCode(t, intake, http.StatusCreated)
	itemID := contents(t, intake)["item"].(map[string]any)["id"].(string)
	claimID := submitEvidenceFixture(t, api, itemID, student, "release-claim-fixture")
	claimPath := "/api/claims/" + claimID
	path := claimPath + "/handover"
	command := handoverFixture()
	requireCode(t, call(api, "GET", path, nil, staff, ""), http.StatusConflict)
	requireCode(t, call(api, "POST", path, command, staff, "release-before-approval"), http.StatusConflict)
	approvedReleaseClaim(t, api, claimPath, admin, "release-claim-approval")
	for _, unauthorized := range []string{student, admin, affairs} {
		requireCode(t, call(api, "GET", path, nil, unauthorized, ""), http.StatusForbidden)
		requireCode(t, call(api, "POST", path, command, unauthorized, "release-denied-fixture"), http.StatusForbidden)
	}
	requireCode(t, call(api, "GET", "/api/handovers", nil, student, ""), http.StatusForbidden)
	requireCode(t, call(api, "GET", "/api/handovers", nil, admin, ""), http.StatusForbidden)
	if rows := contents(t, call(api, "GET", "/api/handovers", nil, affairs, ""))["items"].([]any); len(rows) != 0 {
		t.Fatal("staff queue included another holding point")
	}
	items := contents(t, call(api, "GET", "/api/handovers", nil, staff, ""))["items"].([]any)
	details := contents(t, call(api, "GET", path, nil, staff, ""))
	item := details["item"].(map[string]any)
	if len(items) != 1 || !reflect.DeepEqual(item, items[0]) || item["claimId"] != claimID ||
		details["claimantName"] != "Campus user" || details["descriptionAnswer"] != nil || item["identifyingDetails"] != nil ||
		details["contact"] != nil || item["photoUrl"] != nil {
		t.Fatal("release details are missing or contain private evidence")
	}
	incomplete := command
	incomplete.ReceiptReference, incomplete.Verification, incomplete.SignedAcknowledgment, incomplete.ReleasedAt = " ", "", false, "2026-02-30T13:40"
	rejected := call(api, "POST", path, incomplete, staff, "release-incomplete-fixture")
	requireCode(t, rejected, http.StatusBadRequest)
	if !reflect.DeepEqual(contents(t, rejected)["fields"], []any{"releasedAt", "verification", "receiptReference", "signedAcknowledgment"}) {
		t.Fatal("missing field names for an incomplete handover")
	}

	const key = "release-complete-fixture"
	for _, failure := range []string{
		`CREATE TRIGGER test_return_failure BEFORE INSERT ON dispositions BEGIN SELECT RAISE(ABORT,'fixture'); END`,
		`CREATE TRIGGER test_return_failure BEFORE INSERT ON history WHEN NEW.action='returned' BEGIN SELECT RAISE(ABORT,'fixture'); END`,
	} {
		if _, err := db.Exec(failure); err != nil {
			t.Fatal(err)
		}
		requireCode(t, call(api, "POST", path, command, staff, key), http.StatusServiceUnavailable)
		if count(t, db, "handovers") != 0 || count(t, db, "dispositions") != 0 || count(t, db, "history") != 1 ||
			count(t, db, "items WHERE status='Open'") != 1 {
			t.Fatal("failed handover left a partial record")
		}
		if _, err := db.Exec("DROP TRIGGER test_return_failure"); err != nil {
			t.Fatal(err)
		}
	}
	done := call(api, "POST", path, command, staff, key)
	requireCode(t, done, http.StatusOK)
	receipt := contents(t, done)["handover"].(map[string]any)
	again := call(api, "POST", path, command, staff, key)
	requireCode(t, again, http.StatusOK)
	replayed := contents(t, again)
	if receipt["releasedAt"] != "2026-11-11T09:40:00Z" || receipt["status"] != "Returned" ||
		replayed["replayed"] != true || !reflect.DeepEqual(receipt, replayed["handover"]) {
		t.Fatal("confirmation or retry lost the recorded handover")
	}
	requireCode(t, call(api, "POST", path, command, staff, "release-second-handover"), http.StatusConflict)
	command.ReceiptReference = "Changed"
	requireCode(t, call(api, "POST", path, command, staff, key), http.StatusConflict)
	requireCode(t, call(api, "GET", path, nil, staff, ""), http.StatusConflict)
	requireCode(t, call(api, "POST", "/api/items/"+itemID+"/withdraw", map[string]string{"reason": "Fixture"}, admin, "withdraw-returned-fixture"), http.StatusConflict)
	requireCode(t, call(api, "POST", "/api/items", sample(), staff, "release-item-fixture"), http.StatusOK)
	requireCode(t, call(api, "GET", "/api/items/"+itemID, nil, student, ""), http.StatusNotFound)
	claim := contents(t, call(api, "GET", claimPath, nil, student, ""))["claim"].(map[string]any)
	if claim["status"] != "Approved" || claim["itemStatus"] != "Returned" || claim["receiptReference"] != nil {
		t.Fatal("claimant status did not reflect collection, or leaked handover details")
	}
	if count(t, db, "handovers h JOIN dispositions d ON d.item_id=h.item_id AND d.handover_claim_id=h.claim_id AND d.account_id=h.releasing_account_id AND d.occurred_at=h.released_at WHERE d.kind='Returned'") != 1 ||
		count(t, db, "history WHERE action='returned'") != 1 ||
		count(t, db, "items WHERE status='Returned'") != 1 {
		t.Fatal("handover, disposition, status and history do not agree")
	}
}

func TestConcurrentHandoversSaveOnce(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	itemID := claimItemFixture(t, api, staff)
	claimID := submitEvidenceFixture(t, api, itemID, login(t, api, "student"), "release-race-claim")
	claimPath := "/api/claims/" + claimID
	approvedReleaseClaim(t, api, claimPath, login(t, api, "admin"), "release-race-approval")
	start, results := make(chan struct{}), make(chan int, 2)
	for _, worker := range []struct{ auth, key string }{
		{staff, "release-race-first"}, {login(t, api, "other"), "release-race-second"},
	} {
		go func() {
			<-start
			results <- call(api, "POST", claimPath+"/handover", handoverFixture(), worker.auth, worker.key).Code
		}()
	}
	close(start)
	first, second := <-results, <-results
	if min(first, second) != http.StatusOK || max(first, second) != http.StatusConflict ||
		count(t, db, "handovers") != 1 || count(t, db, "dispositions") != 1 || count(t, db, "history WHERE action='returned'") != 1 {
		t.Fatalf("expected exactly one completed handover, got %d and %d", first, second)
	}
}
