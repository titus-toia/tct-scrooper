package storage

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config holds configuration for S3-compatible storage
type S3Config struct {
	Bucket          string
	Region          string
	Endpoint        string // Optional: for DO Spaces, R2, etc.
	AccessKeyID     string
	SecretAccessKey string
}

// S3Uploader uploads files to S3-compatible storage
type S3Uploader struct {
	client *s3.Client
	bucket string
}

// NewS3Uploader creates a new S3 uploader
func NewS3Uploader(ctx context.Context, cfg S3Config) (*S3Uploader, error) {
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, ""),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}

	var client *s3.Client
	if cfg.Endpoint != "" {
		client = s3.NewFromConfig(awsCfg, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true
		})
	} else {
		client = s3.NewFromConfig(awsCfg)
	}

	return &S3Uploader{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

// Upload uploads data to S3 with the given key
func (u *S3Uploader) Upload(ctx context.Context, key string, data io.Reader, contentType string) error {
	_, err := u.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(u.bucket),
		Key:         aws.String(key),
		Body:        data,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

// PublicURL returns the public URL for an S3 key
func (u *S3Uploader) PublicURL(key string, cfg S3Config) string {
	if cfg.Endpoint != "" && strings.Contains(cfg.Endpoint, "digitaloceanspaces.com") {
		// DO Spaces: https://{bucket}.{region}.digitaloceanspaces.com/{key}
		host := strings.TrimPrefix(cfg.Endpoint, "https://")
		return fmt.Sprintf("https://%s.%s/%s", cfg.Bucket, host, key)
	}
	// AWS S3: https://{bucket}.s3.{region}.amazonaws.com/{key}
	return fmt.Sprintf("https://%s.s3.%s.amazonaws.com/%s", cfg.Bucket, cfg.Region, key)
}
