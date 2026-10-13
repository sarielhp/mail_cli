// Package message_info stores the cache's per-message index: where a message
// sits on disk, and what classification it has been given.
//
// The file is an append-only log of records, rewritten whole when stale
// entries pile up. Records are variable length because message IDs are:
// Gmail's are 16 characters, JMAP's a dozen or so, and Microsoft Graph's run to
// 152 and share an 87-character prefix within a mailbox.
//
// That last case is why the format changed. Version 1 packed the ID into a
// fixed [36]byte and silently truncated anything longer, so every Outlook
// message in an account collided onto one key: storing a message overwrote the
// previous one's entry, and lookups resolved to the wrong file or to none.
// Version 2 records the ID's length and writes it whole.
//
// Version 3 separates a message's spam evidence from the user's decision.
// Version 2 kept one verdict, and every move, re-scan and inbox re-label had
// to clear it without losing the user's: a rule each new code path had to
// remember, and one that was missed. Now the scan writes only the evidence
// (which checks fired, bogofilter's score, and a stamp saying what it was
// computed against) and the user writes only the override, so neither can
// overwrite the other.
package message_info

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

// Compact is what the index remembers about one message: the date it was
// received, packed as YYYYMMDD, and its classification. Checks, BogoScore and
// Stamp hold the scan's evidence and mean nothing unless FlagScanned is set;
// Override is the user's decision and is independent of them.
type Compact struct {
	Date  uint32
	Flags uint8
	// Override is the user's decision, OverrideNone when they made none.
	Override uint8
	// Checks are the spam checks that fired, a bit set owned by cache/msg.
	Checks uint8
	// BogoScore is bogofilter's spamicity, 0 to 1.
	BogoScore float32
	// Stamp identifies the filter and settings the evidence was computed
	// against; evidence with another stamp is stale.
	Stamp uint32
}

const (
	flagIsRead uint8 = 1 << iota
	flagHasICS
	// FlagScanned says Checks, BogoScore and Stamp hold evidence.
	FlagScanned
)

// The user's decision about a message.
const (
	OverrideNone uint8 = iota
	OverrideHam
	OverrideSpam
)

// WithoutEvidence returns c with its scan evidence removed and the user's
// override, read state and calendar marker kept.
func (c Compact) WithoutEvidence() Compact {
	return Compact{Date: c.Date, Flags: c.Flags &^ FlagScanned, Override: c.Override}
}

// Version 2 and 1 records kept one verdict in the flag byte. Only the part
// that was the user's survives conversion; the rest is re-scanned.
const (
	v2IsSpam     uint8 = 1 << 2
	v2Classified uint8 = 1 << 5
	v2SourceUser uint8 = 3
)

func fromV2(date uint32, flags uint8) Compact {
	c := Compact{Date: date, Flags: flags & (flagIsRead | flagHasICS)}
	if flags&v2Classified != 0 && flags>>6 == v2SourceUser {
		c.Override = OverrideHam
		if flags&v2IsSpam != 0 {
			c.Override = OverrideSpam
		}
	}
	return c
}

const (
	// legacyVersion wrote a fixed 46-byte record with a truncated [36]byte ID.
	legacyVersion    = 0x01
	legacyEntrySize  = 46
	legacyIDSize     = 36
	legacyPayloadLen = legacyEntrySize - 1

	// v2Version is a length-prefixed ID followed by date, flags and a score.
	v2Version = 0x02

	// Version is the current record format: a length-prefixed ID followed by
	// date, flags, override, checks, bogofilter score and stamp.
	Version = 0x03

	// maxIDLen is a sanity bound, not a format limit. A record claiming more
	// than this is a corrupt or misaligned file rather than a real ID.
	maxIDLen = 4096
)

// payloadSize is the fixed part of a record after the ID.
const (
	payloadSize   = 4 + 1 + 1 + 1 + 4 + 4
	v2PayloadSize = 4 + 1 + 4
)

// WriteOne appends one record: version, ID length, ID, then the payload.
func WriteOne(w io.Writer, id string, c Compact) error {
	if id == "" {
		return errors.New("message_info: refusing to write an empty ID")
	}
	if len(id) > maxIDLen {
		return fmt.Errorf("message_info: ID of %d bytes exceeds the %d-byte bound", len(id), maxIDLen)
	}

	buf := make([]byte, 0, 3+len(id)+payloadSize)
	buf = append(buf, Version)
	buf = binary.BigEndian.AppendUint16(buf, uint16(len(id)))
	buf = append(buf, id...)
	buf = binary.BigEndian.AppendUint32(buf, c.Date)
	buf = append(buf, c.Flags, c.Override, c.Checks)
	// The score is a float and is stored as one. Version 1 wrote it as an
	// integer, which truncated 6.75 to 6 on its way to disk while the
	// in-memory value kept its fraction, so a score changed across a restart.
	buf = binary.BigEndian.AppendUint32(buf, math.Float32bits(c.BogoScore))
	buf = binary.BigEndian.AppendUint32(buf, c.Stamp)

	_, err := w.Write(buf)
	return err
}

// ErrDamaged reports that ReadAll stopped on a byte that is not a valid
// record -- a bad version tag, or a record truncated partway through (a torn
// O_APPEND write, a crash mid-flush, a flipped bit). The map ReadAll returns
// alongside it holds every record read before the damage and is safe to use;
// what it is not safe to do is treat the file itself as clean afterward. The
// append path only ever adds to the end of the file, so anything appended
// after undetected damage is bytes that ReadAll will never reach again on a
// later read -- silently and permanently lost the next time the index is
// compacted from that (incomplete) map. A caller that gets ErrDamaged should
// rewrite the file from the salvaged map before allowing any further append,
// so the appended tail lands after a clean file rather than after garbage.
var ErrDamaged = errors.New("message_info: file contains a corrupted record")

// Stats describes the log behind an index rather than the index itself.
type Stats struct {
	// Records counts every record in the file, superseded ones included.
	Records int
	// Old counts version 1 and 2 records, which are converted as they are read.
	Old int
}

// ReadAll reads every record, accepting every format version so an index
// written before a change still loads.
func ReadAll(r io.Reader) (map[string]Compact, error) {
	result, _, err := readAll(r)
	return result, err
}

func readAll(r io.Reader) (map[string]Compact, Stats, error) {
	result := make(map[string]Compact)
	var stats Stats
	br := newByteReader(r)

	for {
		version, err := br.ReadByte()
		if err == io.EOF {
			return result, stats, nil
		}
		if err != nil {
			return nil, stats, fmt.Errorf("reading message_info: %w", err)
		}

		var id string
		var c Compact
		switch version {
		case Version:
			id, c, err = readCurrentRecord(br)
		case v2Version:
			id, c, err = readV2Record(br)
		case legacyVersion:
			id, c, err = readLegacyRecord(br)
		default:
			// A byte that is neither version means the file is damaged from
			// here on. Returning what was read keeps the salvageable entries.
			return result, stats, ErrDamaged
		}
		if err != nil {
			return result, stats, ErrDamaged
		}
		stats.Records++
		if version != Version {
			stats.Old++
		}
		result[id] = c
	}
}

func readCurrentRecord(br *byteReader) (string, Compact, error) {
	id, rest, err := readIDAndPayload(br, payloadSize)
	if err != nil {
		return "", Compact{}, err
	}
	return id, Compact{
		Date:      binary.BigEndian.Uint32(rest[0:4]),
		Flags:     rest[4],
		Override:  rest[5],
		Checks:    rest[6],
		BogoScore: math.Float32frombits(binary.BigEndian.Uint32(rest[7:11])),
		Stamp:     binary.BigEndian.Uint32(rest[11:15]),
	}, nil
}

// readV2Record reads a version 2 record, keeping only the user's verdict.
func readV2Record(br *byteReader) (string, Compact, error) {
	id, rest, err := readIDAndPayload(br, v2PayloadSize)
	if err != nil {
		return "", Compact{}, err
	}
	return id, fromV2(binary.BigEndian.Uint32(rest[0:4]), rest[4]), nil
}

func readIDAndPayload(br *byteReader, size int) (string, []byte, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(br, lenBuf[:]); err != nil {
		return "", nil, err
	}
	idLen := int(binary.BigEndian.Uint16(lenBuf[:]))
	if idLen == 0 || idLen > maxIDLen {
		return "", nil, fmt.Errorf("implausible ID length %d", idLen)
	}
	buf := make([]byte, idLen+size)
	if _, err := io.ReadFull(br, buf); err != nil {
		return "", nil, err
	}
	return string(buf[:idLen]), buf[idLen:], nil
}

// readLegacyRecord reads a version 1 record. Its ID was zero-padded to 36
// bytes; its verdict carried no source, so none of it is kept.
func readLegacyRecord(br *byteReader) (string, Compact, error) {
	buf := make([]byte, legacyPayloadLen)
	if _, err := io.ReadFull(br, buf); err != nil {
		return "", Compact{}, err
	}
	id := trimZeros(buf[:legacyIDSize])
	if id == "" {
		return "", Compact{}, errors.New("legacy record has an empty ID")
	}
	rest := buf[legacyIDSize:]
	return id, fromV2(binary.BigEndian.Uint32(rest[0:4]), rest[4]), nil
}

func trimZeros(b []byte) string {
	for i, c := range b {
		if c == 0 {
			return string(b[:i])
		}
	}
	return string(b)
}

// byteReader adapts any reader to the ReadByte the record loop needs.
type byteReader struct {
	io.Reader
	br  io.ByteReader
	one [1]byte
}

func newByteReader(r io.Reader) *byteReader {
	br, _ := r.(io.ByteReader)
	return &byteReader{Reader: r, br: br}
}

func (b *byteReader) ReadByte() (byte, error) {
	if b.br != nil {
		return b.br.ReadByte()
	}
	if _, err := io.ReadFull(b.Reader, b.one[:]); err != nil {
		if err == io.ErrUnexpectedEOF {
			return 0, io.EOF
		}
		return 0, err
	}
	return b.one[0], nil
}

// Entry pairs a message ID with its Compact, which is the unit the log is
// written in.
type Entry struct {
	ID      string
	Compact Compact
}

// AppendToFile appends one record to the log at path, creating the file if
// needed. It takes no lock of its own: writers must serialize their appends.
func AppendToFile(path string, id string, c Compact) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open message_info: %w", err)
	}
	defer f.Close()
	return WriteOne(f, id, c)
}

// AppendBatchToFile appends every entry to the log at path in one buffered
// pass, creating the file if needed. An empty batch writes nothing, and a
// failure part-way through leaves the entries already written in place.
func AppendBatchToFile(path string, entries []Entry) error {
	if len(entries) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("open message_info: %w", err)
	}
	defer f.Close()
	bw := bufio.NewWriterSize(f, 64*1024)
	for _, e := range entries {
		if err := WriteOne(bw, e.ID, e.Compact); err != nil {
			return err
		}
	}
	return bw.Flush()
}

// ReadFileStats reads the index at path and says what its log holds. On
// ErrDamaged the map and stats cover the records before the damage.
func ReadFileStats(path string) (map[string]Compact, Stats, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]Compact), Stats{}, nil
		}
		return nil, Stats{}, fmt.Errorf("open message_info: %w", err)
	}
	defer f.Close()
	return readAll(bufio.NewReaderSize(f, 64*1024))
}

// ReadFromFile reads the whole log at path into a map keyed by message ID. A
// missing file reads as an empty index; a damaged log returns the records read
// before the damage together with ErrDamaged.
func ReadFromFile(path string) (map[string]Compact, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string]Compact), nil
		}
		return nil, fmt.Errorf("open message_info: %w", err)
	}
	defer f.Close()
	return ReadAll(bufio.NewReaderSize(f, 64*1024))
}

// RewriteFile replaces the whole log with entries, atomically.
//
// It previously used os.Create, which truncates in place: a reader or a crash
// during compaction of a large index saw a short file, and because the loader
// treats a read error as "empty cache", the next write compacted an empty map
// over it. Writing to a temp file in the same directory and renaming means the
// old contents stay intact until the new file is complete.
func RewriteFile(path string, entries map[string]Compact) error {
	tmpPath := fmt.Sprintf("%s.tmp-%d", path, os.Getpid())
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("create message_info temp: %w", err)
	}
	cleanup := func() { _ = f.Close(); _ = os.Remove(tmpPath) }
	bw := bufio.NewWriterSize(f, 64*1024)
	for id, c := range entries {
		if err := WriteOne(bw, id, c); err != nil {
			cleanup()
			return err
		}
	}
	if err := bw.Flush(); err != nil {
		cleanup()
		return fmt.Errorf("flush message_info temp: %w", err)
	}
	if err := f.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync message_info temp: %w", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close message_info temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename message_info: %w", err)
	}
	return nil
}
