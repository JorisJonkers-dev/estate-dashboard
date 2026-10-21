package domain_test

import (
	"testing"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
)

func TestARunsOutcomeIsReadFromWhatItRecordedAndHowItEnded(t *testing.T) {
	isolated := []domain.CompositionFragment{{Project: "app", Outcome: "unchanged"}, {Project: "knowledge", Outcome: "isolated"}}
	stale := []domain.CompositionFragment{{Project: "grimoire", Outcome: "stale"}}
	clean := []domain.CompositionFragment{{Project: "app", Outcome: "moved"}}
	for _, c := range []struct {
		name string
		run  domain.CompositionRun
		want string
	}{
		{"a green run composed everything", domain.CompositionRun{Conclusion: "success", Fragments: clean}, "composed"},
		{"a red run that set a fragment aside isolated it", domain.CompositionRun{Conclusion: "failure", Fragments: isolated}, "isolated"},
		{"a stale fragment is set aside too", domain.CompositionRun{Conclusion: "failure", Fragments: stale}, "isolated"},
		{"a red run that set nothing aside failed", domain.CompositionRun{Conclusion: "failure", Fragments: clean}, "failed"},
		{"a composition that refused outright recorded no fragment and failed", domain.CompositionRun{Conclusion: "failure"}, "failed"},
		{"a cancelled run failed, whatever it set aside", domain.CompositionRun{Conclusion: "cancelled", Fragments: isolated}, "failed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := c.run.Outcome(); got != c.want {
				t.Fatalf("Outcome() = %q, want %q", got, c.want)
			}
		})
	}
}
