package domain

import "time"

// CompositionFragment is one fragment a composition run read, as the run recorded it.
type CompositionFragment struct {
	Project string
	// Version is empty for a Project that published no fragment.
	Version string
	// Outcome is moved, unchanged, isolated, stale or held.
	Outcome string
	Reason  string
	// Published is zero for a Project that published no fragment.
	Published time.Time
}

// CompositionRun is one run of the estate's compose.yml, with what it recorded in its
// `composition` artifact.
type CompositionRun struct {
	ID      int64
	At      time.Time
	Trigger string
	// Conclusion is GitHub's: success, failure, cancelled and so on.
	Conclusion string
	URL        string
	Toolkit    string
	SnapshotAt time.Time
	Lock       string
	Moved      []string
	Fragments  []CompositionFragment
}

// Outcome is composed, isolated or failed. A run turns red last when it set a fragment aside
// (estate .github/scripts/delivery-report.sh verdict), so a red run is isolated where it recorded
// one, and failed otherwise. A composition that refused outright recorded no fragment, so it failed.
func (r CompositionRun) Outcome() string {
	if r.Conclusion == "success" {
		return "composed"
	}
	if r.Conclusion == "failure" {
		for _, f := range r.Fragments {
			if f.Outcome == "isolated" || f.Outcome == "stale" {
				return "isolated"
			}
		}
	}
	return "failed"
}
