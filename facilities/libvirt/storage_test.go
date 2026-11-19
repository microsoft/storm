package libvirt

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"testing"
)

func qcow2Header(virtualSize uint64) []byte {
	h := make([]byte, 72)
	copy(h, qcow2Magic)
	binary.BigEndian.PutUint32(h[4:], 3)
	binary.BigEndian.PutUint64(h[24:], virtualSize)
	return h
}

func TestInspectImageHeader(t *testing.T) {
	q := qcow2Header(10 << 30)
	f, c, err := inspectImageHeader(bytes.NewReader(q), int64(len(q)))
	if err != nil || f != formatQcow2 || c != 10<<30 {
		t.Errorf("qcow2: %q %d %v", f, c, err)
	}

	raw := bytes.Repeat([]byte{0xAA}, 4096)
	f, c, err = inspectImageHeader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil || f != formatRaw || c != 4096 {
		t.Errorf("raw: %q %d %v", f, c, err)
	}

	// Smaller than a qcow2 header: treated as raw.
	tiny := []byte("QFI\xfb")
	f, _, err = inspectImageHeader(bytes.NewReader(tiny), int64(len(tiny)))
	if err != nil || f != formatRaw {
		t.Errorf("tiny: %q %v", f, err)
	}

	if _, _, err := inspectImageHeader(bytes.NewReader(nil), 0); err == nil {
		t.Error("empty image accepted")
	}
	zero := qcow2Header(0)
	if _, _, err := inspectImageHeader(bytes.NewReader(zero), int64(len(zero))); err == nil {
		t.Error("qcow2 with zero virtual size accepted")
	}
}

func TestCtxReaderStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := ctxReader{ctx, bytes.NewReader(make([]byte, 100))}
	buf := make([]byte, 10)
	if _, err := r.Read(buf); err != nil {
		t.Fatalf("first read: %v", err)
	}
	cancel()
	if _, err := r.Read(buf); !errors.Is(err, context.Canceled) {
		t.Fatalf("read after cancel = %v, want context.Canceled", err)
	}
}
