package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
)

// Native research executes inside the measured local Linux root. Its /tmp is
// private local ext4, not the Windows evidence mount. No model/environment value
// selects this spool and no automatic TempDir cleanup erases a failed prefix.
func behavior76NewPrivateStreamSpool(destination string, identity behavior76StreamIdentity) (string, error) {
	parent := os.TempDir()
	if runtime.GOOS == "linux" {
		parent = "/tmp"
	}
	spool, err := os.MkdirTemp(parent, "behavior76-authority-spool-")
	if err != nil {
		return "", err
	}
	if err = os.Chmod(spool, 0700); err != nil {
		return "", err
	}
	body, err := json.Marshal(map[string]any{"version": "behavior76-private-authority-spool-v1", "identity": identity, "destination": destination})
	if err != nil || len(body) > behavior76StreamHeader {
		return "", errors.New("private observer identity bound")
	}
	if err = behavior76WritePrivateStreamFile(filepath.Join(spool, "identity.json"), body); err != nil {
		return "", err
	}
	return spool, nil
}

func behavior76WritePrivateStreamFile(path string, body []byte) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(body)
	syncErr := file.Sync()
	closeErr := file.Close()
	return errors.Join(writeErr, syncErr, closeErr)
}

func behavior76ReadExactStreamSegment(directory string, segment behavior76StreamSegment, index int) ([]byte, error) {
	if segment.Path != fmt.Sprintf("authority-segment-%06d.jsonl.gz", index+1) || segment.Compressed < 1 || segment.Compressed > behavior76StreamBytes {
		return nil, errors.New("observer segment identity bound")
	}
	path := filepath.Join(directory, segment.Path)
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != segment.Compressed {
		return nil, errors.New("observer segment unavailable")
	}
	body, err := os.ReadFile(path)
	if err != nil || int64(len(body)) != segment.Compressed || behavior76Hash(body) != segment.SHA256 {
		return nil, errors.New("observer segment bytes changed")
	}
	return body, nil
}

// The caller holds trace.mu. Qualified callers have already joined the real
// workers. Unqualified closeout can publish only this exact unsealed prefix;
// no tail is flushed and no quiescence/completeness is inferred in that path.
func (o *behavior76AuthorityTrace) publishStreamLocked() error {
	for i, segment := range o.stream.segments {
		if i >= o.stream.published {
			body, err := behavior76ReadExactStreamSegment(o.stream.directory, segment, i)
			if err != nil {
				return err
			}
			if err = behavior76WritePrivateStreamFile(filepath.Join(o.stream.destination, segment.Path), body); err != nil {
				return err
			}
		}
		if _, err := behavior76ReadExactStreamSegment(o.stream.destination, segment, i); err != nil {
			return err
		}
		if i >= o.stream.published {
			o.stream.published++
		}
	}
	return nil
}

// Remove only our enumerated duplicate chunks after every destination byte was
// synced and read back. No recursive deletion occurs. The tiny private identity
// remains. A failure makes the public header unsealed/GLOBAL; already removed
// duplicates still have their verified destination copies, never a lost original.
func (o *behavior76AuthorityTrace) retireStreamSpoolLocked() error {
	if o.stream.retired {
		return nil
	}
	if o.stream.published != len(o.stream.segments) {
		return errors.New("observer publication incomplete")
	}
	for i, segment := range o.stream.segments {
		if _, err := behavior76ReadExactStreamSegment(o.stream.directory, segment, i); err != nil {
			return err
		}
		if _, err := behavior76ReadExactStreamSegment(o.stream.destination, segment, i); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(o.stream.directory, segment.Path)); err != nil {
			return err
		}
	}
	o.stream.retired = true
	return nil
}

func (o *behavior76AuthorityTrace) retainUnsealedStreamLocked() {
	body, err := json.Marshal(o.streamSnapshotLocked(append([]behavior76AuthorityTraceEvent{}, o.events...)))
	if err != nil || len(body) > behavior76StreamHeader {
		o.failStreamLocked("private_unsealed_header_bound")
		return
	}
	if err = behavior76WritePrivateStreamFile(filepath.Join(o.stream.directory, "unsealed-authority-trace.json"), body); err != nil {
		o.failStreamLocked("private_unsealed_header_write")
	}
}

func (o *behavior76AuthorityTrace) unsealedStreamEvidence() behavior76AuthorityTraceSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.stream != nil {
		o.stream.global.Store(true)
		o.stream.recordingClosed = true
		o.stream.sealed = false
		if err := o.publishStreamLocked(); err != nil {
			o.failStreamLocked("unsealed_segment_publication_or_readback")
		}
		o.retainUnsealedStreamLocked()
	}
	return o.snapshotLocked()
}

func TestBehavior76AuthorityStreamStorageProfile(t *testing.T) {
	directory := t.TempDir()
	measuredOutput := false
	if runtime.GOOS == "linux" {
		if info, err := os.Stat("/src20/.cache/behavior76"); err == nil && info.IsDir() {
			directory, err = os.MkdirTemp("/src20/.cache/behavior76", "public-spool-profile-")
			if err != nil {
				t.Fatal("new public destination unavailable")
			}
			measuredOutput = true
		}
	}
	var global atomic.Bool
	trace := newBehavior76AuthorityTrace()
	if err := trace.enableStream(directory, behavior76PublicStreamBinding(), &global); err != nil {
		t.Fatal(err)
	}
	spool := trace.stream.directory
	if spool == directory || runtime.GOOS == "linux" && filepath.Dir(spool) != "/tmp" {
		t.Fatal("callback storage was not the code-owned private local spool")
	}
	if info, err := os.Stat(spool); err != nil || runtime.GOOS == "linux" && info.Mode().Perm() != 0700 {
		t.Fatal("private spool permissions unavailable")
	}
	if measuredOutput {
		filesystem, err := behavior76SpoolFilesystem(t.Context(), spool)
		if err != nil {
			t.Fatal("actual private spool filesystem unavailable")
		}
		t.Log("actual private spool filesystem witness", behavior76JSON(filesystem))
	}
	var maximum time.Duration
	for range behavior76StreamBuffer {
		span := trace.start(context.Background(), "query", "other_query", "", nil)
		started := time.Now()
		trace.end(context.Background(), span, "query", nil, nil)
		maximum = max(maximum, time.Since(started))
	}
	before := trace.snapshot()
	if global.Load() || before.stream == nil || before.stream.Sealed || len(before.stream.Segments) != 2 || before.Dropped != 0 || before.OpenSpans != 0 {
		t.Fatal("ordinary private callback recording failed")
	}
	for i, segment := range before.stream.Segments {
		if _, err := behavior76ReadExactStreamSegment(spool, segment, i); err != nil {
			t.Fatal("private segment did not retain its exact bytes")
		}
		if _, err := os.Stat(filepath.Join(directory, segment.Path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("callback performed destination publication")
		}
	}
	sealed := trace.sealStream()
	if global.Load() || sealed.stream == nil || !sealed.stream.Sealed || sealed.Failed || len(sealed.stream.Segments) != 2 || maximum >= 750*time.Millisecond {
		t.Fatal("private callback/seal exceeded the unchanged authority bound or failed")
	}
	for i, segment := range sealed.stream.Segments {
		if _, err := behavior76ReadExactStreamSegment(directory, segment, i); err != nil {
			t.Fatal("published segment failed exact readback")
		}
		if _, err := os.Stat(filepath.Join(spool, segment.Path)); !errors.Is(err, os.ErrNotExist) {
			t.Fatal("verified duplicate segment was not retired")
		}
	}
	behavior76Write(t, filepath.Join(directory, "authority-trace.json"), sealed)
	t.Logf("PRIVATE_SPOOL_PUBLIC_PROFILE events=%d segments=2 destination_on_measured_output=%t maximum_end_callback_us=%d original_deadline_ms=750 destination_IO_inside_callback=false native_auth_calls=0 original_closed_failure_not_reclassified=true", sealed.stream.EventCount, measuredOutput, maximum.Microseconds())
}

func TestBehavior76AuthorityStreamUnsealedPrivatePrefix(t *testing.T) {
	directory := t.TempDir()
	var global atomic.Bool
	trace := newBehavior76AuthorityTrace()
	if err := trace.enableStream(directory, behavior76PublicStreamBinding(), &global); err != nil {
		t.Fatal(err)
	}
	for range behavior76StreamBuffer/2 + 1 {
		span := trace.start(context.Background(), "query", "other_query", "", nil)
		trace.end(context.Background(), span, "query", nil, nil)
	}
	prefix := trace.unsealedStreamEvidence()
	if !global.Load() || prefix.stream.Sealed || prefix.Failed || prefix.OpenSpans != 0 || prefix.Dropped != 0 || len(prefix.stream.Segments) != 1 || len(prefix.stream.Tail) != 2 {
		t.Fatal("unqualified prefix became a complete trace or lost observed metadata")
	}
	segment := prefix.stream.Segments[0]
	for _, path := range []string{trace.stream.directory, directory} {
		if _, err := behavior76ReadExactStreamSegment(path, segment, 0); err != nil {
			t.Fatal("unsealed private/public prefix bytes unavailable")
		}
	}
	raw, err := os.ReadFile(filepath.Join(trace.stream.directory, "unsealed-authority-trace.json"))
	var retained behavior76AuthorityStreamSnapshot
	if err != nil || json.Unmarshal(raw, &retained) != nil || retained.Sealed || retained.EventCount != prefix.stream.EventCount || len(retained.Tail) != 2 {
		t.Fatal("private unsealed tail/identity not retained")
	}
	scope := newBehavior76FixtureScope(nil, &global)
	if _, err := scope.begin(context.Background(), nil); err == nil {
		t.Fatal("unqualified closeout permitted another admission")
	}
}
