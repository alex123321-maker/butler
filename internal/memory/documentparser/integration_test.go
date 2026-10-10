package documentparser

import (
	"bytes"
	"compress/zlib"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/memory/ingestion"
)

func pdf(objects []string) []byte {
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	positions := []int{0}
	for i, obj := range objects {
		positions = append(positions, b.Len())
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, obj)
	}
	start := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(positions))
	for _, p := range positions[1:] {
		fmt.Fprintf(&b, "%010d 00000 n \n", p)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF", len(positions), start)
	return b.Bytes()
}
func stream(raw []byte, dict string) string {
	return fmt.Sprintf("<< /Length %d %s >>\nstream\n%s\nendstream", len(raw), dict, raw)
}
func textFixture() []byte {
	text := "Русский документ"
	var encoded, mapping strings.Builder
	for _, r := range text {
		fmt.Fprintf(&encoded, "%04X", r)
		fmt.Fprintf(&mapping, "<%04X> <%04X>\n", r, r)
	}
	cmap := fmt.Sprintf("/CIDInit /ProcSet findresource begin 12 dict begin begincmap /CMapType 2 def 1 begincodespacerange <0000> <FFFF> endcodespacerange %d beginbfchar\n%s endbfchar endcmap CMapName currentdict /CMap defineresource pop end end", len([]rune(text)), mapping.String())
	content := []byte("BT /F1 24 Tf 30 160 Td <" + encoded.String() + "> Tj ET")
	return pdf([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 600 200] /Resources << /Font << /F1 4 0 R >> >> /Contents 5 0 R >>", "<< /Type /Font /Subtype /Type0 /BaseFont /DejaVuSans /Encoding /Identity-H /DescendantFonts [6 0 R] /ToUnicode 7 0 R >>", stream(content, ""), "<< /Type /Font /Subtype /CIDFontType2 /BaseFont /DejaVuSans /CIDSystemInfo << /Registry (Adobe) /Ordering (Identity) /Supplement 0 >> >>", stream([]byte(cmap), "")})
}
func scanFixture(t *testing.T) []byte {
	t.Helper()
	raw, e := os.ReadFile(filepath.Join("testdata", "russian.png"))
	if e != nil {
		t.Fatal(e)
	}
	img, _, e := image.Decode(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	bounds := img.Bounds()
	var rgb bytes.Buffer
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := img.At(x, y).RGBA()
			rgb.Write([]byte{byte(r >> 8), byte(g >> 8), byte(b >> 8)})
		}
	}
	var compressed bytes.Buffer
	w := zlib.NewWriter(&compressed)
	_, _ = w.Write(rgb.Bytes())
	_ = w.Close()
	return pdf([]string{"<< /Type /Catalog /Pages 2 0 R >>", "<< /Type /Pages /Kids [3 0 R] /Count 1 >>", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 1200 400] /Resources << /XObject << /I 4 0 R >> >> /Contents 5 0 R >>", stream(compressed.Bytes(), fmt.Sprintf("/Type /XObject /Subtype /Image /Width %d /Height %d /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /FlateDecode", bounds.Dx(), bounds.Dy())), stream([]byte("q 1200 0 0 400 0 0 cm /I Do Q"), "")})
}
func blankFixture(pages int) []byte {
	objects := []string{"<< /Type /Catalog /Pages 2 0 R >>", "", "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 100 100] /Resources << >> /Contents 4 0 R >>", stream([]byte(""), "")}
	var kids strings.Builder
	kids.WriteString("3 0 R ")
	for i := 1; i < pages; i++ {
		objects = append(objects, objects[2])
		fmt.Fprintf(&kids, "%d 0 R ", len(objects))
	}
	objects[1] = fmt.Sprintf("<< /Type /Pages /Kids [%s] /Count %d >>", kids.String(), pages)
	return pdf(objects)
}
func TestActualRussianPDFTextLayerAndScannedOCR(t *testing.T) {
	if os.Getenv("BUTLER_TEST_DOCUMENT_PARSER") != "true" {
		t.Skip("dedicated fixed Poppler/Tesseract sandbox required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	engine := &Engine{}
	layers, e := engine.TextLayer(ctx, textFixture())
	if e != nil || len(layers) != 1 || !strings.Contains(strings.Join(strings.Fields(layers[0].Text), " "), "Русский документ") {
		t.Fatal("actual Unicode text layer", layers, e)
	}
	if _, e = engine.Recognize(ctx, textFixture()); e != ErrHasText {
		t.Fatal("OCR replaced existing layer", e)
	}
	r, e := (ingestion.Parser{Pages: engine}).Parse(ctx, ingestion.Input{Format: "pdf", Path: "скан.pdf", Raw: scanFixture(t)})
	if e != nil || !r.Uncertain || !strings.Contains(strings.ToLower(r.Text), "русский документ") || !strings.Contains(strings.ToLower(r.Text), "стабильно") {
		t.Fatal("actual Russian OCR", r, e)
	}
	if !strings.Contains(string(r.Fragments[0].Locator), "tesseract_rus_eng_v1") {
		t.Fatal("OCR provenance lost", string(r.Fragments[0].Locator))
	}
	if _, e = engine.Recognize(ctx, []byte("%PDF- corrupt")); e == nil {
		t.Fatal("corrupt PDF became successful OCR")
	}
	// Actual text extraction must reject an excessive page count before OCR.
	if _, e = (ingestion.Parser{Pages: engine}).Parse(ctx, ingestion.Input{Format: "pdf", Path: "blank.pdf", Raw: blankFixture(1)}); !errors.Is(e, ingestion.ErrOCR) {
		t.Fatal("blank document fabricated text", e)
	}
	if _, e = engine.TextLayer(ctx, blankFixture(MaxPages+1)); !errors.Is(e, ingestion.ErrLimit) {
		t.Fatal("page bound", e)
	}
	cancelled, stop := context.WithCancel(ctx)
	stop()
	if _, e = engine.TextLayer(cancelled, textFixture()); !errors.Is(e, context.Canceled) {
		t.Fatal("cancelled parser continued", e)
	}
	items, e := filepath.Glob(filepath.Join(os.TempDir(), "butler-document-*"))
	if e != nil || len(items) != 0 {
		t.Fatal("private parser temporary bytes retained", items, e)
	}
}

func TestActualFixedParserHTTPBoundary(t *testing.T) {
	if os.Getenv("BUTLER_TEST_DOCUMENT_PARSER_HTTP") != "true" {
		t.Skip("dedicated internal parser service required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	p := ingestion.Parser{Pages: ingestion.NewPDFClient()}
	for _, fixture := range []struct {
		raw       []byte
		uncertain bool
	}{{textFixture(), false}, {scanFixture(t), true}} {
		r, e := p.Parse(ctx, ingestion.Input{Format: "pdf", Path: "русский.pdf", Raw: fixture.raw})
		if e != nil || r.Uncertain != fixture.uncertain || !strings.Contains(strings.ToLower(strings.Join(strings.Fields(r.Text), " ")), "русский документ") {
			t.Fatal("fixed private HTTP parser", r, e)
		}
	}
	if _, e := p.Parse(ctx, ingestion.Input{Format: "pdf", Path: "corrupt.pdf", Raw: []byte("%PDF-corrupt")}); e == nil {
		t.Fatal("corrupt HTTP response accepted")
	}
	if _, e := p.Parse(ctx, ingestion.Input{Format: "pdf", Path: "too-many-pages.pdf", Raw: blankFixture(MaxPages + 1)}); !errors.Is(e, ingestion.ErrLimit) {
		t.Fatal("native page limit lost at HTTP boundary", e)
	}
}
