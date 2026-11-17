package backend

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"strings"
	"time"

	"golang.org/x/image/draw"
)

const (
	maxPhotoInput  = 10_000_000
	maxStoredPhoto = 1_000_000
	maxIntakeJSON  = 32_768
	maxPhotoPixels = 32_000_000
	maxCameraJPEG  = 50_000_000
)

var errPhotoDimensions = errors.New("photo exceeds the decode limit")

type itemPhoto struct {
	content []byte
	mime    string
}

// Read several bounded request bodies while keeping decoded images to one at a time.
func (a *API) waitForPhotoDecoder(w http.ResponseWriter, r *http.Request) bool {
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case a.photoDecoder <- struct{}{}:
		if r.Context().Err() == nil {
			return true
		}
		<-a.photoDecoder
	case <-timer.C:
	case <-r.Context().Done():
	}
	photoBusy(w)
	return false
}

func photoBusy(w http.ResponseWriter) {
	w.Header().Set("Retry-After", "2")
	fail(w, http.StatusServiceUnavailable, "unavailable")
}

func (a *API) takePhotoUpload(w http.ResponseWriter, r *http.Request) (release func(), ok bool) {
	if !isMultipart(r) {
		return func() {}, true
	}
	select {
	case a.photoUploads <- struct{}{}:
		return func() { <-a.photoUploads }, true
	default:
		photoBusy(w)
		return nil, false
	}
}

func isMultipart(r *http.Request) bool {
	return strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/")
}

func readPhotoSubmission(w http.ResponseWriter, r *http.Request, data any) ([]byte, bool) {
	if !isMultipart(r) {
		return nil, readBody(w, r, data)
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		fail(w, 400, "invalidPhoto")
		return nil, false
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxPhotoInput+2*maxIntakeJSON)
	reader, err := r.MultipartReader()
	if err != nil {
		fail(w, 400, "invalidPhoto")
		return nil, false
	}
	var metadata, photo []byte
	for {
		part, err := reader.NextRawPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			failIntakeRead(w, err)
			return nil, false
		}
		switch part.FormName() {
		case "item":
			if metadata != nil || part.FileName() != "" {
				fail(w, 400, "invalidFields")
				return nil, false
			}
			metadata, err = io.ReadAll(io.LimitReader(part, maxIntakeJSON+1))
			if len(metadata) > maxIntakeJSON {
				fail(w, 413, "invalidFields")
				return nil, false
			}
		case "photo":
			if photo != nil || part.FileName() == "" {
				fail(w, 400, "invalidPhoto")
				return nil, false
			}
			photo, err = io.ReadAll(io.LimitReader(part, maxPhotoInput+1))
			if len(photo) > maxPhotoInput {
				fail(w, 413, "photoTooLarge")
				return nil, false
			}
			if len(photo) == 0 && err == nil {
				fail(w, 400, "invalidPhoto")
				return nil, false
			}
		default:
			fail(w, 400, "invalidFields")
			return nil, false
		}
		if err != nil {
			failIntakeRead(w, err)
			return nil, false
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(metadata))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(data); err != nil || decoder.Decode(new(any)) != io.EOF {
		fail(w, 400, "invalidFields")
		return nil, false
	}
	return photo, true
}

func failIntakeRead(w http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		fail(w, 413, "photoTooLarge")
		return
	}
	fail(w, 400, "invalidPhoto")
}

func intakeFingerprint(data any, photo []byte) string {
	if photo == nil {
		return hash(encode(data))
	}
	digest := sha256.Sum256(photo)
	return hash(encode(data) + ":" + hex.EncodeToString(digest[:]))
}

func reducePhoto(input []byte) (itemPhoto, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 ||
		config.Height < 1 {
		return itemPhoto{}, errors.New("unsupported photo")
	}
	if config.Width > maxPhotoPixels/config.Height && (format != "jpeg" ||
		config.Width > maxCameraJPEG/config.Height || !usesBaselineCameraSampling(input)) {
		return itemPhoto{}, errPhotoDimensions
	}
	source, _, err := image.Decode(bytes.NewReader(input))
	if err != nil {
		return itemPhoto{}, err
	}
	// Re-encode pixels so uploaded metadata and trailing bytes are never served.
	width, height := config.Width, config.Height
	if longest := max(width, height); longest > 1600 {
		width, height = max(1, width*1600/longest), max(1, height*1600/longest)
	}
	for {
		pixels := source
		if width != config.Width || height != config.Height {
			scaled := image.NewNRGBA(image.Rect(0, 0, width, height))
			draw.ApproxBiLinear.Scale(scaled, scaled.Bounds(), source, source.Bounds(), draw.Src, nil)
			pixels = scaled
		}
		var output bytes.Buffer
		var contentType string
		if format == "png" {
			encoder := png.Encoder{CompressionLevel: png.BestSpeed}
			err = encoder.Encode(&output, pixels)
			contentType = "image/png"
		} else {
			err = jpeg.Encode(&output, pixels, &jpeg.Options{Quality: 82})
			contentType = "image/jpeg"
		}
		if err != nil {
			return itemPhoto{}, err
		}
		if output.Len() <= maxStoredPhoto {
			return itemPhoto{content: output.Bytes(), mime: contentType}, nil
		}
		width, height = max(1, width*3/4), max(1, height*3/4)
	}
}

// Baseline 4:2:0 JPEG needs fewer samples and has no progressive coefficient buffer.
func usesBaselineCameraSampling(input []byte) bool {
	const (
		markerPrefix  = 0xff
		baselineFrame = 0xc0
		startScan     = 0xda
		endOfImage    = 0xd9
	)
	// DecodeConfig already checked the start-of-image marker. Walk only pre-scan segments.
	for remaining := input[2:]; len(remaining) >= 4 && remaining[0] == markerPrefix; {
		for len(remaining) > 0 && remaining[0] == markerPrefix {
			remaining = remaining[1:]
		}
		if len(remaining) < 3 || remaining[0] == startScan || remaining[0] == endOfImage {
			return false
		}
		marker := remaining[0]
		length := int(binary.BigEndian.Uint16(remaining[1:3]))
		if length < 2 || length >= len(remaining) {
			return false
		}
		if marker == baselineFrame {
			frame := remaining[3 : 1+length]
			return len(frame) == 15 && frame[0] == 8 && frame[5] == 3 &&
				frame[7] == 0x22 && frame[10] == 0x11 && frame[13] == 0x11
		}
		remaining = remaining[1+length:]
	}
	return false
}
