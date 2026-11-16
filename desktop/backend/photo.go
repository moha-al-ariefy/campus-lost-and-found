package backend

import (
	"bytes"
	"crypto/sha256"
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

	"golang.org/x/image/draw"
)

const (
	maxPhotoInput  = 10_000_000
	maxStoredPhoto = 1_000_000
	maxIntakeJSON  = 32_768
	maxPhotoPixels = 32_000_000
)

type itemPhoto struct {
	content []byte
	mime    string
}

func readIntake(w http.ResponseWriter, r *http.Request, data *Intake) ([]byte, bool) {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/") {
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

func intakeFingerprint(data Intake, photo []byte) string {
	if photo == nil {
		return hash(encode(data))
	}
	digest := sha256.Sum256(photo)
	return hash(encode(data) + ":" + hex.EncodeToString(digest[:]))
}

func reducePhoto(input []byte) (itemPhoto, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(input))
	if err != nil || (format != "jpeg" && format != "png") || config.Width < 1 ||
		config.Height < 1 || config.Width > maxPhotoPixels/config.Height {
		return itemPhoto{}, errors.New("unsupported photo or dimensions")
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
