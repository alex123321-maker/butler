package resourceexec

import "context"

// LiveAuthority verifies an already running dispatch; it cannot begin/replay it,
// extend its lease or create a permission. Network runtimes require this check
// independently of initial admission before releasing each chunk or message.
type LiveAuthority interface {
	Check(context.Context, RuntimeRequest, string) (RuntimeSpec, error)
}

type liveCheckKey struct{}

func LiveCheck(ctx context.Context) (func(context.Context, string) error, bool) {
	check, ok := ctx.Value(liveCheckKey{}).(func(context.Context, string) error)
	return check, ok && check != nil
}
