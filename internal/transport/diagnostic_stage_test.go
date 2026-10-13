package transport

import (
	"context"
	"reflect"
	"testing"
)

func TestProviderStageObserverIsRequestLocalAndContentFree(t *testing.T) {
	base := context.Background()
	var stages []ProviderStage
	ctx := WithProviderStageObserver(base, func(s ProviderStage) { stages = append(stages, s) })
	ObserveProviderStage(base, ProviderStageHTTP)
	ObserveProviderStage(ctx, ProviderStage("PRIVATE_TOKEN_OR_BODY"))
	ObserveProviderStage(context.WithValue(ctx, struct{}{}, "private unrelated context"), ProviderStageAuthentication)
	ObserveProviderStage(ctx, ProviderStageStream)
	if !reflect.DeepEqual(stages, []ProviderStage{ProviderStageAuthentication, ProviderStageStream}) || WithProviderStageObserver(base, nil) != base {
		t.Fatal("observer leaked across requests or accepted unbounded content")
	}
}
