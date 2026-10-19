package browsercontrol

import (
	"context"
	"strconv"

	"google.golang.org/grpc/metadata"
)

const dispatchEpochHeader = "butler-browser-epoch"

type dispatchEpochKey struct{}

// WithDispatchEpoch pins a Broker admission to one control generation. This
// private transport field is never part of model arguments or action hashes.
func WithDispatchEpoch(ctx context.Context, epoch int64) context.Context {
	md, _ := metadata.FromOutgoingContext(ctx)
	md = md.Copy()
	md.Set(dispatchEpochHeader, strconv.FormatInt(epoch, 10))
	return metadata.NewOutgoingContext(context.WithValue(ctx, dispatchEpochKey{}, epoch), md)
}

func admittedEpoch(ctx context.Context) (int64, bool) {
	epoch, ok := ctx.Value(dispatchEpochKey{}).(int64)
	return epoch, ok
}

// IncomingDispatchEpoch rejects duplicate, negative and malformed metadata.
func IncomingDispatchEpoch(ctx context.Context) (int64, bool) {
	md, _ := metadata.FromIncomingContext(ctx)
	values := md.Get(dispatchEpochHeader)
	if len(values) != 1 {
		return 0, false
	}
	epoch, err := strconv.ParseInt(values[0], 10, 64)
	return epoch, err == nil && epoch >= 0
}
