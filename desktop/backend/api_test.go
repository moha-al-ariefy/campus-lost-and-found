package backend

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testPassword = "local-test-secret-only"

func fixture(t *testing.T) (*API, *sql.DB) {
	t.Helper()
	db, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err = SeedAccounts(db, testPassword); err != nil {
		t.Fatal(err)
	}
	if err = CreateAccount(db, "other", "Other staff", "other", "staff", []string{"reception"}, testPassword); err != nil {
		t.Fatal(err)
	}
	return NewAPI(db), db
}
func call(api *API, method, path string, body any, cookie, key string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, "http://localhost"+path, strings.NewReader(encode(body)))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Campus-Request", "1")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Idempotency-Key", key)
	response := httptest.NewRecorder()
	api.ServeHTTP(response, request)
	return response
}
func login(t *testing.T, api *API, name string) string {
	t.Helper()
	response := call(api, "POST", "/api/login", map[string]any{"login": name, "password": testPassword}, "", "")
	if response.Code != 200 {
		t.Fatalf("login %s: %d %s", name, response.Code, response.Body)
	}
	return response.Result().Cookies()[0].String()
}
func contents(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func sample() Intake {
	return Intake{Category: "electronics", Description: "Private laptop description", Location: "Library", DateFound: "2026-11-10", ReceivedAt: "2026-11-11T09:00", HoldingPoint: "reception", IdentifyingDetails: "Private serial ABC-123", Color: "Black", Brand: "Example", Building: "Main"}
}
func requireCode(t *testing.T, response *httptest.ResponseRecorder, code int) {
	t.Helper()
	if response.Code != code {
		t.Fatalf("expected %d; got %d: %s", code, response.Code, response.Body)
	}
}
func count(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var value int
	if err := db.QueryRow("SELECT count(*) FROM " + table).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestAuthenticationAndStoredRoles(t *testing.T) {
	api, db := fixture(t)
	requireCode(t, call(api, "GET", "/api/items", nil, "", ""), 401)
	bad := call(api, "POST", "/api/login", map[string]string{"login": "student", "password": "bad"}, "", "")
	unknown := call(api, "POST", "/api/login", map[string]string{"login": "missing", "password": testPassword}, "", "")
	requireCode(t, bad, 401)
	requireCode(t, unknown, 401)
	if bad.Body.String() != unknown.Body.String() {
		t.Fatal("login reveals unknown account")
	}
	forged := call(api, "POST", "/api/login", map[string]string{"login": "student", "password": testPassword, "role": "admin"}, "", "")
	requireCode(t, forged, 200)
	if contents(t, forged)["account"].(map[string]any)["role"] != "campus" {
		t.Fatal("client role trusted")
	}
	cookie := forged.Result().Cookies()[0].String()
	requireCode(t, call(api, "POST", "/api/items", sample(), cookie, "test-submission-001"), 403)
	requireCode(t, call(api, "POST", "/api/logout", nil, cookie, ""), 200)
	requireCode(t, call(api, "GET", "/api/session", nil, cookie, ""), 401)
	if count(t, db, "items") != 0 {
		t.Fatal("unauthorized write")
	}
}

func TestPrivateFieldSeparation(t *testing.T) {
	api, _ := fixture(t)
	staff := login(t, api, "reception")
	created := call(api, "POST", "/api/items", sample(), staff, "test-submission-002")
	requireCode(t, created, 201)
	id := contents(t, created)["item"].(map[string]any)["id"].(string)
	for _, name := range []string{"student", "other"} {
		cookie := login(t, api, name)
		response := call(api, "GET", "/api/items/"+id, nil, cookie, "")
		requireCode(t, response, 200)
		item := contents(t, response)["item"].(map[string]any)
		for _, field := range []string{"description", "identifyingDetails", "brand", "color", "creatorId"} {
			if _, present := item[field]; present {
				t.Fatalf("%s sees %s", name, field)
			}
		}
		requireCode(t, call(api, "GET", "/api/items/"+id+"/history", nil, cookie, ""), 403)
	}
	for _, cookie := range []string{staff, login(t, api, "admin")} {
		item := contents(t, call(api, "GET", "/api/items/"+id, nil, cookie, ""))["item"].(map[string]any)
		if item["identifyingDetails"] != sample().IdentifyingDetails {
			t.Fatal("private details missing")
		}
		directory := call(api, "GET", "/api/items", nil, cookie, "")
		if strings.Contains(directory.Body.String(), "ABC-123") || strings.Contains(directory.Body.String(), "description") {
			t.Fatal("directory exposes private fields")
		}
	}
}

func TestValidationAndHoldingPoint(t *testing.T) {
	api, db := fixture(t)
	cookie := login(t, api, "reception")
	bad := []Intake{}
	value := sample()
	value.Description = " "
	bad = append(bad, value)
	value = sample()
	value.DateFound = "2026-02-31"
	bad = append(bad, value)
	value = sample()
	value.ReceivedAt = "2026-02-31T10:00"
	bad = append(bad, value)
	value = sample()
	value.DateFound = "2026-11-12"
	bad = append(bad, value)
	value = sample()
	value.Category = "missing"
	bad = append(bad, value)
	value = sample()
	value.Location = strings.Repeat("x", 151)
	bad = append(bad, value)
	for _, value := range bad {
		requireCode(t, call(api, "POST", "/api/items", value, cookie, "test-submission-003"), 400)
	}
	value = sample()
	value.HoldingPoint = "student-affairs"
	requireCode(t, call(api, "POST", "/api/items", value, cookie, "test-submission-003"), 403)
	if count(t, db, "items") != 0 || count(t, db, "history") != 0 {
		t.Fatal("invalid write persisted")
	}
}

func TestConcurrentRetryAndChangedPayload(t *testing.T) {
	api, db := fixture(t)
	cookie := login(t, api, "reception")
	responses := make([]*httptest.ResponseRecorder, 10)
	var wait sync.WaitGroup
	for index := range responses {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			responses[index] = call(api, "POST", "/api/items", sample(), cookie, "test-submission-004")
		}(index)
	}
	wait.Wait()
	created := 0
	ids := map[string]bool{}
	for _, response := range responses {
		if response.Code == 201 {
			created++
		} else {
			requireCode(t, response, 200)
		}
		ids[contents(t, response)["item"].(map[string]any)["id"].(string)] = true
	}
	if created != 1 || len(ids) != 1 || count(t, db, "items") != 1 || count(t, db, "history") != 1 {
		t.Fatal("retry was duplicated")
	}
	changed := sample()
	changed.Location = "Cafeteria"
	requireCode(t, call(api, "POST", "/api/items", changed, cookie, "test-submission-004"), 409)
}

func TestHistoryFailureRollsBackEverything(t *testing.T) {
	api, db := fixture(t)
	cookie := login(t, api, "reception")
	if _, err := db.Exec("CREATE TRIGGER fail_history BEFORE INSERT ON history BEGIN SELECT RAISE(ABORT, 'test history failure'); END"); err != nil {
		t.Fatal(err)
	}
	requireCode(t, call(api, "POST", "/api/items", sample(), cookie, "test-submission-005"), 503)
	for _, table := range []string{"items", "history", "submissions"} {
		if count(t, db, table) != 0 {
			t.Fatalf("partial write in %s", table)
		}
	}
	db.Exec("DROP TRIGGER fail_history")
	requireCode(t, call(api, "POST", "/api/items", sample(), cookie, "test-submission-005"), 201)
	if _, err := db.Exec("UPDATE history SET action='changed'"); err == nil {
		t.Fatal("history updated")
	}
	if _, err := db.Exec("DELETE FROM history"); err == nil {
		t.Fatal("history deleted")
	}
}

func TestFiltersPaginationAndFinalItems(t *testing.T) {
	api, db := fixture(t)
	cookie := login(t, api, "reception")
	for index := 0; index < 22; index++ {
		value := sample()
		if index == 0 {
			value.Location = "Cafeteria"
		}
		requireCode(t, call(api, "POST", "/api/items", value, cookie, fmt.Sprintf("test-search-key-%03d", index)), 201)
	}
	query := "/api/items?category=electronics&location=library&holdingPoint=reception&from=2026-11-10&to=2026-11-10"
	first := contents(t, call(api, "GET", query, nil, cookie, ""))
	if first["total"] != float64(21) || len(first["items"].([]any)) != 20 {
		t.Fatal("filter or page size mismatch")
	}
	second := contents(t, call(api, "GET", query+"&page=2", nil, cookie, ""))
	if len(second["items"].([]any)) != 1 {
		t.Fatal("second page mismatch")
	}
	requireCode(t, call(api, "GET", "/api/items?from=2026-11-11&to=2026-11-10", nil, cookie, ""), 400)
	requireCode(t, call(api, "GET", "/api/items?brand=Example", nil, cookie, ""), 400)
	db.Exec("UPDATE items SET status='Returned'")
	if contents(t, call(api, "GET", "/api/items", nil, cookie, ""))["total"] != float64(0) {
		t.Fatal("closed items visible")
	}
}

func TestPersistenceAndRetryAfterRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := SeedAccounts(db, testPassword); err != nil {
		t.Fatal(err)
	}
	api := NewAPI(db)
	cookie := login(t, api, "reception")
	original := call(api, "POST", "/api/items", sample(), cookie, "test-submission-007")
	requireCode(t, original, 201)
	db.Close()
	db, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	api = NewAPI(db)
	replay := call(api, "POST", "/api/items", sample(), login(t, api, "reception"), "test-submission-007")
	requireCode(t, replay, 200)
	if contents(t, original)["item"].(map[string]any)["id"] != contents(t, replay)["item"].(map[string]any)["id"] {
		t.Fatal("restart duplicated item")
	}
	if count(t, db, "history") != 1 {
		t.Fatal("restart duplicated history")
	}
}

func TestCrossOriginWrites(t *testing.T) {
	api, _ := fixture(t)
	for _, origin := range []string{"https://unrelated.example", ""} {
		request := httptest.NewRequest("POST", "/api/login", strings.NewReader(`{"login":"student","password":"local-test-secret-only"}`))
		request.Header.Set("Origin", origin)
		if origin != "" {
			request.Header.Set("X-Campus-Request", "1")
		}
		response := httptest.NewRecorder()
		api.ServeHTTP(response, request)
		requireCode(t, response, 403)
	}
}

func TestFirstRunSetupCannotResetAccounts(t *testing.T) {
	db, err := OpenStore(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	api := NewAPI(db)
	api.AllowSetup = true
	if contents(t, call(api, "GET", "/api/setup", nil, "", ""))["needsSetup"] != true {
		t.Fatal("first run not detected")
	}
	requireCode(t, call(api, "POST", "/api/setup", map[string]string{"password": "short"}, "", ""), 400)
	requireCode(t, call(api, "POST", "/api/setup", map[string]string{"password": testPassword}, "", ""), 201)
	requireCode(t, call(api, "POST", "/api/setup", map[string]string{"password": "a different password"}, "", ""), 409)
	if count(t, db, "accounts") != 4 {
		t.Fatal("setup duplicated accounts")
	}
	_ = login(t, api, "admin")
}
