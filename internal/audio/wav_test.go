package audio

import "testing"

func TestCanonicalAudioBounds(t *testing.T) {
	w, e := Encode(make([]byte, Rate*2), Rate)
	if e != nil {
		t.Fatal(e)
	}
	p, e := Decode(w, Rate, 1)
	if e != nil || len(p.Samples) != Rate*2 {
		t.Fatal(e)
	}
	for _, mutation := range []func([]byte){func(b []byte) { b[22] = 2 }, func(b []byte) { b[20] = 3 }, func(b []byte) { b[40] ^= 1 }, func(b []byte) { b[24] = 0 }, func(b []byte) { b[16] = 18 }} {
		b := append([]byte(nil), w...)
		mutation(b)
		if _, e := Decode(b, Rate, 1); e == nil {
			t.Fatal("invalid header accepted")
		}
	}
	if _, e := Decode(append(w, 0), Rate, 1); e == nil {
		t.Fatal("trailing byte accepted")
	}
	if _, e := p.Segment(0, Rate+1); e == nil {
		t.Fatal("segment past original")
	}
}
