//go:build phase29preflight

package app

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Failure cases: extra cross-block repeats, changed original repeat targets,
// pure-miss rewriting, an explicit count overwritten, and partial formal blocks.
func TestControlledScheduleFixedBlocks(t *testing.T) {
	wantRepeats := map[int]int{21: 1, 42: 22, 63: 43, 84: 64,
		121: 101, 142: 122, 163: 143, 184: 164, 221: 201, 242: 222, 263: 243, 284: 264}
	for _, test := range []struct{ count, every, repeats int }{{100, 21, 4}, {300, 21, 12}, {300, 0, 0}} {
		t.Run(fmt.Sprintf("count=%d/every=%d", test.count, test.every), func(t *testing.T) {
			samples := controlledScheduleSamples(t, test.count, test.every)
			var repeats int
			for position, sample := range samples {
				index, want := position+1, position+1
				if repeat, exists := wantRepeats[index]; exists && test.every > 0 {
					want = repeat
				}
				if !sample.OK || sample.Index != want {
					t.Errorf("slot %d: ok=%t requested index=%d, want %d", index, sample.OK, sample.Index, want)
				}
				if sample.Index != index {
					repeats++
				}
			}
			if repeats != test.repeats {
				t.Errorf("repeats=%d, want %d", repeats, test.repeats)
			}
		})
	}
}

func controlledScheduleSamples(t *testing.T, count, every int) []shipSample {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"content":"fixture response"}}]}`)
	}))
	t.Cleanup(server.Close)
	options := liveRequestOptions{client: server.Client(), url: server.URL, model: "fixture", hitEvery: every, origin: time.Now()}
	samples, sem := make([]shipSample, count), make(chan struct{}, 1)
	var group sync.WaitGroup
	var active atomic.Int64
	for index := 1; index <= count; index++ {
		sem <- struct{}{}
		group.Add(1)
		liveScheduledRequest{options: options, index: index, sem: sem, samples: samples, group: &group, active: &active}.execute()
	}
	group.Wait()
	return samples
}

func TestControlledHitHonorsRequestCount(t *testing.T) {
	for _, test := range []struct{ value, want string }{{"", "100"}, {"100", "100"}, {"300", "300"}} {
		t.Run("count="+test.value, func(t *testing.T) {
			t.Setenv("PHASE29_COUNT", test.value)
			t.Setenv("PHASE29_TIMING_DISABLED", "false")
			configureControlledHit(t, controlledHitScenario{mode: "on", reuseMode: "semantic"})
			if count := os.Getenv("PHASE29_COUNT"); count != test.want {
				t.Errorf("configured count=%s, want %s", count, test.want)
			}
			if os.Getenv("PHASE29_MEMO_CAPACITY") != "0" || os.Getenv("PHASE29_REUSE_MODE") != "semantic" {
				t.Fatal("controlled hit profile changed")
			}
		})
	}
}

func TestControlledProfileRejectsPartialBlocks(t *testing.T) {
	if os.Getenv("VELOXMESH_CONTROLLED_COUNT_CHILD") == "true" {
		configureControlledProfile(t, controlledProfile{mode: "off", capacity: "0"})
		return
	}
	for _, count := range []string{"0", "99", "101", "invalid"} {
		t.Run("count="+count, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestControlledProfileRejectsPartialBlocks$", "-test.timeout=60s", "-test.v")
			command.Env = append(os.Environ(), "VELOXMESH_CONTROLLED_COUNT_CHILD=true", "PHASE29_COUNT="+count)
			output, err := command.CombinedOutput()
			if err == nil || !strings.Contains(string(output), "controlled count must be a positive multiple of 100") {
				t.Fatalf("invalid count=%s was not rejected correctly: error=%v output=%s", count, err, output)
			}
		})
	}
}

func TestControlledHitUsesActualSampleCount(t *testing.T) {
	t.Setenv("PHASE29_COUNT", "300")
	samples := make([]shipSample, 300)
	for index := range samples {
		samples[index] = shipSample{OK: true, Concurrent: 1}
	}
	controlledHitSamples(t, samples, controlledHitExpectation{scenario: controlledHitScenario{mode: "off"}})
}
