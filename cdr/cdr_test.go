// Copyright (c) 2026 Matt Jones. All rights reserved.
// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package cdr_test

//fusa:test REQ-CDR-004
//fusa:test REQ-CDR-005
//fusa:test REQ-CDR-006

import (
	"math"
	"testing"

	"github.com/SoundMatt/go-DDS/cdr"
)

func roundtrip(t *testing.T, encode func(*cdr.Encoder), decode func(*cdr.Decoder)) {
	t.Helper()
	e := cdr.NewEncoder()
	encode(e)
	d, err := cdr.NewDecoder(e.Bytes())
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	decode(d)
}

func TestBool(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteBool(true); e.WriteBool(false) },
		func(d *cdr.Decoder) {
			v, err := d.ReadBool()
			if err != nil || !v {
				t.Errorf("ReadBool true: got %v, %v", v, err)
			}
			v, err = d.ReadBool()
			if err != nil || v {
				t.Errorf("ReadBool false: got %v, %v", v, err)
			}
		},
	)
}

func TestUint8(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteUint8(0xFF) },
		func(d *cdr.Decoder) {
			v, err := d.ReadUint8()
			if err != nil || v != 0xFF {
				t.Errorf("ReadUint8: got %v, %v", v, err)
			}
		},
	)
}

func TestInt16(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteInt16(-1234) },
		func(d *cdr.Decoder) {
			v, err := d.ReadInt16()
			if err != nil || v != -1234 {
				t.Errorf("ReadInt16: got %v, %v", v, err)
			}
		},
	)
}

func TestUint16(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteUint16(0xABCD) },
		func(d *cdr.Decoder) {
			v, err := d.ReadUint16()
			if err != nil || v != 0xABCD {
				t.Errorf("ReadUint16: got %v, %v", v, err)
			}
		},
	)
}

func TestInt32(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteInt32(-123456789) },
		func(d *cdr.Decoder) {
			v, err := d.ReadInt32()
			if err != nil || v != -123456789 {
				t.Errorf("ReadInt32: got %v, %v", v, err)
			}
		},
	)
}

func TestUint32(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteUint32(0xDEADBEEF) },
		func(d *cdr.Decoder) {
			v, err := d.ReadUint32()
			if err != nil || v != 0xDEADBEEF {
				t.Errorf("ReadUint32: got %v, %v", v, err)
			}
		},
	)
}

func TestInt64(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteInt64(-9876543210) },
		func(d *cdr.Decoder) {
			v, err := d.ReadInt64()
			if err != nil || v != -9876543210 {
				t.Errorf("ReadInt64: got %v, %v", v, err)
			}
		},
	)
}

func TestUint64(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteUint64(0xCAFEBABEDEADBEEF) },
		func(d *cdr.Decoder) {
			v, err := d.ReadUint64()
			if err != nil || v != 0xCAFEBABEDEADBEEF {
				t.Errorf("ReadUint64: got %v, %v", v, err)
			}
		},
	)
}

func TestFloat32(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteFloat32(3.14) },
		func(d *cdr.Decoder) {
			v, err := d.ReadFloat32()
			if err != nil || math.Abs(float64(v-3.14)) > 0.001 {
				t.Errorf("ReadFloat32: got %v, %v", v, err)
			}
		},
	)
}

func TestFloat64(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteFloat64(2.718281828) },
		func(d *cdr.Decoder) {
			v, err := d.ReadFloat64()
			if err != nil || math.Abs(v-2.718281828) > 1e-9 {
				t.Errorf("ReadFloat64: got %v, %v", v, err)
			}
		},
	)
}

func TestString(t *testing.T) {
	cases := []string{"", "hello", "go-DDS IDL", "unicode: 日本語"}
	for _, s := range cases {
		t.Run(s, func(t *testing.T) {
			roundtrip(t,
				func(e *cdr.Encoder) { e.WriteString(s) },
				func(d *cdr.Decoder) {
					v, err := d.ReadString()
					if err != nil || v != s {
						t.Errorf("ReadString: got %q, %v; want %q", v, err, s)
					}
				},
			)
		})
	}
}

func TestBytes(t *testing.T) {
	data := []byte{0x00, 0x01, 0xFF, 0x80}
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteBytes(data) },
		func(d *cdr.Decoder) {
			v, err := d.ReadBytes()
			if err != nil {
				t.Fatalf("ReadBytes: %v", err)
			}
			if len(v) != len(data) {
				t.Fatalf("len: got %d, want %d", len(v), len(data))
			}
			for i := range data {
				if v[i] != data[i] {
					t.Errorf("byte[%d]: got %d, want %d", i, v[i], data[i])
				}
			}
		},
	)
}

func TestAlignment(t *testing.T) {
	// Write a bool (1 byte) then an int32 (must be 4-byte aligned).
	roundtrip(t,
		func(e *cdr.Encoder) {
			e.WriteBool(true)
			e.WriteInt32(42)
		},
		func(d *cdr.Decoder) {
			b, bErr := d.ReadBool()
			n, nErr := d.ReadInt32()
			if bErr != nil || !b || n != 42 || nErr != nil {
				t.Errorf("alignment: got bool=%v bErr=%v int32=%v nErr=%v", b, bErr, n, nErr)
			}
		},
	)
}

func TestMultiField(t *testing.T) {
	// Simulate a struct with mixed types.
	roundtrip(t,
		func(e *cdr.Encoder) {
			e.WriteString("ECU-1")
			e.WriteFloat64(95.5)
			e.WriteInt64(1749000000000)
			e.WriteBool(true)
			e.WriteUint32(7)
		},
		func(d *cdr.Decoder) {
			s, err := d.ReadString()
			if err != nil {
				t.Fatalf("ReadString: %v", err)
			}
			f, err := d.ReadFloat64()
			if err != nil {
				t.Fatalf("ReadFloat64: %v", err)
			}
			ts, err := d.ReadInt64()
			if err != nil {
				t.Fatalf("ReadInt64: %v", err)
			}
			ok, err := d.ReadBool()
			if err != nil {
				t.Fatalf("ReadBool: %v", err)
			}
			n, err := d.ReadUint32()
			if err != nil {
				t.Fatalf("ReadUint32: %v", err)
			}
			if s != "ECU-1" || math.Abs(f-95.5) > 1e-9 || ts != 1749000000000 || !ok || n != 7 {
				t.Errorf("multi-field: got s=%q f=%v ts=%v ok=%v n=%v", s, f, ts, ok, n)
			}
		},
	)
}

func TestDecoder_TooShort(t *testing.T) {
	d, err := cdr.NewDecoder([]byte{0x00}) // shorter than 4-byte header
	if d != nil {
		t.Error("expected nil decoder on error")
	}
	if err == nil {
		t.Error("expected error for short buffer")
	}
}

func TestDecoder_BadScheme(t *testing.T) {
	d, err := cdr.NewDecoder([]byte{0xFF, 0xFF, 0x00, 0x00})
	if d != nil {
		t.Error("expected nil decoder on error")
	}
	if err == nil {
		t.Error("expected error for unsupported scheme")
	}
}

func TestEncoder_Len(t *testing.T) {
	e := cdr.NewEncoder()
	initial := e.Len()
	if initial != 4 { // encapsulation header is 4 bytes
		t.Errorf("initial Len: got %d, want 4", initial)
	}
	e.WriteInt32(42)
	if e.Len() != 8 {
		t.Errorf("after WriteInt32 Len: got %d, want 8", e.Len())
	}
}

func TestEncoder_WriteInt8(t *testing.T) {
	roundtrip(t,
		func(e *cdr.Encoder) { e.WriteInt8(-7) },
		func(d *cdr.Decoder) {
			v, err := d.ReadInt8()
			if err != nil {
				t.Fatalf("ReadInt8: %v", err)
			}
			if v != -7 {
				t.Errorf("got %d, want -7", v)
			}
		},
	)
}

func TestDecoder_Remaining(t *testing.T) {
	e := cdr.NewEncoder()
	e.WriteInt32(1)
	e.WriteInt32(2)
	d, err := cdr.NewDecoder(e.Bytes())
	if err != nil {
		t.Fatalf("NewDecoder: %v", err)
	}
	before := d.Remaining()
	if before != 8 { // two int32 fields = 8 bytes
		t.Errorf("Remaining before read: got %d, want 8", before)
	}
	_, readErr := d.ReadInt32()
	if readErr != nil {
		t.Fatalf("ReadInt32: %v", readErr)
	}
	after := d.Remaining()
	if after != 4 {
		t.Errorf("Remaining after one read: got %d, want 4", after)
	}
}

func TestDecoder_Underrun(t *testing.T) {
	e := cdr.NewEncoder()
	e.WriteInt32(1)
	data := e.Bytes()
	d, newErr := cdr.NewDecoder(data)
	if newErr != nil {
		t.Fatalf("NewDecoder: %v", newErr)
	}
	v, readErr := d.ReadInt32()
	_ = v
	if readErr != nil {
		t.Fatalf("first ReadInt32: %v", readErr)
	}
	v2, err := d.ReadInt32() // underrun
	_ = v2
	if err == nil {
		t.Error("expected error on underrun")
	}
}

// TestDecoder_Underrun_AllTypes covers the need() error return branch in every
// typed Read* method that has no prior underrun test.
func TestDecoder_Underrun_AllTypes(t *testing.T) {
	// emptyDecoder returns a Decoder backed by a CDR buffer with no payload.
	emptyDecoder := func(t *testing.T) *cdr.Decoder {
		t.Helper()
		d, err := cdr.NewDecoder(cdr.NewEncoder().Bytes())
		if err != nil {
			t.Fatalf("NewDecoder: %v", err)
		}
		return d
	}

	t.Run("ReadBool", func(t *testing.T) {
		_, err := emptyDecoder(t).ReadBool()
		if err == nil {
			t.Fatal("expected underrun error")
		}
	})
	t.Run("ReadUint8", func(t *testing.T) {
		_, err := emptyDecoder(t).ReadUint8()
		if err == nil {
			t.Fatal("expected underrun error")
		}
	})
	t.Run("ReadInt16", func(t *testing.T) {
		_, err := emptyDecoder(t).ReadInt16()
		if err == nil {
			t.Fatal("expected underrun error")
		}
	})
	t.Run("ReadUint16", func(t *testing.T) {
		_, err := emptyDecoder(t).ReadUint16()
		if err == nil {
			t.Fatal("expected underrun error")
		}
	})
	t.Run("ReadString_data", func(t *testing.T) {
		// Write a uint32 length = 4, but no actual string bytes follow.
		e := cdr.NewEncoder()
		e.WriteUint32(4)
		d, err := cdr.NewDecoder(e.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := d.ReadString()
		if readErr == nil {
			t.Fatal("expected underrun error for string data")
		}
	})
	t.Run("ReadString_zero_length", func(t *testing.T) {
		// CDR string with explicit length=0 covers the n==0 defensive branch.
		e := cdr.NewEncoder()
		e.WriteUint32(0)
		d, err := cdr.NewDecoder(e.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		s, readErr := d.ReadString()
		if readErr != nil {
			t.Fatalf("ReadString: %v", readErr)
		}
		if s != "" {
			t.Errorf("got %q, want empty string", s)
		}
	})
	t.Run("ReadBytes_data", func(t *testing.T) {
		// Write a uint32 length = 4, but no actual bytes follow.
		e := cdr.NewEncoder()
		e.WriteUint32(4)
		d, err := cdr.NewDecoder(e.Bytes())
		if err != nil {
			t.Fatal(err)
		}
		_, readErr := d.ReadBytes()
		if readErr == nil {
			t.Fatal("expected underrun error for bytes data")
		}
	})
}
