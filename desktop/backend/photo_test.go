package backend

import (
	"bytes"
	"database/sql"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func jpegFixture(t *testing.T, shade uint8) []byte {
	t.Helper()
	pixels := image.NewRGBA(image.Rect(0, 0, 10, 10))
	pixels.Set(5, 5, color.RGBA{R: shade, A: 255})
	var content bytes.Buffer
	if err := jpeg.Encode(&content, pixels, nil); err != nil {
		t.Fatal(err)
	}
	return content.Bytes()
}

func upload(t *testing.T, api *API, cookie, key string, photos ...[]byte) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	api.ServeHTTP(response, photoRequest(t, cookie, key, photos...))
	return response
}

func photoRequest(t *testing.T, cookie, key string, photos ...[]byte) *http.Request {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("item", encode(sample())); err != nil {
		t.Fatal(err)
	}
	for _, photo := range photos {
		field, err := writer.CreateFormFile("photo", "fixture.jpg")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := field.Write(photo); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "http://localhost/api/items", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.Header.Set("X-Campus-Request", "1")
	request.Header.Set("Cookie", cookie)
	request.Header.Set("Idempotency-Key", key)
	return request
}

type pausedPhotoUpload struct {
	io.ReadCloser
	reading chan struct{}
	resume  chan struct{}
}

func (body *pausedPhotoUpload) Read(buffer []byte) (int, error) {
	select {
	case <-body.reading:
	default:
		close(body.reading)
	}
	<-body.resume
	return body.ReadCloser.Read(buffer)
}

func TestSlowPhotoBodyDoesNotBlockAnotherUpload(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	slow := photoRequest(t, staff, "photo-slow-first-upload", jpegFixture(t, 40))
	body := &pausedPhotoUpload{ReadCloser: slow.Body, reading: make(chan struct{}), resume: make(chan struct{})}
	slow.Body = body
	slowResult := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		api.ServeHTTP(slowResult, slow)
	}()
	defer func() {
		close(body.resume)
		<-done
		if !t.Failed() {
			requireCode(t, slowResult, http.StatusCreated)
			if count(t, db, "item_photos") != 2 || count(t, db, "history") != 2 {
				t.Fatal("overlapping requests did not save both photos and history entries")
			}
		}
	}()
	select {
	case <-body.reading:
	case <-time.After(3 * time.Second):
		t.Fatal("first upload did not reach its body")
	}
	fast := upload(t, api, staff, "photo-fast-next-upload", jpegFixture(t, 90))
	requireCode(t, fast, http.StatusCreated)
}

func TestPhotoOverloadLeavesRequestRetryable(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	photo := jpegFixture(t, 60)
	key := "photo-overload-submission"
	for range cap(api.photoUploads) {
		api.photoUploads <- struct{}{}
	}
	rejected := upload(t, api, staff, key, photo)
	requireCode(t, rejected, http.StatusServiceUnavailable)
	if rejected.Header().Get("Retry-After") != "2" || count(t, db, "submissions") != 0 {
		t.Fatal("overload did not provide a safe retry")
	}
	<-api.photoUploads
	requireCode(t, upload(t, api, staff, key, photo), http.StatusCreated)
}

func TestPhotoLimitPermissionsAndRetry(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	photo := make([]byte, maxPhotoInput)
	copy(photo, jpegFixture(t, 40))

	created := upload(t, api, staff, "photo-submission-001", photo)
	requireCode(t, created, 201)
	item := contents(t, created)["item"].(map[string]any)
	path := item["photoUrl"].(string)
	stored := call(api, "GET", path, nil, staff, "")
	requireCode(t, stored, 200)
	if stored.Header().Get("Content-Type") != "image/jpeg" || stored.Header().Get("Cache-Control") != "no-store" ||
		stored.Body.Len() > maxStoredPhoto {
		t.Fatal("wrong photo type, cache policy or stored size")
	}
	if _, err := jpeg.Decode(bytes.NewReader(stored.Body.Bytes())); err != nil {
		t.Fatal("stored photo is not readable", err)
	}
	second := upload(t, api, staff, "photo-submission-002", jpegFixture(t, 255))
	requireCode(t, second, 201)
	secondPath := contents(t, second)["item"].(map[string]any)["photoUrl"].(string)
	otherPhoto := call(api, "GET", secondPath, nil, staff, "")
	requireCode(t, otherPhoto, 200)
	if bytes.Equal(stored.Body.Bytes(), otherPhoto.Body.Bytes()) {
		t.Fatal("different records returned the same photo")
	}
	requireCode(t, call(api, "GET", path, nil, "", ""), 401)
	for _, username := range []string{"student", "other"} {
		viewer := login(t, api, username)
		requireCode(t, call(api, "GET", path, nil, viewer, ""), 404)
		details := contents(t, call(api, "GET", "/api/items/"+item["id"].(string), nil, viewer, ""))
		if _, leaked := details["item"].(map[string]any)["photoUrl"]; leaked {
			t.Fatal("photo URL disclosed to", username)
		}
	}
	requireCode(t, call(api, "GET", path, nil, login(t, api, "admin"), ""), 200)
	requireCode(t, upload(t, api, staff, "photo-submission-001", photo), 200)
	requireCode(t, upload(t, api, staff, "photo-submission-001", jpegFixture(t, 210)), 409)
	requireCode(t, upload(t, api, staff, "photo-submission-001", []byte("not an image")), 409)
	if count(t, db, "item_photos") != 2 || count(t, db, "history") != 2 {
		t.Fatal("retry created an extra photo or history record")
	}
}

func TestPhoneResolutionJPEGIsReduced(t *testing.T) {
	api, _ := fixture(t)
	staff := login(t, api, "reception")
	source := image.NewYCbCr(image.Rect(0, 0, 8000, 6000), image.YCbCrSubsampleRatio420)
	for index := range source.Y {
		source.Y[index] = uint8(index / 8 % 223)
	}
	var input bytes.Buffer
	if err := jpeg.Encode(&input, source, nil); err != nil {
		t.Fatal(err)
	}
	if input.Len() > maxPhotoInput {
		t.Fatal("camera fixture exceeds the upload limit", input.Len())
	}
	created := upload(t, api, staff, "photo-camera-resolution", input.Bytes())
	requireCode(t, created, 201)
	path := contents(t, created)["item"].(map[string]any)["photoUrl"].(string)
	stored := call(api, "GET", path, nil, staff, "")
	requireCode(t, stored, 200)
	pixels, err := jpeg.Decode(bytes.NewReader(stored.Body.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if stored.Body.Len() > maxStoredPhoto || pixels.Bounds().Size() != image.Pt(1600, 1200) {
		t.Fatal("phone photo was not reduced to the storage and dimension limits")
	}
	requireCode(t, upload(t, api, staff, "photo-camera-resolution", input.Bytes()), 200)

	tooWide := bytes.Clone(input.Bytes())
	frame := bytes.Index(tooWide, []byte{0xff, 0xc0})
	if frame < 0 {
		t.Fatal("camera fixture has no baseline frame")
	}
	tooWide[frame+7], tooWide[frame+8] = 0xff, 0xff
	rejected := upload(t, api, staff, "photo-camera-oversize", tooWide)
	requireCode(t, rejected, 400)
	if contents(t, rejected)["code"] != "photoDimensions" {
		t.Fatal("dimension rejection did not identify the problem")
	}
}

func TestPhotoRejectionAndAtomicFailure(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	photo := jpegFixture(t, 140)
	key := "photo-submission-003"
	requireCode(t, upload(t, api, staff, key, []byte("%PDF-1.7")), 400)
	requireCode(t, upload(t, api, staff, key, photo, photo), 400)
	requireCode(t, upload(t, api, staff, key, make([]byte, maxPhotoInput+1)), 413)

	if _, err := db.Exec(`CREATE TRIGGER test_history_failure BEFORE INSERT ON history
		BEGIN SELECT RAISE(ABORT, 'fixture failure'); END;`); err != nil {
		t.Fatal(err)
	}
	requireCode(t, upload(t, api, staff, key, photo), 503)
	for _, table := range []string{"items", "item_photos", "history", "submissions"} {
		if count(t, db, table) != 0 {
			t.Fatal("failed submission left data in", table)
		}
	}
	if _, err := db.Exec("DROP TRIGGER test_history_failure"); err != nil {
		t.Fatal(err)
	}
	requireCode(t, upload(t, api, staff, key, photo), 201)
}

func TestLockedStoreLeavesPhotoSubmissionRetryable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "campus.db")
	db, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := SeedAccounts(db, testPassword); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("PRAGMA busy_timeout=25"); err != nil {
		t.Fatal(err)
	}
	api := NewAPI(db)
	staff := login(t, api, "reception")
	photo := jpegFixture(t, 140)
	key := "photo-locked-store-retry"

	writer, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer writer.Close()
	writer.SetMaxOpenConns(1)
	if _, err := writer.Exec("BEGIN IMMEDIATE"); err != nil {
		t.Fatal(err)
	}
	requireCode(t, upload(t, api, staff, key, photo), http.StatusServiceUnavailable)
	for _, table := range []string{"items", "item_photos", "history", "submissions"} {
		if count(t, db, table) != 0 {
			t.Fatal("storage lock left data in", table)
		}
	}
	if _, err := writer.Exec("ROLLBACK"); err != nil {
		t.Fatal(err)
	}
	requireCode(t, upload(t, api, staff, key, photo), http.StatusCreated)
	requireCode(t, upload(t, api, staff, key, photo), http.StatusOK)
	for _, table := range []string{"items", "item_photos", "history", "submissions"} {
		if count(t, db, table) != 1 {
			t.Fatal("retry failed to save exactly once in", table)
		}
	}
}

func TestFullStorePreservesExistingRecordsAndAllowsPhotoRetry(t *testing.T) {
	api, db := fixture(t)
	staff := login(t, api, "reception")
	prior := call(api, "POST", "/api/items", sample(), staff, "photo-full-prior-record")
	requireCode(t, prior, http.StatusCreated)
	priorID := contents(t, prior)["item"].(map[string]any)["id"].(string)
	pixels := image.NewGray(image.Rect(0, 0, 320, 240))
	random := rand.New(rand.NewPCG(9, 13))
	for index := range pixels.Pix {
		pixels.Pix[index] = byte(random.Uint32())
	}
	var input bytes.Buffer
	if err := jpeg.Encode(&input, pixels, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	var allocated, limit int
	if err := db.QueryRow("PRAGMA page_count").Scan(&allocated); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(fmt.Sprintf("PRAGMA max_page_count=%d", allocated)).Scan(&limit); err != nil || limit != allocated {
		t.Fatalf("could not pin SQLite to its current page count: limit=%d err=%v", limit, err)
	}

	key := "photo-full-store-retry"
	requireCode(t, upload(t, api, staff, key, input.Bytes()), http.StatusServiceUnavailable)
	requireCode(t, call(api, "GET", "/api/items/"+priorID, nil, staff, ""), http.StatusOK)
	if count(t, db, "items") != 1 || count(t, db, "history") != 1 ||
		count(t, db, "submissions") != 1 || count(t, db, "item_photos") != 0 {
		t.Fatal("failed write changed existing records or left a partial photo submission")
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", allocated+256)); err != nil {
		t.Fatal(err)
	}
	requireCode(t, upload(t, api, staff, key, input.Bytes()), http.StatusCreated)
	requireCode(t, upload(t, api, staff, key, input.Bytes()), http.StatusOK)
	if count(t, db, "items") != 2 || count(t, db, "history") != 2 ||
		count(t, db, "submissions") != 2 || count(t, db, "item_photos") != 1 {
		t.Fatal("retry did not save the photo, item and history exactly once")
	}
}

func TestLargePNGIsReducedAndStaysReadable(t *testing.T) {
	pixels := image.NewRGBA(image.Rect(0, 0, 1800, 1600))
	random := rand.New(rand.NewPCG(7, 11))
	for index := 0; index < len(pixels.Pix); index += 4 {
		value := random.Uint32()
		pixels.Pix[index], pixels.Pix[index+1], pixels.Pix[index+2] = byte(value), byte(value>>8), byte(value>>16)
		pixels.Pix[index+3] = 255
	}
	var input bytes.Buffer
	encoder := png.Encoder{CompressionLevel: png.BestSpeed}
	if err := encoder.Encode(&input, pixels); err != nil {
		t.Fatal(err)
	}
	if input.Len() <= maxStoredPhoto || input.Len() > maxPhotoInput {
		t.Fatal("PNG fixture is outside the intended test range", input.Len())
	}
	output, err := reducePhoto(input.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if output.mime != "image/png" || len(output.content) > maxStoredPhoto {
		t.Fatal("stored PNG exceeds the limit or has the wrong media type")
	}
	if _, err := png.Decode(bytes.NewReader(output.content)); err != nil {
		t.Fatal("stored PNG is not readable", err)
	}
}

func TestPhotoBackupRestoresRecordAndHistory(t *testing.T) {
	api, db := fixture(t)
	admin := login(t, api, "admin")
	created := upload(t, api, admin, "photo-submission-004", jpegFixture(t, 150))
	requireCode(t, created, 201)
	item := contents(t, created)["item"].(map[string]any)
	path := item["photoUrl"].(string)
	original := call(api, "GET", path, nil, admin, "")
	requireCode(t, original, 200)

	destination := filepath.Join(t.TempDir(), "snapshot.db")
	if err := BackupStore(db, destination); err != nil {
		t.Fatal(err)
	}
	if err := BackupStore(db, destination); err == nil {
		t.Fatal("backup overwrote an existing snapshot")
	}
	restored, err := OpenStore(destination)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	restoredAPI := NewAPI(restored)
	restoredAdmin := login(t, restoredAPI, "admin")
	result := call(restoredAPI, "GET", path, nil, restoredAdmin, "")
	requireCode(t, result, 200)
	if !bytes.Equal(original.Body.Bytes(), result.Body.Bytes()) {
		t.Fatal("restored photo differs")
	}
	for _, table := range []string{"items", "item_photos", "history", "submissions"} {
		if count(t, restored, table) != count(t, db, table) {
			t.Fatal("restored record count differs in", table)
		}
	}
}

func TestProxyIdentityOnlyFromLocalProxy(t *testing.T) {
	api, _ := fixture(t)
	api.TrustLocalProxy = true
	request := httptest.NewRequest("GET", "http://localhost/api/session", nil)
	request.Header.Set("X-Real-IP", "203.0.113.40")
	request.RemoteAddr = "203.0.113.20:443"
	if api.clientIP(request) != "203.0.113.20" {
		t.Fatal("used identity from an untrusted peer")
	}
	request.RemoteAddr = "127.0.0.1:443"
	if api.clientIP(request) != "203.0.113.40" {
		t.Fatal("ignored identity set by the local proxy")
	}
}
