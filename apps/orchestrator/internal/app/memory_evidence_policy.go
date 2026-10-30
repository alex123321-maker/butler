package app

import (
	"context"
	flow "github.com/butler/butler/apps/orchestrator/internal/orchestrator"
	"github.com/butler/butler/internal/memory/experience"
	"github.com/butler/butler/internal/memory/readcontrol"
)

func memoryEvidencePolicy(s *MemoryReads) flow.MemoryEvidencePolicy {
	if s == nil || s.experience == nil {
		return nil
	}
	return s
}

// CheckProposal is an optional-read optimization, not tool authorization. Flow
// checks fatal state before dispatch; the owning reader also checks inside the
// actual Broker dispatch and returns typed limits without reading further DATA.
// No process-local counters or historical run/session result can reset its state.
func (s *MemoryReads) CheckProposal(ctx context.Context, run, tool, raw string) error {
	if s == nil || s.experience == nil || tool != readcontrol.Search && tool != readcontrol.Open && tool != readcontrol.Neighbors {
		return nil
	}
	if err := s.sources.CheckRunSources(ctx, run); err != nil {
		return err
	}
	rows, err := s.pool.Query(ctx, `SELECT c.tool_name,COALESCE(c.args,'{}'::jsonb)::text,COALESCE(c.result,'{}'::jsonb)::text,c.status,
 c.status='completed' AND c.finished_at IS NOT NULL AND EXISTS(SELECT 1 FROM action_intents i JOIN action_effects e ON e.action_id=i.effect_id WHERE i.run_id=c.run_id AND i.tool_call_id=c.tool_call_id AND i.binding->>'tool_name'=c.tool_name AND e.state='effect_confirmed')
 FROM tool_calls c WHERE c.run_id=$1 AND c.tool_name IN('memory.search','memory.open','memory.neighbors') ORDER BY c.started_at,c.tool_call_id LIMIT 9`, run)
	if err != nil {
		return err
	}
	defer rows.Close()
	var history []experience.ReadObservation
	for rows.Next() {
		var h experience.ReadObservation
		if err = rows.Scan(&h.Tool, &h.ArgsJSON, &h.ResultJSON, &h.Status, &h.Confirmed); err != nil {
			return err
		}
		history = append(history, h)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	return experience.CheckReadProposal(history, tool, raw)
}
