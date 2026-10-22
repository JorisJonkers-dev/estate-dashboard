package domain_test

import (
	"slices"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/health/domain"
)

const step = 5 * time.Second

var (
	open = time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC)
	shut = open.Add(10 * time.Minute)
)

func at(seconds int) time.Time { return open.Add(time.Duration(seconds) * time.Second) }

func window(o domain.Observed) domain.Window { return domain.WindowOf(o, open, shut, step) }

// states is a container's changes as "state@seconds".
func states(c domain.ContainerSpan) []string {
	out := make([]string, 0, len(c.Changes))
	for _, ch := range c.Changes {
		out = append(out, string(ch.State)+"@"+ch.At.Sub(open).String())
	}
	return out
}

func samples(by map[int]domain.ContainerSample) map[time.Time]domain.ContainerSample {
	out := map[time.Time]domain.ContainerSample{}
	for s, sample := range by {
		out[at(s)] = sample
	}
	return out
}

func TestAPodsContainersAreItsSidecarsAndInitContainersNotItsProcess(t *testing.T) {
	pod := "auth-api-7f8c9d6b5-kq2lm"
	w := window(domain.Observed{
		Readiness: domain.Series{pod: {at(0): 1}},
		Containers: []domain.ContainerSeries{
			{Pod: pod, Name: "auth-api", Samples: samples(map[int]domain.ContainerSample{0: {Running: true, Ready: true}})},
			{Pod: pod, Name: "exporter", Samples: samples(map[int]domain.ContainerSample{0: {Running: true}, 5: {Running: true, Ready: true}})},
			{Pod: pod, Name: "vault-agent-init", Init: true, Samples: samples(map[int]domain.ContainerSample{0: {}, 5: {Ended: "Completed"}})},
			{Pod: pod, Name: "vpn", Init: true, Restarts: true, Samples: samples(map[int]domain.ContainerSample{0: {Running: true, Ready: true}})},
			{Pod: pod, Name: "unseen", Init: true, Samples: map[time.Time]domain.ContainerSample{}},
			{Pod: "another-pod", Name: "stray", Samples: samples(map[int]domain.ContainerSample{0: {Running: true}})},
		},
	})
	got := w.Pods[0].Containers
	var names []string
	for _, c := range got {
		names = append(names, c.Name+" "+string(c.Kind))
	}
	if want := []string{"exporter sidecar", "vault-agent-init init", "vpn sidecar"}; !slices.Equal(names, want) {
		t.Fatalf("containers = %v", names)
	}
	if s := states(got[0]); !slices.Equal(s, []string{"running@0s", "ready@5s"}) {
		t.Fatalf("exporter = %v", s)
	}
	if s := states(got[1]); !slices.Equal(s, []string{"waiting@0s", "succeeded@5s"}) {
		t.Fatalf("init = %v", s)
	}
}

func TestAPodWithNoContainerNamedForItsProcessHasTheFirstForIt(t *testing.T) {
	pod := "vault-csi-provider-6swqn"
	running := samples(map[int]domain.ContainerSample{0: {Running: true}})
	w := window(domain.Observed{
		Readiness: domain.Series{pod: {at(0): 1}},
		Containers: []domain.ContainerSeries{
			{Pod: pod, Name: "provider", Samples: running},
			{Pod: pod, Name: "agent", Samples: running},
		},
	})
	if c := w.Pods[0].Containers; len(c) != 1 || c[0].Name != "provider" {
		t.Fatalf("containers = %+v", c)
	}
}

func TestAContainerStateIsWhatEachSampleSays(t *testing.T) {
	pod := "notes-6d9f7c5b8-2hxlq"
	w := window(domain.Observed{
		Readiness: domain.Series{pod: {at(0): 1}},
		Containers: []domain.ContainerSeries{
			{Pod: pod, Name: "notes", Samples: samples(map[int]domain.ContainerSample{0: {Running: true}})},
			{Pod: pod, Name: "migrate", Init: true, Samples: samples(map[int]domain.ContainerSample{0: {Running: true}, 5: {Ended: "Error"}, 10: {}, 15: {Ready: true}})},
			{Pod: pod, Name: "backup", Samples: samples(map[int]domain.ContainerSample{0: {Ready: true}, 5: {Ended: "OOMKilled"}})},
		},
	})
	got := w.Pods[0].Containers
	if s := states(got[0]); !slices.Equal(s, []string{"waiting@0s", "failed@5s"}) {
		t.Fatalf("backup = %v", s)
	}
	if s := states(got[1]); !slices.Equal(s, []string{"running@0s", "failed@5s", "waiting@10s", "succeeded@15s"}) {
		t.Fatalf("migrate = %v", s)
	}
}

func TestAnInitContainerThatRanBetweenTwoSamplesStillRan(t *testing.T) {
	pod := "auth-api-7f8c9d6b5-kq2lm"
	started := func(s int) *time.Time { t := at(s); return &t }
	init := func(name string, from *time.Time, by map[int]domain.ContainerSample) domain.ContainerSeries {
		return domain.ContainerSeries{Pod: pod, Name: name, Init: true, Started: from, Samples: samples(by)}
	}
	quick := map[int]domain.ContainerSample{0: {}, 5: {}, 10: {Ended: "Completed"}}
	w := window(domain.Observed{
		Readiness: domain.Series{pod: {at(0): 1}},
		Containers: []domain.ContainerSeries{
			{Pod: pod, Name: "auth-api", Samples: samples(map[int]domain.ContainerSample{0: {Running: true}})},
			init("a-between", started(7), quick),
			init("b-no-record", nil, quick),
			init("c-seen-running", started(3), map[int]domain.ContainerSample{0: {}, 5: {Running: true}, 10: {Ended: "Completed"}}),
			init("d-before-the-window", started(-60), map[int]domain.ContainerSample{0: {Ended: "Completed"}}),
			init("e-before-it-waited", started(-5), quick),
			init("f-after-it-ended", started(12), quick),
			init("g-still-waiting", started(3), map[int]domain.ContainerSample{0: {}}),
		},
	})
	want := map[string][]string{
		"a-between":           {"waiting@0s", "running@7s", "succeeded@10s"},
		"b-no-record":         {"waiting@0s", "succeeded@10s"},
		"c-seen-running":      {"waiting@0s", "running@5s", "succeeded@10s"},
		"d-before-the-window": {"succeeded@0s"},
		"e-before-it-waited":  {"waiting@0s", "succeeded@10s"},
		"f-after-it-ended":    {"waiting@0s", "succeeded@10s"},
		"g-still-waiting":     {"waiting@0s"},
	}
	for _, c := range w.Pods[0].Containers {
		if s := states(c); !slices.Equal(s, want[c.Name]) {
			t.Fatalf("%s = %v, want %v", c.Name, s, want[c.Name])
		}
	}
}

func phases(by map[int]string) map[time.Time]string {
	out := map[time.Time]string{}
	for s, phase := range by {
		out[at(s)] = phase
	}
	return out
}

func TestARunIsAJobsPodFromWhenItStartedToHowItEnded(t *testing.T) {
	recorded := func(s int) *time.Time { t := at(s); return &t }
	w := window(domain.Observed{Jobs: []domain.JobPod{
		{Pod: "policy", Job: "vault-policy-29877720", CronJob: "vault-policy", Start: recorded(38), End: recorded(42), Phases: phases(map[int]string{40: "Running", 45: "Succeeded", 50: "Succeeded"})},
		{Pod: "migration", Job: "auth-migration-up", Phases: phases(map[int]string{20: "Pending", 25: "Running", 30: "Failed"})},
		{Pod: "backup", Job: "postgres-data-backup-29877195", CronJob: "postgres-data-backup", Phases: phases(map[int]string{500: "Running", 600: "Running"})},
		{Pod: "deleted", Job: "catalog", Phases: phases(map[int]string{100: "Running", 105: "Running"})},
		{Pod: "brief", Job: "catalog-2", Phases: phases(map[int]string{200: "Succeeded"})},
		{Pod: "yesterdays", Job: "vault-policy-29876280", End: recorded(-3600), Phases: phases(map[int]string{0: "Succeeded", 600: "Succeeded"})},
		{Pod: "unrecorded", Job: "vault-policy-29876340", Phases: phases(map[int]string{0: "Succeeded", 600: "Succeeded"})},
		{Pod: "never-sampled", Job: "x", Phases: map[time.Time]string{}},
		{Pod: "brief-too", Job: "catalog-3", Phases: phases(map[int]string{200: "Succeeded"})},
	}})
	type run struct {
		pod, cron, outcome string
		start              time.Time
		end                *time.Time
	}
	var got []run
	for _, r := range w.Runs {
		got = append(got, run{r.Pod, r.CronJob, string(r.Outcome), r.Start, r.End})
	}
	want := []run{
		{"migration", "", "failed", at(20), recorded(30)},
		{"policy", "vault-policy", "succeeded", at(38), recorded(42)},
		{"deleted", "", "unknown", at(100), recorded(105)},
		{"brief", "", "succeeded", at(200), recorded(200)},
		{"brief-too", "", "succeeded", at(200), recorded(200)},
		{"backup", "postgres-data-backup", "running", at(500), nil},
	}
	if len(got) != len(want) {
		t.Fatalf("runs = %+v", got)
	}
	for i := range want {
		g, wnt := got[i], want[i]
		if g.pod != wnt.pod || g.cron != wnt.cron || g.outcome != wnt.outcome || !g.start.Equal(wnt.start) || (g.end == nil) != (wnt.end == nil) || (g.end != nil && !g.end.Equal(*wnt.end)) {
			t.Fatalf("run %d = %+v, want %+v", i, g, wnt)
		}
	}
}
