package backend

import (
	"net/http"
	"testing"
)

func TestClaimSubmissionEligibilityAndPrivacy(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	student := login(t, api, "student")
	admin := login(t, api, "admin")
	created := call(api, "POST", "/api/items", sample(), staff, "claimable-item-fixture")
	requireCode(t, created, http.StatusCreated)
	item := contents(t, created)["item"].(map[string]any)
	itemID := item["id"].(string)
	if item["canClaim"] != true {
		t.Fatal("Open item should allow a claim")
	}
	input := claimInput{ItemID: itemID, DescriptionAnswer: " ", LossAnswer: "Library yesterday"}
	requireCode(t, call(api, "POST", "/api/claims", input, student, "claim-test-blank-answer"), http.StatusBadRequest)
	input.DescriptionAnswer, input.Contact = "Private distinguishing mark", "fixture@example.test"
	claimed := call(api, "POST", "/api/claims", input, student, "claim-test-same-key")
	requireCode(t, claimed, http.StatusCreated)
	receipt := contents(t, claimed)["claim"].(map[string]any)
	if receipt["status"] != "Submitted" || receipt["itemId"] != itemID {
		t.Fatal("incorrect submission receipt")
	}
	claimID := receipt["id"].(string)
	requireCode(t, call(api, "POST", "/api/claims", input, student, "claim-test-same-key"), http.StatusOK)
	duplicate := call(api, "POST", "/api/claims", input, student, "claim-test-duplicate-key")
	requireCode(t, duplicate, http.StatusConflict)
	if contents(t, duplicate)["claimId"] != claimID {
		t.Fatal("duplicate claim refusal did not offer the existing claim identifier")
	}
	changed := input
	changed.LossAnswer = "Changed"
	requireCode(t, call(api, "POST", "/api/claims", changed, student, "claim-test-same-key"), http.StatusConflict)
	path := "/api/claims/" + claimID
	requireCode(t, call(api, "GET", path, nil, staff, ""), http.StatusNotFound)
	requireCode(t, call(api, "GET", path, nil, "", ""), http.StatusUnauthorized)
	for _, reader := range []string{student, admin} {
		response := call(api, "GET", path, nil, reader, "")
		requireCode(t, response, http.StatusOK)
		detail := contents(t, response)["claim"].(map[string]any)
		if detail["descriptionAnswer"] != input.DescriptionAnswer || detail["contact"] != input.Contact {
			t.Fatal("claim details missing")
		}
	}
	listed := contents(t, call(api, "GET", "/api/claims", nil, staff, ""))["claims"].([]any)
	if len(listed) != 0 {
		t.Fatal("item creator saw another account's claim")
	}
	visible := contents(t, call(api, "GET", "/api/items/"+itemID, nil, student, ""))["item"].(map[string]any)
	if visible["status"] != "Open" || visible["canClaim"] != false {
		t.Fatal("claim changed item status or allowed a duplicate")
	}
	if _, err := db.Exec("UPDATE claims SET status='Approved' WHERE id=?", claimID); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", "/api/claims", input, staff, "claim-test-after-approval"), http.StatusConflict)
	replayed := contents(t, call(api, "POST", "/api/claims", input, student, "claim-test-same-key"))["claim"].(map[string]any)
	if replayed["status"] != "Submitted" {
		t.Fatal("replay mutated a committed receipt")
	}
	if _, err := db.Exec("UPDATE items SET status='Withdrawn' WHERE id=?", itemID); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", "/api/claims", input, admin, "claim-test-after-withdrawal"), http.StatusConflict)
	if count(t, db, "claims") != 1 || count(t, db, "claim_history") != 1 || count(t, db, "claim_submissions") != 1 {
		t.Fatal("refusals or retries changed claim storage")
	}
}

func TestConcurrentClaimAndHistoryFailure(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	student := login(t, api, "student")
	created := call(api, "POST", "/api/items", sample(), staff, "claim-concurrent-item")
	requireCode(t, created, http.StatusCreated)
	input := claimInput{ItemID: contents(t, created)["item"].(map[string]any)["id"].(string), DescriptionAnswer: "Description", LossAnswer: "Library"}
	if _, err := db.Exec(`CREATE TRIGGER claim_fixture_failure BEFORE INSERT ON claim_history BEGIN SELECT RAISE(ABORT,'test'); END;`); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", "/api/claims", input, student, "claim-retry-after-db-error"), http.StatusServiceUnavailable)
	for _, table := range []string{"claims", "claim_history", "claim_submissions"} {
		if count(t, db, table) != 0 {
			t.Fatal("failed history left a partial write in", table)
		}
	}
	if _, err := db.Exec("DROP TRIGGER claim_fixture_failure"); err != nil {
		t.Fatal(err)
	}
	start, statuses := make(chan struct{}), make(chan int, 2)
	for _, key := range []string{"claim-retry-after-db-error", "claim-racing-distinct-key"} {
		go func() { <-start; statuses <- call(api, "POST", "/api/claims", input, student, key).Code }()
	}
	close(start)
	first, second := <-statuses, <-statuses
	if min(first, second) != http.StatusCreated || max(first, second) != http.StatusConflict {
		t.Fatalf("concurrent statuses %d %d", first, second)
	}
	for _, table := range []string{"claims", "claim_history", "claim_submissions"} {
		if count(t, db, table) != 1 {
			t.Fatal("concurrent request caused incorrect cardinality in", table)
		}
	}
}
