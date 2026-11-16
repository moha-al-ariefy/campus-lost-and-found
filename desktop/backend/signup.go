package backend

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var campusLoginPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,39}$`)

func (a *API) signup(w http.ResponseWriter, r *http.Request) {
	if !a.EnableSignup {
		fail(w, 404, "notFound")
		return
	}
	var form struct {
		Name     string `json:"name"`
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if !readBody(w, r, &form) {
		return
	}
	name := strings.TrimSpace(form.Name)
	login := strings.ToLower(strings.TrimSpace(form.Login))
	if !campusLoginPattern.MatchString(login) || name == "" || utf8.RuneCountInString(name) > 80 ||
		strings.IndexFunc(name, unicode.IsControl) >= 0 ||
		utf8.RuneCountInString(form.Password) < 15 || len(form.Password) > 200 ||
		strings.TrimSpace(form.Password) == "" {
		fail(w, 400, "invalidSignup")
		return
	}
	if !a.takeAuthSlot(w, r, "signup", 5) {
		return
	}
	defer func() { <-a.loginSlots }()
	var saltBytes [16]byte
	if _, err := rand.Read(saltBytes[:]); err != nil {
		fail(w, 503, "unavailable")
		return
	}
	salt := "s2:" + hex.EncodeToString(saltBytes[:])
	passwordHash, err := derivePassword(form.Password, salt)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	// New accounts never inherit a role or assignment from the request.
	result, err := a.DB.Exec(`INSERT INTO accounts(id,name,login,role,holding_points,salt,password_hash)
		VALUES(?,?,?,'campus','[]',?,?) ON CONFLICT(login) DO NOTHING`,
		"campus-"+rand.Text(), name, login, salt, hex.EncodeToString(passwordHash))
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	inserted, err := result.RowsAffected()
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	if inserted != 1 {
		fail(w, 409, "signupUnavailable")
		return
	}
	reply(w, 201, map[string]bool{"ok": true})
}
