// Package relay implements the ShellSync cloud relay core: the wire frame
// protocol, per-connection sessions, and the hub that matches pairing codes
// to daemons and pipes multiplexed byte streams between phones and daemons.
//
// Wire protocol (v1, see doc/跨网络改造/跨网络配对方案.md §5.2):
//
//	Control frames — WebSocket TEXT messages, JSON:
//	  {"t":"hello",  "role":"daemon"|"client", "ver":1}
//	  {"t":"reg",    "devId":"…", "sign":"<HMAC-SHA256 hex>"}
//	  {"t":"code",   "code":"482913", "ttl":120}          // daemon → relay
//	  {"t":"claim",  "code":"482913"}                      // client → relay
//	  {"t":"claim",  "devId":"…"}                          // relay → client (ack)
//	  {"t":"open",   "devId":"…"}                          // client → relay
//	  {"t":"open",   "streamId":7}                         // relay → both sides
//	  {"t":"accept", "streamId":7}                         // daemon → relay → client
//	  {"t":"close",  "streamId":7, "why":"…"}              // either side
//	  {"t":"error",  "code":"rate_limited"|…, "why":"…"}   // relay → peer
//
//	Data frames — WebSocket BINARY messages:
//	  [streamId:4B big-endian][len:4B big-endian][payload ≤ 32KiB]
//
// The relay never inspects tunnel payloads: it routes opaque bytes by
// streamId only. Business authentication (bearer tokens) happens inside the
// tunnel between phone and daemon.
package relay

import (
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
)

// Protocol version negotiated in the hello frame.
const ProtocolVersion = 1

// Control frame types.
const (
	THello  = "hello"
	TReg    = "reg"
	TCode   = "code"
	TClaim  = "claim"
	TOpen   = "open"
	TAccept = "accept"
	TClose  = "close"
	TError  = "error"
)

// Connection roles from the hello frame.
const (
	RoleDaemon = "daemon"
	RoleClient = "client"
)

// MaxChunk is the largest payload the frame encoder accepts. Writers should
// chunk larger buffers (io.Copy-style 32KiB reads naturally satisfy this).
const MaxChunk = 32 * 1024

// maxControlSize bounds control (JSON text) messages.
const maxControlSize = 16 * 1024

// Frame is a decoded control frame. Field "code" doubles as the pairing code
// (TCode/TClaim) and the error code (TError) — distinguished by T.
type Frame struct {
	T        string `json:"t"`
	Role     string `json:"role,omitempty"`
	Ver      int    `json:"ver,omitempty"`
	DevID    string `json:"devId,omitempty"`
	Sign     string `json:"sign,omitempty"`
	Nonce    string `json:"nonce,omitempty"`
	Ts       int64  `json:"ts,omitempty"`
	Code     string `json:"code,omitempty"`
	TTL      int    `json:"ttl,omitempty"`
	StreamID uint32 `json:"streamId,omitempty"`
	Why      string `json:"why,omitempty"`
}

// EncodeControl marshals a control frame to JSON text bytes.
func EncodeControl(f Frame) ([]byte, error) {
	if f.T == "" {
		return nil, errors.New("relay: control frame missing t")
	}
	return json.Marshal(f)
}

// DecodeControl parses a control frame. Errors on empty/oversized/invalid
// JSON or a missing/unknown t.
func DecodeControl(b []byte) (Frame, error) {
	if len(b) == 0 {
		return Frame{}, errors.New("relay: empty control frame")
	}
	if len(b) > maxControlSize {
		return Frame{}, fmt.Errorf("relay: control frame too large (%d bytes)", len(b))
	}
	var f Frame
	if err := json.Unmarshal(b, &f); err != nil {
		return Frame{}, fmt.Errorf("relay: bad control frame JSON: %w", err)
	}
	switch f.T {
	case THello, TReg, TCode, TClaim, TOpen, TAccept, TClose, TError:
		return f, nil
	default:
		return Frame{}, fmt.Errorf("relay: unknown frame type %q", f.T)
	}
}

// EncodeData builds a binary data frame: [streamId:4B][len:4B][payload].
func EncodeData(streamID uint32, payload []byte) ([]byte, error) {
	if len(payload) > MaxChunk {
		return nil, fmt.Errorf("relay: data chunk %d bytes exceeds max %d", len(payload), MaxChunk)
	}
	out := make([]byte, 8+len(payload))
	binary.BigEndian.PutUint32(out[0:4], streamID)
	binary.BigEndian.PutUint32(out[4:8], uint32(len(payload)))
	copy(out[8:], payload)
	return out, nil
}

// DecodeData parses a binary data frame.
func DecodeData(b []byte) (streamID uint32, payload []byte, err error) {
	if len(b) < 8 {
		return 0, nil, fmt.Errorf("relay: data frame too short (%d bytes)", len(b))
	}
	streamID = binary.BigEndian.Uint32(b[0:4])
	n := binary.BigEndian.Uint32(b[4:8])
	if uint64(n) > MaxChunk {
		return 0, nil, fmt.Errorf("relay: data frame length %d exceeds max %d", n, MaxChunk)
	}
	if int(n) != len(b)-8 {
		return 0, nil, fmt.Errorf("relay: data frame length mismatch (hdr %d, got %d)", n, len(b)-8)
	}
	return streamID, b[8:], nil
}
