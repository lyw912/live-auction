package gateway

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"live-auction/backend/internal/storage"
)

func TestUploadItemImageStoresViaBackend(t *testing.T) {
	db := openMonitorDB(t)
	rdb := openMonitorRedis(t)
	cfg := testConfig()
	if cfg.MinIOEndpoint == "" {
		cfg.MinIOEndpoint = "localhost:9000"
	}
	if cfg.MinIORootUser == "" {
		cfg.MinIORootUser = "liveauction"
	}
	if cfg.MinIORootPass == "" {
		cfg.MinIORootPass = "liveauction123"
	}
	if cfg.S3Bucket == "" {
		cfg.S3Bucket = "live-auction-items"
	}
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIORootUser, cfg.MinIORootPass, ""),
		Secure: cfg.S3UseSSL,
	})
	if err != nil {
		t.Fatalf("open minio: %v", err)
	}
	deps := &storage.Dependencies{Postgres: db, Redis: rdb, MinIO: minioClient, Bucket: cfg.S3Bucket}
	router := NewRouter(cfg, deps, slog.Default())

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", `form-data; name="file"; filename="upload-test.png"`)
	header.Set("Content-Type", "image/png")
	part, err := writer.CreatePart(header)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Set(0, 0, color.RGBA{R: 47, G: 124, B: 246, A: 255})
	if err := png.Encode(part, img); err != nil {
		t.Fatalf("png encode: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/items/upload", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	for key, values := range userHeaders("host_1", "host") {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("upload status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"public_url"`)) || !bytes.Contains(rec.Body.Bytes(), []byte(`"object_name"`)) {
		t.Fatalf("upload response missing url/object: %s", rec.Body.String())
	}

	bad := httptest.NewRequest(http.MethodPost, "/api/items/upload", bytes.NewBufferString("not multipart"))
	for key, values := range userHeaders("host_1", "host") {
		for _, value := range values {
			bad.Header.Add(key, value)
		}
	}
	rec = httptest.NewRecorder()
	router.ServeHTTP(rec, bad)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad upload status = %d body=%s", rec.Code, rec.Body.String())
	}
}
