package domain

import (
	"regexp"
	"slices"
	"strings"
	"time"
)

// ReadinessCheck is whether a pod was ready at one moment.
type ReadinessCheck struct {
	At    time.Time
	Ready bool
}

// PodSpan is one pod over a release's window: when it was first seen, first ready and last seen,
// and its readiness at each step.
type PodSpan struct {
	Pod     string
	Process string
	Start   *time.Time
	Ready   *time.Time
	// End is when it was last seen, where it was gone before the window ended.
	End    *time.Time
	Checks []ReadinessCheck
	// Containers is each of its sidecars and init containers; the container that is its Process
	// is the pod's own readiness.
	Containers []ContainerSpan
}

// ContainerKind is how long a container of a pod runs.
type ContainerKind string

// A sidecar runs beside its pod's Process for as long as the pod does; an init container runs
// once, before it, and ends.
const (
	SidecarContainer ContainerKind = "sidecar"
	InitContainer    ContainerKind = "init"
)

// ContainerState is what a container was doing at one moment.
type ContainerState string

// The states a container is in: waiting to start, running but not ready, running and ready, or
// ended, having succeeded or failed.
const (
	Waiting   ContainerState = "waiting"
	Running   ContainerState = "running"
	Ready     ContainerState = "ready"
	Succeeded ContainerState = "succeeded"
	Failed    ContainerState = "failed"
)

// ContainerChange is the state a container went into at one moment.
type ContainerChange struct {
	At    time.Time
	State ContainerState
}

// ContainerSpan is one sidecar or init container of a pod over a window: each change of its state.
type ContainerSpan struct {
	Name    string
	Kind    ContainerKind
	Changes []ContainerChange
}

// RunOutcome is how a run of a Job ended.
type RunOutcome string

// A run is still running, succeeded, failed, or ended unknown: gone before it was seen to end.
const (
	RunRunning   RunOutcome = "running"
	RunSucceeded RunOutcome = "succeeded"
	RunFailed    RunOutcome = "failed"
	RunUnknown   RunOutcome = "unknown"
)

// Run is one pod a Job ran: it starts, does its work and ends, and is never ready, so it is no
// Process switching to a release.
type Run struct {
	Pod string
	Job string
	// CronJob is the CronJob that made the Job, where one did.
	CronJob string
	Start   time.Time
	// End is when it ended; nil while it runs.
	End     *time.Time
	Outcome RunOutcome
}

// CanaryWeight is the share of traffic Flagger sent a Process's canary from one moment on.
type CanaryWeight struct {
	Process string
	At      time.Time
	Weight  int
}

// Window is what a Project's pods, canaries and Jobs did over a release's window.
type Window struct {
	Pods    []PodSpan
	Weights []CanaryWeight
	Runs    []Run
}

// Series is a metric's values by the series' identifying label, and by when each was sampled.
type Series map[string]map[time.Time]float64

// Observed is what the metrics say of a Project's pods over a window, before it is laid out.
type Observed struct {
	// Readiness is each pod that serves, by pod; Weights each canary's weight, by Process.
	Readiness Series
	Weights   Series
	// Containers is each container of the pods that serve, the Process's own among them.
	Containers []ContainerSeries
	// Jobs is each pod a Job ran.
	Jobs []JobPod
}

// ContainerSample is what the metrics say of a container at one step.
type ContainerSample struct {
	Running bool
	Ready   bool
	// Ended is the reason it had terminated with, where it had: Completed is a success.
	Ended string
}

// ContainerSeries is one container of a pod over a window, sampled each step.
type ContainerSeries struct {
	Pod  string
	Name string
	// Init is an init container's. Restarts is an init container that restarts with its pod, so
	// runs beside its Process for as long as it does: a sidecar.
	Init     bool
	Restarts bool
	// Started is when it last started, where the metrics say.
	Started *time.Time
	Samples map[time.Time]ContainerSample
}

// JobPod is one pod a Job ran, sampled each step.
type JobPod struct {
	Pod     string
	Job     string
	CronJob string
	// Start and End are when the pod started and finished by its own record, where the metrics
	// say: a Job's pod is often done between two samples.
	Start  *time.Time
	End    *time.Time
	Phases map[time.Time]string
}

var (
	// podSuffix is the five characters Kubernetes ends a generated pod name with.
	podSuffix = regexp.MustCompile(`-[a-z0-9]{5}$`)
	// templateHash is a ReplicaSet's pod-template hash, which a Deployment's pod name carries.
	templateHash = regexp.MustCompile(`-[a-z0-9]*[0-9][a-z0-9]*$`)
	// ordinal is a StatefulSet pod's number.
	ordinal = regexp.MustCompile(`-[0-9]+$`)
)

// ProcessOf is the Process a pod's name belongs to: its generated suffix and template hash taken
// off, a StatefulSet's ordinal taken off, and Flagger's -primary taken off.
func ProcessOf(pod string) string {
	name := pod
	if podSuffix.MatchString(name) {
		name = podSuffix.ReplaceAllString(name, "")
		if hash := templateHash.FindString(name); len(hash) >= 9 && len(hash) <= 11 {
			name = strings.TrimSuffix(name, hash)
		}
	} else {
		name = ordinal.ReplaceAllString(name, "")
	}
	return strings.TrimSuffix(name, "-primary")
}

// WindowOf lays what was observed out over a window from start to end, sampled every step: the
// readiness and containers of each pod, the weight of each canary, and each run of a Job. A pod
// last seen more than two steps before the end was gone.
func WindowOf(o Observed, start, end time.Time, step time.Duration) Window {
	w := Window{Pods: []PodSpan{}, Weights: []CanaryWeight{}, Runs: []Run{}}
	containers := map[string][]ContainerSeries{}
	for _, c := range o.Containers {
		containers[c.Pod] = append(containers[c.Pod], c)
	}
	for pod, points := range o.Readiness {
		span := spanOf(pod, points, end, step)
		span.Containers = containersOf(span.Process, containers[pod])
		w.Pods = append(w.Pods, span)
	}
	slices.SortFunc(w.Pods, func(a, b PodSpan) int {
		if c := strings.Compare(a.Process, b.Process); c != 0 {
			return c
		}
		if c := a.Checks[0].At.Compare(b.Checks[0].At); c != 0 {
			return c
		}
		return strings.Compare(a.Pod, b.Pod)
	})
	for canary, points := range o.Weights {
		w.Weights = append(w.Weights, changesOf(canary, points)...)
	}
	slices.SortFunc(w.Weights, func(a, b CanaryWeight) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.Process, b.Process)
	})
	for _, j := range o.Jobs {
		if run, ok := runOf(j, start, end, step); ok {
			w.Runs = append(w.Runs, run)
		}
	}
	slices.SortFunc(w.Runs, func(a, b Run) int {
		if c := a.Start.Compare(b.Start); c != 0 {
			return c
		}
		return strings.Compare(a.Pod, b.Pod)
	})
	return w
}

// containersOf is each sidecar and init container of a pod of process, by name: every container
// but the one that is the Process, which is the one named for it, else the first.
func containersOf(process string, all []ContainerSeries) []ContainerSpan {
	main := ""
	for _, c := range all {
		if !c.Init && (c.Name == process || main == "" || (main != process && c.Name < main)) {
			main = c.Name
		}
	}
	out := []ContainerSpan{}
	for _, c := range all {
		if c.Init || c.Name != main {
			if span := containerOf(c); len(span.Changes) > 0 {
				out = append(out, span)
			}
		}
	}
	slices.SortFunc(out, func(a, b ContainerSpan) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// containerOf is each change of a container's state, from when it started where the samples
// never saw it run before it ended.
func containerOf(c ContainerSeries) ContainerSpan {
	span := ContainerSpan{Name: c.Name, Kind: SidecarContainer}
	if c.Init && !c.Restarts {
		span.Kind = InitContainer
	}
	times := make([]time.Time, 0, len(c.Samples))
	for at := range c.Samples {
		times = append(times, at)
	}
	slices.SortFunc(times, time.Time.Compare)
	for _, at := range times {
		state := stateOf(c.Samples[at], span.Kind)
		if n := len(span.Changes); n == 0 || span.Changes[n-1].State != state {
			span.Changes = append(span.Changes, ContainerChange{At: at, State: state})
		}
	}
	span.Changes = withStart(span.Changes, c.Started)
	return span
}

// stateOf is what one sample says a container was doing. An init container that is ready and no
// longer runs has done its work.
func stateOf(s ContainerSample, kind ContainerKind) ContainerState {
	switch {
	case s.Ended == "Completed":
		return Succeeded
	case s.Ended != "":
		return Failed
	case s.Running && s.Ready:
		return Ready
	case s.Running:
		return Running
	case s.Ready && kind == InitContainer:
		return Succeeded
	default:
		return Waiting
	}
}

// withStart puts in when a container started, where it ended before any sample saw it run: an
// init container that ran for a second between two samples still ran.
func withStart(changes []ContainerChange, started *time.Time) []ContainerChange {
	if started == nil {
		return changes
	}
	for i, c := range changes {
		switch c.State {
		case Waiting:
			continue
		case Running, Ready:
			return changes
		case Succeeded, Failed:
			if i == 0 || !started.After(changes[i-1].At) || !started.Before(c.At) {
				return changes
			}
			return slices.Insert(changes, i, ContainerChange{At: *started, State: Running})
		}
	}
	return changes
}

// runOf is a Job's pod as a run of it: from when it started to when it ended, and how. A run that
// ended before the window started is not one of the window's.
func runOf(j JobPod, start, end time.Time, step time.Duration) (Run, bool) {
	times := make([]time.Time, 0, len(j.Phases))
	for at := range j.Phases {
		times = append(times, at)
	}
	if len(times) == 0 {
		return Run{}, false
	}
	slices.SortFunc(times, time.Time.Compare)
	first, last := times[0], times[len(times)-1]
	run := Run{Pod: j.Pod, Job: j.Job, CronJob: j.CronJob, Start: first, Outcome: RunRunning}
	if j.Start != nil {
		run.Start = *j.Start
	}
	if outcome, ok := ended(j.Phases[last]); ok {
		at, known := endOf(j, times, start)
		if !known {
			return Run{}, false
		}
		run.Outcome, run.End = outcome, &at
	} else if last.Before(end.Add(-2 * step)) {
		run.Outcome, run.End = RunUnknown, &last
	}
	if run.End != nil && run.End.Before(start) {
		return Run{}, false
	}
	return run, true
}

// endOf is when a run that ended did: by its own record, else the first step it was seen ended.
// Without its record, a run already over when the window opened ended before it, so is unknown.
func endOf(j JobPod, times []time.Time, start time.Time) (time.Time, bool) {
	if j.End != nil {
		return *j.End, true
	}
	i := slices.IndexFunc(times, func(t time.Time) bool { _, done := ended(j.Phases[t]); return done })
	return times[i], i > 0 || times[i].After(start)
}

// ended is the outcome a pod's phase is the end of, where it is one.
func ended(phase string) (RunOutcome, bool) {
	switch phase {
	case "Succeeded":
		return RunSucceeded, true
	case "Failed":
		return RunFailed, true
	default:
		return "", false
	}
}

func spanOf(pod string, points map[time.Time]float64, end time.Time, step time.Duration) PodSpan {
	span := PodSpan{Pod: pod, Process: ProcessOf(pod)}
	for _, at := range sortedTimes(points) {
		ready := points[at] >= 1
		span.Checks = append(span.Checks, ReadinessCheck{At: at, Ready: ready})
		if span.Start == nil {
			span.Start = &at
		}
		if ready && span.Ready == nil {
			span.Ready = &at
		}
	}
	if n := len(span.Checks); n > 0 && span.Checks[n-1].At.Before(end.Add(-2*step)) {
		last := span.Checks[n-1].At
		span.End = &last
	}
	return span
}

// changesOf is each moment a canary's weight changed, its first value included.
func changesOf(canary string, points map[time.Time]float64) []CanaryWeight {
	var out []CanaryWeight
	previous := -1
	for _, at := range sortedTimes(points) {
		if weight := int(points[at]); weight != previous {
			out = append(out, CanaryWeight{Process: canary, At: at, Weight: weight})
			previous = weight
		}
	}
	return out
}

func sortedTimes(points map[time.Time]float64) []time.Time {
	times := make([]time.Time, 0, len(points))
	for at := range points {
		times = append(times, at)
	}
	slices.SortFunc(times, time.Time.Compare)
	return times
}
