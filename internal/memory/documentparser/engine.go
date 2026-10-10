// Package documentparser is a fixed local PDF/OCR engine. Content cannot select
// commands, arguments, paths, URLs or credentials.
package documentparser

import (
	"bytes"
	"context"
	"errors"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/butler/butler/internal/memory/ingestion"
)

const MaxPages = 16
const MaxOutput = 8 << 20

var ErrHasText = errors.New("document has a usable text layer")

type Engine struct {
	slots chan struct{}
	once  sync.Once
}
type limitedBuffer struct{ b bytes.Buffer }

func (w *limitedBuffer) Write(p []byte) (int, error) {
	if w.b.Len()+len(p) > MaxOutput {
		return 0, ingestion.ErrLimit
	}
	return w.b.Write(p)
}

func (e *Engine) withFile(ctx context.Context, raw []byte, fn func(context.Context, string, string) ([]ingestion.Page, error)) ([]ingestion.Page, error) {
	if len(raw) == 0 || len(raw) > ingestion.MaxBytes || !bytes.HasPrefix(raw, []byte("%PDF-")) {
		return nil, ingestion.ErrCorrupt
	}
	e.once.Do(func() { e.slots = make(chan struct{}, 1) })
	select {
	case e.slots <- struct{}{}:
		defer func() { <-e.slots }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	dir, err := os.MkdirTemp("", "butler-document-")
	if err != nil {
		return nil, ingestion.ErrCorrupt
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "source.pdf")
	if err = os.WriteFile(path, raw, 0600); err != nil {
		return nil, ingestion.ErrCorrupt
	}
	return fn(ctx, dir, path)
}
func command(ctx context.Context, name string, args ...string) ([]byte, error) {
	// Fixed absolute executables installed in the dedicated image. Stdout and
	// stderr are never copied into public errors, logs or model context.
	c := exec.CommandContext(ctx, "/usr/bin/"+name, args...)
	var out limitedBuffer
	c.Stdout = &out
	c.Stderr = io.Discard
	err := c.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if errors.Is(err, ingestion.ErrLimit) {
		return nil, ingestion.ErrLimit
	}
	if err != nil {
		return nil, ingestion.ErrCorrupt
	}
	return out.b.Bytes(), nil
}
func textLayer(ctx context.Context, path string) ([]ingestion.Page, error) {
	raw, err := command(ctx, "pdftotext", "-layout", "-enc", "UTF-8", path, "-")
	if err != nil {
		return nil, err
	}
	if !utf8.Valid(raw) {
		return nil, ingestion.ErrCorrupt
	}
	parts := strings.Split(string(raw), "\f")
	if len(parts) > 0 && strings.TrimSpace(parts[len(parts)-1]) == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) == 0 || len(parts) > MaxPages {
		return nil, ingestion.ErrLimit
	}
	pages := make([]ingestion.Page, len(parts))
	for i, text := range parts {
		pages[i] = ingestion.Page{Text: text, Number: i + 1, Method: "poppler_text_layer"}
	}
	return pages, nil
}
func (e *Engine) TextLayer(ctx context.Context, raw []byte) ([]ingestion.Page, error) {
	return e.withFile(ctx, raw, func(ctx context.Context, _ string, path string) ([]ingestion.Page, error) {
		return textLayer(ctx, path)
	})
}
func (e *Engine) Recognize(ctx context.Context, raw []byte) ([]ingestion.Page, error) {
	return e.withFile(ctx, raw, func(ctx context.Context, dir, path string) ([]ingestion.Page, error) {
		layers, err := textLayer(ctx, path)
		if err != nil {
			return nil, err
		}
		for _, p := range layers {
			if strings.TrimSpace(p.Text) != "" {
				return nil, ErrHasText
			}
		}
		prefix := filepath.Join(dir, "page")
		if _, err = command(ctx, "pdftoppm", "-png", "-scale-to", "1600", "-r", "96", path, prefix); err != nil {
			return nil, err
		}
		files, err := filepath.Glob(prefix + "-*.png")
		if err != nil || len(files) != len(layers) || len(files) > MaxPages {
			return nil, ingestion.ErrCorrupt
		}
		pageNumber := func(path string) int {
			n, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(filepath.Base(path), "page-"), ".png"))
			return n
		}
		sort.Slice(files, func(i, j int) bool { return pageNumber(files[i]) < pageNumber(files[j]) })
		pages := make([]ingestion.Page, len(files))
		for i, file := range files {
			if pageNumber(file) != i+1 {
				return nil, ingestion.ErrCorrupt
			}
			info, err := os.Lstat(file)
			if err != nil || !info.Mode().IsRegular() || info.Size() > 16<<20 {
				return nil, ingestion.ErrLimit
			}
			f, err := os.Open(file)
			if err != nil {
				return nil, ingestion.ErrCorrupt
			}
			cfg, err := png.DecodeConfig(f)
			_ = f.Close()
			if err != nil || cfg.Width > 1600 || cfg.Height > 1600 || cfg.Width*cfg.Height > 2560000 {
				return nil, ingestion.ErrLimit
			}
			text, err := command(ctx, "tesseract", file, "stdout", "-l", "rus+eng", "--psm", "6")
			if err != nil {
				return nil, err
			}
			if !utf8.Valid(text) {
				return nil, ingestion.ErrCorrupt
			}
			pages[i] = ingestion.Page{Text: string(text), Number: i + 1, Method: "tesseract_rus_eng_v1", Uncertain: true}
		}
		return pages, nil
	})
}
