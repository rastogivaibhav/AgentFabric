package models

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// EncodeCursor encodes two opaque parts into a base64url cursor token
// that is safe to embed in query strings.
func EncodeCursor(part1, part2 string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(part1 + "|" + part2))
}

// DecodeCursor splits a cursor token back into its two constituent parts.
// Returns empty strings when the token is missing, malformed, or corrupt.
func DecodeCursor(cursor string) (part1, part2 string) {
	b, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return "", ""
	}
	parts := strings.SplitN(string(b), "|", 2)
	if len(parts) != 2 {
		return "", ""
	}
	return parts[0], parts[1]
}

// EncodeTraceCursor encodes a (start_time_ns, trace_id) keyset position.
func EncodeTraceCursor(startNs int64, traceID string) string {
	return EncodeCursor(strconv.FormatInt(startNs, 10), traceID)
}

// DecodeTraceCursor unpacks a trace cursor.
// ok is false when the cursor is absent or unparseable — callers should treat
// that as "no cursor" (i.e. start from the beginning of the result set).
func DecodeTraceCursor(cursor string) (startNs int64, traceID string, ok bool) {
	p1, p2 := DecodeCursor(cursor)
	if p1 == "" || p2 == "" {
		return 0, "", false
	}
	ns, err := strconv.ParseInt(p1, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return ns, p2, true
}

// EncodeRunCursor encodes a (start_time, run_id) keyset position.
func EncodeRunCursor(startTime time.Time, runID string) string {
	return EncodeCursor(strconv.FormatInt(startTime.UnixNano(), 10), runID)
}

// DecodeRunCursor unpacks a run cursor.
func DecodeRunCursor(cursor string) (startTime time.Time, runID string, ok bool) {
	p1, p2 := DecodeCursor(cursor)
	if p1 == "" || p2 == "" {
		return time.Time{}, "", false
	}
	ns, err := strconv.ParseInt(p1, 10, 64)
	if err != nil {
		return time.Time{}, "", false
	}
	return time.Unix(0, ns).UTC(), p2, true
}
