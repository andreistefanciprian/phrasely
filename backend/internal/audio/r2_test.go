package audio

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/smithy-go"
)

type fakeS3 struct {
	getOutput *s3.GetObjectOutput
	getErr    error
	getFunc   func(context.Context) (*s3.GetObjectOutput, error)
	putInput  *s3.PutObjectInput
	putErr    error
	putFunc   func(context.Context) (*s3.PutObjectOutput, error)
}

func (f *fakeS3) GetObject(ctx context.Context, _ *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if f.getFunc != nil {
		return f.getFunc(ctx)
	}
	return f.getOutput, f.getErr
}

func (f *fakeS3) PutObject(ctx context.Context, input *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	f.putInput = input
	if f.putFunc != nil {
		return f.putFunc(ctx)
	}
	return &s3.PutObjectOutput{}, f.putErr
}

func TestR2CacheGetReturnsBufferedAudio(t *testing.T) {
	client := &fakeS3{getOutput: &s3.GetObjectOutput{
		Body:          io.NopCloser(strings.NewReader("mp3 bytes")),
		ContentLength: ptr(int64(len("mp3 bytes"))),
	}}
	cache := testR2Cache(client)

	got, err := cache.Get(context.Background(), "phrase-audio/user/hash.mp3")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(got) != "mp3 bytes" {
		t.Fatalf("audio = %q", got)
	}
}

func TestR2CacheGetDistinguishesMissingObject(t *testing.T) {
	cache := testR2Cache(&fakeS3{getErr: &smithy.GenericAPIError{Code: "NoSuchKey", Message: "missing"}})

	_, err := cache.Get(context.Background(), "missing.mp3")
	if !errors.Is(err, ErrCacheMiss) {
		t.Fatalf("error = %v, want ErrCacheMiss", err)
	}
}

func TestR2CacheGetDoesNotTreatStorageFailureAsMiss(t *testing.T) {
	cache := testR2Cache(&fakeS3{getErr: &smithy.GenericAPIError{Code: "InternalError", Message: "unavailable"}})

	_, err := cache.Get(context.Background(), "audio.mp3")
	if err == nil || errors.Is(err, ErrCacheMiss) {
		t.Fatalf("error = %v, want non-miss storage error", err)
	}
}

func TestR2CacheGetRejectsOversizedObject(t *testing.T) {
	tests := []struct {
		name   string
		output *s3.GetObjectOutput
	}{
		{
			name: "declared length",
			output: &s3.GetObjectOutput{
				Body:          io.NopCloser(strings.NewReader("small")),
				ContentLength: ptr(int64(5)),
			},
		},
		{
			name: "streamed length",
			output: &s3.GetObjectOutput{
				Body: io.NopCloser(strings.NewReader("12345")),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := testR2Cache(&fakeS3{getOutput: tt.output})
			cache.maxAudioBytes = 4
			if _, err := cache.Get(context.Background(), "large.mp3"); err == nil {
				t.Fatal("Get succeeded")
			}
		})
	}
}

func TestR2CachePutSendsPrivateMP3Object(t *testing.T) {
	client := &fakeS3{}
	cache := testR2Cache(client)

	if err := cache.Put(context.Background(), "phrase-audio/user/hash.mp3", []byte("mp3")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	input := client.putInput
	if input == nil || *input.Bucket != "private-audio" || *input.Key != "phrase-audio/user/hash.mp3" {
		t.Fatalf("put input = %+v", input)
	}
	if input.ContentType == nil || *input.ContentType != "audio/mpeg" {
		t.Fatalf("content type = %v", input.ContentType)
	}
	got, err := io.ReadAll(input.Body)
	if err != nil || string(got) != "mp3" {
		t.Fatalf("body = %q, err = %v", got, err)
	}
}

func TestR2CachePutRejectsOversizedAudioWithoutUpload(t *testing.T) {
	client := &fakeS3{}
	cache := testR2Cache(client)
	cache.maxAudioBytes = 3

	if err := cache.Put(context.Background(), "large.mp3", []byte("1234")); err == nil {
		t.Fatal("Put succeeded")
	}
	if client.putInput != nil {
		t.Fatal("PutObject was called")
	}
}

func TestR2CacheOperationsTimeOut(t *testing.T) {
	waitForCancellation := func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}
	tests := []struct {
		name string
		run  func(*R2Cache) error
	}{
		{
			name: "get",
			run: func(cache *R2Cache) error {
				_, err := cache.Get(context.Background(), "audio.mp3")
				return err
			},
		},
		{
			name: "put",
			run: func(cache *R2Cache) error {
				return cache.Put(context.Background(), "audio.mp3", []byte("mp3"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &fakeS3{
				getFunc: func(ctx context.Context) (*s3.GetObjectOutput, error) {
					return nil, waitForCancellation(ctx)
				},
				putFunc: func(ctx context.Context) (*s3.PutObjectOutput, error) {
					return nil, waitForCancellation(ctx)
				},
			}
			cache := testR2Cache(client)
			cache.requestTimeout = time.Millisecond

			if err := tt.run(cache); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("error = %v, want context.DeadlineExceeded", err)
			}
		})
	}
}

func TestNewR2CacheValidatesConfiguration(t *testing.T) {
	valid := R2Config{
		Endpoint:        "https://account.r2.cloudflarestorage.com",
		Bucket:          "audio",
		AccessKeyID:     "key",
		SecretAccessKey: "secret",
	}
	tests := []struct {
		name   string
		mutate func(*R2Config)
	}{
		{name: "endpoint", mutate: func(c *R2Config) { c.Endpoint = "" }},
		{name: "invalid endpoint", mutate: func(c *R2Config) { c.Endpoint = "://bad" }},
		{name: "bucket", mutate: func(c *R2Config) { c.Bucket = "" }},
		{name: "access key", mutate: func(c *R2Config) { c.AccessKeyID = "" }},
		{name: "secret key", mutate: func(c *R2Config) { c.SecretAccessKey = "" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)
			if _, err := NewR2Cache(cfg); err == nil {
				t.Fatalf("NewR2Cache(%+v) succeeded", cfg)
			}
		})
	}
}

func testR2Cache(client s3API) *R2Cache {
	return &R2Cache{
		client:         client,
		bucket:         "private-audio",
		maxAudioBytes:  maxAudioBytes,
		requestTimeout: r2RequestTimeout,
	}
}

func ptr[T any](value T) *T { return &value }
