package backend

import (
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
)

func TestWebOriginKeepsSessionAndWriteGuards(t *testing.T) {
	api, _ := fixture(t)
	api.SecureCookies = true
	api.AllowedOrigin = "https://api.example.test"
	api.AllowedWebOrigin = "https://campus.example.test"
	credentials := encode(map[string]string{"login": "student", "password": testPassword})
	for _, trial := range []struct {
		origin, marker string
		want           int
	}{
		{api.AllowedWebOrigin + ".other.test", "1", http.StatusForbidden},
		{api.AllowedWebOrigin, "", http.StatusForbidden},
		{"null", "1", http.StatusForbidden},
		{api.AllowedOrigin, "1", http.StatusOK},
		{api.AllowedWebOrigin, "1", http.StatusOK},
	} {
		login := httptest.NewRequest(http.MethodPost, "https://campus.example.test/api/login", strings.NewReader(credentials))
		login.Header.Set("Origin", trial.origin)
		login.Header.Set("X-Campus-Request", trial.marker)
		response := httptest.NewRecorder()
		api.ServeHTTP(response, login)
		requireCode(t, response, trial.want)
		if trial.want != http.StatusOK {
			continue
		}
		cookies := response.Result().Cookies()
		if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].Domain != "" ||
			cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Path != "/api" ||
			response.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("login must return a private, host-only session cookie and disable caching")
		}
		requireCode(t, call(api, "GET", "/api/session", nil, cookies[0].String(), ""), http.StatusOK)
	}
}

func TestBrowserSessionThroughHTTPSRewrite(t *testing.T) {
	api, _ := fixture(t)
	api.SecureCookies = true
	upstream := httptest.NewServer(api)
	defer upstream.Close()
	destination, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	web := httptest.NewTLSServer(httputil.NewSingleHostReverseProxy(destination))
	defer web.Close()
	api.AllowedWebOrigin = web.URL
	client := web.Client()
	client.Jar, err = cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	post := func(path string, value any, key string) {
		t.Helper()
		request, err := http.NewRequest(http.MethodPost, web.URL+path, strings.NewReader(encode(value)))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Origin", web.URL)
		request.Header.Set("X-Campus-Request", "1")
		request.Header.Set("Idempotency-Key", key)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusCreated && response.StatusCode != http.StatusOK {
			t.Fatalf("%s returned %d", path, response.StatusCode)
		}
	}
	post("/api/login", map[string]string{"login": "student", "password": testPassword}, "")
	post("/api/reports", LostReportInput{Category: "other", DateLost: "2026-11-10", Description: "Browser fixture"}, "browser-origin-save-fixture")
	response, err := client.Get(web.URL + "/api/session")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("browser session did not survive the rewrite: %d", response.StatusCode)
	}
}
