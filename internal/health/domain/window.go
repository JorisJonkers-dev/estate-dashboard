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
}

// CanaryWeight is the share of traffic Flagger sent a Process's canary from one moment on.
type CanaryWeight struct {
	Process string
	At      time.Time
	Weight  int
}

// Window is what a Project's pods and canaries did over a release's window.
type Window struct {
	Pods    []PodSpan
	Weights []CanaryWeight
}

// Series is a metric's values by the series' identifying label, and by when each was sampled.
type Series map[string]map[time.Time]float64

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

// WindowOf lays the readiness of each pod and the weight of each canary out over a window that
// ends at end, sampled every step. A pod last seen more than two steps before the end was gone.
func WindowOf(readiness, weights Series, end time.Time, step time.Duration) Window {
	w := Window{Pods: []PodSpan{}, Weights: []CanaryWeight{}}
	for pod, points := range readiness {
		w.Pods = append(w.Pods, spanOf(pod, points, end, step))
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
	for canary, points := range weights {
		w.Weights = append(w.Weights, changesOf(canary, points)...)
	}
	slices.SortFunc(w.Weights, func(a, b CanaryWeight) int {
		if c := a.At.Compare(b.At); c != 0 {
			return c
		}
		return strings.Compare(a.Process, b.Process)
	})
	return w
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
