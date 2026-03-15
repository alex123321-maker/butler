package episodic

import "testing"

func TestVectorLiteral(t *testing.T) {
	t.Parallel()
	if got := vectorLiteral([]float32{0.1, 0.2, 0.3}); got != "[0.1,0.2,0.3]" {
		t.Fatalf("vectorLiteral = %q", got)
	}
}
