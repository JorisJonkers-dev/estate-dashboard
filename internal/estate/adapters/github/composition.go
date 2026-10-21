package github

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
)

const (
	// workflow is the estate's delivery workflow, whose runs record what they composed.
	workflow = "compose.yml"
	// artifactName is the artifact each run records itself in (estate .github/workflows/compose.yml).
	artifactName = "composition"
	// recordFile is the file in it the dashboard reads.
	recordFile = "run.json"
	// maxRecord is the most of run.json the adapter reads.
	maxRecord = 1 << 20
	// titlePrefix starts every run's title; what follows says why it ran.
	titlePrefix = "Deliver to production: "
)

type runList struct {
	Runs []struct {
		ID         int64     `json:"id"`
		Title      string    `json:"display_title"`
		Conclusion string    `json:"conclusion"`
		Started    time.Time `json:"run_started_at"`
		URL        string    `json:"html_url"`
	} `json:"workflow_runs"`
}

type artifact struct {
	ID      int64  `json:"id"`
	Name    string `json:"name"`
	Expired bool   `json:"expired"`
}

type artifactList struct {
	Artifacts []artifact `json:"artifacts"`
}

// runRecord is run.json as estate's delivery-report.sh record writes it.
type runRecord struct {
	Toolkit    *string  `json:"toolkit"`
	SnapshotAt *string  `json:"snapshotAt"`
	Lock       *string  `json:"lock"`
	Moved      []string `json:"moved"`
	Fragments  []struct {
		Project   string  `json:"project"`
		Version   *string `json:"version"`
		Outcome   string  `json:"outcome"`
		Reason    *string `json:"reason"`
		Published *string `json:"published"`
	} `json:"fragments"`
}

// errNoRecord marks a run that has nothing to read, and never will: it ran before runs recorded
// one, its artifact expired, or the artifact is not a record.
var errNoRecord = errors.New("github: the run holds no record")

// CompositionRuns implements domain.Repository: up to limit completed runs of compose.yml, newest
// first, each read from its `composition` artifact. A finished run never changes, so each is read
// once; a run with no record to read is left out.
func (e *Estate) CompositionRuns(ctx context.Context, limit int) ([]domain.CompositionRun, error) {
	var listed runList
	query := url.Values{"per_page": {strconv.Itoa(limit)}, "status": {"completed"}}
	if err := e.do(ctx, http.MethodGet, "/repos/"+e.repository+"/actions/workflows/"+workflow+"/runs", query, nil, &listed); err != nil {
		return nil, err
	}
	out := make([]domain.CompositionRun, 0, len(listed.Runs))
	for _, r := range listed.Runs {
		e.mu.Lock()
		known, seen := e.recorded[r.ID]
		e.mu.Unlock()
		if !seen {
			fresh := domain.CompositionRun{
				ID: r.ID, At: r.Started.UTC(), Trigger: strings.TrimPrefix(r.Title, titlePrefix),
				Conclusion: r.Conclusion, URL: r.URL,
			}
			err := e.readRecord(ctx, r.ID, &fresh)
			switch {
			case errors.Is(err, errNoRecord):
				known = nil
			case err != nil:
				// GitHub did not answer for this run; it is read again next time.
				continue
			default:
				known = &fresh
			}
			e.mu.Lock()
			if e.recorded == nil {
				e.recorded = map[int64]*domain.CompositionRun{}
			}
			e.recorded[r.ID] = known
			e.mu.Unlock()
		}
		if known != nil {
			out = append(out, *known)
		}
	}
	return out, nil
}

// readRecord fills run from the record its artifact holds.
func (e *Estate) readRecord(ctx context.Context, id int64, run *domain.CompositionRun) error {
	var artifacts artifactList
	query := url.Values{"name": {artifactName}}
	if err := e.do(ctx, http.MethodGet, "/repos/"+e.repository+"/actions/runs/"+strconv.FormatInt(id, 10)+"/artifacts", query, nil, &artifacts); err != nil {
		return err
	}
	i := slices.IndexFunc(artifacts.Artifacts, func(a artifact) bool { return a.Name == artifactName && !a.Expired })
	if i < 0 {
		return errNoRecord
	}
	archive, err := e.download(ctx, "/repos/"+e.repository+"/actions/artifacts/"+strconv.FormatInt(artifacts.Artifacts[i].ID, 10)+"/zip")
	if err != nil {
		return err
	}
	raw, err := unzip(archive, recordFile)
	if err != nil {
		return errNoRecord
	}
	var rec runRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return errNoRecord
	}
	run.Toolkit = deref(rec.Toolkit)
	run.SnapshotAt = parseTime(rec.SnapshotAt)
	run.Lock = deref(rec.Lock)
	run.Moved = append([]string{}, rec.Moved...)
	run.Fragments = make([]domain.CompositionFragment, 0, len(rec.Fragments))
	for _, f := range rec.Fragments {
		if !knownOutcome(f.Outcome) {
			continue
		}
		run.Fragments = append(run.Fragments, domain.CompositionFragment{
			Project: f.Project, Version: deref(f.Version), Outcome: f.Outcome, Reason: deref(f.Reason), Published: parseTime(f.Published),
		})
	}
	return nil
}

// download fetches an artifact's archive. GitHub answers with a redirect to its storage, which is
// followed without the token: the token is for GitHub's API alone.
func (e *Estate) download(ctx context.Context, path string) ([]byte, error) {
	client := *e.client
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.api.JoinPath(path).String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	_ = res.Body.Close()
	if res.StatusCode != http.StatusFound {
		return nil, fmt.Errorf("%w: GET %s answered %d", errStatus, path, res.StatusCode)
	}
	location, err := res.Location()
	if err != nil {
		return nil, err
	}
	stored, err := http.NewRequestWithContext(ctx, http.MethodGet, location.String(), nil)
	if err != nil {
		return nil, err
	}
	res, err = client.Do(stored)
	if err != nil {
		return nil, err
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: the stored artifact answered %d", errStatus, res.StatusCode)
	}
	return io.ReadAll(io.LimitReader(res.Body, maxBody))
}

// unzip returns the file named name in archive.
func unzip(archive []byte, name string) ([]byte, error) {
	r, err := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if err != nil {
		return nil, err
	}
	f, err := r.Open(name)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	return io.ReadAll(io.LimitReader(f, maxRecord))
}

// knownOutcome reports whether outcome is one the contract names.
func knownOutcome(outcome string) bool {
	switch outcome {
	case "moved", "unchanged", "isolated", "stale", "held":
		return true
	}
	return false
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func parseTime(s *string) time.Time {
	if s == nil {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339, *s)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
