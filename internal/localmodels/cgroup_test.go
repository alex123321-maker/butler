package localmodels

import (
	"context"
	"fmt"
	"sync"
	"testing"

	pb "github.com/butler/butler/internal/gen/localmodels/v1"
)

func TestCgroupAdmissionCleanInactiveCacheOnly(t *testing.T) {
	for _, tc := range []struct {
		name, stat string
		want       uint64
	}{
		{"clean", "file 800\ninactive_file 600\nfile_dirty 0\nfile_writeback 0\n", 800},
		{"dirty", "file 800\ninactive_file 600\nfile_dirty 100\nfile_writeback 50\n", 650},
		{"fully_dirty", "file 800\ninactive_file 200\nfile_dirty 200\nfile_writeback 100\n", 200},
		{"active_file_and_anon_remain_charged", "file 800\ninactive_file 0\nfile_dirty 0\nfile_writeback 0\n", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, e := cgroupHeadroom(1200, 1000, []byte(tc.stat))
			if e != nil || got != tc.want {
				t.Fatal(got, e)
			}
		})
	}
}
func TestCgroupAdmissionMalformedMissingOrImpossibleDenied(t *testing.T) {
	for _, raw := range []string{"", "file 800\ninactive_file 600\nfile_dirty 0\n", "file x\ninactive_file 600\nfile_dirty 0\nfile_writeback 0\n", "file 800 1\ninactive_file 600\nfile_dirty 0\nfile_writeback 0\n", "file 1001\ninactive_file 600\nfile_dirty 0\nfile_writeback 0\n", "file 800\ninactive_file 801\nfile_dirty 0\nfile_writeback 0\n", "file 800\ninactive_file 600\nfile_dirty 700\nfile_writeback 200\n", "file 800\nfile 800\ninactive_file 600\nfile_dirty 0\nfile_writeback 0\n", "file 800\ninactive_file -1\nfile_dirty 0\nfile_writeback 0\n", "file 18446744073709551616\ninactive_file 0\nfile_dirty 0\nfile_writeback 0\n"} {
		if _, e := cgroupHeadroom(1200, 1000, []byte(raw)); e == nil {
			t.Fatal("invalid cgroup telemetry admitted")
		}
	}
	if _, e := cgroupHeadroom(1000, 1000, []byte("file 800\ninactive_file 600\nfile_dirty 0\nfile_writeback 0")); e == nil {
		t.Fatal("hard cap reached was credited")
	}
}

func TestCleanCacheCreditDoesNotOvercommitConcurrentLanes(t *testing.T) {
	const mib = uint64(1024 * 1024)
	raw := []byte(fmt.Sprintf("file %d\ninactive_file %d\nfile_dirty 0\nfile_writeback 0\n", 6*1024*mib, 4*1024*mib))
	headroom, err := cgroupHeadroom(8*1024*mib, 7*1024*mib, raw)
	if err != nil {
		t.Fatal(err)
	}
	ram := &fakeRAM{used: 100, available: uint32(headroom / mib)}
	p := profile()
	p.PeakRamMib = 4000
	cfg := Config{RAMTelemetry: ram, RAMBudgetMiB: 6144, RAMReserveMiB: 512, RAMGate: &RAMGate{}}
	first := newManager(t, cfg, []*pb.ModelProfile{p}, &fakeBackend{}, &fakeTelemetry{})
	second := newManager(t, cfg, []*pb.ModelProfile{p}, &fakeBackend{}, &fakeTelemetry{})
	start := make(chan struct{})
	results := make(chan func(), 8)
	var wg sync.WaitGroup
	for i := range 8 {
		m := first
		if i%2 == 1 {
			m = second
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			release, ok := m.acquireRAM(context.Background(), p, "gpu")
			if !ok {
				release = nil
			}
			results <- release
		}()
	}
	close(start)
	wg.Wait()
	close(results)
	var releases []func()
	for release := range results {
		if release != nil {
			releases = append(releases, release)
		}
	}
	if len(releases) != 1 {
		t.Fatalf("expected one held reservation, got %d", len(releases))
	}
	releases[0]()
	release, ok := second.acquireRAM(context.Background(), p, "gpu")
	if !ok {
		t.Fatal("released allowance not reusable")
	}
	release()
}
