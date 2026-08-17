package relay

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestControlRoundtrip(t *testing.T) {
	frames := []Frame{
		{T: THello, Role: RoleClient, Ver: ProtocolVersion},
		{T: THello, Role: RoleDaemon, Ver: ProtocolVersion},
		{T: TReg, DevID: "a3f9c2", Sign: "deadbeef", Nonce: "n", Ts: 1234567890},
		{T: TCode, Code: "482913", TTL: 120},
		{T: TClaim, Code: "482913"},
		{T: TClaim, DevID: "a3f9c2"},
		{T: TOpen, DevID: "a3f9c2"},
		{T: TOpen, StreamID: 7},
		{T: TAccept, StreamID: 7},
		{T: TClose, StreamID: 7, Why: "eof"},
		{T: TError, Code: "rate_limited", Why: "too many claims"},
	}
	for _, f := range frames {
		b, err := EncodeControl(f)
		if err != nil {
			t.Fatalf("encode %+v: %v", f, err)
		}
		got, err := DecodeControl(b)
		if err != nil {
			t.Fatalf("decode %s: %v", b, err)
		}
		if got != f {
			t.Fatalf("roundtrip mismatch:\n want %+v\n got  %+v", f, got)
		}
	}
}

func TestDecodeControlErrors(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"empty", "", "empty"},
		{"bad json", "{nope", "JSON"},
		{"missing t", `{"role":"client"}`, "unknown frame type"},
		{"unknown t", `{"t":"wat"}`, "unknown frame type"},
		{"wrong type t", `{"t":42}`, "JSON"},
	}
	for _, c := range cases {
		if _, err := DecodeControl([]byte(c.raw)); err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want error containing %q", c.name, err, c.want)
		}
	}
	if _, err := DecodeControl(make([]byte, maxControlSize+1)); err == nil {
		t.Error("oversized control frame accepted")
	}
	if _, err := EncodeControl(Frame{}); err == nil {
		t.Error("frame without t accepted")
	}
}

func TestDataRoundtrip(t *testing.T) {
	payloads := [][]byte{
		{},
		[]byte("hello daemon"),
		make([]byte, MaxChunk),
	}
	for i, p := range payloads {
		b, err := EncodeData(42, p)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		id, got, err := DecodeData(b)
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		if id != 42 || string(got) != string(p) {
			t.Fatalf("case %d mismatch: id=%d len=%d", i, id, len(got))
		}
	}
}

func TestDataDecodeErrors(t *testing.T) {
	bad := [][]byte{
		nil,
		{0, 0, 0, 1, 0},                    // too short
		{0, 0, 0, 1, 0, 0, 0xFF, 0xFF},     // length > MaxChunk
		{0, 0, 0, 1, 0, 0, 0, 5, 'a', 'b'}, // length mismatch
	}
	for i, b := range bad {
		if _, _, err := DecodeData(b); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	if _, err := EncodeData(1, make([]byte, MaxChunk+1)); err == nil {
		t.Error("oversized chunk accepted")
	}
}

// TestDataFrameJSONSeparation guards against a control frame accidentally
// being parseable as data (and vice versa): JSON text starts with '{' (0x7B).
func TestDataFrameJSONSeparation(t *testing.T) {
	ctrl, _ := EncodeControl(Frame{T: THello, Role: RoleClient, Ver: 1})
	if _, _, err := DecodeData(ctrl); err == nil {
		t.Error("control frame decoded as data frame")
	}
	data, _ := EncodeData(1, []byte("x"))
	if _, err := DecodeControl(data); err == nil {
		t.Error("data frame decoded as control frame")
	}
	var f Frame
	if err := json.Unmarshal(data, &f); err == nil {
		t.Error("data frame parsed as JSON control frame")
	}
}
