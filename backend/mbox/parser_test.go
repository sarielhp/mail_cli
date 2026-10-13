package mbox

import (
	"bytes"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
)

func TestParseMboxStream_DuplicateMessageIDs(t *testing.T) {
	cacheDir := t.TempDir()

	mboxContent := `From sender@example.com Fri Sep 18 10:00:00 2026
From: sender@example.com
To: recipient@example.com
Subject: Alert 1
Message-ID: <duplicate-alert@example.com>
Date: Fri, 18 Sep 2026 10:00:00 +0000

First duplicate message content

From sender@example.com Fri Sep 18 10:01:00 2026
From: sender@example.com
To: recipient@example.com
Subject: Alert 2
Message-ID: <duplicate-alert@example.com>
Date: Fri, 18 Sep 2026 10:01:00 +0000

Second duplicate message content

From sender@example.com Fri Sep 18 10:02:00 2026
From: sender@example.com
To: recipient@example.com
Subject: Alert 3
Message-ID: <duplicate-alert@example.com>
Date: Fri, 18 Sep 2026 10:02:00 +0000

Third duplicate message content
`

	ids, err := parseMboxStream(strings.NewReader(mboxContent), cachedir.At(cacheDir), time.Now())
	if err != nil {
		t.Fatalf("parseMboxStream failed: %v", err)
	}

	if len(ids) != 3 {
		t.Fatalf("expected 3 IDs, got %d", len(ids))
	}

	expectedIDs := []string{
		"duplicate-alert@example.com",
		"duplicate-alert@example.com-1",
		"duplicate-alert@example.com-2",
	}

	for i, expected := range expectedIDs {
		if ids[i] != expected {
			t.Errorf("ID [%d]: expected %q, got %q", i, expected, ids[i])
		}
	}

	// Verify all 3 messages are preserved without being overwritten
	expectedBodies := []string{
		"First duplicate message content",
		"Second duplicate message content",
		"Third duplicate message content",
	}

	for i, id := range ids {
		raw, err := msg.Read(cachedir.At(cacheDir), email.ID(id))
		if err != nil {
			t.Fatalf("failed to read cached message for %q: %v", id, err)
		}
		if !bytes.Contains(raw, []byte(expectedBodies[i])) {
			t.Errorf("message %q payload mismatch: expected to contain %q", id, expectedBodies[i])
		}
	}
}

func TestIngestMbox_IncrementalAppend(t *testing.T) {
	cacheDir := t.TempDir()
	mboxFile := filepath.Join(t.TempDir(), "test.mbox")

	msg1 := "From sender@example.com Fri Sep 18 10:00:00 2026\n" +
		"From: sender@example.com\n" +
		"To: recipient@example.com\n" +
		"Subject: First\n" +
		"Message-ID: <msg-1@example.com>\n" +
		"Date: Fri, 18 Sep 2026 10:00:00 +0000\n\n" +
		"Body 1\n\n"

	msg2 := "From sender@example.com Fri Sep 18 10:01:00 2026\n" +
		"From: sender@example.com\n" +
		"To: recipient@example.com\n" +
		"Subject: Second\n" +
		"Message-ID: <msg-2@example.com>\n" +
		"Date: Fri, 18 Sep 2026 10:01:00 +0000\n\n" +
		"Body 2\n\n"

	if err := os.WriteFile(mboxFile, []byte(msg1+msg2), 0600); err != nil {
		t.Fatalf("failed to write mbox file: %v", err)
	}

	// 1. Initial ingestion
	ids, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil {
		t.Fatalf("IngestMbox initial failed: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}

	// 2. Ingest again without modification (fast cache hit)
	idsCached, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil {
		t.Fatalf("IngestMbox cache hit failed: %v", err)
	}
	if len(idsCached) != 2 {
		t.Fatalf("expected 2 IDs from cache hit, got %d", len(idsCached))
	}

	// 3. Append 3rd message and duplicate ID of msg1
	msg3 := "From sender@example.com Fri Sep 18 10:02:00 2026\n" +
		"From: sender@example.com\n" +
		"To: recipient@example.com\n" +
		"Subject: Third (duplicate ID)\n" +
		"Message-ID: <msg-1@example.com>\n" +
		"Date: Fri, 18 Sep 2026 10:02:00 +0000\n\n" +
		"Body 3\n\n"

	f, err := os.OpenFile(mboxFile, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("failed to open mbox for append: %v", err)
	}
	if _, writeErr := f.WriteString(msg3); writeErr != nil {
		_ = f.Close()
		t.Fatalf("failed to append msg3: %v", writeErr)
	}
	_ = f.Close()

	idsAppended, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil {
		t.Fatalf("IngestMbox append failed: %v", err)
	}
	if len(idsAppended) != 3 || idsAppended[2] != "msg-1@example.com-1" {
		t.Fatalf("expected 3 IDs with [msg-1@example.com-1] appended, got %v", idsAppended)
	}
	inboxIDs, err := folderindex.IDs(cachedir.At(cacheDir), "INBOX")
	if err != nil {
		t.Fatalf("IDs INBOX failed: %v", err)
	}
	if len(inboxIDs) != 3 {
		t.Fatalf("expected 3 IDs in INBOX index, got %d", len(inboxIDs))
	}

	// Verify all 3 messages are preserved in cache
	for _, expectedID := range []string{"msg-1@example.com", "msg-2@example.com", "msg-1@example.com-1"} {
		raw, readErr := msg.Read(cachedir.At(cacheDir), email.ID(expectedID))
		if readErr != nil {
			t.Errorf("failed to read cached message %q: %v", expectedID, readErr)
		}
		if len(raw) == 0 {
			t.Errorf("cached message %q is empty", expectedID)
		}
	}

	// 4. File truncation triggers full rebuild
	if writeErr := os.WriteFile(mboxFile, []byte(msg1), 0600); writeErr != nil {
		t.Fatalf("failed to truncate mbox file: %v", writeErr)
	}
	idsTruncated, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil {
		t.Fatalf("IngestMbox after truncate failed: %v", err)
	}
	if len(idsTruncated) != 1 || idsTruncated[0] != "msg-1@example.com" {
		t.Fatalf("expected [msg-1@example.com] after truncate rebuild, got %v", idsTruncated)
	}
}

func TestParseMboxStream_LargeBufferResetAndLines(t *testing.T) {
	cacheDir := t.TempDir()

	// Construct message with lines longer than 4KB buffer and > 8MB message
	longLine := strings.Repeat("A", 10000) + "\n"
	largeMsg := "From sender@example.com Fri Sep 18 10:00:00 2026\n" +
		"From: sender@example.com\n" +
		"To: recipient@example.com\n" +
		"Subject: Large Message\n" +
		"Message-ID: <large-msg@example.com>\n" +
		"Date: Fri, 18 Sep 2026 10:00:00 +0000\n\n" +
		strings.Repeat(longLine, 900) // ~9 MB body

	smallMsg := "From sender@example.com Fri Sep 18 10:01:00 2026\n" +
		"From: sender@example.com\n" +
		"To: recipient@example.com\n" +
		"Subject: Small Followup\n" +
		"Message-ID: <small-msg@example.com>\n" +
		"Date: Fri, 18 Sep 2026 10:01:00 +0000\n\n" +
		"Small body\n"

	combined := largeMsg + smallMsg
	ids, err := parseMboxStream(strings.NewReader(combined), cachedir.At(cacheDir), time.Now())
	if err != nil {
		t.Fatalf("parseMboxStream failed on large buffer: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}
	if ids[0] != "large-msg@example.com" || ids[1] != "small-msg@example.com" {
		t.Errorf("unexpected IDs: %v", ids)
	}
}

func TestSaveMboxMeta_AtomicWrite(t *testing.T) {
	cacheDir := t.TempDir()
	mboxFile := filepath.Join(cacheDir, "test.mbox")
	if err := os.WriteFile(mboxFile, []byte("From dummy\n\n"), 0600); err != nil {
		t.Fatalf("failed to write dummy mbox: %v", err)
	}
	fi, err := os.Stat(mboxFile)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	seenIDs := map[string]int{"msg-1": 1}
	ids := []string{"msg-1"}

	// First write
	if saveErr := saveMboxMeta(cachedir.At(cacheDir), mboxFile, fi, email.IDs(ids), "TestMbox", "tailhash123"); saveErr != nil {
		t.Fatalf("saveMboxMeta failed: %v", saveErr)
	}

	metaP := metaPath(cacheDir)
	metaFi, err := os.Stat(metaP)
	if err != nil {
		t.Fatalf("meta file stat failed: %v", err)
	}
	if metaFi.Mode().Perm() != 0600 {
		t.Errorf("expected 0600 permissions, got %o", metaFi.Mode().Perm())
	}

	meta, err := loadMboxMeta(cacheDir)
	if err != nil {
		t.Fatalf("loadMboxMeta failed: %v", err)
	}
	if meta == nil || meta.TailHash != "tailhash123" || meta.Count != 1 {
		t.Errorf("metadata content mismatch: %+v", meta)
	}

	// Second write (atomic overwrite of existing meta file)
	seenIDs["msg-2"] = 1
	ids = append(ids, "msg-2")
	if saveErr := saveMboxMeta(cachedir.At(cacheDir), mboxFile, fi, email.IDs(ids), "TestMbox", "tailhash456"); saveErr != nil {
		t.Fatalf("second saveMboxMeta failed: %v", saveErr)
	}

	meta2, err := loadMboxMeta(cacheDir)
	if err != nil {
		t.Fatalf("second loadMboxMeta failed: %v", err)
	}
	if meta2 == nil || meta2.TailHash != "tailhash456" || meta2.Count != 2 {
		t.Errorf("updated metadata content mismatch: %+v", meta2)
	}
}

func TestParseMboxStream_MboxrdUnescaping(t *testing.T) {
	cacheDir := t.TempDir()

	mboxContent := "From sender@example.com Fri Sep 18 10:00:00 2026\n" +
		"From: sender@example.com\n" +
		"To: recipient@example.com\n" +
		"Subject: Mboxrd Test\n" +
		"Message-ID: <mboxrd-test@example.com>\n" +
		"\n" +
		">From alice@example.com\n" +
		">>From bob@example.com\n" +
		">>>From charlie@example.com\n" +
		"Normal text line\n" +
		">Not a From line\n"

	ids, err := parseMboxStream(strings.NewReader(mboxContent), cachedir.At(cacheDir), time.Now())
	if err != nil {
		t.Fatalf("parseMboxStream failed: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 ID, got %d", len(ids))
	}

	raw, err := msg.Read(cachedir.At(cacheDir), email.ID(ids[0]))
	if err != nil {
		t.Fatalf("msg.Read failed: %v", err)
	}

	rawStr := string(raw)
	// Assert unescaped contents
	if !strings.Contains(rawStr, "\nFrom alice@example.com\n") {
		t.Errorf("expected unescaped 'From alice', got:\n%s", rawStr)
	}
	if !strings.Contains(rawStr, "\n>From bob@example.com\n") {
		t.Errorf("expected unescaped '>From bob', got:\n%s", rawStr)
	}
	if !strings.Contains(rawStr, "\n>>From charlie@example.com\n") {
		t.Errorf("expected unescaped '>>From charlie', got:\n%s", rawStr)
	}
	if !strings.Contains(rawStr, "\n>Not a From line\n") {
		t.Errorf("expected '>Not a From line' to remain unchanged, got:\n%s", rawStr)
	}
}

func TestResetMboxLine_BoundedMemory(t *testing.T) {
	small := make([]byte, 100, 32*1024)
	resSmall := resetMboxLine(small)
	if cap(resSmall) != 32*1024 {
		t.Errorf("expected retained cap %d, got %d", 32*1024, cap(resSmall))
	}
	if len(resSmall) != 0 {
		t.Errorf("expected len 0, got %d", len(resSmall))
	}

	oversized := make([]byte, 100, 2*1024*1024)
	resOversized := resetMboxLine(oversized)
	if cap(resOversized) != initialLineCapacity {
		t.Errorf("expected reset cap %d, got %d", initialLineCapacity, cap(resOversized))
	}
	if len(resOversized) != 0 {
		t.Errorf("expected len 0, got %d", len(resOversized))
	}
}

func TestMboxIndexRecords_AppendAndLoad(t *testing.T) {
	cacheDir := t.TempDir()

	records := []mboxIndexRecord{
		{ID: "msg-1", BaseID: "base-1", SeenCount: 1},
		{ID: "msg-2", BaseID: "base-2", SeenCount: 1},
	}
	if err := writeIndexRecords(recordsPath(cacheDir), records); err != nil {
		t.Fatalf("writeIndexRecords failed: %v", err)
	}

	appended := []mboxIndexRecord{
		{ID: "msg-1-1", BaseID: "base-1", SeenCount: 2},
	}
	if err := appendIndexRecords(recordsPath(cacheDir), appended); err != nil {
		t.Fatalf("appendIndexRecords failed: %v", err)
	}

	seen, err := loadSeenIDs(cacheDir)
	if err != nil {
		t.Fatalf("loadSeenIDs failed: %v", err)
	}
	if seen["base-1"] != 2 {
		t.Errorf("expected base-1 seen count 2, got %d", seen["base-1"])
	}
	if seen["base-2"] != 1 {
		t.Errorf("expected base-2 seen count 1, got %d", seen["base-2"])
	}
}

func TestSaveMboxMeta_ScalarOnly(t *testing.T) {
	cacheDir := t.TempDir()
	mboxFile := filepath.Join(cacheDir, "test.mbox")
	if err := os.WriteFile(mboxFile, []byte("From x\n\n"), 0600); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	fi, err := os.Stat(mboxFile)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}

	if saveErr := saveMboxMeta(cachedir.At(cacheDir), mboxFile, fi, []email.ID{"id-1"}, "DisplayName", "tail123"); saveErr != nil {
		t.Fatalf("saveMboxMeta failed: %v", saveErr)
	}

	data, err := os.ReadFile(metaPath(cacheDir))
	if err != nil {
		t.Fatalf("read meta failed: %v", err)
	}
	if strings.Contains(string(data), "seen_ids") {
		t.Errorf("meta file should not contain seen_ids: %s", string(data))
	}
}

func TestIngestMbox_RepeatedIncrementalAppendsDuplicateIDs(t *testing.T) {
	cacheDir := t.TempDir()
	mboxFile := filepath.Join(t.TempDir(), "test.mbox")

	makeMsg := func(idx int, mid, body string) string {
		return fmt.Sprintf("From sender@example.com Fri Sep 18 10:0%d:00 2026\n"+
			"From: sender@example.com\n"+
			"To: recipient@example.com\n"+
			"Subject: Msg %d\n"+
			"Message-ID: <%s>\n"+
			"Date: Fri, 18 Sep 2026 10:0%d:00 +0000\n\n"+
			"%s\n\n", idx, idx, mid, idx, body)
	}

	// 1. Initial ingestion with 1 message
	if err := os.WriteFile(mboxFile, []byte(makeMsg(1, "dup@example.com", "first")), 0600); err != nil {
		t.Fatalf("write mbox: %v", err)
	}
	ids, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil || len(ids) != 1 || ids[0] != "dup@example.com" {
		t.Fatalf("initial ingest failed: %v, ids=%v", err, ids)
	}

	// 2. First incremental append with same Message-ID
	f, err := os.OpenFile(mboxFile, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open mbox: %v", err)
	}
	_, _ = f.WriteString(makeMsg(2, "dup@example.com", "second"))
	_ = f.Close()

	idsAppended1, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil || len(idsAppended1) != 2 || idsAppended1[1] != "dup@example.com-1" {
		t.Fatalf("first append failed: %v, ids=%v", err, idsAppended1)
	}

	// 3. Second incremental append with same Message-ID
	f2, err := os.OpenFile(mboxFile, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open mbox: %v", err)
	}
	_, _ = f2.WriteString(makeMsg(3, "dup@example.com", "third"))
	_ = f2.Close()

	idsAppended2, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil || len(idsAppended2) != 3 || idsAppended2[2] != "dup@example.com-2" {
		t.Fatalf("second append failed: %v, ids=%v", err, idsAppended2)
	}

	// Verify all 3 messages are in INBOX index
	inboxIDs, err := folderindex.IDs(cachedir.At(cacheDir), "INBOX")
	if err != nil {
		t.Fatalf("label.IDs INBOX failed: %v", err)
	}
	expected := []email.ID{"dup@example.com", "dup@example.com-1", "dup@example.com-2"}
	if len(inboxIDs) != len(expected) {
		t.Fatalf("expected %d ids, got %d: %v", len(expected), len(inboxIDs), inboxIDs)
	}
	for i, exp := range expected {
		if inboxIDs[i] != exp {
			t.Errorf("expected [%d]=%q, got %q", i, exp, inboxIDs[i])
		}
	}
}

func TestCompactMboxRecords(t *testing.T) {
	cacheDir := t.TempDir()

	initial := []mboxIndexRecord{
		{ID: "m1", BaseID: "base-1", SeenCount: 1},
		{ID: "m2", BaseID: "base-2", SeenCount: 1},
	}
	if err := writeIndexRecords(recordsPath(cacheDir), initial); err != nil {
		t.Fatalf("write initial records: %v", err)
	}

	appends := []mboxIndexRecord{
		{ID: "m1-1", BaseID: "base-1", SeenCount: 2},
		{ID: "m1-2", BaseID: "base-1", SeenCount: 3},
	}
	if err := appendIndexRecords(recordsPath(cacheDir), appends); err != nil {
		t.Fatalf("append records: %v", err)
	}

	if _, err := compactMboxRecords(cacheDir); err != nil {
		t.Fatalf("compactMboxRecords failed: %v", err)
	}

	seen, err := loadSeenIDs(cacheDir)
	if err != nil {
		t.Fatalf("loadSeenIDs failed: %v", err)
	}
	if seen["base-1"] != 3 {
		t.Errorf("expected base-1 count 3, got %d", seen["base-1"])
	}
	if seen["base-2"] != 1 {
		t.Errorf("expected base-2 count 1, got %d", seen["base-2"])
	}
}

func TestLoadIndexRecords_CombinedSeenAndLatest(t *testing.T) {
	cacheDir := t.TempDir()

	initial := []mboxIndexRecord{
		{ID: "m1", BaseID: "base-1", SeenCount: 1},
		{ID: "m2", BaseID: "base-2", SeenCount: 1},
	}
	if err := writeIndexRecords(recordsPath(cacheDir), initial); err != nil {
		t.Fatalf("writeIndexRecords failed: %v", err)
	}

	appends := []mboxIndexRecord{
		{ID: "m1-1", BaseID: "base-1", SeenCount: 2},
		{ID: "m1-2", BaseID: "base-1", SeenCount: 3},
	}
	if err := appendIndexRecords(recordsPath(cacheDir), appends); err != nil {
		t.Fatalf("appendIndexRecords failed: %v", err)
	}

	seen, latest, err := loadIndexRecords(cacheDir)
	if err != nil {
		t.Fatalf("loadIndexRecords failed: %v", err)
	}
	if seen["base-1"] != 3 || seen["base-2"] != 1 {
		t.Errorf("unexpected seenIDs: %+v", seen)
	}
	if latest["base-1"].SeenCount != 3 || latest["base-1"].ID != "m1-2" {
		t.Errorf("unexpected latest for base-1: %+v", latest["base-1"])
	}
	if latest["base-2"].SeenCount != 1 || latest["base-2"].ID != "m2" {
		t.Errorf("unexpected latest for base-2: %+v", latest["base-2"])
	}

	// Legacy meta fallback test
	legacyDir := t.TempDir()
	legacyMeta := `{"path":"dummy","size":100,"count":1,"seen_ids":{"legacy-base":4}}`
	if writeErr := os.WriteFile(metaPath(legacyDir), []byte(legacyMeta), 0600); writeErr != nil {
		t.Fatalf("write legacy meta failed: %v", writeErr)
	}
	seenLeg, latestLeg, err := loadIndexRecords(legacyDir)
	if err != nil {
		t.Fatalf("loadIndexRecords legacy failed: %v", err)
	}
	if seenLeg["legacy-base"] != 4 {
		t.Errorf("expected legacy seen count 4, got %d", seenLeg["legacy-base"])
	}
	if latestLeg["legacy-base"].SeenCount != 4 || latestLeg["legacy-base"].ID != "legacy-base-3" {
		t.Errorf("unexpected synthesized legacy record: %+v", latestLeg["legacy-base"])
	}
}

func TestTryAppendMbox_CompactionThresholdAndReuse(t *testing.T) {
	cacheDir := t.TempDir()
	mboxFile := filepath.Join(t.TempDir(), "test.mbox")

	msg1 := "From sender@example.com Fri Sep 18 10:00:00 2026\n" +
		"From: sender@example.com\n" +
		"To: recipient@example.com\n" +
		"Subject: Initial\n" +
		"Message-ID: <initial@example.com>\n" +
		"Date: Fri, 18 Sep 2026 10:00:00 +0000\n\n" +
		"Body 1\n\n"

	if err := os.WriteFile(mboxFile, []byte(msg1), 0600); err != nil {
		t.Fatalf("write initial mbox failed: %v", err)
	}

	ids, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil || len(ids) != 1 {
		t.Fatalf("initial IngestMbox failed: %v, ids=%v", err, ids)
	}

	// Inflate records file beyond 64KB threshold with dummy duplicate entries
	var inflated []mboxIndexRecord
	for i := 0; i < 1500; i++ {
		inflated = append(inflated, mboxIndexRecord{
			ID:        fmt.Sprintf("bulk-id-%d-1", i),
			BaseID:    fmt.Sprintf("bulk-id-%d", i),
			SeenCount: 2,
		})
	}
	if appendErr := appendIndexRecords(recordsPath(cacheDir), inflated); appendErr != nil {
		t.Fatalf("appendIndexRecords inflated failed: %v", appendErr)
	}

	fiPre, err := os.Stat(recordsPath(cacheDir))
	if err != nil {
		t.Fatalf("stat recordsPath failed: %v", err)
	}
	if fiPre.Size() <= 64*1024 {
		t.Fatalf("expected records file > 64KB, got %d bytes", fiPre.Size())
	}

	// Now append a new message to the mbox file
	msg2 := "From sender@example.com Fri Sep 18 10:01:00 2026\n" +
		"From: sender@example.com\n" +
		"To: recipient@example.com\n" +
		"Subject: Second\n" +
		"Message-ID: <second@example.com>\n" +
		"Date: Fri, 18 Sep 2026 10:01:00 +0000\n\n" +
		"Body 2\n\n"

	f, err := os.OpenFile(mboxFile, os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		t.Fatalf("open mbox for append failed: %v", err)
	}
	if _, writeErr := f.WriteString(msg2); writeErr != nil {
		_ = f.Close()
		t.Fatalf("append to mbox failed: %v", writeErr)
	}
	_ = f.Close()

	// Ingest appended mbox: should execute tryAppendMbox and trigger maybeCompactRecords
	idsAppended, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil {
		t.Fatalf("IngestMbox after append failed: %v", err)
	}
	if len(idsAppended) != 2 {
		t.Fatalf("expected 2 IDs after append, got %d", len(idsAppended))
	}

	// Verify compaction occurred: file size should be compacted
	meta, err := loadMboxMeta(cacheDir)
	if err != nil {
		t.Fatalf("loadMboxMeta failed: %v", err)
	}
	if meta.LastCompactedSize == 0 {
		t.Errorf("expected LastCompactedSize to be set after compaction, got 0")
	}

	fiPost, err := os.Stat(recordsPath(cacheDir))
	if err != nil {
		t.Fatalf("stat post-compaction records failed: %v", err)
	}
	if fiPost.Size() != meta.LastCompactedSize {
		t.Errorf("records file size %d != meta.LastCompactedSize %d", fiPost.Size(), meta.LastCompactedSize)
	}

	// Verify seen IDs are intact after compaction
	seen, err := loadSeenIDs(cacheDir)
	if err != nil {
		t.Fatalf("loadSeenIDs post-compaction failed: %v", err)
	}
	if seen["bulk-id-0"] != 2 || seen["bulk-id-1499"] != 2 {
		t.Errorf("seen counts corrupted after compaction: %v", seen["bulk-id-0"])
	}
}

func TestIngestMbox_RebuildClearsStaleIndexRecords(t *testing.T) {
	cacheDir := t.TempDir()
	mboxFile := filepath.Join(t.TempDir(), "test.mbox")

	initialContent := "From sender@example.com Fri Sep 18 10:00:00 2026\n" +
		"From: sender@example.com\n" +
		"Message-ID: <dup-id@example.com>\n" +
		"Subject: First\n\nBody 1\n\n" +
		"From sender@example.com Fri Sep 18 10:00:01 2026\n" +
		"From: sender@example.com\n" +
		"Message-ID: <dup-id@example.com>\n" +
		"Subject: Duplicate\n\nBody 2\n\n"

	if err := os.WriteFile(mboxFile, []byte(initialContent), 0600); err != nil {
		t.Fatalf("write initial mbox failed: %v", err)
	}

	idsInitial, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil {
		t.Fatalf("IngestMbox initial failed: %v", err)
	}
	if len(idsInitial) != 2 {
		t.Fatalf("expected 2 IDs initially, got %d", len(idsInitial))
	}

	seenPre, _, err := loadIndexRecords(cacheDir)
	if err != nil {
		t.Fatalf("loadIndexRecords pre failed: %v", err)
	}
	if seenPre["dup-id@example.com"] != 2 {
		t.Fatalf("expected dup-id count 2, got %d", seenPre["dup-id@example.com"])
	}

	rebuiltContent := "From sender@example.com Fri Sep 18 10:00:00 2026\n" +
		"From: sender@example.com\n" +
		"Message-ID: <unique-1@example.com>\n" +
		"Subject: Unique 1\n\nBody 1\n\n" +
		"From sender@example.com Fri Sep 18 10:00:01 2026\n" +
		"From: sender@example.com\n" +
		"Message-ID: <unique-2@example.com>\n" +
		"Subject: Unique 2\n\nBody 2\n\n"

	if writeErr := os.WriteFile(mboxFile, []byte(rebuiltContent), 0600); writeErr != nil {
		t.Fatalf("write rebuilt mbox failed: %v", writeErr)
	}

	idsRebuilt, err := IngestMbox(mboxFile, cachedir.At(cacheDir), "TestAccount")
	if err != nil {
		t.Fatalf("IngestMbox rebuilt failed: %v", err)
	}
	if len(idsRebuilt) != 2 {
		t.Fatalf("expected 2 IDs after rebuild, got %d", len(idsRebuilt))
	}

	seenPost, latestPost, err := loadIndexRecords(cacheDir)
	if err != nil {
		t.Fatalf("loadIndexRecords post failed: %v", err)
	}
	if len(seenPost) != 0 || len(latestPost) != 0 {
		t.Errorf("expected empty index records after rebuild, got seen: %+v, latest: %+v", seenPost, latestPost)
	}
}
