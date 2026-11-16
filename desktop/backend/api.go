package backend

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

type API struct {
	DB              *sql.DB
	AllowSetup      bool
	SecureCookies   bool
	AllowedOrigin   string
	TrustLocalProxy bool
	EnableSignup    bool
	photoSlots      chan struct{}
	loginSlots      chan struct{}
	mu              sync.Mutex
	attempts        map[string]attempt
}
type attempt struct {
	count int
	reset time.Time
}

type Intake struct {
	Category           string `json:"category"`
	Description        string `json:"description"`
	Location           string `json:"location"`
	DateFound          string `json:"dateFound"`
	ReceivedAt         string `json:"receivedAt"`
	HoldingPoint       string `json:"holdingPoint"`
	IdentifyingDetails string `json:"identifyingDetails"`
	Color              string `json:"color"`
	Brand              string `json:"brand"`
	Building           string `json:"building"`
}

func NewAPI(db *sql.DB) *API {
	return &API{
		DB: db, attempts: make(map[string]attempt),
		photoSlots: make(chan struct{}, 1), loginSlots: make(chan struct{}, 4),
	}
}
func hash(value string) string {
	result := sha256.Sum256([]byte(value))
	return hex.EncodeToString(result[:])
}
func encode(value any) string { data, _ := json.Marshal(value); return string(data) }
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func fail(w http.ResponseWriter, status int, code string) {
	reply(w, status, map[string]any{"code": code})
}
func readBody(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		fail(w, 400, "invalidFields")
		return false
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		fail(w, 400, "invalidFields")
		return false
	}
	return true
}
func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie("campus_session")
	if err != nil {
		return ""
	}
	return cookie.Value
}

func (a *API) actor(r *http.Request) (*Account, error) {
	account := &Account{}
	var points string
	err := a.DB.QueryRow(`SELECT a.id,a.name,a.role,a.holding_points FROM accounts a JOIN sessions s ON a.id=s.account_id WHERE s.token_hash=? AND s.expires_at>?`, hash(sessionToken(r)), time.Now().UnixMilli()).Scan(&account.ID, &account.Name, &account.Role, &points)
	if err == nil {
		err = json.Unmarshal([]byte(points), &account.HoldingPoints)
	}
	return account, err
}

func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != "GET" && r.Method != "HEAD" {
		origin := r.Header.Get("Origin")
		validOrigin := origin == "" || origin == a.AllowedOrigin || slices.Contains([]string{"http://127.0.0.1:3000", "http://localhost:3000", "http://127.0.0.1:4200", "http://localhost:4200", "http://wails.localhost", "https://wails.localhost", "wails://wails.localhost"}, origin)
		if r.Header.Get("X-Campus-Request") != "1" || !validOrigin {
			fail(w, 403, "forbidden")
			return
		}
	}
	if r.URL.Path == "/api/setup" {
		a.setup(w, r)
		return
	}
	if r.URL.Path == "/api/login" && r.Method == "POST" {
		a.login(w, r)
		return
	}
	if r.URL.Path == "/api/signup" && r.Method == "POST" {
		a.signup(w, r)
		return
	}
	actor, err := a.actor(r)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 401, "sessionExpired")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	switch {
	case r.URL.Path == "/api/session" && r.Method == "GET":
		reply(w, 200, map[string]any{"account": actor})
	case r.URL.Path == "/api/logout" && r.Method == "POST":
		if _, err := a.DB.Exec("DELETE FROM sessions WHERE token_hash=?", hash(sessionToken(r))); err != nil {
			fail(w, 503, "unavailable")
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "campus_session", Value: "", Path: "/api", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.SecureCookies, MaxAge: -1})
		reply(w, 200, map[string]bool{"ok": true})
	case r.URL.Path == "/api/config" && r.Method == "GET":
		reply(w, 200, map[string]any{"categories": Categories, "holdingPoints": HoldingPoints})
	case r.URL.Path == "/api/items" && r.Method == "GET":
		a.search(w, r)
	case r.URL.Path == "/api/items" && r.Method == "POST":
		a.createItem(w, r, actor)
	case strings.HasPrefix(r.URL.Path, "/api/items/") && strings.HasSuffix(r.URL.Path, "/withdraw") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/items/"), "/withdraw")
		a.withdrawItem(w, r, id, actor)
	case strings.HasPrefix(r.URL.Path, "/api/items/") && r.Method == "GET":
		id := strings.TrimPrefix(r.URL.Path, "/api/items/")
		if strings.HasSuffix(id, "/photo") {
			a.itemPhoto(w, strings.TrimSuffix(id, "/photo"), actor)
			return
		}
		if strings.HasSuffix(id, "/history") {
			a.history(w, strings.TrimSuffix(id, "/history"), actor)
			return
		}
		item, err := loadItem(a.DB, id, actor)
		if errors.Is(err, sql.ErrNoRows) {
			fail(w, 404, "notFound")
			return
		}
		if err != nil {
			fail(w, 503, "unavailable")
			return
		}
		reply(w, 200, map[string]any{"item": item})
	default:
		fail(w, 404, "notFound")
	}
}

func (a *API) setup(w http.ResponseWriter, r *http.Request) {
	if !a.AllowSetup {
		state := map[string]bool{"needsSetup": false}
		if a.EnableSignup {
			state["signupEnabled"] = true
		}
		reply(w, 200, state)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var count int
	if err := a.DB.QueryRow("SELECT count(*) FROM accounts").Scan(&count); err != nil {
		fail(w, 503, "unavailable")
		return
	}
	if r.Method == "GET" {
		reply(w, 200, map[string]bool{"needsSetup": count == 0})
		return
	}
	if r.Method != "POST" || count != 0 {
		fail(w, 409, "setupComplete")
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !readBody(w, r, &body) {
		return
	}
	if len(body.Password) < 12 || len(body.Password) > 200 {
		fail(w, 400, "setupPassword")
		return
	}
	if err := SeedAccounts(a.DB, body.Password); err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 201, map[string]bool{"ok": true})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var credentials struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	// Ignore client role claims. Only the stored account decides access.
	r.Body = http.MaxBytesReader(w, r.Body, 32768)
	if json.NewDecoder(r.Body).Decode(&credentials) != nil || len(credentials.Password) == 0 || len(credentials.Password) > 200 || len(credentials.Login) > 100 {
		fail(w, 401, "loginFailed")
		return
	}
	if !a.takeAuthSlot(w, r, "login", 15) {
		return
	}
	defer func() { <-a.loginSlots }()
	now := time.Now()
	var account Account
	var points, salt, expected string
	err := a.DB.QueryRow("SELECT id,name,role,holding_points,salt,password_hash FROM accounts WHERE login=?", strings.ToLower(strings.TrimSpace(credentials.Login))).Scan(&account.ID, &account.Name, &account.Role, &points, &salt, &expected)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		fail(w, 503, "unavailable")
		return
	}
	found := err == nil
	if !found {
		salt = strings.Repeat("0", 32)
		expected = strings.Repeat("0", 128)
	}
	computed, err := derivePassword(credentials.Password, salt)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	correct := subtle.ConstantTimeCompare([]byte(hex.EncodeToString(computed)), []byte(expected)) == 1
	if !found || !correct {
		fail(w, 401, "loginFailed")
		return
	}
	_ = json.Unmarshal([]byte(points), &account.HoldingPoints)
	random := make([]byte, 32)
	if _, err := rand.Read(random); err != nil {
		fail(w, 503, "unavailable")
		return
	}
	token := hex.EncodeToString(random)
	tx, err := a.DB.Begin()
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM sessions WHERE expires_at<=? OR token_hash=?", now.UnixMilli(), hash(sessionToken(r))); err == nil {
		_, err = tx.Exec("INSERT INTO sessions VALUES (?,?,?)", hash(token), account.ID, now.Add(8*time.Hour).UnixMilli())
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "campus_session", Value: token, Path: "/api", HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: a.SecureCookies, MaxAge: 28800})
	reply(w, 200, map[string]any{"account": account})
}

func (a *API) takeAuthSlot(w http.ResponseWriter, r *http.Request, action string, maxAttempts int) bool {
	ip := action + ":" + a.clientIP(r)
	a.mu.Lock()
	now := time.Now()
	for key, value := range a.attempts {
		if now.After(value.reset) {
			delete(a.attempts, key)
		}
	}
	value := a.attempts[ip]
	if value.reset.IsZero() {
		if len(a.attempts) >= 4096 {
			a.mu.Unlock()
			w.Header().Set("Retry-After", "60")
			fail(w, 429, "rateLimited")
			return false
		}
		value.reset = now.Add(time.Minute)
	}
	value.count++
	a.attempts[ip] = value
	a.mu.Unlock()
	if value.count > maxAttempts {
		w.Header().Set("Retry-After", "60")
		fail(w, 429, "rateLimited")
		return false
	}
	select {
	case a.loginSlots <- struct{}{}:
		return true
	default:
		w.Header().Set("Retry-After", "2")
		fail(w, 503, "unavailable")
		return false
	}
}

func (a *API) clientIP(r *http.Request) string {
	peer, _, _ := net.SplitHostPort(r.RemoteAddr)
	if a.TrustLocalProxy && net.ParseIP(peer).IsLoopback() {
		if forwarded := net.ParseIP(r.Header.Get("X-Real-IP")); forwarded != nil {
			return forwarded.String()
		}
	}
	return peer
}

func validDate(value string) bool {
	parsed, err := time.Parse("2006-01-02", value)
	return err == nil && parsed.Format("2006-01-02") == value
}
func (a *API) search(w http.ResponseWriter, r *http.Request) {
	filters := r.URL.Query()
	for key, values := range filters {
		if !slices.Contains([]string{"category", "location", "holdingPoint", "from", "to", "page"}, key) || len(values) != 1 {
			fail(w, 400, "invalidFilters")
			return
		}
	}
	category, point, location, from, to := filters.Get("category"), filters.Get("holdingPoint"), filters.Get("location"), filters.Get("from"), filters.Get("to")
	if (category != "" && !slices.Contains(Categories, category)) || (point != "" && !slices.Contains(HoldingPoints, point)) || utf8.RuneCountInString(location) > 150 || (from != "" && !validDate(from)) || (to != "" && !validDate(to)) || (from != "" && to != "" && from > to) {
		fail(w, 400, "invalidFilters")
		return
	}
	page := 1
	if filters.Has("page") {
		var err error
		page, err = strconv.Atoi(filters.Get("page"))
		if err != nil || page < 1 || page > 10000 {
			fail(w, 400, "invalidFilters")
			return
		}
	}
	clauses := []string{"status='Open'"}
	args := []any{}
	for _, filter := range []struct{ value, clause string }{{category, "category=?"}, {point, "holding_point=?"}, {from, "date_found>=?"}, {to, "date_found<=?"}, {location, "instr(lower(location),lower(?))>0"}} {
		if filter.value != "" {
			clauses = append(clauses, filter.clause)
			args = append(args, filter.value)
		}
	}
	where := strings.Join(clauses, " AND ")
	var total int
	if err := a.DB.QueryRow("SELECT count(*) FROM items WHERE "+where, args...).Scan(&total); err != nil {
		fail(w, 503, "unavailable")
		return
	}
	args = append(args, (page-1)*20)
	rows, err := a.DB.Query("SELECT id,category,location,date_found,holding_point FROM items WHERE "+where+" ORDER BY date_found DESC,id LIMIT 20 OFFSET ?", args...)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, category, location, date, point string
		if err := rows.Scan(&id, &category, &location, &date, &point); err != nil {
			fail(w, 503, "unavailable")
			return
		}
		items = append(items, map[string]any{"id": id, "category": category, "location": location, "dateFound": date, "holdingPoint": point, "status": "Open"})
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"items": items, "total": total, "page": page})
}

type queryer interface{ QueryRow(string, ...any) *sql.Row }

func loadItem(db queryer, id string, actor *Account) (map[string]any, error) {
	return loadItemView(db, id, actor, false)
}

func loadItemView(db queryer, id string, actor *Account, ownReplay bool) (map[string]any, error) {
	var category, location, date, point, status, creator, private string
	var hasPhoto bool
	err := db.QueryRow(`SELECT category,location,date_found,holding_point,status,creator_id,private_data,
		EXISTS(SELECT 1 FROM item_photos WHERE item_id=items.id)
		FROM items WHERE id=? AND (status='Open' OR (status='Withdrawn' AND (?='admin' OR (? AND creator_id=?))))`,
		id, actor.Role, ownReplay, actor.ID).Scan(&category, &location, &date, &point, &status, &creator, &private, &hasPhoto)
	if err != nil {
		return nil, err
	}
	item := map[string]any{"id": id, "category": category, "location": location, "dateFound": date, "holdingPoint": point, "status": status}
	// Keep the clues off the wire. CSS is not a privacy policy.
	if actor.Role == "admin" || actor.ID == creator {
		var fields map[string]any
		if err := json.Unmarshal([]byte(private), &fields); err != nil {
			return nil, err
		}
		for key, value := range fields {
			item[key] = value
		}
		item["creatorId"] = creator
		if hasPhoto {
			item["photoUrl"] = "/api/items/" + id + "/photo"
		}
	}
	return item, nil
}

func (a *API) itemPhoto(w http.ResponseWriter, id string, actor *Account) {
	var photo itemPhoto
	err := a.DB.QueryRow(`SELECT p.mime,p.content FROM item_photos p JOIN items i ON i.id=p.item_id
		WHERE i.id=? AND (i.creator_id=? OR ?='admin')`, id, actor.ID, actor.Role).Scan(&photo.mime, &photo.content)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, 404, "notFound")
		return
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	w.Header().Set("Content-Type", photo.mime)
	w.Header().Set("Content-Length", strconv.Itoa(len(photo.content)))
	_, _ = w.Write(photo.content)
}

func validate(data *Intake) []string {
	fields := []string{}
	for _, field := range []struct {
		name     string
		value    *string
		max      int
		required bool
	}{
		{"description", &data.Description, 2000, true}, {"location", &data.Location, 150, true},
		{"identifyingDetails", &data.IdentifyingDetails, 500, false}, {"color", &data.Color, 500, false}, {"brand", &data.Brand, 500, false}, {"building", &data.Building, 500, false},
	} {
		*field.value = strings.TrimSpace(*field.value)
		if (field.required && *field.value == "") || utf8.RuneCountInString(*field.value) > field.max {
			fields = append(fields, field.name)
		}
	}
	if !slices.Contains(Categories, data.Category) {
		fields = append(fields, "category")
	}
	if !slices.Contains(HoldingPoints, data.HoldingPoint) {
		fields = append(fields, "holdingPoint")
	}
	if !validDate(data.DateFound) {
		fields = append(fields, "dateFound")
	}
	parsed, err := time.Parse("2006-01-02T15:04", data.ReceivedAt)
	if err != nil || parsed.Format("2006-01-02T15:04") != data.ReceivedAt {
		fields = append(fields, "receivedAt")
	}
	if len(data.ReceivedAt) >= 10 && data.DateFound > data.ReceivedAt[:10] {
		fields = append(fields, "dateFound")
	}
	return fields
}

var submissionPattern = regexp.MustCompile(`^[a-zA-Z0-9-]{16,100}$`)

func (a *API) createItem(w http.ResponseWriter, r *http.Request, actor *Account) {
	if actor.Role == "campus" {
		fail(w, 403, "forbidden")
		return
	}
	var data Intake
	if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/") {
		select {
		case a.photoSlots <- struct{}{}:
			defer func() { <-a.photoSlots }()
		default:
			w.Header().Set("Retry-After", "2")
			fail(w, 503, "unavailable")
			return
		}
	}
	inputPhoto, valid := readIntake(w, r, &data)
	if !valid {
		return
	}
	if fields := validate(&data); len(fields) > 0 {
		reply(w, 400, map[string]any{"code": "invalidFields", "fields": fields})
		return
	}
	if actor.Role != "admin" && !slices.Contains(actor.HoldingPoints, data.HoldingPoint) {
		fail(w, 403, "forbidden")
		return
	}
	key := r.Header.Get("Idempotency-Key")
	if !submissionPattern.MatchString(key) {
		fail(w, 400, "submissionKeyRequired")
		return
	}
	fingerprint := intakeFingerprint(data, inputPhoto)
	var photo itemPhoto
	if inputPhoto != nil {
		var err error
		photo, err = reducePhoto(inputPhoto)
		if err != nil {
			fail(w, 400, "invalidPhoto")
			return
		}
	}
	tx, err := a.DB.Begin()
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer tx.Rollback()
	var previousHash, id string
	err = tx.QueryRow("SELECT request_hash,item_id FROM submissions WHERE account_id=? AND submission_key=?", actor.ID, key).Scan(&previousHash, &id)
	if err == nil {
		if previousHash != fingerprint {
			fail(w, 409, "submissionChanged")
			return
		}
		item, err := loadItemView(tx, id, actor, true)
		if err != nil {
			fail(w, 503, "unavailable")
			return
		}
		reply(w, 200, map[string]any{"item": item, "replayed": true})
		return
	}
	if !errors.Is(err, sql.ErrNoRows) {
		fail(w, 503, "unavailable")
		return
	}
	random := make([]byte, 16)
	if _, err := rand.Read(random); err != nil {
		fail(w, 503, "unavailable")
		return
	}
	id = "CLF-" + hex.EncodeToString(random)
	timestamp := time.Now().UTC().Format(time.RFC3339Nano)
	private := encode(map[string]any{"description": data.Description, "identifyingDetails": data.IdentifyingDetails, "receivedAt": data.ReceivedAt, "color": data.Color, "brand": data.Brand, "building": data.Building})
	_, err = tx.Exec("INSERT INTO items(id,category,location,date_found,holding_point,creator_id,private_data,created_at) VALUES(?,?,?,?,?,?,?,?)", id, data.Category, data.Location, data.DateFound, data.HoldingPoint, actor.ID, private, timestamp)
	if err == nil && inputPhoto != nil {
		_, err = tx.Exec("INSERT INTO item_photos(item_id,mime,content) VALUES(?,?,?)", id, photo.mime, photo.content)
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO history(record_id,action,account_id,timestamp,values_json) VALUES(?,?,?,?,?)", id, "created", actor.ID, timestamp, encode(data))
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO submissions VALUES(?,?,?,?)", actor.ID, key, fingerprint, id)
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	item, err := loadItem(tx, id, actor)
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 201, map[string]any{"item": item, "replayed": false})
}

func (a *API) history(w http.ResponseWriter, id string, actor *Account) {
	if actor.Role != "admin" {
		fail(w, 403, "forbidden")
		return
	}
	rows, err := a.DB.Query(`SELECT h.sequence,h.action,h.timestamp,a.name,
		CASE WHEN h.action='withdrawn' THEN json_extract(h.values_json,'$.reason') ELSE NULL END
		FROM history h JOIN accounts a ON a.id=h.account_id WHERE h.record_id=? ORDER BY h.sequence`, id)
	if err != nil {
		fail(w, 503, "unavailable")
		return
	}
	defer rows.Close()
	entries := []map[string]any{}
	for rows.Next() {
		var sequence int
		var action, timestamp, name string
		var reason sql.NullString
		if rows.Scan(&sequence, &action, &timestamp, &name, &reason) != nil {
			fail(w, 503, "unavailable")
			return
		}
		entry := map[string]any{"sequence": sequence, "action": action, "timestamp": timestamp, "actor": name}
		if reason.Valid {
			entry["reason"] = reason.String
		}
		entries = append(entries, entry)
	}
	if rows.Err() != nil {
		fail(w, 503, "unavailable")
		return
	}
	reply(w, 200, map[string]any{"entries": entries})
}
