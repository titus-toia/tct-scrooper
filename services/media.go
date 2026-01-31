package services

import (
	"context"
	"time"

	"github.com/google/uuid"
	"tct_scrooper/models"
	"tct_scrooper/storage"
)

// MediaService handles media queueing and retrieval
type MediaService struct {
	store *storage.PostgresStore
}

// NewMediaService creates a new MediaService
func NewMediaService(store *storage.PostgresStore) *MediaService {
	return &MediaService{store: store}
}

// EnqueueParams contains parameters for enqueueing media
type EnqueueParams struct {
	OriginalURL string
	MediaType   string // image, video, document
	Category    string // listing, agent, brokerage
	Province    string // for listing media S3 paths
	City        string // for listing media S3 paths
}

// Enqueue creates a media row with original_url and status=pending.
// Returns the media ID (existing or new).
func (s *MediaService) Enqueue(ctx context.Context, params EnqueueParams) (uuid.UUID, error) {
	// Check if media already exists
	existing, err := s.store.GetMediaByOriginalURL(ctx, params.OriginalURL)
	if err != nil {
		return uuid.Nil, err
	}
	if existing != nil {
		return existing.ID, nil
	}

	// Create new media entry
	media := &models.Media{
		ID:          uuid.New(),
		OriginalURL: params.OriginalURL,
		MediaType:   params.MediaType,
		Category:    params.Category,
		Province:    params.Province,
		City:        params.City,
		Status:      models.MediaStatusPending,
		Attempts:    0,
		CreatedAt:   time.Now(),
	}

	if err := s.store.UpsertMedia(ctx, media); err != nil {
		return uuid.Nil, err
	}

	return media.ID, nil
}

// GetPending returns pending media items for the worker to process
func (s *MediaService) GetPending(ctx context.Context, limit int) ([]models.Media, error) {
	return s.store.GetPendingMedia(ctx, limit)
}

// MarkUploaded marks a media item as successfully uploaded
func (s *MediaService) MarkUploaded(ctx context.Context, id uuid.UUID, s3Key string, contentHash string) error {
	return s.store.UpdateMediaStatus(ctx, id, models.MediaStatusUploaded, &s3Key, contentHash, 0)
}

// MarkFailed marks a media item as failed (increments attempts)
func (s *MediaService) MarkFailed(ctx context.Context, id uuid.UUID, attempts int) error {
	status := models.MediaStatusPending
	if attempts >= 3 {
		status = models.MediaStatusFailed
	}
	return s.store.UpdateMediaStatus(ctx, id, status, nil, "", attempts)
}

// GetQueueDepth returns the count of pending media items by status
func (s *MediaService) GetQueueDepth(ctx context.Context) (map[string]int, error) {
	query := `
		SELECT status, COUNT(*) as count
		FROM media
		GROUP BY status`

	rows, err := s.store.Pool().Query(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, err
		}
		counts[status] = count
	}

	return counts, rows.Err()
}
