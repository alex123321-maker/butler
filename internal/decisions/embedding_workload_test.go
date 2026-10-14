package decisions

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"github.com/butler/butler/internal/memory/semantic"
)

func TestActualEmbeddingCPUWorkload(t *testing.T) {
	token := os.Getenv("BUTLER_TEST_LOCAL_MODELS_TOKEN_FILE")
	if token == "" {
		t.Skip("isolated placement benchmark required")
	}
	f := newFixture(t)
	backend, closeBackend, e := semantic.NewPinnedBackend(f.target, token)
	if e != nil {
		t.Fatal(e)
	}
	defer closeBackend()
	resident := func() string {
		client := &http.Client{Transport: &http.Transport{Proxy: nil}, Timeout: 5 * time.Second}
		r, err := client.Get("http://decision-qwen:11434/api/ps")
		if err != nil {
			t.Fatal(err)
		}
		defer r.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(r.Body, 65536))
		var s struct {
			Models []struct {
				Name, Digest string
				Size         int64 `json:"size"`
				VRAM         int64 `json:"size_vram"`
			}
		}
		if json.Unmarshal(b, &s) != nil || len(s.Models) != 1 || s.Models[0].Name != qwenEvaluationModel || s.Models[0].VRAM <= 0 || s.Models[0].VRAM != s.Models[0].Size {
			t.Fatal("Qwen residency unavailable")
		}
		return s.Models[0].Digest
	}
	before := resident()
	var rows []map[string]any
	for _, workload := range []struct {
		name, text string
		batch      int
		task       pb.EmbeddingTask
	}{
		{"short_query", "Какие задачи требуют проверки результата?", 1, pb.EmbeddingTask_EMBEDDING_TASK_QUERY},
		{"document", "Отчёт: проверка выполнена для текущей версии проекта. " + strings.Repeat("Зафиксированы состояние сервиса и результаты обязательных тестов. ", 18), 1, pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT},
		{"four_documents", strings.Repeat("Запись рабочего журнала описывает изменённую конфигурацию сервера. ", 12), 4, pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT},
	} {
		texts := []*pb.TextInput{}
		for i := 0; i < workload.batch; i++ {
			texts = append(texts, &pb.TextInput{Id: string(rune('a' + i)), Text: workload.text})
		}
		for repeat := 0; repeat < 4; repeat++ {
			start := time.Now()
			out, err := backend.Infer(context.Background(), &pb.InferRequest{RequestId: workload.name, ProfileId: "embeddinggemma2-text-bf16-v1", CallerId: "decision-embedding-placement", CpuOnly: true, TimeoutMs: 60000, Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Texts: texts, Task: workload.task, Dimensions: 256}}})
			if err != nil || out.Device != "cpu" || len(out.GetEmbed().Vectors) != workload.batch {
				t.Fatal("CPU embedding workload failed", err)
			}
			rows = append(rows, map[string]any{"workload": workload.name, "repeat": repeat, "batch": workload.batch, "input_unicode_chars": len([]rune(workload.text)), "wall_ms": time.Since(start).Milliseconds(), "device": out.Device, "process_ram_mib": out.PeakRamMib, "qwen_gpu_was_kept_loaded": true})
		}
	}
	if resident() != before {
		t.Fatal("Qwen model changed during CPU benchmark")
	}
	raw, _ := json.MarshalIndent(rows, "", "  ")
	if e = os.WriteFile(os.Getenv("BUTLER_TEST_DECISION_REPORT"), raw, 0644); e != nil {
		t.Fatal(e)
	}
}
