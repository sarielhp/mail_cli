package mbox

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"github.com/sarielhp/mail_cli/cache/cachedir"
	"github.com/sarielhp/mail_cli/email"
	"io"
	"log/slog"
	"net/mail"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/sarielhp/mail_cli/cache"
	"github.com/sarielhp/mail_cli/cache/dirlock"
	"github.com/sarielhp/mail_cli/cache/folderindex"
	"github.com/sarielhp/mail_cli/cache/msg"
	"github.com/sarielhp/mail_cli/cfg_acc"
)

type mboxMeta struct {
	Path              string    `json:"path"`
	Size              int64     `json:"size"`
	ModTime           time.Time `json:"mod_time"`
	Count             int       `json:"count"`
	TailHash          string    `json:"tail_hash,omitempty"`
	LastCompactedSize int64     `json:"last_compacted_size,omitempty"`
}

type mboxIndexRecord struct {
	ID        string `json:"id"`
	BaseID    string `json:"base_id"`
	SeenCount int    `json:"seen_count"`
}

func metaPath(cacheDir string) string {
	return filepath.Join(cacheDir, "mbox_meta.json")
}

func recordsPath(cacheDir string) string {
	return filepath.Join(cacheDir, "mbox_records.jsonl")
}

func loadMboxMeta(cacheDir string) (*mboxMeta, error) {
	data, err := os.ReadFile(metaPath(cacheDir))
	if err != nil {
		return nil, err
	}
	var meta mboxMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	return &meta, nil
}

func checkCachedMeta(cacheDir cachedir.Account, mboxPath string, fi os.FileInfo) (*mboxMeta, []email.ID, bool) {
	meta, err := loadMboxMeta(cacheDir.Path())
	if err != nil {
		return nil, nil, false
	}
	if meta.Path != mboxPath || meta.Size != fi.Size() || !meta.ModTime.Equal(fi.ModTime()) {
		return meta, nil, false
	}
	ids, err := folderindex.IDs(cacheDir, "INBOX")
	if err != nil || len(ids) != meta.Count {
		return meta, nil, false
	}
	return meta, ids, true
}

func computeTailHash(r io.ReaderAt, size int64) (string, error) {
	if size <= 0 {
		return "", nil
	}
	sample := int64(4096)
	if sample > size {
		sample = size
	}
	buf := make([]byte, sample)
	n, err := r.ReadAt(buf, size-sample)
	if err != nil && err != io.EOF {
		return "", err
	}
	h := sha256.Sum256(buf[:n])
	return fmt.Sprintf("%x", h), nil
}

func appendIndexRecords(path string, records []mboxIndexRecord) error {
	if len(records) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	bw := bufio.NewWriterSize(f, 64*1024)
	enc := json.NewEncoder(bw)
	for _, record := range records {
		if err := enc.Encode(record); err != nil {
			return err
		}
	}
	return bw.Flush()
}

func writeIndexRecords(path string, records []mboxIndexRecord) error {
	tmpPath := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	bw := bufio.NewWriterSize(f, 64*1024)
	enc := json.NewEncoder(bw)
	for _, record := range records {
		if err := enc.Encode(record); err != nil {
			_ = f.Close()
			_ = os.Remove(tmpPath)
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		_ = f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

func compactFromRecords(path string, latest map[string]mboxIndexRecord) (int64, error) {
	records := make([]mboxIndexRecord, 0, len(latest))
	for _, rec := range latest {
		records = append(records, rec)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].BaseID < records[j].BaseID
	})
	if err := writeIndexRecords(path, records); err != nil {
		return 0, err
	}
	if fi, err := os.Stat(path); err == nil {
		return fi.Size(), nil
	}
	return 0, nil
}

func compactMboxRecords(cacheDir string) (int64, error) {
	_, latest, err := loadIndexRecords(cacheDir)
	if err != nil {
		return 0, err
	}
	return compactFromRecords(recordsPath(cacheDir), latest)
}

func maybeCompactRecords(cacheDir string, lastCompacted int64, latestByBase map[string]mboxIndexRecord) int64 {
	fiRec, err := os.Stat(recordsPath(cacheDir))
	if err != nil || fiRec.Size() <= 64*1024 {
		return lastCompacted
	}
	if lastCompacted != 0 && fiRec.Size() < lastCompacted*2 {
		return lastCompacted
	}
	newSize, err := compactFromRecords(recordsPath(cacheDir), latestByBase)
	if err != nil {
		return lastCompacted
	}
	return newSize
}

func loadIndexRecords(cacheDir string) (map[string]int, map[string]mboxIndexRecord, error) {
	seenIDs := make(map[string]int)
	latest := make(map[string]mboxIndexRecord)
	p := recordsPath(cacheDir)
	f, err := os.Open(p)
	if err == nil {
		defer f.Close()
		scanner := bufio.NewScanner(bufio.NewReaderSize(f, 64*1024))
		for scanner.Scan() {
			var rec mboxIndexRecord
			if decodeErr := json.Unmarshal(scanner.Bytes(), &rec); decodeErr == nil && rec.BaseID != "" {
				if rec.SeenCount > seenIDs[rec.BaseID] {
					seenIDs[rec.BaseID] = rec.SeenCount
					latest[rec.BaseID] = rec
				}
			}
		}
		return seenIDs, latest, scanner.Err()
	}
	if !os.IsNotExist(err) {
		return nil, nil, err
	}
	legacyData, err := os.ReadFile(metaPath(cacheDir))
	if err != nil {
		return seenIDs, latest, nil
	}
	var legacy struct {
		SeenIDs map[string]int `json:"seen_ids"`
	}
	if err := json.Unmarshal(legacyData, &legacy); err == nil && legacy.SeenIDs != nil {
		for base, count := range legacy.SeenIDs {
			seenIDs[base] = count
			if count > 1 {
				latest[base] = mboxIndexRecord{
					ID:        fmt.Sprintf("%s-%d", base, count-1),
					BaseID:    base,
					SeenCount: count,
				}
			}
		}
		return seenIDs, latest, nil
	}
	return seenIDs, latest, nil
}

func loadSeenIDs(cacheDir string) (map[string]int, error) {
	seen, _, err := loadIndexRecords(cacheDir)
	return seen, err
}

func saveMboxMeta(cacheDir cachedir.Account, mboxPath string, fi os.FileInfo, ids []email.ID, _ string, tailHash string) error {
	if err := folderindex.ReplaceAll(cacheDir, "INBOX", ids); err != nil {
		return err
	}
	cs := &cache.DiskCacheStore{Dir: cacheDir}
	_ = cs.SaveLabelItems([]cfg_acc.Item{
		{Name: "INBOX", FullName: "INBOX"},
	})
	meta := mboxMeta{
		Path:     mboxPath,
		Size:     fi.Size(),
		ModTime:  fi.ModTime(),
		Count:    len(ids),
		TailHash: tailHash,
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return dirlock.AtomicWrite(metaPath(cacheDir.Path()), data, 0600)
}

func saveMboxMetaScalar(cacheDir, mboxPath string, fi os.FileInfo, count int, _ string, tailHash string, lastCompactedSize int64) error {
	meta := mboxMeta{
		Path:              mboxPath,
		Size:              fi.Size(),
		ModTime:           fi.ModTime(),
		Count:             count,
		TailHash:          tailHash,
		LastCompactedSize: lastCompactedSize,
	}
	data, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return dirlock.AtomicWrite(metaPath(cacheDir), data, 0600)
}

func tryAppendMbox(f *os.File, fi os.FileInfo, meta *mboxMeta, cacheDir cachedir.Account, mboxPath string, displayName string) ([]email.ID, bool, error) {
	if meta == nil || meta.Path != mboxPath || fi.Size() <= meta.Size || meta.TailHash == "" {
		return nil, false, nil
	}
	release, lockErr := dirlock.Guard(cacheDir.Path())
	if lockErr != nil {
		slog.Debug("cache lock unavailable, continuing with in-process lock only",
			slog.String("dir", cacheDir.Path()), slog.Any("error", lockErr))
	}
	defer release()

	cachedIDs, err := folderindex.IDs(cacheDir, "INBOX")
	if err != nil || len(cachedIDs) != meta.Count {
		return nil, false, nil
	}
	tail, err := computeTailHash(f, meta.Size)
	if err != nil || tail != meta.TailHash {
		return nil, false, nil
	}
	if _, seekErr := f.Seek(meta.Size, io.SeekStart); seekErr != nil {
		return nil, false, seekErr
	}
	seenIDs, latestByBase, err := loadIndexRecords(cacheDir.Path())
	if err != nil {
		return nil, false, err
	}
	addedIDs, newRecords, err := parseMboxStreamWithSeen(f, cacheDir, fi.ModTime(), seenIDs, true)
	if err != nil {
		return nil, false, err
	}
	if len(newRecords) > 0 {
		if appendErr := appendIndexRecords(recordsPath(cacheDir.Path()), newRecords); appendErr != nil {
			return nil, false, appendErr
		}
		for _, rec := range newRecords {
			latestByBase[rec.BaseID] = rec
		}
	}
	lastCompacted := maybeCompactRecords(cacheDir.Path(), meta.LastCompactedSize, latestByBase)
	if appendErr := folderindex.AppendBatch(cacheDir, "INBOX", addedIDs); appendErr != nil {
		return nil, false, appendErr
	}
	newTail, err := computeTailHash(f, fi.Size())
	if err != nil {
		return nil, false, err
	}
	newCount := meta.Count + len(addedIDs)
	if err := saveMboxMetaScalar(cacheDir.Path(), mboxPath, fi, newCount, displayName, newTail, lastCompacted); err != nil {
		return nil, false, err
	}
	// Fresh slice rather than appending into cachedIDs, so the slice handed
	// back to the caller never shares the folder index's backing array.
	allIDs := make([]email.ID, 0, len(cachedIDs)+len(addedIDs))
	allIDs = append(allIDs, cachedIDs...)
	allIDs = append(allIDs, addedIDs...)
	return allIDs, true, nil
}

func sanitizeMessageID(mid string) string {
	mid = strings.TrimSpace(mid)
	mid = strings.TrimPrefix(mid, "<")
	mid = strings.TrimSuffix(mid, ">")
	if mid == "" {
		return ""
	}
	var sb strings.Builder
	for _, r := range mid {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '@' {
			sb.WriteRune(r)
		} else {
			sb.WriteRune('_')
		}
	}
	res := sb.String()
	if len(res) > 120 {
		h := fmt.Sprintf("%x", sha256.Sum256([]byte(res)))[:16]
		return res[:100] + "_" + h
	}
	return res
}

const (
	initialLineCapacity = 64 * 1024
	maxRetainedLine     = 1 * 1024 * 1024
)

func resetMboxLine(line []byte) []byte {
	if cap(line) > maxRetainedLine {
		return make([]byte, 0, initialLineCapacity)
	}
	return line[:0]
}

func processMboxMessage(batch *msg.StoreBatch, cacheDir cachedir.Account, raw []byte, fallbackDate time.Time, seenIDs map[string]int, isAppend bool) (string, mboxIndexRecord, error) {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "", mboxIndexRecord{}, nil
	}
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	mid := ""
	date := fallbackDate
	if err == nil && parsed != nil {
		mid = sanitizeMessageID(parsed.Header.Get("Message-ID"))
		if dStr := parsed.Header.Get("Date"); dStr != "" {
			if parsedDate, parseErr := mail.ParseDate(dStr); parseErr == nil && !parsedDate.IsZero() {
				date = parsedDate
			}
		}
	}
	if mid == "" {
		mid = fmt.Sprintf("mbox-%x", sha256.Sum256(raw))[:24]
	}
	baseMid := mid
	count := seenIDs[baseMid]
	if isAppend && count == 0 && !cacheDir.IsZero() {
		if ex, _ := msg.Exists(cacheDir, email.ID(baseMid)); ex {
			count = 1
		}
	}
	if count > 0 {
		mid = fmt.Sprintf("%s-%d", baseMid, count)
	}
	seenIDs[baseMid] = count + 1

	if err := batch.Store(email.ID(mid), raw, date); err != nil {
		return "", mboxIndexRecord{}, err
	}
	var rec mboxIndexRecord
	if count > 0 {
		rec = mboxIndexRecord{
			ID:        mid,
			BaseID:    baseMid,
			SeenCount: seenIDs[baseMid],
		}
	}
	return mid, rec, nil
}

func appendProcessedMessage(ids *[]email.ID, records *[]mboxIndexRecord, batch *msg.StoreBatch, cacheDir cachedir.Account, raw []byte, fallbackDate time.Time, seenIDs map[string]int, isAppend bool) {
	id, rec, err := processMboxMessage(batch, cacheDir, raw, fallbackDate, seenIDs, isAppend)
	if err == nil && id != "" {
		*ids = append(*ids, email.ID(id))
		if rec.BaseID != "" {
			*records = append(*records, rec)
		}
	}
}

func parseMboxStream(r io.Reader, cacheDir cachedir.Account, fallbackDate time.Time) ([]string, error) {
	ids, _, err := parseMboxStreamWithSeen(r, cacheDir, fallbackDate, make(map[string]int), false)
	return email.IDStrings(ids), err
}

func parseMboxStreamWithSeen(r io.Reader, cacheDir cachedir.Account, fallbackDate time.Time, seenIDs map[string]int, isAppend bool) ([]email.ID, []mboxIndexRecord, error) {
	if seenIDs == nil {
		seenIDs = make(map[string]int)
	}
	batch, err := msg.NewStoreBatch(cacheDir)
	if err != nil {
		return nil, nil, err
	}
	defer batch.Close()

	reader := bufio.NewReaderSize(r, initialLineCapacity)
	var ids []email.ID
	var records []mboxIndexRecord
	var currentMsg bytes.Buffer
	inMessage := false
	line := make([]byte, 0, initialLineCapacity)

	for {
		fragment, readErr := reader.ReadSlice('\n')
		line = append(line, fragment...)
		if readErr == bufio.ErrBufferFull {
			continue
		}
		if readErr != nil && readErr != io.EOF {
			return nil, nil, readErr
		}
		if len(line) == 0 && readErr == io.EOF {
			break
		}

		if bytes.HasPrefix(line, []byte("From ")) {
			if inMessage && currentMsg.Len() > 0 {
				appendProcessedMessage(&ids, &records, batch, cacheDir, currentMsg.Bytes(), fallbackDate, seenIDs, isAppend)
				if currentMsg.Cap() > 8<<20 {
					currentMsg = bytes.Buffer{}
				} else {
					currentMsg.Reset()
				}
			}
			inMessage = true
			line = resetMboxLine(line)
			if readErr == io.EOF {
				break
			}
			continue
		}

		if inMessage {
			toWrite := line
			if bytes.HasPrefix(toWrite, []byte(">")) && bytes.HasPrefix(bytes.TrimLeft(toWrite, ">"), []byte("From ")) {
				toWrite = toWrite[1:]
			}
			_, _ = currentMsg.Write(toWrite)
		}
		line = resetMboxLine(line)
		if readErr == io.EOF {
			break
		}
	}

	if inMessage && currentMsg.Len() > 0 {
		appendProcessedMessage(&ids, &records, batch, cacheDir, currentMsg.Bytes(), fallbackDate, seenIDs, isAppend)
	}
	if err := batch.Close(); err != nil {
		return nil, nil, err
	}
	return ids, records, nil
}

// IngestMbox parses an on-disk mbox file and indexes its messages into cacheDir.
func IngestMbox(mboxPath string, cacheDir cachedir.Account, displayName string) ([]email.ID, error) {
	f, err := os.Open(mboxPath)
	if err != nil {
		return nil, fmt.Errorf("open mbox %q: %w", mboxPath, err)
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat mbox %q: %w", mboxPath, err)
	}
	meta, cachedIDs, ok := checkCachedMeta(cacheDir, mboxPath, fi)
	if ok {
		return cachedIDs, nil
	}

	if isEmlPath(mboxPath) {
		return ingestEml(f, fi, cacheDir, mboxPath, displayName)
	}

	if appendedIDs, handled, appendErr := tryAppendMbox(f, fi, meta, cacheDir, mboxPath, displayName); handled {
		return appendedIDs, appendErr
	}

	if _, seekErr := f.Seek(0, io.SeekStart); seekErr != nil {
		return nil, fmt.Errorf("seek mbox %q: %w", mboxPath, seekErr)
	}
	seenIDs := make(map[string]int)
	ids, records, err := parseMboxStreamWithSeen(f, cacheDir, fi.ModTime(), seenIDs, false)
	if err != nil {
		return nil, fmt.Errorf("parse mbox %q: %w", mboxPath, err)
	}
	if writeErr := writeIndexRecords(recordsPath(cacheDir.Path()), records); writeErr != nil {
		return nil, fmt.Errorf("write index records: %w", writeErr)
	}
	tail, err := computeTailHash(f, fi.Size())
	if err != nil {
		return nil, fmt.Errorf("compute tail hash: %w", err)
	}
	if err := saveMboxMeta(cacheDir, mboxPath, fi, ids, displayName, tail); err != nil {
		return nil, fmt.Errorf("save mbox metadata: %w", err)
	}
	return ids, nil
}

// isEmlPath reports whether path names a single RFC 822 message rather than
// an mbox. The suffix decides, so a message is never run through the mbox
// parser, which would drop it for lacking a From_ line and unescape >From.
func isEmlPath(path string) bool {
	return strings.EqualFold(filepath.Ext(path), ".eml")
}

// ingestEml stores a .eml file as the view's only message.
func ingestEml(f *os.File, fi os.FileInfo, cacheDir cachedir.Account, emlPath string, displayName string) ([]email.ID, error) {
	raw, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", emlPath, err)
	}
	if checkErr := checkEml(raw); checkErr != nil {
		return nil, fmt.Errorf("%q is not an email message: %w", emlPath, checkErr)
	}
	batch, err := msg.NewStoreBatch(cacheDir)
	if err != nil {
		return nil, err
	}
	defer batch.Close()
	id, _, err := processMboxMessage(batch, cacheDir, raw, fi.ModTime(), make(map[string]int), false)
	if err != nil {
		return nil, fmt.Errorf("store %q: %w", emlPath, err)
	}
	if err := batch.Close(); err != nil {
		return nil, err
	}
	ids := []email.ID{email.ID(id)}
	if err := saveMboxMeta(cacheDir, emlPath, fi, ids, displayName, ""); err != nil {
		return nil, fmt.Errorf("save metadata: %w", err)
	}
	return ids, nil
}

// checkEml requires a parsable header carrying at least one of the fields
// every real message has, so an arbitrary text file is refused.
func checkEml(raw []byte) error {
	parsed, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	for _, key := range []string{"From", "Date", "Subject", "Message-ID"} {
		if parsed.Header.Get(key) != "" {
			return nil
		}
	}
	return fmt.Errorf("no From, Date, Subject or Message-ID header")
}
