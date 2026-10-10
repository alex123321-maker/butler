package transport

import (
	"context"
	"errors"
	"testing"
)

func TestSerializedInputUsesPrivateBoundAndConservativeDefault(t *testing.T) {
	if err := CheckSerializedInput(context.Background(), int(DefaultInputByteLimit)); err != nil {
		t.Fatal(err)
	}
	for _, ctx := range []context.Context{context.Background(), WithInputByteLimit(context.Background(), 10)} {
		err := CheckSerializedInput(ctx, int(DefaultInputByteLimit+1))
		var bounded *Error
		if !errors.As(err, &bounded) || bounded.Type != ErrorTypeInputBytesExceeded {
			t.Fatal("byte limit bypass", err)
		}
	}
}
