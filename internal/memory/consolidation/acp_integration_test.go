package consolidation

import (
	"strings"
	"testing"
	"time"

	"github.com/butler/butler/internal/actionauth"
	"github.com/butler/butler/internal/budget"
	"github.com/butler/butler/internal/memory/acpqualification"
	"github.com/butler/butler/internal/memory/jobs"
)

func TestACPConsolidationPreservesDuplicatesAndUnknown(t *testing.T) {
	c := acpqualification.Open(t, "consolidation-v1", 1)
	text := "Гипотеза: сервер A быстрее; это не проверено. Исключение: проект Сфера использует B."
	unknown := "Проверка проекта Сфера завершилась ошибкой. Отправка UNKNOWN; не повторять без проверки."
	f := newWorkflowOnPool(t, c.Pool, c.Manifest, text, text, unknown)
	b := budget.NewController(c.Pool, c.Policy)
	b.SetAuthorityResolver(actionauth.ResolveBudgetAuthority)
	f.store.Jobs = jobs.New(c.Pool, f.store.Catalog, b, nil)
	f.worker = &jobs.ModelWorker{Jobs: f.store.Jobs, Provider: c.Provider, Model: "gemini-3.8-flash", Handlers: map[string]jobs.ModelHandler{Version: Handler{}}}
	q := f.request()
	q.ViewKey = "acp-consolidation-v1"
	q.Policy.ID = "acp-consolidation-v1"
	q.Policy.AccountingMode = "opaque_subscription"
	q.Policy.Job.Allowance = c.Allowance()
	p := f.prepare(t, q)
	processed := false
	for scan := 0; scan < 20; scan++ {
		ok, err := f.process(t)
		if err != nil {
			t.Fatal(err)
		}
		if ok {
			processed = true
			break
		}
		time.Sleep(500 * time.Millisecond)
	}
	if !processed {
		t.Fatal("ordinary scanner did not admit consolidation")
	}
	var id string
	if err := f.pool.QueryRow(f.ctx, `SELECT g.source_id FROM memory_consolidation_generations g JOIN memory_consolidation_inputs i ON i.source_id=g.input_source_id WHERE i.policy_id=$1`, p.ID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	preview, err := f.store.Preview(f.ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(mustJSON(preview))
	if !strings.Contains(raw, unknown) || !strings.Contains(raw, text) {
		t.Fatal("consolidation discarded uncertainty or exact evidence")
	}
}
