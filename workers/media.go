package workers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/google/uuid"
	"tct_scrooper/models"
	"tct_scrooper/storage"
)

// MediaWorker downloads media files, hashes them, and uploads to S3
type MediaWorker struct {
	store      *storage.PostgresStore
	httpClient *http.Client
	uploader   S3Uploader
	proxyURL   string
}

// S3Uploader interface for uploading to S3-compatible storage
type S3Uploader interface {
	Upload(ctx context.Context, key string, data io.Reader, contentType string) error
}

// NewMediaWorker creates a new media worker
func NewMediaWorker(store *storage.PostgresStore, uploader S3Uploader, proxyURL string) *MediaWorker {
	client := &http.Client{
		Timeout: 60 * time.Second, // Longer timeout for media downloads
	}

	// TODO: configure proxy transport if proxyURL is set

	return &MediaWorker{
		store:      store,
		httpClient: client,
		uploader:   uploader,
		proxyURL:   proxyURL,
	}
}

// ProcessResult contains the outcome of processing a media item
type MediaProcessResult struct {
	MediaID     uuid.UUID
	S3Key       string
	ContentHash string
	Size        int64
	Error       error
}

// Process downloads a media file, computes hash, and uploads to S3
func (w *MediaWorker) Process(ctx context.Context, media *models.Media) MediaProcessResult {
	result := MediaProcessResult{MediaID: media.ID}

	// Download the file
	req, err := http.NewRequestWithContext(ctx, "GET", media.OriginalURL, nil)
	if err != nil {
		result.Error = fmt.Errorf("create request: %w", err)
		return result
	}

	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")
	req.Header.Set("Accept", "image/*,*/*")

	resp, err := w.httpClient.Do(req)
	if err != nil {
		result.Error = fmt.Errorf("download: %w", err)
		return result
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		result.Error = fmt.Errorf("download status: %d", resp.StatusCode)
		return result
	}

	// Read into memory (for hashing and upload)
	// For large files, you'd want to stream to disk first
	data, err := io.ReadAll(io.LimitReader(resp.Body, 50*1024*1024)) // 50MB limit
	if err != nil {
		result.Error = fmt.Errorf("read body: %w", err)
		return result
	}

	result.Size = int64(len(data))

	// Compute SHA256 hash
	hash := sha256.Sum256(data)
	result.ContentHash = hex.EncodeToString(hash[:])

	// Generate S3 key: media/{hash_prefix}/{hash}.{ext}
	ext := guessExtension(media.OriginalURL, resp.Header.Get("Content-Type"))
	result.S3Key = fmt.Sprintf("media/%s/%s%s", result.ContentHash[:2], result.ContentHash, ext)

	// Upload to S3
	if w.uploader != nil {
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/jpeg"
		}

		reader := &bytesReader{data: data}
		if err := w.uploader.Upload(ctx, result.S3Key, reader, contentType); err != nil {
			result.Error = fmt.Errorf("upload: %w", err)
			return result
		}
	}

	return result
}

// bytesReader wraps []byte to implement io.Reader
type bytesReader struct {
	data []byte
	pos  int
}

func (r *bytesReader) Read(p []byte) (n int, err error) {
	if r.pos >= len(r.data) {
		return 0, io.EOF
	}
	n = copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}

// guessExtension determines file extension from URL or content-type
func guessExtension(url, contentType string) string {
	// Try URL first
	ext := strings.ToLower(path.Ext(url))
	if ext != "" && isImageExt(ext) {
		return ext
	}

	// Fall back to content-type
	switch contentType {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	default:
		return ".jpg" // Default
	}
}

func isImageExt(ext string) bool {
	switch ext {
	case ".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".tiff":
		return true
	}
	return false
}

// Run starts the media worker loop
func (w *MediaWorker) Run(ctx context.Context, batchSize int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Println("Media worker stopping")
			return
		case <-ticker.C:
			w.processBatch(ctx, batchSize)
		}
	}
}

func (w *MediaWorker) processBatch(ctx context.Context, batchSize int) {
	media, err := w.store.GetPendingMedia(ctx, batchSize)
	if err != nil {
		log.Printf("Media worker: query error: %v", err)
		return
	}

	if len(media) == 0 {
		return
	}

	log.Printf("Media worker: processing %d items", len(media))

	var processed, failed int
	for i := range media {
		m := &media[i]

		result := w.Process(ctx, m)

		if result.Error != nil {
			log.Printf("Media worker: failed %s: %v", m.OriginalURL, result.Error)
			failed++

			// Increment attempts, mark as failed if too many
			newAttempts := m.Attempts + 1
			status := models.MediaStatusPending
			if newAttempts >= 3 {
				status = models.MediaStatusFailed
			}
			w.store.UpdateMediaStatus(ctx, m.ID, status, nil, "", newAttempts)
			continue
		}

		// Update media record with S3 key and hash
		if err := w.store.UpdateMediaStatus(ctx, m.ID, models.MediaStatusUploaded, &result.S3Key, result.ContentHash, m.Attempts); err != nil {
			log.Printf("Media worker: failed to update %s: %v", m.ID, err)
			failed++
			continue
		}

		processed++
		log.Printf("Media worker: uploaded %s -> %s (%d bytes)", m.ID, result.S3Key, result.Size)

		// Rate limit between downloads
		time.Sleep(200 * time.Millisecond)
	}

	if processed > 0 || failed > 0 {
		log.Printf("Media worker: processed %d, failed %d", processed, failed)
	}
}

// NoOpUploader is a placeholder that skips actual S3 upload
type NoOpUploader struct{}

func (u *NoOpUploader) Upload(ctx context.Context, key string, data io.Reader, contentType string) error {
	// Just drain the reader
	io.Copy(io.Discard, data)
	return nil
}

// NewNoOpUploader creates an uploader that does nothing (for testing)
func NewNoOpUploader() *NoOpUploader {
	return &NoOpUploader{}
}
