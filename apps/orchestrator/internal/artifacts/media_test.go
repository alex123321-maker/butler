package artifacts

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/png"
	"testing"
)

func TestCommunicationMediaValidation(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 32, 24))); err != nil {
		t.Fatal(err)
	}
	h := sha256.Sum256(b.Bytes())
	good := Media{Name: "picture.png", MIME: "image/png", SHA256: hex.EncodeToString(h[:]), Data: b.Bytes()}
	if good.Validate() != nil {
		t.Fatal("valid retained image rejected")
	}
	for _, kind := range []string{"path", "header", "mime", "digest", "empty", "oversize", "corrupt-image"} {
		t.Run(kind, func(t *testing.T) {
			m := good
			switch kind {
			case "path":
				m.Name = "../picture.png"
			case "header":
				m.Name = "file\r\nInjected: x"
			case "mime":
				m.MIME = "image/svg+xml"
			case "digest":
				m.SHA256 = "different"
			case "empty":
				m.Data = nil
			case "oversize":
				m.Data = bytes.Repeat([]byte{1}, MaxMediaBytes+1)
			case "corrupt-image":
				m.Data = []byte("not an image")
				sum := sha256.Sum256(m.Data)
				m.SHA256 = hex.EncodeToString(sum[:])
			}
			if m.Validate() == nil {
				t.Fatal("invalid media accepted", kind)
			}
		})
	}
	repo := newMemoryRepo()
	if _, err := NewService(repo).SaveMedia(context.Background(), "run", "session", good.Name, good.MIME, good.Data); err == nil || len(repo.items) != 0 {
		t.Fatal("media save without owned run")
	}
}
