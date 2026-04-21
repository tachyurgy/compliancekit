package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type Buckets struct {
	Documents  string
	SignedPDFs string
	AuditTrail string
	RawUploads string
}

type Client struct {
	s3       *s3.Client
	presign  *s3.PresignClient
	buckets  Buckets
	endpoint string // non-empty only for local MinIO/test
}

type Config struct {
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	EndpointURL     string // optional override (MinIO / LocalStack)
	Buckets         Buckets
}

func New(ctx context.Context, cfg Config) (*Client, error) {
	loadOpts := []func(*awsconfig.LoadOptions) error{
		awsconfig.WithRegion(cfg.Region),
	}
	if cfg.AccessKeyID != "" && cfg.SecretAccessKey != "" {
		loadOpts = append(loadOpts, awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(cfg.AccessKeyID, cfg.SecretAccessKey, "")))
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("storage: load aws config: %w", err)
	}

	s3Opts := []func(*s3.Options){}
	if cfg.EndpointURL != "" {
		endpoint := cfg.EndpointURL
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
			o.UsePathStyle = true
		})
	}
	cli := s3.NewFromConfig(awsCfg, s3Opts...)
	return &Client{
		s3:       cli,
		presign:  s3.NewPresignClient(cli),
		buckets:  cfg.Buckets,
		endpoint: cfg.EndpointURL,
	}, nil
}

func (c *Client) Buckets() Buckets { return c.buckets }

// PutDocument uploads a blob to the documents bucket.
func (c *Client) PutDocument(ctx context.Context, key, mimeType string, body io.Reader) error {
	return c.put(ctx, c.buckets.Documents, key, mimeType, body)
}

// GetDocument fetches a blob from the documents bucket. Caller must Close the reader.
func (c *Client) GetDocument(ctx context.Context, key string) (io.ReadCloser, string, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(c.buckets.Documents),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, "", fmt.Errorf("storage: get: %w", err)
	}
	mime := ""
	if out.ContentType != nil {
		mime = *out.ContentType
	}
	return out.Body, mime, nil
}

// PresignPutURL returns a presigned S3 PUT URL a browser can upload directly to.
func (c *Client) PresignPutURL(ctx context.Context, bucket, key, mimeType string, ttl time.Duration) (string, error) {
	if bucket == "" {
		bucket = c.buckets.RawUploads
	}
	req, err := c.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		ContentType: aws.String(mimeType),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("storage: presign put: %w", err)
	}
	return req.URL, nil
}

// PresignGetURL returns a short-lived GET URL for a document.
func (c *Client) PresignGetURL(ctx context.Context, bucket, key string, ttl time.Duration) (string, error) {
	if bucket == "" {
		bucket = c.buckets.Documents
	}
	req, err := c.presign.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(ttl))
	if err != nil {
		return "", fmt.Errorf("storage: presign get: %w", err)
	}
	return req.URL, nil
}

// PutAuditJSON serializes v as JSON and writes it to the audit-trail bucket.
func (c *Client) PutAuditJSON(ctx context.Context, key string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("storage: audit marshal: %w", err)
	}
	return c.put(ctx, c.buckets.AuditTrail, key, "application/json", bytes.NewReader(b))
}

// PutObject puts an arbitrary object to the given bucket/key. Used by the
// data export worker to upload the finished ZIP.
func (c *Client) PutObject(ctx context.Context, bucket, key, mimeType string, body io.Reader) error {
	return c.put(ctx, bucket, key, mimeType, body)
}

// GetObject fetches an arbitrary object. Caller must Close the reader.
func (c *Client) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("storage: get %s/%s: %w", bucket, key, err)
	}
	return out.Body, nil
}

// ListPrefix yields every key under bucket/prefix. Returns keys (not full URIs).
// Callers paginate server-side; we return a fully materialized slice because
// the expected result size (per-tenant exports) is small.
func (c *Client) ListPrefix(ctx context.Context, bucket, prefix string) ([]string, error) {
	var keys []string
	var continuation *string
	for {
		out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuation,
		})
		if err != nil {
			return nil, fmt.Errorf("storage: list %s/%s: %w", bucket, prefix, err)
		}
		for _, o := range out.Contents {
			if o.Key == nil {
				continue
			}
			keys = append(keys, *o.Key)
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return keys, nil
		}
		continuation = out.NextContinuationToken
	}
}

// DeleteAllForProvider removes every object whose key begins with providers/<id>/.
// Used on GDPR delete + subscription churn cleanup.
func (c *Client) DeleteAllForProvider(ctx context.Context, providerID string) error {
	prefix := "providers/" + providerID + "/"
	for _, bucket := range []string{c.buckets.Documents, c.buckets.SignedPDFs, c.buckets.AuditTrail, c.buckets.RawUploads} {
		if err := c.deletePrefix(ctx, bucket, prefix); err != nil {
			return fmt.Errorf("storage: delete prefix %s/%s: %w", bucket, prefix, err)
		}
	}
	return nil
}

func (c *Client) put(ctx context.Context, bucket, key, mimeType string, body io.Reader) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(bucket),
		Key:         aws.String(key),
		Body:        body,
		ContentType: aws.String(mimeType),
	})
	if err != nil {
		return fmt.Errorf("storage: put %s/%s: %w", bucket, key, err)
	}
	return nil
}

func (c *Client) deletePrefix(ctx context.Context, bucket, prefix string) error {
	var continuation *string
	for {
		out, err := c.s3.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuation,
		})
		if err != nil {
			return err
		}
		if len(out.Contents) == 0 {
			return nil
		}
		objs := make([]types.ObjectIdentifier, 0, len(out.Contents))
		for _, o := range out.Contents {
			objs = append(objs, types.ObjectIdentifier{Key: o.Key})
		}
		if _, err := c.s3.DeleteObjects(ctx, &s3.DeleteObjectsInput{
			Bucket: aws.String(bucket),
			Delete: &types.Delete{Objects: objs, Quiet: aws.Bool(true)},
		}); err != nil {
			return err
		}
		if out.IsTruncated == nil || !*out.IsTruncated {
			return nil
		}
		continuation = out.NextContinuationToken
	}
}
