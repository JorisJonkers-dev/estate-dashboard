// Package github implements the estate context's Repository port on GitHub's API, with a token
// that may read the Estate repository and nothing else: the pins under projects/, the commits
// that moved each, and the open issues.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"sigs.k8s.io/yaml"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/estate/domain"
)

const (
	// maxBody is the most of a reply the adapter reads.
	maxBody = 8 << 20
	// maxIssues is the most open issues one read returns: one page.
	maxIssues = 100
	// annotation is the prefix of what a Pause or a Rollback records on a pin.
	annotation = "estate.jorisjonkers.dev/"
)

// project is what a Project's directory may be called: what a pin file's path is built from.
var project = regexp.MustCompile(`^[a-z0-9_][a-z0-9_-]{0,62}$`)

// Estate reads one Estate repository.
type Estate struct {
	api        *url.URL
	repository string
	token      string
	client     *http.Client
}

var _ domain.Repository = (*Estate)(nil)

// New returns a reader of repository (owner/name) through the API at api.
func New(api, repository, token string, client *http.Client) (*Estate, error) {
	u, err := url.Parse(api)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("github: %q is not an http(s) URL", api)
	}
	owner, name, found := strings.Cut(repository, "/")
	if !found || owner == "" || name == "" || strings.Contains(name, "/") {
		return nil, fmt.Errorf("github: %q is not owner/name", repository)
	}
	if token == "" {
		return nil, errors.New("github: no token")
	}
	return &Estate{api: u, repository: repository, token: token, client: client}, nil
}

var errStatus = errors.New("github: unexpected status")

func (e *Estate) do(ctx context.Context, method, path string, query url.Values, body any, into any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(encoded)
	}
	u := e.api.JoinPath(path)
	u.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+e.token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := e.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return err
	}
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: %s %s answered %d", errStatus, method, path, res.StatusCode)
	}
	return json.Unmarshal(raw, into)
}

// pinsQuery reads every projects/<project>/source.yaml on the default branch in one request.
const pinsQuery = `query($owner: String!, $name: String!) {
  repository(owner: $owner, name: $name) {
    object(expression: "HEAD:projects") {
      ... on Tree { entries { name type object { ... on Tree { entries { name object { ... on Blob { text } } } } } } }
    }
  }
}`

type pinsReply struct {
	Data struct {
		Repository struct {
			Object *struct {
				Entries []struct {
					Name   string `json:"name"`
					Type   string `json:"type"`
					Object struct {
						Entries []struct {
							Name   string `json:"name"`
							Object struct {
								Text *string `json:"text"`
							} `json:"object"`
						} `json:"entries"`
					} `json:"object"`
				} `json:"entries"`
			} `json:"object"`
		} `json:"repository"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// Pins implements domain.Repository: one Pin per projects/<project>/source.yaml.
func (e *Estate) Pins(ctx context.Context) ([]domain.Pin, error) {
	owner, name, _ := strings.Cut(e.repository, "/")
	var reply pinsReply
	body := map[string]any{"query": pinsQuery, "variables": map[string]string{"owner": owner, "name": name}}
	if err := e.do(ctx, http.MethodPost, "/graphql", nil, body, &reply); err != nil {
		return nil, fmt.Errorf("github: read the pins: %w", err)
	}
	if len(reply.Errors) > 0 {
		return nil, fmt.Errorf("github: read the pins: %s", reply.Errors[0].Message)
	}
	if reply.Data.Repository.Object == nil {
		return []domain.Pin{}, nil
	}
	pins := []domain.Pin{}
	for _, dir := range reply.Data.Repository.Object.Entries {
		if dir.Type != "tree" {
			continue
		}
		for _, file := range dir.Object.Entries {
			if file.Name != "source.yaml" || file.Object.Text == nil {
				continue
			}
			pins = append(pins, pinOf(dir.Name, *file.Object.Text))
		}
	}
	return pins, nil
}

// source is as much of an OCIRepository as a pin shows.
type source struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Annotations map[string]string `json:"annotations"`
	} `json:"metadata"`
	Spec struct {
		Ref struct {
			Digest string `json:"digest"`
		} `json:"ref"`
	} `json:"spec"`
}

// pinOf reads the OCIRepository of a pin file, which holds it among the Kustomizations of its
// units. A file that does not read is a pin with no digest.
func pinOf(project, text string) domain.Pin {
	pin := domain.Pin{Project: project}
	for _, document := range strings.Split("\n"+text, "\n---") {
		var s source
		if yaml.Unmarshal([]byte(document), &s) != nil || s.Kind != "OCIRepository" {
			continue
		}
		pin.Digest = s.Spec.Ref.Digest
		a := s.Metadata.Annotations
		if by := a[annotation+"paused-by"]; by != "" {
			pin.Paused = &domain.Pause{By: by, At: a[annotation+"paused-at"], Reason: a[annotation+"paused-reason"]}
		}
		if fragment := a[annotation+"rollback-fragment"]; fragment != "" {
			pin.RolledBack = &domain.Rollback{Version: a[annotation+"rollback-version"], Fragment: fragment}
		}
		break
	}
	return pin
}

// Deploys implements domain.Repository: the commits that touched the Project's pin file.
func (e *Estate) Deploys(ctx context.Context, name string, limit int) ([]domain.Deploy, error) {
	if !project.MatchString(name) {
		return nil, fmt.Errorf("%w: %q", domain.ErrNoProject, name)
	}
	var commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message   string `json:"message"`
			Committer struct {
				Date time.Time `json:"date"`
			} `json:"committer"`
		} `json:"commit"`
	}
	query := url.Values{"path": {"projects/" + name + "/source.yaml"}, "per_page": {strconv.Itoa(limit)}}
	if err := e.do(ctx, http.MethodGet, "/repos/"+e.repository+"/commits", query, nil, &commits); err != nil {
		return nil, fmt.Errorf("github: read the deploys of %s: %w", name, err)
	}
	deploys := make([]domain.Deploy, 0, len(commits))
	for _, c := range commits {
		subject, _, _ := strings.Cut(c.Commit.Message, "\n")
		deploys = append(deploys, domain.Deploy{Commit: c.SHA, At: c.Commit.Committer.Date.UTC(), Message: subject})
	}
	return deploys, nil
}

// Issues implements domain.Repository: the open issues, pull requests left out.
func (e *Estate) Issues(ctx context.Context) ([]domain.Issue, error) {
	var issues []struct {
		Number      int       `json:"number"`
		Title       string    `json:"title"`
		HTMLURL     string    `json:"html_url"`
		UpdatedAt   time.Time `json:"updated_at"`
		PullRequest *struct{} `json:"pull_request"`
		Labels      []struct {
			Name string `json:"name"`
		} `json:"labels"`
	}
	query := url.Values{"state": {"open"}, "per_page": {strconv.Itoa(maxIssues)}}
	if err := e.do(ctx, http.MethodGet, "/repos/"+e.repository+"/issues", query, nil, &issues); err != nil {
		return nil, fmt.Errorf("github: read the issues: %w", err)
	}
	out := make([]domain.Issue, 0, len(issues))
	for _, i := range issues {
		if i.PullRequest != nil {
			continue
		}
		labels := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			labels = append(labels, l.Name)
		}
		out = append(out, domain.Issue{Number: i.Number, Title: i.Title, URL: i.HTMLURL, Labels: labels, UpdatedAt: i.UpdatedAt.UTC()})
	}
	return out, nil
}

// Absent is the repository of a dashboard that runs without one: every read is ErrNoRepository.
type Absent struct{}

var _ domain.Repository = Absent{}

// Pins implements domain.Repository.
func (Absent) Pins(context.Context) ([]domain.Pin, error) { return nil, domain.ErrNoRepository }

// Deploys implements domain.Repository.
func (Absent) Deploys(context.Context, string, int) ([]domain.Deploy, error) {
	return nil, domain.ErrNoRepository
}

// Issues implements domain.Repository.
func (Absent) Issues(context.Context) ([]domain.Issue, error) { return nil, domain.ErrNoRepository }
