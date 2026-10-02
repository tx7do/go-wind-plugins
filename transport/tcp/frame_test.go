package tcp

import (
	"bytes"
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

// fakeConn adapts a byte slice into a net.Conn so the frame helpers can be
// exercised without a live peer. Only reads are meaningful for these tests.
type fakeConn struct {
	bytes.Reader
}

func (fakeConn) Write(b []byte) (int, error)      { return len(b), nil }
func (fakeConn) Close() error                     { return nil }
func (fakeConn) LocalAddr() net.Addr              { return nil }
func (fakeConn) RemoteAddr() net.Addr             { return nil }
func (fakeConn) SetDeadline(time.Time) error      { return nil }
func (fakeConn) SetReadDeadline(time.Time) error  { return nil }
func (fakeConn) SetWriteDeadline(time.Time) error { return nil }

// ---------------------------------------------------------------------------
// NetPacket
// ---------------------------------------------------------------------------

func TestNetPacketMarshalUnmarshal(t *testing.T) {
	tests := []struct {
		name    string
		msgType NetMessageType
		payload []byte
	}{
		{"empty payload", 1, nil},
		{"short payload", 2, []byte("hello")},
		{"max type value", 0xFFFFFFFF, []byte{0x01, 0x02}},
		{"zero type", 0, []byte("x")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// The packet wire format is little endian: 4-byte type + payload.
			in := NetPacket{Type: tc.msgType, Payload: tc.payload}
			buf, err := in.Marshal()
			if err != nil {
				t.Fatalf("Marshal() error: %v", err)
			}
			if len(buf) != 4+len(tc.payload) {
				t.Fatalf("Marshal() length = %d, want %d", len(buf), 4+len(tc.payload))
			}
			if got := binary.LittleEndian.Uint32(buf); got != uint32(tc.msgType) {
				t.Errorf("wire type = %d, want %d", got, tc.msgType)
			}

			var out NetPacket
			if err := out.Unmarshal(buf); err != nil {
				t.Fatalf("Unmarshal() error: %v", err)
			}
			if out.Type != tc.msgType {
				t.Errorf("Type = %d, want %d", out.Type, tc.msgType)
			}
			if !bytes.Equal(out.Payload, tc.payload) {
				t.Errorf("Payload = %v, want %v", out.Payload, tc.payload)
			}
		})
	}
}

func TestNetPacketUnmarshalShortInput(t *testing.T) {
	var msg NetPacket
	// Fewer than the 4 type bytes must fail instead of panicking.
	if err := msg.Unmarshal([]byte{0x01, 0x02}); err == nil {
		t.Error("Unmarshal on 2-byte input returned nil error, want failure")
	}
	if err := msg.Unmarshal(nil); err == nil {
		t.Error("Unmarshal on empty input returned nil error, want failure")
	}
}

// ---------------------------------------------------------------------------
// Frame read/write
// ---------------------------------------------------------------------------

func TestWriteFrameReadFrameRoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{"empty payload", []byte{}},
		{"small payload", []byte("hello world")},
		{"binary payload", []byte{0x00, 0xFF, 0x10, 0x00, 0xDE, 0xAD, 0xBE, 0xEF}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()

			writeErr := make(chan error, 1)
			go func() {
				writeErr <- WriteFrame(client, tc.payload)
			}()

			// Bound the read so a regression cannot hang the test.
			if err := server.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
				t.Fatalf("SetReadDeadline failed: %v", err)
			}
			got, err := ReadFrame(server)
			if err != nil {
				t.Fatalf("ReadFrame() error: %v", err)
			}
			if !bytes.Equal(got, tc.payload) {
				t.Errorf("ReadFrame() = %v, want %v", got, tc.payload)
			}
			select {
			case err := <-writeErr:
				if err != nil {
					t.Errorf("WriteFrame() error: %v", err)
				}
			case <-time.After(2 * time.Second):
				t.Fatal("WriteFrame did not return within 2s")
			}
		})
	}
}

func TestWriteFrameTooLarge(t *testing.T) {
	// A payload above maxFrameSize must be rejected before any write.
	err := WriteFrame(&fakeConn{}, make([]byte, maxFrameSize+1))
	if err == nil {
		t.Fatal("WriteFrame with oversized payload returned nil error")
	}
	if got := err.Error(); got != "frame too large" {
		t.Errorf("error = %q, want %q", got, "frame too large")
	}
}

func TestReadFrameInvalidSize(t *testing.T) {
	// A length prefix above maxFrameSize is a protocol error.
	buf := make([]byte, 4)
	binary.LittleEndian.PutUint32(buf, maxFrameSize+1)

	input := append(buf, make([]byte, 16)...)
	if _, err := ReadFrame(&fakeConn{Reader: *bytes.NewReader(input)}); err == nil {
		t.Error("ReadFrame with oversized length prefix returned nil error")
	}
}

func TestReadFrameTruncated(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"header cut short", []byte{0x02, 0x00}},
		{"payload cut short", []byte{0x05, 0x00, 0x00, 0x00, 'a', 'b'}},
		{"no data at all", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ReadFrame(&fakeConn{Reader: *bytes.NewReader(tc.input)})
			if err == nil {
				t.Error("ReadFrame on truncated input returned nil error, want EOF")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Byte order
// ---------------------------------------------------------------------------

func TestByteOrderSwitch(t *testing.T) {
	// byteOrder is a package-level global; restore little endian so the
	// other tests keep the documented default wire format.
	defer WithLittleEndian()

	WithBigEndian()
	if byteOrder != binary.BigEndian {
		t.Error("WithBigEndian did not switch the package byte order")
	}

	// In big endian mode both the packet type and the frame length prefix
	// must follow the network byte order.
	msg := NetPacket{Type: 0x01020304, Payload: []byte("ab")}
	buf, err := msg.Marshal()
	if err != nil {
		t.Fatalf("Marshal() error: %v", err)
	}
	if got := binary.BigEndian.Uint32(buf); got != 0x01020304 {
		t.Errorf("big endian wire type = %#x, want %#x", got, 0x01020304)
	}

	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	payload := []byte("xyz")
	writeErr := make(chan error, 1)
	go func() { writeErr <- WriteFrame(client, payload) }()

	if err := server.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline failed: %v", err)
	}
	hdr := make([]byte, 4)
	if _, err := io.ReadFull(server, hdr); err != nil {
		t.Fatalf("read length prefix failed: %v", err)
	}
	if size := binary.BigEndian.Uint32(hdr); size != uint32(len(payload)) {
		t.Errorf("big endian length prefix = %d, want %d", size, len(payload))
	}

	WithLittleEndian()
	if byteOrder != binary.LittleEndian {
		t.Error("WithLittleEndian did not switch the package byte order")
	}
}
