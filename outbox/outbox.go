// Package outbox provides an offline send queue: RFC822 messages spooled to
// disk when offline or when asynchronous queuing is requested.
package outbox

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/sarielhp/mail_cli/localdir"
)

var seqCounter uint64

// Entry describes one spooled message in the outbox.
type Entry struct {
	ID        string    `json:"id"`
	Account   string    `json:"account"`
	To        []string  `json:"to"`
	Subject   string    `json:"subject"`
	CreatedAt time.Time `json:"created_at"`
	Attempts  int       `json:"attempts"`
	LastError string    `json:"last_error,omitempty"`
}

// FlushResult summarizes the transmission outcome for one queued entry.
type FlushResult struct {
	ID      string `json:"id"`
	Account string `json:"account"`
	Subject string `json:"subject"`
	Success bool   `json:"success"`
	Err     string `json:"err,omitempty"`
}

// Dir returns the canonical outbox directory path for a download root.
func Dir(downloadDir string) string {
	return localdir.Path(downloadDir, "outbox")
}

func sanitizeID(id string) string {
	clean := filepath.Base(id)
	clean = strings.TrimSuffix(clean, ".json")
	clean = strings.TrimSuffix(clean, ".eml")
	return clean
}

// generateID names an entry by time, a per-process sequence and a random
// suffix. Time and sequence alone collided across processes: two sends in the
// same millisecond from two mail_cli processes got the same ID, and the second
// atomic write replaced the first message.
func generateID(now time.Time) string {
	seq := atomic.AddUint64(&seqCounter, 1) % 10000
	var nonce [4]byte
	_, _ = rand.Read(nonce[:])
	return fmt.Sprintf("%s-%04d-%s", now.Format("20060102-150405.000"), seq, hex.EncodeToString(nonce[:]))
}

// reserveID claims a fresh ID by creating its metadata file exclusively, so no
// other writer can take the same name even if the random suffix repeated.
func reserveID(dir string, now time.Time) (string, error) {
	for range 8 {
		id := generateID(now)
		f, err := os.OpenFile(filepath.Join(dir, id+".json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		return id, f.Close()
	}
	return "", errors.New("could not allocate a unique outbox ID")
}

// Enqueue spools an outbound message into the outbox directory.
func Enqueue(dir, account string, to []string, subject string, raw []byte) (*Entry, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("creating outbox directory: %w", err)
	}
	now := time.Now().UTC()
	id, err := reserveID(dir, now)
	if err != nil {
		return nil, fmt.Errorf("reserving outbox entry: %w", err)
	}
	entry := &Entry{
		ID:        id,
		Account:   account,
		To:        to,
		Subject:   subject,
		CreatedAt: now,
		Attempts:  0,
	}

	emlPath := filepath.Join(dir, id+".eml")
	jsonPath := filepath.Join(dir, id+".json")
	if err = atomicWrite(emlPath, raw); err != nil {
		_ = os.Remove(jsonPath)
		return nil, fmt.Errorf("writing outbox payload: %w", err)
	}

	metaBytes, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		_ = os.Remove(emlPath)
		_ = os.Remove(jsonPath)
		return nil, fmt.Errorf("marshaling outbox metadata: %w", err)
	}
	if err := atomicWrite(jsonPath, metaBytes); err != nil {
		_ = os.Remove(emlPath)
		_ = os.Remove(jsonPath)
		return nil, fmt.Errorf("writing outbox metadata: %w", err)
	}

	return entry, nil
}

// List returns all queued entries in the outbox, ordered from oldest to newest.
func List(dir string) ([]Entry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading outbox directory: %w", err)
	}

	var results []Entry
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var item Entry
		if err := json.Unmarshal(data, &item); err != nil {
			continue
		}
		if item.ID == "" {
			item.ID = sanitizeID(e.Name())
		}
		results = append(results, item)
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt.Before(results[j].CreatedAt)
	})
	return results, nil
}

// Get returns the metadata entry and raw RFC822 payload for a specific ID.
func Get(dir, id string) (*Entry, []byte, error) {
	cleanID := sanitizeID(id)
	jsonPath := filepath.Join(dir, cleanID+".json")
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading outbox entry %q: %w", cleanID, err)
	}
	var item Entry
	if unmarshalErr := json.Unmarshal(data, &item); unmarshalErr != nil {
		return nil, nil, fmt.Errorf("parsing outbox entry %q: %w", cleanID, unmarshalErr)
	}

	emlPath := filepath.Join(dir, cleanID+".eml")
	raw, err := os.ReadFile(emlPath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading outbox payload %q: %w", cleanID, err)
	}

	return &item, raw, nil
}

// Remove deletes an outbox entry and its associated raw message payload.
func Remove(dir, id string) error {
	cleanID := sanitizeID(id)
	jsonPath := filepath.Join(dir, cleanID+".json")
	emlPath := filepath.Join(dir, cleanID+".eml")
	err1 := os.Remove(jsonPath)
	err2 := os.Remove(emlPath)
	if err1 != nil && !os.IsNotExist(err1) {
		return err1
	}
	if err2 != nil && !os.IsNotExist(err2) {
		return err2
	}
	return nil
}

// RecordAttempt updates the retry counter and last error message for an entry.
func RecordAttempt(dir, id string, attemptErr error) error {
	item, raw, err := Get(dir, id)
	if err != nil {
		return err
	}
	item.Attempts++
	if attemptErr != nil {
		item.LastError = attemptErr.Error()
	} else {
		item.LastError = ""
	}
	metaBytes, err := json.MarshalIndent(item, "", "  ")
	if err != nil {
		return err
	}
	_ = raw // keep payload intact
	return atomicWrite(filepath.Join(dir, sanitizeID(id)+".json"), metaBytes)
}

func atomicWrite(dest string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, dest)
}
