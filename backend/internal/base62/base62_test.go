package base62

import (
	"bytes"
	"testing"
)

func TestRoundtrip(t *testing.T) {
	cases := [][]byte{
		{0x00},
		{0x01},
		{0xff},
		{0x00, 0x00, 0x01},
		[]byte("hello world"),
		bytes.Repeat([]byte{0xab}, 32),
	}
	for _, c := range cases {
		enc := Encode(c)
		dec, err := Decode(enc)
		if err != nil {
			t.Fatalf("decode(%x) err: %v", c, err)
		}
		if !bytes.Equal(dec, c) {
			t.Fatalf("roundtrip mismatch: in=%x enc=%q dec=%x", c, enc, dec)
		}
	}
}

func TestNewIDUniqueness(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := NewID()
		if len(id) < 40 {
			t.Fatalf("id too short: %d", len(id))
		}
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate id on iter %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}

func TestDecodeInvalid(t *testing.T) {
	if _, err := Decode("!!!"); err == nil {
		t.Fatalf("expected error for invalid char")
	}
}

func TestEncodeEmpty(t *testing.T) {
	if got := Encode(nil); got != "" {
		t.Fatalf("empty encode: got %q", got)
	}
}
