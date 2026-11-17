package backend

import (
	"net/http"
	"strings"
	"testing"
)

func rejectReviewFixture(t *testing.T, api *API, path, admin, key, reopenedAt string) string {
	t.Helper()
	result := call(api, "POST", path+"/decision", claimDecisionInput{
		Outcome: "reject", Reason: "The original details need clarification.", ReopenedAt: reopenedAt,
	}, admin, key)
	requireCode(t, result, http.StatusOK)
	return contents(t, result)["claim"].(map[string]any)["decidedAt"].(string)
}

func TestClaimReviewRequestAndReopening(t *testing.T) {
	api, db := fixture(t)
	student := login(t, api, "student")
	staff := login(t, api, "reception")
	admin := login(t, api, "admin")
	itemID := claimItemFixture(t, api, staff)
	id := submitEvidenceFixture(t, api, itemID, student, "onsite-claim-submission")
	path := "/api/claims/" + id
	requestPath, reopenPath := path+"/review-request", path+"/reopen"
	requireCode(t, call(api, "POST", requestPath, reviewRequestInput{}, student, "onsite-early-request"), http.StatusConflict)
	request := reviewRequestInput{DecidedAt: rejectReviewFixture(t, api, path, admin, "onsite-rejection", "")}
	reopen := reopenClaimInput{reviewRequestInput: request, Reason: "Review the additional details on campus."}
	requireCode(t, call(api, "POST", requestPath, request, staff, "onsite-unowned-request"), http.StatusNotFound)
	requireCode(t, call(api, "POST", reopenPath, reopen, student, "onsite-unauthorized-reopen"), http.StatusForbidden)
	requireCode(t, call(api, "POST", reopenPath, reopenClaimInput{reviewRequestInput: request}, admin, "onsite-blank-reason"), http.StatusBadRequest)

	const requestKey = "onsite-request-review"
	if _, err := db.Exec(`CREATE TRIGGER test_review_failure BEFORE INSERT ON claim_history
		WHEN NEW.action='reviewRequested' BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", requestPath, request, student, requestKey), http.StatusServiceUnavailable)
	if count(t, db, "claim_review_requests") != 0 || count(t, db, "claim_submissions") != 2 {
		t.Fatal("failed review request left partial data")
	}
	if _, err := db.Exec("DROP TRIGGER test_review_failure"); err != nil {
		t.Fatal(err)
	}
	saved := call(api, "POST", requestPath, request, student, requestKey)
	requireCode(t, saved, http.StatusOK)
	requestedAt := contents(t, saved)["claim"].(map[string]any)["reviewRequestedAt"]
	if requestedAt == nil {
		t.Fatal("missing request date")
	}
	for _, key := range []string{requestKey, "onsite-distinct-request-key"} {
		again := contents(t, call(api, "POST", requestPath, request, student, key))
		if again["replayed"] != true || again["claim"].(map[string]any)["reviewRequestedAt"] != requestedAt {
			t.Fatal("repeat review request changed the original request")
		}
	}
	list := contents(t, call(api, "GET", "/api/claims", nil, admin, ""))["claims"].([]any)
	detail := contents(t, call(api, "GET", path+"/evidence", nil, admin, ""))["claim"].(map[string]any)
	if list[0].(map[string]any)["reviewRequestedAt"] != requestedAt || detail["reviewRequestedAt"] != requestedAt ||
		detail["status"] != "Rejected" || count(t, db, "claim_review_requests") != 1 || count(t, db, "claim_history") != 3 {
		t.Fatal("administrator could not see the outstanding request, or it changed the decision")
	}

	const reopenKey = "onsite-reopen-claim"
	if _, err := db.Exec(`CREATE TRIGGER test_reopen_failure BEFORE INSERT ON claim_history
		WHEN NEW.action='reopened' BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", reopenPath, reopen, admin, reopenKey), http.StatusServiceUnavailable)
	if count(t, db, "claim_decisions") != 1 || count(t, db, "claim_review_requests") != 1 || count(t, db, "claim_reopenings") != 0 ||
		count(t, db, "claims WHERE status='Rejected'") != 1 || count(t, db, "claim_submissions WHERE submission_key='onsite-reopen-claim'") != 0 {
		t.Fatal("failed reopening changed the rejected claim or its outstanding request")
	}
	if _, err := db.Exec("DROP TRIGGER test_reopen_failure"); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", reopenPath, reopen, admin, reopenKey), http.StatusOK)
	if contents(t, call(api, "POST", reopenPath, reopen, admin, reopenKey))["replayed"] != true {
		t.Fatal("reopening was not repeatable")
	}
	reopen.Reason = "Different explanation."
	requireCode(t, call(api, "POST", reopenPath, reopen, admin, reopenKey), http.StatusConflict)
	view := contents(t, call(api, "GET", path+"/evidence", nil, admin, ""))
	detail = view["claim"].(map[string]any)
	if detail["status"] != "Submitted" || detail["reason"] != nil || detail["decidedAt"] != nil || detail["reviewRequestedAt"] != nil ||
		view["checks"].(map[string]any)["identityCheck"] != checkNotRecorded ||
		view["checks"].(map[string]any)["detailsCheck"] != checkNotRecorded || count(t, db, "claim_history") != 4 ||
		count(t, db, "items WHERE status='Open'") != 1 {
		t.Fatal("reopening did not restart verification or changed the item")
	}
	var history string
	if err := db.QueryRow(`SELECT group_concat(values_json,' ') FROM claim_history WHERE claim_id=?`, id).Scan(&history); err != nil ||
		!strings.Contains(history, "The original details need clarification.") || !strings.Contains(history, "Review the additional details on campus.") {
		t.Fatalf("missing decision or reopening history: %v", err)
	}
	approval := claimDecisionInput{Outcome: "approve", verificationChecks: verificationChecks{DetailsCheck: "Passed", IdentityCheck: "Passed"}}
	requireCode(t, call(api, "POST", path+"/decision", approval, admin, "onsite-stale-approval"), http.StatusConflict)
	approval.ReopenedAt = detail["reopenedAt"].(string)
	approval.IdentityCheck = checkNotRecorded
	requireCode(t, call(api, "POST", path+"/decision", approval, admin, "onsite-renewed-approval"), http.StatusBadRequest)
	approval.IdentityCheck = "Passed"
	requireCode(t, call(api, "POST", path+"/decision", approval, admin, "onsite-renewed-approval"), http.StatusOK)
}

func TestClaimReopeningEligibility(t *testing.T) {
	api, db := fixture(t)
	student := login(t, api, "student")
	staff := login(t, api, "reception")
	admin := login(t, api, "admin")
	itemID := claimItemFixture(t, api, staff)
	path := "/api/claims/" + submitEvidenceFixture(t, api, itemID, student, "onsite-eligibility-first-claim")
	request := reviewRequestInput{DecidedAt: rejectReviewFixture(t, api, path, admin, "onsite-eligibility-reject", "")}
	reopen := reopenClaimInput{reviewRequestInput: request, Reason: "Review the claim in person."}
	second := "/api/claims/" + submitEvidenceFixture(t, api, itemID, student, "onsite-eligibility-second-claim")
	// The stale rejected claim cannot displace the claimant's newer active claim.
	reopenPath := path + "/reopen"
	const reopenKey = "onsite-eligibility-reopen"
	denied := call(api, "POST", reopenPath, reopen, admin, reopenKey)
	requireCode(t, denied, http.StatusConflict)
	if contents(t, denied)["code"] != "claimantHasOtherActiveClaim" {
		t.Fatal("duplicate active claim was not reported")
	}
	rejectReviewFixture(t, api, second, admin, "onsite-eligibility-second-reject", "")
	if _, err := db.Exec("UPDATE items SET status='Withdrawn' WHERE id=?", itemID); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", reopenPath, reopen, admin, reopenKey), http.StatusConflict)
	if _, err := db.Exec("UPDATE items SET status='Open' WHERE id=?", itemID); err != nil {
		t.Fatal(err)
	}
	other := "/api/claims/" + submitEvidenceFixture(t, api, itemID, staff, "onsite-eligibility-competing-claim")
	approval := claimDecisionInput{Outcome: "approve", verificationChecks: verificationChecks{"Passed", "Passed"}}
	requireCode(t, call(api, "POST", other+"/decision", approval, admin, "onsite-competing-approval"), http.StatusOK)
	denied = call(api, "POST", reopenPath, reopen, admin, reopenKey)
	requireCode(t, denied, http.StatusConflict)
	if contents(t, denied)["code"] != "claimAlreadyApproved" || count(t, db, "claim_reopenings") != 0 {
		t.Fatal("reopening bypassed the competing approval")
	}
}
