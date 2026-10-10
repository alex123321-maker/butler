// Run only on the lab against the private auxiliary model service.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

type document struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Title string `json:"title"`
}
type query struct {
	ID       string `json:"id"`
	Text     string `json:"text"`
	Relevant string `json:"relevant"`
	Code     bool   `json:"code"`
}
type fixture struct {
	Documents []document `json:"documents"`
	Queries   []query    `json:"queries"`
}
type measurement struct {
	ID        string `json:"id"`
	Device    string `json:"device"`
	Fallback  string `json:"fallback,omitempty"`
	LatencyMS uint64 `json:"latency_ms"`
	QueueMS   uint64 `json:"queue_ms"`
	PeakMiB   uint32 `json:"peak_mib"`
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9090", "private gRPC address")
	tokenPath := flag.String("token-file", "/data/token", "private token file")
	fixturePath := flag.String("fixture", "/app/russian-retrieval.json", "synthetic fixture")
	reportPath := flag.String("report", "/reports/verification.json", "artifact path")
	expectGPU := flag.Bool("expect-gpu", true, "require at least one actual GPU operation")
	flag.Parse()
	if err := run(*addr, *tokenPath, *fixturePath, *reportPath, *expectGPU); err != nil {
		fmt.Fprintln(os.Stderr, "Local model smoke failed:", err)
		os.Exit(1)
	}
}
func run(addr, tokenPath, fixturePath, reportPath string, expectGPU bool) error {
	token, err := os.ReadFile(tokenPath)
	if err != nil {
		return fmt.Errorf("read private token")
	}
	token = []byte(strings.TrimSpace(string(token)))
	if len(token) < 32 {
		return fmt.Errorf("token length")
	}
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("network isolation check")
	}
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback == 0 && iface.Flags&net.FlagUp != 0 {
			return fmt.Errorf("smoke requires an isolated network namespace")
		}
	}
	connection, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect")
	}
	defer connection.Close()
	client := pb.NewLocalModelsServiceClient(connection)
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if _, err := client.Describe(timeoutCtx, &pb.Empty{}); status.Code(err) != codes.Unauthenticated {
		return fmt.Errorf("anonymous access not rejected")
	}
	ctx := metadata.AppendToOutgoingContext(timeoutCtx, "authorization", "Bearer "+string(token))
	catalog, err := client.Describe(ctx, &pb.Empty{})
	if err != nil || len(catalog.Models) != 1 {
		return fmt.Errorf("catalog unavailable")
	}
	p := catalog.Models[0]
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		return fmt.Errorf("read fixture")
	}
	var f fixture
	if json.Unmarshal(data, &f) != nil {
		return fmt.Errorf("decode fixture")
	}
	var measures []measurement
	var measureMu sync.Mutex
	vectors := map[string][]float32{}
	gpuCount := 0
	embed := func(id string, texts []*pb.TextInput, task pb.EmbeddingTask, dimensions uint32, cpu bool) (*pb.InferResponse, error) {
		response, err := client.Infer(ctx, &pb.InferRequest{RequestId: id, ProfileId: p.Id, CpuOnly: cpu, Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Task: task, Dimensions: dimensions, Texts: texts}}})
		if err != nil {
			return nil, fmt.Errorf("%s: %s", id, status.Code(err))
		}
		if response.GetEmbed() == nil || len(response.GetEmbed().Vectors) != len(texts) || response.Revision != p.Revision {
			return nil, fmt.Errorf("%s output identity", id)
		}
		for i, v := range response.GetEmbed().Vectors {
			if v.Id != texts[i].Id || len(v.Values) != int(dimensions) {
				return nil, fmt.Errorf("%s vector shape", id)
			}
			norm := float64(0)
			for _, n := range v.Values {
				if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
					return nil, fmt.Errorf("nonfinite vector")
				}
				norm += float64(n * n)
			}
			if math.Abs(norm-1) > 1e-4 {
				return nil, fmt.Errorf("vector normalization")
			}
		}
		measureMu.Lock()
		measures = append(measures, measurement{ID: id, Device: response.Device, Fallback: response.FallbackReason, LatencyMS: response.ExecutionMs, QueueMS: response.QueueMs, PeakMiB: response.PeakDeviceMib})
		if response.Device == "gpu" {
			gpuCount++
		}
		measureMu.Unlock()
		return response, nil
	}
	for start := 0; start < len(f.Documents); start += int(p.MaxBatch) {
		end := min(start+int(p.MaxBatch), len(f.Documents))
		var texts []*pb.TextInput
		for _, d := range f.Documents[start:end] {
			texts = append(texts, &pb.TextInput{Id: d.ID, Text: d.Text, Title: d.Title})
		}
		out, err := embed(fmt.Sprintf("index-%d", start), texts, pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT, 768, false)
		if err != nil {
			return err
		}
		for _, v := range out.GetEmbed().Vectors {
			vectors[v.Id] = v.Values
		}
	}
	hits := 0
	var rankings []map[string]any
	for _, q := range f.Queries {
		task := pb.EmbeddingTask_EMBEDDING_TASK_QUERY
		if q.Code {
			task = pb.EmbeddingTask_EMBEDDING_TASK_CODE_QUERY
		}
		out, err := embed(q.ID, []*pb.TextInput{{Id: q.ID, Text: q.Text}}, task, 768, false)
		if err != nil {
			return err
		}
		best := ""
		score := math.Inf(-1)
		for id, v := range vectors {
			s := float64(0)
			for i, n := range v {
				s += float64(n * out.GetEmbed().Vectors[0].Values[i])
			}
			if s > score {
				score = s
				best = id
			}
		}
		if best == q.Relevant {
			hits++
		}
		rankings = append(rankings, map[string]any{"query_id": q.ID, "expected": q.Relevant, "first": best, "correct": best == q.Relevant})
	}
	if expectGPU && gpuCount == 0 {
		return fmt.Errorf("no real GPU inference observed")
	}
	if _, err := embed("mrl-256", []*pb.TextInput{{Id: "mrl", Text: "Как хранить память агента?"}}, pb.EmbeddingTask_EMBEDDING_TASK_QUERY, 256, false); err != nil {
		return err
	}
	// Indexing and search share the same bounded queue and cannot load two workers.
	barrier := make(chan struct{})
	parallelResults := make(chan error, 2)
	for _, task := range []pb.EmbeddingTask{pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT, pb.EmbeddingTask_EMBEDDING_TASK_QUERY} {
		go func(task pb.EmbeddingTask) {
			<-barrier
			id := "parallel-index"
			if task == pb.EmbeddingTask_EMBEDDING_TASK_QUERY {
				id = "parallel-search"
			}
			_, err := embed(id, []*pb.TextInput{{Id: id, Text: "Сессия владеет порядком запросов и блокировками."}}, task, 768, false)
			parallelResults <- err
		}(task)
	}
	close(barrier)
	for range 2 {
		if err := <-parallelResults; err != nil {
			return err
		}
	}
	queuedParallel := false
	for _, measure := range measures {
		if strings.HasPrefix(measure.ID, "parallel-") && measure.QueueMS > 0 {
			queuedParallel = true
		}
	}
	if !queuedParallel {
		return fmt.Errorf("parallel indexing/search did not exercise the queue")
	}
	if !expectGPU && gpuCount != 0 {
		return fmt.Errorf("constrained budget admitted a GPU operation")
	}
	// Exercise nearly the complete configured byte allowance in one batch.
	var longBatch []*pb.TextInput
	for i := range 4 {
		longBatch = append(longBatch, &pb.TextInput{Id: fmt.Sprintf("long-%d", i), Text: strings.Repeat("a ", 1000)})
	}
	if _, err := embed("near-input-limit", longBatch, pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT, 768, false); err != nil {
		return err
	}
	_, err = client.Infer(ctx, &pb.InferRequest{RequestId: "context-limit", ProfileId: p.Id, Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Task: pb.EmbeddingTask_EMBEDDING_TASK_QUERY, Dimensions: 768, Texts: []*pb.TextInput{{Id: "over-context", Text: strings.Repeat("a ", 3000)}}}}})
	if status.Code(err) != codes.InvalidArgument {
		return fmt.Errorf("over-context input was not explicitly rejected: %s", status.Code(err))
	}
	if _, err := embed("cpu-fp32", []*pb.TextInput{{Id: "cpu", Text: "Где хранится долговременная память?"}}, pb.EmbeddingTask_EMBEDDING_TASK_QUERY, 768, true); err != nil {
		return err
	}
	// Unsupported capabilities must explicitly fail, never produce an empty success.
	_, err = client.Infer(ctx, &pb.InferRequest{RequestId: "unsupported-rank", ProfileId: p.Id, Input: &pb.InferRequest_Rerank{Rerank: &pb.RerankInput{Query: "память", Candidates: []*pb.TextInput{{Id: "c", Text: "PostgreSQL"}}}}})
	if status.Code(err) != codes.Unavailable {
		return fmt.Errorf("unsupported rank did not fail safely")
	}
	// Let the configured idle reaper release the worker before canceling a cold load.
	idleDeadline := time.Now().Add(10 * time.Second)
	idle := false
	for time.Now().Before(idleDeadline) {
		s, err := client.Status(ctx, &pb.Empty{})
		if err != nil {
			return fmt.Errorf("status")
		}
		if s.ActiveModel == "" {
			idle = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !idle {
		return fmt.Errorf("idle unload did not finish")
	}
	_, err = client.Infer(ctx, &pb.InferRequest{RequestId: "canceled-cold-load", ProfileId: p.Id, TimeoutMs: 1, Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Task: pb.EmbeddingTask_EMBEDDING_TASK_QUERY, Dimensions: 768, Texts: []*pb.TextInput{{Id: "cancel", Text: "Отмена проверки"}}}}})
	if status.Code(err) != codes.DeadlineExceeded && status.Code(err) != codes.Canceled {
		return fmt.Errorf("cold inference cancellation")
	}
	if _, err := embed("after-cancellation", []*pb.TextInput{{Id: "after", Text: "Система продолжает работать после отмены."}}, pb.EmbeddingTask_EMBEDDING_TASK_QUERY, 768, true); err != nil {
		return err
	}
	s, err := client.Status(ctx, &pb.Empty{})
	if err != nil {
		return fmt.Errorf("final status")
	}
	statusJSON, _ := protojson.Marshal(s)
	catalogJSON, _ := protojson.Marshal(catalog)
	artifact := map[string]any{"generated_at": time.Now().UTC().Format(time.RFC3339), "scope": "synthetic Russian/code smoke; not a production retrieval quality decision", "catalog": json.RawMessage(catalogJSON), "status": json.RawMessage(statusJSON), "measurements": measures, "retrieval_top1": float64(hits) / float64(len(f.Queries)), "rankings": rankings, "authentication": "anonymous RPC rejected", "mrl": "768 and 256 finite L2-normalized", "parallel_index_search": queuedParallel, "context_limit": "explicit InvalidArgument; no truncation", "large_batch": "4 documents, 8000 raw UTF-8 bytes", "idle_unload": idle, "cancellation": "cold-load deadline then successful inference", "external_network": "container launched with --network none", "cpu": "actual FP32 inference"}
	encoded, _ := json.MarshalIndent(artifact, "", "  ")
	if os.WriteFile(reportPath, encoded, 0644) != nil {
		return fmt.Errorf("write verification artifact")
	}
	fmt.Printf("PASS local models: %d retrieval cases, top1 %.3f, GPU operations %d; report %s\n", len(f.Queries), float64(hits)/float64(len(f.Queries)), gpuCount, reportPath)
	return nil
}
