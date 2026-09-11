package audio

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

const (
	r2Region         = "auto"
	r2RequestTimeout = 30 * time.Second
)

// ErrCacheMiss is returned only when R2 definitively reports that an object
// does not exist. Callers must not treat other storage failures as misses.
var ErrCacheMiss = errors.New("audio cache miss")

// R2Config contains the private bucket connection details used by the cache.
type R2Config struct {
	Endpoint        string
	Bucket          string
	AccessKeyID     string
	SecretAccessKey string
}

type s3API interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObject(context.Context, *s3.PutObjectInput, ...func(*s3.Options)) (*s3.PutObjectOutput, error)
}

// R2Cache stores complete, sentence-sized MP3 clips in a private R2 bucket.
type R2Cache struct {
	client         s3API
	bucket         string
	maxAudioBytes  int64
	requestTimeout time.Duration
}

// NewR2Cache constructs an R2 cache using Cloudflare's S3-compatible API.
func NewR2Cache(cfg R2Config) (*R2Cache, error) {
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, fmt.Errorf("r2 endpoint is required")
	}
	endpoint, err := url.Parse(cfg.Endpoint)
	if err != nil || endpoint.Scheme == "" || endpoint.Host == "" || (endpoint.Scheme != "https" && endpoint.Scheme != "http") {
		return nil, fmt.Errorf("r2 endpoint must be an absolute HTTP(S) URL")
	}
	if strings.TrimSpace(cfg.Bucket) == "" {
		return nil, fmt.Errorf("r2 bucket is required")
	}
	if strings.TrimSpace(cfg.AccessKeyID) == "" {
		return nil, fmt.Errorf("r2 access key id is required")
	}
	if strings.TrimSpace(cfg.SecretAccessKey) == "" {
		return nil, fmt.Errorf("r2 secret access key is required")
	}

	awsCfg := aws.Config{
		Region: r2Region,
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretAccessKey,
			"",
		),
	}

	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		options.BaseEndpoint = aws.String(endpoint.String())
		options.UsePathStyle = true
	})
	return &R2Cache{
		client:         client,
		bucket:         cfg.Bucket,
		maxAudioBytes:  maxAudioBytes,
		requestTimeout: r2RequestTimeout,
	}, nil
}

// Get returns one complete cached MP3 while enforcing the same size bound as
// synthesis. Only a provider not-found response is translated to ErrCacheMiss.
func (c *R2Cache) Get(ctx context.Context, key string) ([]byte, error) {
	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	out, err := c.client.GetObject(requestCtx, &s3.GetObjectInput{
		Bucket: aws.String(c.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		if isObjectNotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrCacheMiss, key)
		}
		return nil, fmt.Errorf("get audio from r2: %w", err)
	}
	defer out.Body.Close()

	if out.ContentLength != nil && *out.ContentLength > c.maxAudioBytes {
		return nil, fmt.Errorf("r2 object exceeds %d bytes", c.maxAudioBytes)
	}
	audioBytes, err := io.ReadAll(io.LimitReader(out.Body, c.maxAudioBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read audio from r2: %w", err)
	}
	if int64(len(audioBytes)) > c.maxAudioBytes {
		return nil, fmt.Errorf("r2 object exceeds %d bytes", c.maxAudioBytes)
	}
	return audioBytes, nil
}

// Put uploads one complete MP3. Bucket privacy is managed by R2; no public ACL
// or URL is created by this adapter.
func (c *R2Cache) Put(ctx context.Context, key string, audioBytes []byte) error {
	if int64(len(audioBytes)) > c.maxAudioBytes {
		return fmt.Errorf("audio exceeds %d bytes", c.maxAudioBytes)
	}

	requestCtx, cancel := context.WithTimeout(ctx, c.requestTimeout)
	defer cancel()

	_, err := c.client.PutObject(requestCtx, &s3.PutObjectInput{
		Bucket:        aws.String(c.bucket),
		Key:           aws.String(key),
		Body:          bytes.NewReader(audioBytes),
		ContentLength: aws.Int64(int64(len(audioBytes))),
		ContentType:   aws.String("audio/mpeg"),
	})
	if err != nil {
		return fmt.Errorf("put audio in r2: %w", err)
	}
	return nil
}

func isObjectNotFound(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	return apiErr.ErrorCode() == "NoSuchKey" || apiErr.ErrorCode() == "NotFound"
}
