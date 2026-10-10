// Lab-only proof of real local capabilities, shared queues and parallel lanes.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	pb "github.com/butler/butler/internal/gen/localmodels/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"strings"
	"time"
)

type measure struct {
	Operation, Device, Fallback string
	Start, End                  time.Time
	QueueMS, ExecutionMS        uint64
	GPUPeak, RAMPeak            uint32
}

func main() {
	addr := flag.String("addr", "127.0.0.1:9090", "private gRPC address")
	tokenFile := flag.String("token-file", "/private/token", "private bearer source")
	report := flag.String("report", "/reports/verification-capabilities.json", "safe measurement report")
	flag.Parse()
	if err := runCapabilities(*addr, *tokenFile, *report); err != nil {
		fmt.Fprintln(os.Stderr, "Capability proof failed:", err)
		os.Exit(1)
	}
}
func runCapabilities(addr, tokenFile, report string) error {
	token, err := os.ReadFile(tokenFile)
	if err != nil {
		return fmt.Errorf("private token source")
	}
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("local connection")
	}
	defer conn.Close()
	root, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	client := pb.NewLocalModelsServiceClient(conn)
	if _, err := client.Describe(root, &pb.Empty{}); status.Code(err) != codes.Unauthenticated {
		return fmt.Errorf("anonymous catalog access not rejected")
	}
	ctx := metadata.AppendToOutgoingContext(root, "authorization", "Bearer "+strings.TrimSpace(string(token)))
	catalog, err := client.Describe(ctx, &pb.Empty{})
	if err != nil || len(catalog.Models) != 4 {
		return fmt.Errorf("catalog")
	}
	var measurements []measure
	invoke := func(name, caller string, input *pb.InferRequest) (*pb.InferResponse, measure, error) {
		input.RequestId = name
		input.CallerId = caller
		started := time.Now()
		out, err := client.Infer(ctx, input)
		m := measure{Operation: name, Start: started, End: time.Now()}
		if err != nil {
			return nil, m, status.Errorf(status.Code(err), "%s rejected", name)
		}
		m.Device = out.Device
		m.Fallback = out.FallbackReason
		m.QueueMS = out.QueueMs
		m.ExecutionMS = out.ExecutionMs
		m.GPUPeak = out.PeakDeviceMib
		m.RAMPeak = out.PeakRamMib
		return out, m, nil
	}
	for _, dimensions := range []uint32{768, 256} {
		name := fmt.Sprintf("embed-russian-%d", dimensions)
		out, m, err := invoke(name, "index", &pb.InferRequest{Input: &pb.InferRequest_Embed{Embed: &pb.EmbedInput{Task: pb.EmbeddingTask_EMBEDDING_TASK_DOCUMENT, Dimensions: dimensions, Texts: []*pb.TextInput{{Id: "public", Text: "Butler хранит долговременную память в PostgreSQL."}}}}})
		if err != nil {
			return err
		}
		measurements = append(measurements, m)
		if out.GetEmbed() == nil || len(out.GetEmbed().Vectors) != 1 || len(out.GetEmbed().Vectors[0].Values) != int(dimensions) || out.Device != "gpu" {
			return fmt.Errorf("actual text embedding dimensions/device")
		}
		var norm float64
		for _, value := range out.GetEmbed().Vectors[0].Values {
			if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
				return fmt.Errorf("nonfinite text embedding")
			}
			norm += float64(value) * float64(value)
		}
		if math.Abs(math.Sqrt(norm)-1) > 0.01 {
			return fmt.Errorf("unnormalized text embedding")
		}
	}
	extraction := &pb.InferRequest{Input: &pb.InferRequest_Extract{Extract: &pb.ExtractInput{Text: "Анна работает в компании Яндекс в Москве.", EntityTypes: []string{"person", "company", "location"}, SchemaVersion: "proof-v1"}}}
	out, m, err := invoke("extract-russian", "extractor", extraction)
	if err != nil {
		return err
	}
	measurements = append(measurements, m)
	if out.GetExtract() == nil || len(out.GetExtract().Spans) == 0 || out.Device != "cpu" {
		return fmt.Errorf("no real CPU entity spans")
	}
	spans := out.GetExtract().Spans
	ranking := &pb.InferRequest{Input: &pb.InferRequest_Rerank{Rerank: &pb.RerankInput{Query: "Где Butler хранит долговременную память?", Candidates: []*pb.TextInput{{Id: "postgres", Text: "Butler хранит долговременную память и историю в PostgreSQL. Redis хранит временные очереди."}, {Id: "other", Text: "Автомобиль ездит по дороге. Бананы растут в тёплом климате."}}}}}
	out, m, err = invoke("rank-russian", "search", ranking)
	if err != nil {
		return err
	}
	measurements = append(measurements, m)
	if out.GetRerank() == nil || len(out.GetRerank().Scores) != 2 || out.GetRerank().Scores[0].Id != "postgres" || out.GetRerank().Scores[0].Value <= out.GetRerank().Scores[1].Value || out.Device != "gpu" {
		return fmt.Errorf("real ranking order/device")
	}
	rankScores := out.GetRerank().Scores
	classification := &pb.InferRequest{Input: &pb.InferRequest_Classify{Classify: &pb.ClassifyInput{Text: "Пользователь просит прочитать документацию и посмотреть страницу справки.", Labels: []string{"чтение", "изменение", "удаление"}}}}
	out, m, err = invoke("classify-russian", "classification", classification)
	if err != nil {
		return err
	}
	measurements = append(measurements, m)
	if out.GetClassify() == nil || len(out.GetClassify().Scores) != 1 || out.GetClassify().Scores[0].Id != "чтение" || out.Device != "cpu" {
		return fmt.Errorf("real classification signal")
	}
	classScores := out.GetClassify().Scores
	// Warm CPU and GPU workers now overlap. The GPU lane remains serial while
	// the separate CPU lane can classify without waiting for the GPU result.
	var longCandidates []*pb.TextInput
	for i := range 4 {
		longCandidates = append(longCandidates, &pb.TextInput{Id: fmt.Sprint(i), Text: strings.Repeat("PostgreSQL хранит память и историю; Redis хранит временные очереди. ", 12)})
	}
	concurrentRank := &pb.InferRequest{Input: &pb.InferRequest_Rerank{Rerank: &pb.RerankInput{Query: "Где хранится память?", Candidates: longCandidates}}}
	type result struct {
		m   measure
		err error
	}
	barrier := make(chan struct{})
	results := make(chan result, 2)
	go func() {
		<-barrier
		_, m, err := invoke("concurrent-rank", "search", concurrentRank)
		results <- result{m, err}
	}()
	go func() {
		<-barrier
		_, m, err := invoke("concurrent-classify", "classification", classification)
		results <- result{m, err}
	}()
	close(barrier)
	a, b := <-results, <-results
	if a.err != nil {
		return a.err
	}
	if b.err != nil {
		return b.err
	}
	measurements = append(measurements, a.m, b.m)
	overlap := a.m.Start.Before(b.m.End) && b.m.Start.Before(a.m.End)
	if !overlap {
		return fmt.Errorf("independent lanes did not overlap")
	}
	// Load vision only on demand from the same pinned checkpoint. Synthetic RGB
	// image proves loading/resource behavior, not vision quality or OCR accuracy.
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	for y := range 512 {
		for x := range 512 {
			img.Set(x, y, color.RGBA{uint8(x * 8), uint8(y * 8), 127, 255})
		}
	}
	var encoded bytes.Buffer
	if png.Encode(&encoded, img) != nil {
		return fmt.Errorf("fixture image")
	}
	visionInput := &pb.InferRequest{Input: &pb.InferRequest_Media{Media: &pb.MediaInput{Content: encoded.Bytes(), MimeType: "image/png"}}}
	out, m, err = invoke("vision-load", "media", visionInput)
	visionAdmission := "immediate"
	if status.Code(err) == codes.ResourceExhausted {
		// The conservative complete-process allowance may refuse vision while
		// the CPU extractor is resident. Preserve the full request and wait for
		// this proof's 30-second idle unload; do not widen its RAM/GPU policy.
		select {
		case <-ctx.Done():
			return fmt.Errorf("vision retry canceled")
		case <-time.After(31 * time.Second):
		}
		out, m, err = invoke("vision-after-idle-unload", "media", visionInput)
		visionAdmission = "resource_refusal_then_retry_after_idle_unload"
	}
	if err != nil {
		return err
	}
	measurements = append(measurements, m)
	if out.GetMedia() == nil || len(out.GetMedia().Embedding) != 768 || out.Device != "gpu" {
		return fmt.Errorf("real vision shape/device")
	}
	norm := float64(0)
	for _, v := range out.GetMedia().Embedding {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("nonfinite image vector")
		}
		norm += float64(v * v)
	}
	if math.Abs(norm-1) > 1e-4 {
		return fmt.Errorf("image normalization")
	}
	status, err := client.Status(ctx, &pb.Empty{})
	if err != nil {
		return fmt.Errorf("status")
	}
	statusJSON, _ := protojson.Marshal(status)
	catalogJSON, _ := protojson.Marshal(catalog)
	artifact := map[string]any{"generated_at": time.Now().UTC().Format(time.RFC3339), "scope": "actual pinned model mechanics + small public Russian signals; not production quality acceptance", "catalog": json.RawMessage(catalogJSON), "status": json.RawMessage(statusJSON), "measurements": measurements, "cpu_gpu_overlap": overlap, "russian_spans": spans, "rank_scores": rankScores, "classification_scores": classScores, "vision": "actual 440M text+vision loading; synthetic RGB768 normalized; audio omitted", "vision_admission": visionAdmission, "network": "all models local_files_only; deployment helper separately verifies network isolation"}
	data, _ := json.MarshalIndent(artifact, "", "  ")
	if os.WriteFile(report, data, 0644) != nil {
		return fmt.Errorf("artifact")
	}
	fmt.Println("PASS actual embedding/ranking/extraction/classification/image capabilities; shared GPU+CPU lanes")
	return nil
}
