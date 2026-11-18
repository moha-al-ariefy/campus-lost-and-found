package backend

import (
	"context"
	"net/http"
	"testing"
)

type identityProviderFunc func(context.Context, string, string) (*Account, error)

func (authenticate identityProviderFunc) Authenticate(ctx context.Context, login, password string) (*Account, error) {
	return authenticate(ctx, login, password)
}

func TestReplaceableIdentityProvider(t *testing.T) {
	api, _ := fixture(t)
	accounts := map[string]Account{
		"student":   {ID: "student", Name: "Campus user", Role: "campus"},
		"reception": {ID: "reception", Name: "Reception staff", Role: "staff", HoldingPoints: []string{"reception"}},
		"admin":     {ID: "admin", Name: "Administrator", Role: "admin"},
	}
	api.IdentityProvider = identityProviderFunc(func(_ context.Context, login, password string) (*Account, error) {
		account, ok := accounts[login]
		if !ok || password != testPassword {
			return nil, ErrInvalidCredentials
		}
		return &account, nil
	})
	requireCode(t, call(api, "POST", "/api/login", map[string]string{"login": "student", "password": "wrong"}, "", ""), http.StatusUnauthorized)
	campus, staff, admin := login(t, api, "student"), login(t, api, "reception"), login(t, api, "admin")
	requireCode(t, call(api, "POST", "/api/items", sample(), campus, "provider-campus-denied"), http.StatusForbidden)
	itemID := createActivityItem(t, api, staff, "provider-staff-allowed")
	path := "/api/items/" + itemID + "/history"
	requireCode(t, call(api, "GET", path, nil, staff, ""), http.StatusForbidden)
	requireCode(t, call(api, "GET", path, nil, admin, ""), http.StatusOK)
}

func TestFinderRemainsPrivateAfterCorrection(t *testing.T) {
	api, _ := fixture(t)
	staff, campus := login(t, api, "reception"), login(t, api, "student")
	other, admin := login(t, api, "other"), login(t, api, "admin")
	intake := sample()
	intake.Finder = "Finder fixture"
	saved := call(api, "POST", "/api/items", intake, staff, "finder-create-fixture")
	requireCode(t, saved, http.StatusCreated)
	id := contents(t, saved)["item"].(map[string]any)["id"].(string)
	path := "/api/items/" + id
	for _, viewer := range []string{campus, other} {
		if contents(t, call(api, "GET", path, nil, viewer, ""))["item"].(map[string]any)["finder"] != nil {
			t.Fatal("unrelated viewer could read the finder")
		}
	}
	intake.Finder = "Invalid\nvalue"
	requireCode(t, call(api, "POST", path+"/correction", itemCorrection{Intake: intake}, staff, "finder-invalid-fixture"), http.StatusBadRequest)
	intake.Finder = "Corrected fixture"
	requireCode(t, call(api, "POST", path+"/correction", itemCorrection{Intake: intake}, staff, "finder-correct-fixture"), http.StatusOK)
	for _, viewer := range []string{staff, admin} {
		if contents(t, call(api, "GET", path, nil, viewer, ""))["item"].(map[string]any)["finder"] != intake.Finder {
			t.Fatal("authorized viewer could not read the corrected finder")
		}
	}
	entries := contents(t, call(api, "GET", path+"/history", nil, admin, ""))["entries"].([]any)
	change := entries[1].(map[string]any)["details"].(map[string]any)
	if change["before"].(map[string]any)["finder"] != "Finder fixture" ||
		change["after"].(map[string]any)["finder"] != intake.Finder {
		t.Fatal("finder correction was missing from history")
	}
}
