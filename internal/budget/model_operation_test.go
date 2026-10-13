package budget

import (
	"context"
	"testing"
)

func TestModelRetryOperationWitnessIsRequestLocal(t *testing.T) {
	first := WithModelOperation(context.Background(), "operation-one")
	second := WithModelOperation(context.Background(), "operation-two")
	if value, ok := ModelOperationFromContext(first); !ok || value != "operation-one" {
		t.Fatal("first witness absent")
	}
	if value, ok := ModelOperationFromContext(second); !ok || value != "operation-two" {
		t.Fatal("operation reference became shared")
	}
	if _, ok := ModelOperationFromContext(context.Background()); ok {
		t.Fatal("ambient context guessed admission")
	}
}
