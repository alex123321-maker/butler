package protocol

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestGuardedFramesBoundAllocationBeforeRead(t *testing.T) {
	for _, length := range []uint32{0, 65537, 0xffffffff} {
		var b bytes.Buffer
		binary.Write(&b, binary.LittleEndian, length)
		if _, e := ReadGuardedMessage(&b); e == nil {
			t.Fatal("oversized frame accepted")
		}
	}
	var b bytes.Buffer
	if WriteResponse(&b, Response{ID: "correlated", OK: true}) != nil {
		t.Fatal("fixture frame unavailable")
	}
	value, e := ReadGuardedMessage(&b)
	if e != nil || value.(Response).ID != "correlated" {
		t.Fatal("bounded correlated frame denied", e)
	}
}
