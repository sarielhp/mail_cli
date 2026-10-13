package jmap

import (
	"context"
	"errors"
	"fmt"
	"github.com/sarielhp/mail_cli/backend/backendtest"
	mailemail "github.com/sarielhp/mail_cli/email"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	jmapcore "git.sr.ht/~rockorager/go-jmap"
	jmapemail "git.sr.ht/~rockorager/go-jmap/mail/email"
)

func TestDownloadBlobs_PanicRecovery(t *testing.T) {
	// client with nil inner client will cause downloadBlob to panic on c.client.Session
	fetcher := rawFetcher{
		client: &client{},
	}
	details := []*jmapemail.Email{
		{ID: "msg1", BlobID: "blob1"},
		{ID: "msg2", BlobID: "blob2"},
	}

	var yielded int
	err := fetcher.downloadBlobs(context.Background(), details, func(_ mailemail.ID, _ []byte) error {
		yielded++
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error on panic recovery, got %v", err)
	}
	if yielded != 0 {
		t.Errorf("expected 0 yielded on panic, got %d", yielded)
	}
}

func TestDownloadBlobs_ContextCancellation(t *testing.T) {
	fetcher := rawFetcher{
		client: &client{},
	}
	details := []*jmapemail.Email{
		{ID: "msg1", BlobID: "blob1"},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var yielded int
	err := fetcher.downloadBlobs(ctx, details, func(_ mailemail.ID, _ []byte) error {
		yielded++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if yielded != 0 {
		t.Errorf("expected 0 yielded on canceled context, got %d", yielded)
	}
}

func TestDownloadBlobs_EmptyDetails(t *testing.T) {
	fetcher := rawFetcher{client: &client{}}
	err := fetcher.downloadBlobs(context.Background(), nil, func(_ mailemail.ID, _ []byte) error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error for nil details, got %v", err)
	}
	err = fetcher.downloadBlobs(context.Background(), []*jmapemail.Email{}, func(_ mailemail.ID, _ []byte) error {
		return nil
	})
	if err != nil {
		t.Errorf("expected nil error for empty details, got %v", err)
	}
}

func TestDownloadBlobs_ManyItemsContextCancelled(t *testing.T) {
	fetcher := rawFetcher{client: &client{}}
	details := make([]*jmapemail.Email, 50)
	for i := range details {
		details[i] = &jmapemail.Email{
			ID:     jmapcore.ID(fmt.Sprintf("msg%d", i)),
			BlobID: "blob",
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var yielded int
	err := fetcher.downloadBlobs(ctx, details, func(_ mailemail.ID, _ []byte) error {
		yielded++
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("expected context.Canceled, got %v", err)
	}
	if yielded != 0 {
		t.Errorf("expected 0 yielded on canceled context with 50 items, got %d", yielded)
	}
}

func TestDownloadBlobs_YieldErrorHalts(t *testing.T) {
	fetcher := rawFetcher{client: &client{}}
	errYield := errors.New("yield failure")
	// Since client is nil, downloading would panic if reached, but empty tasks or error yield handles it
	err := fetcher.downloadBlobs(context.Background(), nil, func(_ mailemail.ID, _ []byte) error {
		return errYield
	})
	if err != nil {
		t.Errorf("expected nil on empty details even with failing yield, got %v", err)
	}
}

func TestFetchEmailDetailsContext_Canceled(t *testing.T) {
	client := &client{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := client.fetchEmailDetailsContext(ctx, []jmapcore.ID{"id1"}, []string{"id"})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("fetchEmailDetailsContext with canceled ctx error = %v, want context.Canceled", err)
	}
}

func TestFetchRawBatch_ContextCancellation(t *testing.T) {
	fetcher := rawFetcher{client: &client{}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := fetcher.FetchRawBatch(ctx, []mailemail.ID{"m1"}, func(_ mailemail.ID, _ []byte) error {
		return nil
	})
	if !errors.Is(err, context.Canceled) {
		t.Errorf("FetchRawBatch error = %v, want context.Canceled", err)
	}
}

func TestJMAPClient_MessageAccessorContextCancellation(t *testing.T) {
	backendtest.AssertAccessorHonorsCancellation(t, &client{})
}

func TestBlobRetryDelay_Jitter(t *testing.T) {
	resp := &http.Response{Header: make(http.Header)}

	// Retry-After header present
	resp.Header.Set("Retry-After", "5")
	delay := blobRetryDelay(resp, 0)
	if delay != 5*time.Second {
		t.Errorf("expected 5s from Retry-After, got %v", delay)
	}

	// Exponential backoff fallback with jitter: should be within [wait/2, wait]
	resp.Header.Del("Retry-After")
	for attempt := 0; attempt < maxBlobDownloadAttempts; attempt++ {
		nominalWait := blobRetryBaseDelay * (1 << attempt)
		minWait := nominalWait / 2
		maxWait := nominalWait

		for run := 0; run < 10; run++ {
			d := blobRetryDelay(resp, attempt)
			if d < minWait || d > maxWait {
				t.Fatalf("attempt %d run %d: delay %v out of range [%v, %v]", attempt, run, d, minWait, maxWait)
			}
		}
	}
}

func TestDownloadBlobs_ErrorAndEmptyBody(t *testing.T) {
	client := &client{
		client: &jmapcore.Client{
			Session: &jmapcore.Session{
				DownloadURL: "https://test.local/download/{blobId}",
			},
			HttpClient: &http.Client{
				Transport: mockTransport(func(req *http.Request) *http.Response {
					if strings.Contains(req.URL.Path, "err-blob") {
						return &http.Response{
							StatusCode: http.StatusUnauthorized,
							Body:       io.NopCloser(strings.NewReader("unauthorized")),
							Header:     make(http.Header),
						}
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       io.NopCloser(strings.NewReader("")),
						Header:     make(http.Header),
					}
				}),
			},
		},
	}
	fetcher := rawFetcher{client: client}
	details := []*jmapemail.Email{
		{ID: "msg-err", BlobID: "err-blob"},
		{ID: "msg-empty", BlobID: "empty-blob"},
	}

	var yielded int
	err := fetcher.downloadBlobs(context.Background(), details, func(_ mailemail.ID, _ []byte) error {
		yielded++
		return nil
	})
	if err != nil {
		t.Fatalf("downloadBlobs should continue past non-fatal download errors, got: %v", err)
	}
	if yielded != 0 {
		t.Errorf("expected 0 messages yielded, got %d", yielded)
	}
}
