package main

import (
	"context"
	"log"

	"live-auction/backend/internal/config"
	"live-auction/backend/internal/storage"

	"github.com/minio/minio-go/v7"
)

func main() {
	ctx := context.Background()
	cfg := config.Load()
	deps, err := storage.Open(ctx, cfg, nil)
	if err != nil {
		log.Fatalf("open object store: %v", err)
	}
	defer deps.Close()
	exists, err := deps.MinIO.BucketExists(ctx, deps.Bucket)
	if err != nil {
		log.Fatalf("check bucket: %v", err)
	}
	if !exists {
		if err := deps.MinIO.MakeBucket(ctx, deps.Bucket, minio.MakeBucketOptions{}); err != nil {
			log.Fatalf("make bucket: %v", err)
		}
	}
	log.Printf("bucket ready: %s", cfg.S3Bucket)
}
