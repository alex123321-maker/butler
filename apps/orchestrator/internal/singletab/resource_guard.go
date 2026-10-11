package singletab

import (
	"github.com/butler/butler/internal/resourceexec"
	"github.com/butler/butler/internal/singletabcontrol"
)

// ResourceGuard adapts the owning guarded selection module. The complete
// WorkerGuard/source lock set must already be held by the caller. A legacy
// non-atomic selection has no immutable guarded mapping and cannot satisfy it.
func ResourceGuard(browserInstance string) resourceexec.SingleTabGuard {
	return singletabcontrol.Guard(browserInstance)
}
