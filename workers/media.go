package workers

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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
	triggerCh  chan struct{}
	logFunc    LogFunc
}

func (w *MediaWorker) SetLogger(fn LogFunc) {
	w.logFunc = fn
}

// S3Uploader interface for uploading to S3-compatible storage
type S3Uploader interface {
	Upload(ctx context.Context, key string, data io.Reader, contentType string) error
}

// NewMediaWorker creates a new media worker
func NewMediaWorker(store *storage.PostgresStore, uploader S3Uploader, proxyURL string) *MediaWorker {
	transport := http.DefaultTransport.(*http.Transport).Clone()

	if proxyURL != "" {
		if proxyParsed, err := url.Parse(proxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyParsed)
			log.Printf("Media worker using proxy: %s", proxyParsed.Host)
		}
	}

	client := &http.Client{
		Timeout:   60 * time.Second,
		Transport: transport,
	}

	return &MediaWorker{
		store:      store,
		httpClient: client,
		uploader:   uploader,
		proxyURL:   proxyURL,
		triggerCh:  make(chan struct{}, 1),
		logFunc:    NoOpLogger,
	}
}

// Trigger causes the worker to run immediately
func (w *MediaWorker) Trigger() {
	select {
	case w.triggerCh <- struct{}{}:
	default:
	}
}

// ProcessResult contains the outcome of processing a media item
type MediaProcessResult struct {
	MediaID     uuid.UUID
	S3Key       string
	ContentHash string
	Size        int64
	IsDuplicate bool // true if content already exists in S3 (different URL, same image)
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

	// Generate S3 key based on category
	ext := guessExtension(media.OriginalURL, resp.Header.Get("Content-Type"))
	result.S3Key = generateS3Key(media, result.ContentHash, ext)

	// Check if this s3_key already exists (same content from different URL)
	existingID, err := w.store.GetMediaByS3Key(ctx, result.S3Key)
	if err != nil {
		result.Error = fmt.Errorf("check existing: %w", err)
		return result
	}
	if existingID != uuid.Nil {
		// Content already in S3 from another URL, skip upload
		log.Printf("Media worker: %s already exists (same content as %s), skipping upload", result.S3Key, existingID)
		result.IsDuplicate = true
		return result
	}

	// Upload to S3
	if w.uploader != nil {
		contentType := resp.Header.Get("Content-Type")
		if contentType == "" {
			contentType = "image/jpeg"
		}

		// bytes.NewReader implements io.ReadSeeker so AWS SDK can determine content length
		if err := w.uploader.Upload(ctx, result.S3Key, bytes.NewReader(data), contentType); err != nil {
			result.Error = fmt.Errorf("upload: %w", err)
			return result
		}
	}

	return result
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

// generateS3Key creates the S3 key based on media category and location
// Schema:
//   - listings/{province}/{city}/{hash}.{ext}                - property photos, floor plans
//   - properties/{province}/{city}/{hash}.{ext}              - generic property docs
//   - properties/{province}/{city}/records/{hash}.{ext}      - permits, inspections
//   - properties/{province}/{city}/assessments/{hash}.{ext}  - tax notices, appeals
//   - properties/{province}/{city}/intel/{hash}.{ext}        - screenshots, evidence
//   - agents/{hash}.{ext}                                    - agent headshots
//   - brokerages/{hash}.{ext}                                - brokerage logos
func generateS3Key(media *models.Media, contentHash, ext string) string {
	province := safeProvince(media.Province)
	city := safeCity(media.City)

	switch media.Category {
	case models.MediaCategoryListing:
		return fmt.Sprintf("listings/%s/%s/%s%s", province, city, contentHash, ext)

	case models.MediaCategoryProperty:
		return fmt.Sprintf("properties/%s/%s/%s%s", province, city, contentHash, ext)

	case models.MediaCategoryRecord:
		return fmt.Sprintf("properties/%s/%s/records/%s%s", province, city, contentHash, ext)

	case models.MediaCategoryAssessment:
		return fmt.Sprintf("properties/%s/%s/assessments/%s%s", province, city, contentHash, ext)

	case models.MediaCategoryIntel:
		return fmt.Sprintf("properties/%s/%s/intel/%s%s", province, city, contentHash, ext)

	case models.MediaCategoryAgent:
		return fmt.Sprintf("agents/%s%s", contentHash, ext)

	case models.MediaCategoryBrokerage:
		return fmt.Sprintf("brokerages/%s%s", contentHash, ext)

	default:
		return fmt.Sprintf("media/%s/%s%s", contentHash[:2], contentHash, ext)
	}
}

func safeProvince(p string) string {
	if p == "" {
		return "unknown"
	}
	return sanitizePath(p)
}

func safeCity(c string) string {
	if c == "" {
		return "unknown"
	}
	return sanitizePath(c)
}

// sanitizePath makes a string safe for use in S3 paths
func sanitizePath(s string) string {
	// Replace spaces and special chars with underscores, lowercase
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "'", "")
	s = strings.ReplaceAll(s, ".", "")
	return s
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
		case <-w.triggerCh:
			log.Println("Media worker triggered manually")
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

			newAttempts := m.Attempts + 1
			status := models.MediaStatusPending
			if newAttempts >= 3 {
				status = models.MediaStatusFailed
			}
			w.store.UpdateMediaStatus(ctx, m.ID, status, nil, "", newAttempts)
			continue
		}

		if result.IsDuplicate {
			// Content already in S3 from another URL - copy the s3_key so consumers can find it
			if err := w.store.UpdateMediaStatus(ctx, m.ID, "duplicate", &result.S3Key, result.ContentHash, m.Attempts); err != nil {
				log.Printf("Media worker: failed to mark duplicate %s: %v", m.ID, err)
			}
			processed++
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
		msg := fmt.Sprintf("Batch done: %d uploaded", processed)
		if failed > 0 {
			msg += fmt.Sprintf(", %d failed", failed)
		}
		w.logFunc(models.LogLevelInfo, "media", msg)
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
