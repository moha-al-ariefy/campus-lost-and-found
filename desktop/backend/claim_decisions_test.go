package backend

import (
	"net/http"
	"testing"
)

func submitEvidenceFixture(t *testing.T, api *API, itemID, cookie, key string) string {
	t.Helper()
	result := call(api, "POST", "/api/claims", claimInput{
		ItemID: itemID, DescriptionAnswer: "Private fixture answer", LossAnswer: "Fixture area yesterday", Contact: "fixture@example.test",
	}, cookie, key)
	requireCode(t, result, http.StatusCreated)
	return contents(t, result)["claim"].(map[string]any)["id"].(string)
}

func claimItemFixture(t *testing.T, api *API, staff string) string {
	t.Helper()
	photo := upload(t, api, staff, "claim-review-item-fixture", jpegFixture(t, 91))
	requireCode(t, photo, http.StatusCreated)
	return contents(t, photo)["item"].(map[string]any)["id"].(string)
}

func TestClaimEvidenceAuthorizationAndApprovalGuards(t *testing.T) {
	api, db := fixture(t)
	student := login(t, api, "student")
	staff := login(t, api, "reception")
	admin := login(t, api, "admin")
	itemID := claimItemFixture(t, api, staff)
	claimID := submitEvidenceFixture(t, api, itemID, student, "review-campus-submission")
	path := "/api/claims/" + claimID
	approval := claimDecisionInput{Outcome: "approve", verificationChecks: verificationChecks{DetailsCheck: "Passed", IdentityCheck: "Passed"}}
	for _, actor := range []string{student, staff} {
		requireCode(t, call(api, "GET", path+"/evidence", nil, actor, ""), http.StatusForbidden)
		requireCode(t, call(api, "POST", path+"/decision", approval, actor, "review-forbidden-decision"), http.StatusForbidden)
	}
	evidence := call(api, "GET", path+"/evidence", nil, admin, "")
	requireCode(t, evidence, http.StatusOK)
	view := contents(t, evidence)
	item := view["item"].(map[string]any)
	if item["description"] != sample().Description || item["photoUrl"] == nil || view["claim"].(map[string]any)["descriptionAnswer"] != "Private fixture answer" {
		t.Fatal("administrator evidence was incomplete")
	}
	for _, checks := range []verificationChecks{{}, {"Passed", "Failed"}, {"Failed", "Passed"}, {"Passed", ""}, {"", "Passed"}} {
		refused := call(api, "POST", path+"/decision", claimDecisionInput{Outcome: "approve", verificationChecks: checks}, admin, "review-approval-guards")
		requireCode(t, refused, http.StatusBadRequest)
		if contents(t, refused)["code"] != "claimChecksRequired" {
			t.Fatal("approval did not enforce both checks")
		}
	}
	const approvedKey = "review-approved-command"
	result := call(api, "POST", path+"/decision", approval, admin, approvedKey)
	requireCode(t, result, http.StatusOK)
	approved := contents(t, result)["claim"].(map[string]any)
	if approved["status"] != "Approved" || approved["decidedAt"] == nil {
		t.Fatal("decision receipt missing status or time")
	}
	replayed := contents(t, call(api, "POST", path+"/decision", approval, admin, approvedKey))
	if replayed["replayed"] != true || replayed["claim"].(map[string]any)["decidedAt"] != approved["decidedAt"] {
		t.Fatal("retry did not return the original approval")
	}
	requireCode(t, call(api, "POST", path+"/decision", claimDecisionInput{Outcome: "reject", Reason: "Different command"}, admin, approvedKey), http.StatusConflict)
	requireCode(t, call(api, "POST", "/api/items/"+itemID+"/withdraw", map[string]string{"reason": "Fixture"}, admin, "withdraw-approved-item"), http.StatusConflict)
	itemView := contents(t, call(api, "GET", "/api/items/"+itemID, nil, student, ""))["item"].(map[string]any)
	if itemView["status"] != "Open" || itemView["canClaim"] != false || count(t, db, "claim_history") != 2 || count(t, db, "claim_decisions") != 1 {
		t.Fatal("approval altered item status or wrote duplicate history")
	}
}

func TestClaimDecisionRollbackAndRejection(t *testing.T) {
	api, db := fixture(t)
	student := login(t, api, "student")
	staff := login(t, api, "reception")
	admin := login(t, api, "admin")
	claimID := submitEvidenceFixture(t, api, claimItemFixture(t, api, staff), student, "review-reject-submission")
	path := "/api/claims/" + claimID
	requireCode(t, call(api, "POST", path+"/decision", claimDecisionInput{Outcome: "reject", Reason: " "}, admin, "review-reject-blank"), http.StatusBadRequest)
	rejection := claimDecisionInput{Outcome: "reject", Reason: "Ownership details don’t match."}
	const key = "review-reject-command"
	for _, trigger := range []string{
		`CREATE TRIGGER test_decision_failure BEFORE INSERT ON claim_history WHEN NEW.action='decided' BEGIN SELECT RAISE(ABORT,'fixture'); END`,
		`CREATE TRIGGER test_decision_failure AFTER INSERT ON claim_decisions BEGIN UPDATE claims SET reason='changed' WHERE id=NEW.claim_id; END`,
	} {
		if _, err := db.Exec(trigger); err != nil {
			t.Fatal(err)
		}
		requireCode(t, call(api, "POST", path+"/decision", rejection, admin, key), http.StatusServiceUnavailable)
		view := contents(t, call(api, "GET", path, nil, student, ""))["claim"].(map[string]any)
		if view["status"] != "Submitted" || count(t, db, "claim_decisions") != 0 || count(t, db, "claim_submissions") != 1 || count(t, db, "claim_history") != 1 {
			t.Fatal("failed decision left state, history or replay data")
		}
		if _, err := db.Exec("DROP TRIGGER test_decision_failure"); err != nil {
			t.Fatal(err)
		}
	}
	requireCode(t, call(api, "POST", path+"/decision", rejection, admin, key), http.StatusOK)
	claim := contents(t, call(api, "GET", path, nil, student, ""))["claim"].(map[string]any)
	if claim["status"] != "Rejected" || claim["reason"] != rejection.Reason {
		t.Fatal("claimant cannot see the recorded rejection")
	}
}

func TestConcurrentClaimApprovalsHaveOneWinner(t *testing.T) {
	api, db := fixture(t)
	student := login(t, api, "student")
	staff := login(t, api, "reception")
	admin := login(t, api, "admin")
	if err := CreateAccount(db, "second-admin", "Second administrator", "second-admin", "admin", nil, testPassword); err != nil {
		t.Fatal(err)
	}
	secondAdmin := login(t, api, "second-admin")
	item := claimItemFixture(t, api, staff)
	first := submitEvidenceFixture(t, api, item, student, "claim-first-competing-fixture")
	second := submitEvidenceFixture(t, api, item, staff, "claim-second-competing-fixture")
	command := claimDecisionInput{Outcome: "approve", verificationChecks: verificationChecks{DetailsCheck: "Passed", IdentityCheck: "Passed"}}
	start, results := make(chan struct{}), make(chan int, 2)
	for _, action := range []struct{ id, auth, key string }{
		{first, admin, "approve-first-concurrent"}, {second, secondAdmin, "approve-second-concurrent"},
	} {
		go func() {
			<-start
			results <- call(api, "POST", "/api/claims/"+action.id+"/decision", command, action.auth, action.key).Code
		}()
	}
	close(start)
	left, right := <-results, <-results
	if min(left, right) != http.StatusOK || max(left, right) != http.StatusConflict {
		t.Fatalf("competing decisions returned %d and %d", left, right)
	}
	if count(t, db, "claims WHERE status='Approved'") != 1 || count(t, db, "claim_history") != 3 || count(t, db, "claim_submissions") != 3 {
		t.Fatal("the losing approval altered state or history")
	}
}
