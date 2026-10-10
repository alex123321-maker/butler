package app

import (
	"context"
	"fmt"
	"github.com/butler/butler/apps/orchestrator/internal/completion"
	"github.com/butler/butler/internal/memory/pipeline"
	"github.com/butler/butler/internal/memory/working"
)

type completionAuxiliary struct {
	Working   *working.Store
	Transient *working.TransientStore
	Pipeline  *pipeline.Worker
}

func (a completionAuxiliary) ProcessAuxiliary(ctx context.Context, j completion.AuxiliaryJob) error {
	switch j.Kind {
	case "working_cleanup":
		return a.Working.ClearCompletedRun(ctx, j.SessionKey, j.RunID)
	case "transient_cleanup":
		err := a.Transient.Clear(ctx, j.SessionKey, j.RunID)
		if err == working.ErrTransientStateNotFound {
			return nil
		}
		return err
	case "memory_pipeline":
		if a.Pipeline == nil {
			return fmt.Errorf("memory pipeline unavailable")
		}
		return a.Pipeline.ProcessDurablePostRun(ctx, j.RunID, j.SessionKey)
	}
	return fmt.Errorf("unknown auxiliary operation")
}
