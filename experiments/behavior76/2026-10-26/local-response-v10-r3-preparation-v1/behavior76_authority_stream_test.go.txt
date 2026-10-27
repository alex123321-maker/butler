package app

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
)

const behavior76StreamBuffer = 4096
const behavior76StreamSegments = 16384
const behavior76StreamBytes int64 = 512 << 20
const behavior76StreamLine = 8192
const behavior76StreamHeader = 16 << 20
const behavior76StreamOpen = 4096

type behavior76StreamIdentity struct {
	Window         string `json:"window"`
	Arm            string `json:"arm"`
	Schema         string `json:"schema"`
	Producer       string `json:"producer"`
	ProtocolSHA256 string `json:"protocol_sha256"`
}

type behavior76StreamSegment struct {
	Path               string `json:"path"`
	First              uint64 `json:"first_sequence"`
	Last               uint64 `json:"last_sequence"`
	Events             int    `json:"event_count"`
	Compressed         int64  `json:"compressed_bytes"`
	Uncompressed       int64  `json:"uncompressed_bytes"`
	SHA256             string `json:"sha256"`
	UncompressedSHA256 string `json:"uncompressed_sha256"`
}

type behavior76AuthorityStream struct {
	directory       string
	destination     string
	published       int
	retired         bool
	recordingClosed bool
	identity        behavior76StreamIdentity
	global          *atomic.Bool
	segments        []behavior76StreamSegment
	compressed      int64
	sealed          bool
}

type behavior76AuthorityStreamSnapshot struct {
	Version           string                          `json:"version"`
	Sealed            bool                            `json:"sealed"`
	EventCount        uint64                          `json:"event_count"`
	LastSequence      uint64                          `json:"last_sequence"`
	MaximumBuffered   int                             `json:"maximum_buffered_events"`
	MaximumSegments   int                             `json:"maximum_segments"`
	MaximumCompressed int64                           `json:"maximum_compressed_bytes"`
	TotalCompressed   int64                           `json:"total_compressed_bytes"`
	Identity          behavior76StreamIdentity        `json:"identity"`
	Segments          []behavior76StreamSegment       `json:"segments"`
	Tail              []behavior76AuthorityTraceEvent `json:"tail"`
	Open              uint64                          `json:"open_spans"`
	Dropped           uint64                          `json:"dropped_events"`
	OperationFailures uint64                          `json:"operation_failures"`
	Failed            bool                            `json:"observer_failed"`
}

// Flat v1 serialization is retained exactly. The new protocol publishes an
// actual streamed index, never a fabricated v1 trace with discarded events.
func (s behavior76AuthorityTraceSnapshot) MarshalJSON() ([]byte, error) {
	if s.stream != nil {
		return json.Marshal(s.stream)
	}
	type plain behavior76AuthorityTraceSnapshot
	return json.Marshal(plain(s))
}

func (o *behavior76AuthorityTrace) enableStream(directory string, binding behavior76ScopeBinding, global *atomic.Bool) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stream != nil || global == nil || global.Load() || o.observerFailed || o.dropped != 0 || o.openSpans != 0 || len(o.events) >= behavior76StreamBuffer || binding.Window == "" || binding.Arm == "" || binding.Schema == "" || binding.Producer == "" || len(binding.ProtocolSHA256) != 64 {
		return errors.New("stream observer startup rejected")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return errors.New("private observer directory unavailable")
	}
	marker, err := os.OpenFile(filepath.Join(directory, "authority-stream-created"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return errors.New("new observer directory required")
	}
	if err = marker.Close(); err != nil {
		return err
	}
	identity := behavior76StreamIdentity{binding.Window, binding.Arm, binding.Schema, binding.Producer, binding.ProtocolSHA256}
	spool, err := behavior76NewPrivateStreamSpool(directory, identity)
	if err != nil {
		return err
	}
	o.stream = &behavior76AuthorityStream{directory: spool, destination: directory, identity: identity, global: global, segments: []behavior76StreamSegment{}}
	return nil
}

func (o *behavior76AuthorityTrace) failStreamLocked(category string) {
	if !o.observerFailed {
		log.Printf("behavior76 observer stream failure category=%s window=%s arm=%s", category, o.stream.identity.Window, o.stream.identity.Arm)
	}
	o.observerFailed = true
	o.stream.global.Store(true)
}

func (o *behavior76AuthorityTrace) appendStreamLocked(event behavior76AuthorityTraceEvent) {
	if o.stream.sealed || o.stream.recordingClosed || o.observerFailed || o.openSpans > behavior76StreamOpen {
		o.dropped++
		o.failStreamLocked("closed_failed_or_open_span_bound")
		return
	}
	o.events = append(o.events, event)
	if len(o.events) == behavior76StreamBuffer {
		if err := o.flushStreamLocked(); err != nil {
			o.failStreamLocked("segment_write_or_capacity")
		}
	}
}

// Bounded synchronous batches retain every actual event in the private local
// spool. Cross-filesystem publication does not run inside a pgx callback.
// No callback/error is invented or suppressed; SQL and deadlines are unchanged.
func (o *behavior76AuthorityTrace) flushStreamLocked() error {
	if len(o.events) == 0 {
		return nil
	}
	if len(o.events) > behavior76StreamBuffer || len(o.stream.segments) >= behavior76StreamSegments {
		return errors.New("observer segment bound")
	}
	var compressed bytes.Buffer
	writer, err := gzip.NewWriterLevel(&compressed, gzip.BestSpeed)
	if err != nil {
		return err
	}
	rawHash := sha256.New()
	var rawBytes int64
	for _, event := range o.events {
		line, err := json.Marshal(event)
		if err != nil || len(line) > behavior76StreamLine {
			_ = writer.Close()
			return errors.New("observer event encoding bound")
		}
		line = append(line, '\n')
		if _, err = writer.Write(line); err != nil {
			_ = writer.Close()
			return err
		}
		_, _ = rawHash.Write(line)
		rawBytes += int64(len(line))
	}
	if err = writer.Close(); err != nil {
		return err
	}
	if o.stream.compressed+int64(compressed.Len()) > behavior76StreamBytes {
		return errors.New("observer compressed-byte bound")
	}
	name := fmt.Sprintf("authority-segment-%06d.jsonl.gz", len(o.stream.segments)+1)
	file, err := os.OpenFile(filepath.Join(o.stream.directory, name), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(compressed.Bytes())
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	segment := behavior76StreamSegment{Path: name, First: o.events[0].Sequence, Last: o.events[len(o.events)-1].Sequence, Events: len(o.events), Compressed: int64(compressed.Len()), Uncompressed: rawBytes, SHA256: behavior76Hash(compressed.Bytes()), UncompressedSHA256: hex.EncodeToString(rawHash.Sum(nil))}
	o.stream.segments = append(o.stream.segments, segment)
	o.stream.compressed += segment.Compressed
	o.events = o.events[:0]
	return nil
}

func (o *behavior76AuthorityTrace) streamSnapshotLocked(tail []behavior76AuthorityTraceEvent) *behavior76AuthorityStreamSnapshot {
	segments := append([]behavior76StreamSegment{}, o.stream.segments...)
	return &behavior76AuthorityStreamSnapshot{Version: "behavior76-authority-pg-trace-stream-v2", Sealed: o.stream.sealed, EventCount: o.sequence, LastSequence: o.sequence, MaximumBuffered: behavior76StreamBuffer, MaximumSegments: behavior76StreamSegments, MaximumCompressed: behavior76StreamBytes, TotalCompressed: o.stream.compressed, Identity: o.stream.identity, Segments: segments, Tail: tail, Open: o.openSpans, Dropped: o.dropped, OperationFailures: o.operationFailures, Failed: o.observerFailed}
}

// Publish only after the fixture's actual worker/process/lease quiescence.
// A failed or late observer remains a shared GLOBAL fault, including after seal.
func (o *behavior76AuthorityTrace) sealStream() behavior76AuthorityTraceSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stream != nil && !o.stream.sealed {
		alreadyClosed := o.stream.recordingClosed
		o.stream.recordingClosed = true
		if alreadyClosed || o.stream.global.Load() || o.openSpans != 0 || o.observerFailed || o.dropped != 0 {
			o.failStreamLocked("unsettled_seal")
		} else if err := o.flushStreamLocked(); err != nil {
			o.failStreamLocked("final_segment_write_or_capacity")
		}
		if err := o.publishStreamLocked(); err != nil {
			o.failStreamLocked("segment_publication_or_readback")
		}
		if !o.observerFailed && o.openSpans == 0 && o.dropped == 0 {
			if err := o.retireStreamSpoolLocked(); err != nil {
				o.failStreamLocked("private_spool_retirement")
			} else {
				o.stream.sealed = true
			}
		}
		index := o.streamSnapshotLocked(append([]behavior76AuthorityTraceEvent{}, o.events...))
		encoded, err := json.MarshalIndent(index, "", "  ")
		if err != nil || len(encoded)+1 > behavior76StreamHeader {
			o.failStreamLocked("terminal_header_bound")
			o.stream.sealed = false
		}
		if !o.stream.sealed {
			o.retainUnsealedStreamLocked()
		}
	}
	return o.snapshotLocked()
}

func behavior76PublicStreamBinding() behavior76ScopeBinding {
	return behavior76ScopeBinding{Window: "public-stream-mechanism", Arm: "public-observer-only", Schema: "memory_public", Producer: "public-simulated-events-not-native", ProtocolSHA256: behavior76Hash([]byte("public observer mechanism"))}
}

// The final fixture must fail its actual test even if no subsequent arm checks
// GLOBAL. Run the expected-failing closeout in an owned subprocess, retaining
// the real failed index and inventory for assertions in the parent.
func TestBehavior76AuthorityStreamFinalSealFailure(t *testing.T) {
	const key = "BUTLER_BEHAVIOR76_STREAM_SEAL_FAILURE_HELPER"
	if directory := os.Getenv(key); directory != "" {
		var global atomic.Bool
		trace := newBehavior76AuthorityTrace()
		if err := trace.enableStream(directory, behavior76PublicStreamBinding(), &global); err != nil {
			t.Fatal(err)
		}
		span := trace.start(context.Background(), "query", "other_query", "", nil)
		trace.end(context.Background(), span, "query", nil, nil)
		if err := os.WriteFile(filepath.Join(directory, "authority-segment-000001.jsonl.gz"), []byte("exclusive collision preserved"), 0600); err != nil {
			t.Fatal(err)
		}
		anchor := filepath.Join(directory, "source-anchor")
		if err := os.WriteFile(anchor, []byte("public exact deletion anchor"), 0600); err != nil {
			t.Fatal(err)
		}
		f := &behavior76Fixture{t: t, authorityTrace: trace, researchStop: &global, streamQuiescent: true, manifestPath: anchor}
		f.sealFixtureEvidence(directory)
		if global.Load() {
			t.Log("sealed_failure_retained_global=true")
		}
		return
	}
	directory := t.TempDir()
	command := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestBehavior76AuthorityStreamFinalSealFailure$", "-test.v")
	command.Env = append(os.Environ(), key+"="+directory)
	output, err := command.CombinedOutput()
	if err == nil || !bytes.Contains(output, []byte("sealed_failure_retained_global=true")) || !bytes.Contains(output, []byte("stream evidence retained but observer/quiescence seal failed globally")) {
		t.Fatal("last-arm seal failure was not an actual failed closeout", err, string(output))
	}
	var index behavior76AuthorityStreamSnapshot
	body, err := os.ReadFile(filepath.Join(directory, "authority-trace.json"))
	if err != nil || json.Unmarshal(body, &index) != nil || index.Sealed || !index.Failed || index.EventCount != 2 {
		t.Fatal("failed original observer evidence missing", err)
	}
	if _, err = os.Stat(filepath.Join(directory, "sealed-files.json")); err != nil {
		t.Fatal("failure inventory missing", err)
	}
	body, err = os.ReadFile(filepath.Join(directory, "authority-segment-000001.jsonl.gz"))
	if err != nil || string(body) != "exclusive collision preserved" {
		t.Fatal("collision file was replaced", err)
	}
}

func TestBehavior76AuthorityStreamBeyondLegacyCapacity(t *testing.T) {
	directory := t.TempDir()
	if output := os.Getenv("BUTLER_BEHAVIOR76_STREAM_PUBLIC_OUTPUT"); output != "" {
		if output != "/src20/.cache/behavior76/stream-go-public-v1" {
			t.Fatal("fixed public output required")
		}
		if err := os.Mkdir(output, 0700); err != nil {
			t.Fatal("new public evidence directory required", err)
		}
		directory = output
	}
	var global atomic.Bool
	trace := newBehavior76AuthorityTrace()
	if err := trace.enableStream(directory, behavior76PublicStreamBinding(), &global); err != nil {
		t.Fatal(err)
	}
	operations := behavior76AuthorityTraceLimit/2 + 17
	for i := 0; i < operations; i++ {
		span := trace.start(context.Background(), "query", "other_query", "", nil)
		var err error
		if i >= operations-2 {
			err = context.Canceled
		}
		trace.end(context.Background(), span, "query", err, nil)
		if len(trace.events) >= behavior76StreamBuffer {
			t.Fatal("bounded event memory exceeded")
		}
	}
	result := trace.sealStream()
	if global.Load() || result.Failed || result.OpenSpans != 0 || result.Dropped != 0 || result.OperationFailures != 2 || result.stream == nil || !result.stream.Sealed || result.stream.EventCount != uint64(operations*2) || len(result.stream.Tail) != 0 {
		t.Fatal("lossless stream did not seal actual events/counters")
	}
	var decoded []behavior76AuthorityTraceEvent
	for _, segment := range result.stream.Segments {
		body, err := os.ReadFile(filepath.Join(directory, segment.Path))
		if err != nil || behavior76Hash(body) != segment.SHA256 || int64(len(body)) != segment.Compressed {
			t.Fatal("actual compressed bytes mismatch", err)
		}
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(reader)
		closeErr := reader.Close()
		if err != nil || closeErr != nil || behavior76Hash(raw) != segment.UncompressedSHA256 || int64(len(raw)) != segment.Uncompressed {
			t.Fatal("actual raw bytes mismatch", err, closeErr)
		}
		lines := bytes.Split(bytes.TrimSuffix(raw, []byte{'\n'}), []byte{'\n'})
		if len(lines) != segment.Events {
			t.Fatal("chunk event count changed")
		}
		for _, line := range lines {
			var event behavior76AuthorityTraceEvent
			if json.Unmarshal(line, &event) != nil {
				t.Fatal("actual event invalid")
			}
			decoded = append(decoded, event)
		}
	}
	if len(decoded) != operations*2 {
		t.Fatal("events dropped beyond original65536 boundary")
	}
	for i, event := range decoded {
		if event.Sequence != uint64(i+1) {
			t.Fatal("actual event order lost")
		}
	}
	behavior76Write(t, filepath.Join(directory, "authority-trace.json"), result)
	t.Logf("public simulated observer events only: %d exact original events/%d bounded segments; real canceled errors retained2; native/auth=0", len(decoded), len(result.stream.Segments))
}

func TestBehavior76AuthorityStreamWriteFailureAndLateEventStop(t *testing.T) {
	for _, kind := range []string{"create_new_collision", "late_after_seal", "late_after_unsealed_close"} {
		t.Run(kind, func(t *testing.T) {
			directory := t.TempDir()
			var global atomic.Bool
			trace := newBehavior76AuthorityTrace()
			if err := trace.enableStream(directory, behavior76PublicStreamBinding(), &global); err != nil {
				t.Fatal(err)
			}
			span := trace.start(context.Background(), "query", "other_query", "", nil)
			trace.end(context.Background(), span, "query", nil, nil)
			if kind == "create_new_collision" {
				path := filepath.Join(directory, "authority-segment-000001.jsonl.gz")
				if err := os.WriteFile(path, []byte("retained existing file"), 0600); err != nil {
					t.Fatal(err)
				}
				result := trace.sealStream()
				if result.stream.Sealed || !result.Failed {
					t.Fatal("failed write became qualified sealed evidence")
				}
				body, err := os.ReadFile(path)
				if err != nil || string(body) != "retained existing file" {
					t.Fatal("existing evidence overwritten", err)
				}
			} else {
				var retained string
				if kind == "late_after_unsealed_close" {
					result := trace.unsealedStreamEvidence()
					if result.stream.Sealed || result.Failed || !trace.stream.recordingClosed {
						t.Fatal("unqualified prefix was not terminally closed")
					}
					body, err := os.ReadFile(filepath.Join(trace.stream.directory, "unsealed-authority-trace.json"))
					if err != nil {
						t.Fatal("exact private prefix unavailable")
					}
					retained = behavior76Hash(body)
				} else if result := trace.sealStream(); !result.stream.Sealed || result.Failed {
					t.Fatal("healthy seal failed")
				}
				before := trace.snapshot()
				late := trace.start(context.Background(), "query", "other_query", "", nil)
				trace.end(context.Background(), late, "query", nil, nil)
				if kind == "late_after_unsealed_close" {
					after := trace.snapshot()
					body, err := os.ReadFile(filepath.Join(trace.stream.directory, "unsealed-authority-trace.json"))
					if err != nil || behavior76Hash(body) != retained || len(after.Events) != len(before.Events) || len(after.stream.Segments) != len(before.stream.Segments) || after.Dropped != before.Dropped+2 || !after.Failed {
						t.Fatal("late callback rewrote the frozen prefix or escaped the global fault")
					}
				}
			}
			if !global.Load() || !trace.failed() {
				t.Fatal("observer fault did not seal shared GLOBAL")
			}
			scope := newBehavior76FixtureScope(nil, &global)
			called := false
			if _, err := scope.begin(context.Background(), func() error { called = true; return nil }); err == nil || called {
				t.Fatal("observer fault admitted a later reservation")
			}
		})
	}
}
