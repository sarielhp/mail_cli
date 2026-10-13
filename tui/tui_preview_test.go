package tui

import (
	"strings"
	"testing"

	"github.com/sarielhp/mail_cli/email"
)

func TestGraphicCacheKey(t *testing.T) {
	data1 := []byte("image_data_one")
	data2 := []byte("image_data_two")

	k1 := getCacheKey(data1, 80, 24, false)
	k2 := getCacheKey(data1, 80, 24, false)
	kDiffData := getCacheKey(data2, 80, 24, false)
	kDiffDim := getCacheKey(data1, 100, 24, false)
	kDiffInvert := getCacheKey(data1, 80, 24, true)

	if k1 != k2 {
		t.Fatalf("expected identical cache keys for identical input, got %q vs %q", k1, k2)
	}
	if k1 == kDiffData {
		t.Errorf("expected different keys for different data")
	}
	if k1 == kDiffDim {
		t.Errorf("expected different keys for different dimensions")
	}
	if k1 == kDiffInvert {
		t.Errorf("expected different keys for different invert flag")
	}
}

func TestIsImageAttachment(t *testing.T) {
	cases := []struct {
		filename    string
		contentType string
		data        []byte
		want        bool
	}{
		{"photo.png", "image/png", nil, true},
		{"photo.jpg", "", nil, true},
		{"photo.jpeg", "", nil, true},
		{"anim.gif", "image/gif", nil, true},
		{"vector.svg", "", nil, true},
		{"unknown.bin", "", []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}, true},
		{"unknown.bin", "", []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 0x4a, 0x46}, true},
		{"doc.pdf", "application/pdf", []byte("%PDF-1.4"), false},
		{"notes.txt", "text/plain", []byte("hello world"), false},
	}

	for _, tc := range cases {
		att := email.Attachment{
			Filename:    tc.filename,
			ContentType: tc.contentType,
			Data:        tc.data,
		}
		if got := isImageAttachment(att); got != tc.want {
			t.Errorf("isImageAttachment(%q, %q) = %v, want %v", tc.filename, tc.contentType, got, tc.want)
		}
	}
}

func TestRenderGraphicAttachmentPreview_EmptyData(t *testing.T) {
	att := email.Attachment{Filename: "img.png", Data: nil}
	_, err := renderGraphicAttachmentPreview(att, 40, 20, false)
	if err == nil {
		t.Fatalf("expected error for empty attachment data, got nil")
	}
	if !strings.Contains(err.Error(), "empty attachment data") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestRenderGraphicAttachmentPreview_Cached(t *testing.T) {
	att := email.Attachment{Filename: "img.png", Data: []byte("sample_image_bytes")}
	key := getCacheKey(att.Data, 40, 20, false)

	graphicCacheLock.Lock()
	graphicCache[key] = "cached_preview_output"
	graphicCacheLock.Unlock()

	defer func() {
		graphicCacheLock.Lock()
		delete(graphicCache, key)
		graphicCacheLock.Unlock()
	}()

	res, err := renderGraphicAttachmentPreview(att, 40, 20, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != "cached_preview_output" {
		t.Errorf("got %q, want 'cached_preview_output'", res)
	}
}
