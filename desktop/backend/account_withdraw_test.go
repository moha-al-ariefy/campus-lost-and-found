package backend

import (
	"strings"
	"testing"
)

func TestSignupCreatesOnlyCampusAccount(t *testing.T) {
	api, db := fixture(t)
	form := map[string]string{"name": "Campus Example", "login": "Example.User", "password": testPassword}
	requireCode(t, call(api, "POST", "/api/signup", form, "", ""), 404)
	api.EnableSignup = true
	form["role"] = "admin"
	requireCode(t, call(api, "POST", "/api/signup", form, "", ""), 400)
	delete(form, "role")
	form["password"] = "short"
	requireCode(t, call(api, "POST", "/api/signup", form, "", ""), 400)
	form["password"] = testPassword
	requireCode(t, call(api, "POST", "/api/signup", form, "", ""), 201)
	form["login"] = "example.user"
	requireCode(t, call(api, "POST", "/api/signup", form, "", ""), 409)
	cookie := login(t, api, "example.user")
	session := contents(t, call(api, "GET", "/api/session", nil, cookie, ""))
	account := session["account"].(map[string]any)
	if account["role"] != "campus" || len(account["holdingPoints"].([]any)) != 0 {
		t.Fatal("self signup granted a privileged role")
	}
	var salt, hash string
	if err := db.QueryRow("SELECT salt,password_hash FROM accounts WHERE login=?", "example.user").Scan(&salt, &hash); err != nil ||
		!strings.HasPrefix(salt, "s2:") || len(hash) != 128 || hash == testPassword {
		t.Fatal("signup did not store the expected salted password hash", err)
	}
	requireCode(t, call(api, "POST", "/api/items", sample(), cookie, "signup-forbidden-001"), 403)
	requireCode(t, call(api, "POST", "/api/logout", nil, cookie, ""), 200)
	requireCode(t, call(api, "GET", "/api/session", nil, cookie, ""), 401)
}

func TestWithdrawItemRetainsRecordPhotoAndHistory(t *testing.T) {
	api, db := fixture(t)
	staff, admin, student := login(t, api, "reception"), login(t, api, "admin"), login(t, api, "student")
	imageBytes := jpegFixture(t, 48)
	created := upload(t, api, staff, "withdraw-create-001", imageBytes)
	requireCode(t, created, 201)
	original := contents(t, created)["item"].(map[string]any)
	path := "/api/items/" + original["id"].(string)
	reason := map[string]string{"reason": "Duplicate record"}
	key := "withdraw-request-001"
	requireCode(t, call(api, "POST", path+"/withdraw", reason, staff, key), 403)
	requireCode(t, call(api, "POST", path+"/withdraw", map[string]string{"reason": "  "}, admin, key), 400)
	requireCode(t, call(api, "DELETE", path, nil, admin, key), 404)
	if _, err := db.Exec(`CREATE TRIGGER test_withdraw_failure BEFORE INSERT ON history
		WHEN NEW.action='withdrawn' BEGIN SELECT RAISE(ABORT, 'fixture failure'); END;`); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", path+"/withdraw", reason, admin, key), 503)
	requireCode(t, call(api, "GET", path, nil, student, ""), 200)
	if _, err := db.Exec("DROP TRIGGER test_withdraw_failure"); err != nil {
		t.Fatal(err)
	}
	result := call(api, "POST", path+"/withdraw", reason, admin, key)
	requireCode(t, result, 200)
	if contents(t, result)["item"].(map[string]any)["status"] != "Withdrawn" {
		t.Fatal("withdrawal returned the wrong status")
	}
	replay := call(api, "POST", path+"/withdraw", reason, admin, key)
	requireCode(t, replay, 200)
	if contents(t, replay)["replayed"] != true {
		t.Fatal("withdraw retry did not replay")
	}
	requireCode(t, call(api, "POST", path+"/withdraw", map[string]string{"reason": "Changed"}, admin, key), 409)
	requireCode(t, call(api, "POST", path+"/withdraw", reason, admin, "withdraw-request-002"), 409)
	requireCode(t, call(api, "GET", path, nil, student, ""), 404)
	requireCode(t, call(api, "GET", path, nil, admin, ""), 200)
	requireCode(t, call(api, "GET", path+"/photo", nil, admin, ""), 200)
	requireCode(t, upload(t, api, staff, "withdraw-create-001", imageBytes), 200)
	list := contents(t, call(api, "GET", "/api/items", nil, admin, ""))
	if list["total"] != float64(0) || count(t, db, "item_photos") != 1 || count(t, db, "items") != 1 {
		t.Fatal("withdrawal left a public item or discarded stored data")
	}
	history := contents(t, call(api, "GET", path+"/history", nil, admin, ""))["entries"].([]any)
	if len(history) != 2 || history[1].(map[string]any)["reason"] != reason["reason"] {
		t.Fatal("withdrawal history missing or duplicated")
	}
}
