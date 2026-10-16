package common

import (
	"bytes"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
)

// KeyBodyRecord is the gin context key holding the *BodyRecord of a relay
// exchange. The record is created when body recording is enabled (global
// log_setting switch plus the per-token flag) and read back when the usage or
// error log row is written, so request_bodies rows are keyed to that row.

const KeyBodyRecord = "key_body_record"

// BodyRecord accumulates the raw request and response bodies of one relay
// exchange. Bodies are recorded whole or not at all: when either side would
// exceed MaxBytes it is skipped entirely and flagged, never truncated (see
// docs/adr/0002-full-body-or-skip.md).
type BodyRecord struct {
	Enabled        bool
	MaxBytes       int
	Format         string // response protocol shape: "openai", "claude" or ""
	StreamResponse bool   // response arrived as text/event-stream

	RequestBody    []byte
	RequestSkipped bool

	respBuf     bytes.Buffer
	RespSkipped bool
}

func NewBodyRecord(maxBytes int, format string) *BodyRecord {
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	return &BodyRecord{Enabled: true, MaxBytes: maxBytes, Format: format}
}

func SetBodyRecord(c *gin.Context, br *BodyRecord) {
	if c != nil && br != nil {
		c.Set(KeyBodyRecord, br)
	}
}

func GetBodyRecord(c *gin.Context) *BodyRecord {
	if c == nil {
		return nil
	}
	v, exists := c.Get(KeyBodyRecord)
	if !exists {
		return nil
	}
	br, _ := v.(*BodyRecord)
	return br
}

// RecordRequest stores the client request body snapshot. A snapshot larger
// than MaxBytes is skipped and flagged instead of truncated.
func (br *BodyRecord) RecordRequest(b []byte) {
	if br == nil || !br.Enabled || br.RequestBody != nil {
		return
	}
	if len(b) > br.MaxBytes {
		br.RequestSkipped = true
		return
	}
	br.RequestBody = b
}

// SnapshotRequest stores the client request body as received, before any
// protocol conversion or cross-channel retry. A body larger than MaxBytes is
// skipped and flagged instead of truncated.
func (br *BodyRecord) SnapshotRequest(storage BodyStorage) {
	if br == nil || !br.Enabled || br.RequestBody != nil || storage == nil {
		return
	}
	if storage.Size() > int64(br.MaxBytes) {
		br.RequestSkipped = true
		return
	}
	body, err := storage.Bytes()
	if err != nil || len(body) == 0 {
		return
	}
	br.RequestBody = body
}

// ResetResponse drops any response accumulated so far, so a cross-channel
// retry attempt records only its own upstream response.
func (br *BodyRecord) ResetResponse() {
	if br == nil {
		return
	}
	br.respBuf.Reset()
	br.RespSkipped = false
}

// AppendResponse accumulates one raw fragment of the upstream response. Once
// the accumulated size would exceed MaxBytes the whole side is skipped.
func (br *BodyRecord) AppendResponse(data []byte) {
	if br == nil || !br.Enabled || br.RespSkipped || len(data) == 0 {
		return
	}
	if br.respBuf.Len()+len(data) > br.MaxBytes {
		br.RespSkipped = true
		br.respBuf.Reset()
		return
	}
	br.respBuf.Write(data)
}

// ResponseBody returns the response side for storage. A streamed response is
// reassembled: OpenAI chat and Claude messages streams are merged into one
// complete JSON object, other streams keep the raw event payloads.
func (br *BodyRecord) ResponseBody() string {
	if br == nil || br.RespSkipped {
		return ""
	}
	raw := br.respBuf.String()
	if raw == "" {
		return ""
	}
	if !br.StreamResponse {
		return raw
	}
	switch br.Format {
	case "openai":
		return reassembleOpenAIStream(raw)
	case "claude":
		return reassembleClaudeStream(raw)
	default:
		return raw
	}
}

// TeeResponseBody replaces resp.Body with a reader that accumulates the bytes
// passing through into the request's BodyRecord (whole-or-skip, see BodyRecord).
func TeeResponseBody(c *gin.Context, resp io.ReadCloser, contentType string) io.ReadCloser {
	br := GetBodyRecord(c)
	if br == nil || !br.Enabled || resp == nil {
		return resp
	}
	isJSON := strings.HasPrefix(contentType, "application/json")
	isEventStream := strings.HasPrefix(contentType, "text/event-stream")
	if !isJSON && !isEventStream {
		return resp
	}
	br.StreamResponse = isEventStream
	br.ResetResponse()
	tee := io.TeeReader(resp, &bodyRecordWriter{br: br})
	return io.NopCloser(tee)
}

type bodyRecordWriter struct {
	br *BodyRecord
}

func (w *bodyRecordWriter) Write(p []byte) (int, error) {
	w.br.AppendResponse(p)
	return len(p), nil
}
