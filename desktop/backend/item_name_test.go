package backend

import (
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func TestItemNameSearchUsesPublicFields(t *testing.T) {
	api, _ := fixture(t)
	staff := login(t, api, "reception")
	campus := login(t, api, "student")
	data := sample()
	data.PublicName = "  Charging Cable  "
	key := "item-name-submission"
	requireCode(t, call(api, "POST", "/api/items", data, campus, key), http.StatusForbidden)
	created := call(api, "POST", "/api/items", data, staff, key)
	requireCode(t, created, http.StatusCreated)
	id := contents(t, created)["item"].(map[string]any)["id"].(string)
	data.PublicName = "Charging Cable"
	requireCode(t, call(api, "POST", "/api/items", data, staff, key), http.StatusOK)
	data.PublicName = "Adapter"
	requireCode(t, call(api, "POST", "/api/items", data, staff, key), http.StatusConflict)

	matched := call(api, "GET", "/api/items?publicName=CHARGING&category=electronics", nil, campus, "")
	requireCode(t, matched, http.StatusOK)
	results := contents(t, matched)
	items := results["items"].([]any)
	if len(items) != 1 || items[0].(map[string]any)["publicName"] != "Charging Cable" || items[0].(map[string]any)["id"] != id {
		t.Fatal("public name search did not return the recorded item")
	}
	details := contents(t, call(api, "GET", "/api/items/"+id, nil, campus, ""))["item"].(map[string]any)
	if details["publicName"] != "Charging Cable" || details["description"] != nil || details["identifyingDetails"] != nil {
		t.Fatal("public name or private-field separation was lost on the detail screen")
	}
	for _, query := range []string{"publicName=Private", "publicName=charging&category=keys", "publicName=%25"} {
		filtered := call(api, "GET", "/api/items?"+query, nil, campus, "")
		requireCode(t, filtered, http.StatusOK)
		if contents(t, filtered)["total"] != float64(0) {
			t.Fatal("name filter was not literal, public-only and combined with other filters")
		}
	}
	for _, invalid := range []string{strings.Repeat("x", 81), "Cable\nName"} {
		data.PublicName = invalid
		requireCode(t, call(api, "POST", "/api/items", data, staff, "invalid-name-submission"), http.StatusBadRequest)
		requireCode(t, call(api, "GET", "/api/items?publicName="+url.QueryEscape(invalid), nil, campus, ""), http.StatusBadRequest)
	}
}

func TestItemNameUpgradeKeepsOldRecordsAndRetryKeys(t *testing.T) {
	path := filepath.Join(t.TempDir(), "campus.db")
	db, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SeedAccounts(db, testPassword); err != nil {
		db.Close()
		t.Fatal(err)
	}
	api := NewAPI(db)
	cookie := login(t, api, "reception")
	const key = "legacy-item-name-fixture"
	created := call(api, "POST", "/api/items", sample(), cookie, key)
	requireCode(t, created, http.StatusCreated)
	if _, err := db.Exec("ALTER TABLE items DROP COLUMN public_name"); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	replayed := call(NewAPI(db), "POST", "/api/items", sample(), cookie, key)
	requireCode(t, replayed, http.StatusOK)
	item := contents(t, replayed)["item"].(map[string]any)
	if item["publicName"] != "" || item["id"] != contents(t, created)["item"].(map[string]any)["id"] || count(t, db, "items") != 1 {
		t.Fatal("upgrading the previous schema lost data or changed an existing submission")
	}
}
