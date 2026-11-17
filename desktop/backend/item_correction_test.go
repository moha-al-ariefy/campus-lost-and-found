package backend

import (
	"net/http"
	"testing"
)

func TestCorrectOpenItem(t *testing.T) {
	api, db := fixture(t)
	creator, other, admin := login(t, api, "reception"), login(t, api, "other"), login(t, api, "admin")
	itemID := claimItemFixture(t, api, creator)
	path := "/api/items/" + itemID
	original := contents(t, call(api, "GET", path, nil, creator, ""))["item"].(map[string]any)
	update := itemCorrection{Intake: sample()}
	update.Description = "Corrected description"
	update.DateFound = "2026-11-09"
	const key = "intake-correction-key"
	requireCode(t, call(api, "POST", path+"/correction", update, other, key), http.StatusForbidden)
	invalid := update
	invalid.Description = ""
	requireCode(t, call(api, "POST", path+"/correction", invalid, creator, key), http.StatusBadRequest)
	if _, err := db.Exec(`CREATE TRIGGER fixture_correction_failure BEFORE INSERT ON history WHEN NEW.action='corrected'
		BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", path+"/correction", update, creator, key), http.StatusServiceUnavailable)
	unchanged := contents(t, call(api, "GET", path, nil, creator, ""))["item"].(map[string]any)
	if unchanged["description"] != original["description"] || count(t, db, "item_activity_submissions") != 0 {
		t.Fatal("failed correction left a partial edit")
	}
	if _, err := db.Exec("DROP TRIGGER fixture_correction_failure"); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", path+"/correction", update, creator, key), http.StatusOK)
	retry := call(api, "POST", path+"/correction", update, creator, key)
	requireCode(t, retry, http.StatusOK)
	if contents(t, retry)["replayed"] != true || count(t, db, "history WHERE action='corrected'") != 1 {
		t.Fatal("retry repeated the correction")
	}
	requireCode(t, call(api, "POST", path+"/correction", update, admin, "intake-stale-correction"), http.StatusConflict)
	current := contents(t, call(api, "GET", path, nil, admin, ""))["item"].(map[string]any)
	if current["id"] != itemID || current["creatorId"] != original["creatorId"] || current["photoUrl"] != original["photoUrl"] ||
		current["description"] != update.Description || current["dateFound"] != update.DateFound {
		t.Fatal("correction changed intake identity or lost details")
	}
	events := contents(t, call(api, "GET", path+"/history", nil, admin, ""))["entries"].([]any)
	event := events[1].(map[string]any)["details"].(map[string]any)
	if event["before"].(map[string]any)["description"] != original["description"] ||
		event["after"].(map[string]any)["description"] != update.Description {
		t.Fatal("history lost the old or corrected details")
	}
	update.Revision = int64(current["correctionRevision"].(float64))
	transfer := custodyTransfer{From: "reception", To: "student-affairs"}
	requireCode(t, call(api, "POST", path+"/custody", transfer, admin, "intake-correction-transfer"), http.StatusOK)
	requireCode(t, call(api, "POST", path+"/correction", update, creator, "intake-moved-correction"), http.StatusForbidden)
	update.HoldingPoint = transfer.To
	requireCode(t, call(api, "POST", path+"/correction", update, admin, "intake-admin-correction"), http.StatusOK)
	requireCode(t, call(api, "POST", path+"/withdraw", map[string]string{"reason": "fixture"}, admin, "intake-correction-withdraw"), http.StatusOK)
	requireCode(t, call(api, "POST", path+"/correction", update, admin, "intake-final-correction"), http.StatusConflict)
}
