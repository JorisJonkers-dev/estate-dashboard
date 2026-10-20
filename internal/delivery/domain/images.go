package domain

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// VersionAnnotation prefixes the pod-template annotation the render writes per container, set to the
// version the images lock records for its image (deploy-kit spec/v1/10-project-intent.md#the-label-set).
const VersionAnnotation = "image.jorisjonkers.dev/"

// VersionSource is where a container's version comes from.
type VersionSource string

// The places a version comes from.
const (
	FromLock    VersionSource = "lock"
	FromTag     VersionSource = "tag"
	FromNowhere VersionSource = "unknown"
)

// Image is one image reference, split: its repository, and the tag and digest it names.
type Image struct {
	Repository string
	Tag        string
	Digest     string
}

// versionShaped is a tag that names a version: digits and dots first, any suffix after.
var versionShaped = regexp.MustCompile(`^v?\d+(\.\d+)+([._+-][0-9A-Za-z._+-]+)?$`)

// ParseImage splits ref, writing a repository Docker Hub serves the way the registry knows it.
func ParseImage(ref string) Image {
	var img Image
	if before, digest, found := strings.Cut(ref, "@"); found {
		ref, img.Digest = before, digest
	}
	if colon := strings.LastIndex(ref, ":"); colon > strings.LastIndex(ref, "/") {
		ref, img.Tag = ref[:colon], ref[colon+1:]
	}
	first, _, nested := strings.Cut(ref, "/")
	switch {
	case !nested:
		ref = "docker.io/library/" + ref
	case !strings.ContainsAny(first, ".:") && first != "localhost":
		ref = "docker.io/" + ref
	}
	img.Repository = ref
	return img
}

// Name is an image's short name: the last part of its repository.
func (i Image) Name() string {
	return i.Repository[strings.LastIndex(i.Repository, "/")+1:]
}

// VersionOf is a container's version: the annotation the render wrote, else a version-shaped tag,
// else none.
func VersionOf(annotated string, img Image) (string, VersionSource) {
	switch {
	case annotated != "":
		return annotated, FromLock
	case versionShaped.MatchString(img.Tag):
		return img.Tag, FromTag
	default:
		return "", FromNowhere
	}
}

// DigestOf is the digest an image ID names, as the kubelet reports it: what follows its `@`.
func DigestOf(imageID string) string {
	if at := strings.LastIndex(imageID, "@"); at >= 0 {
		return imageID[at+1:]
	}
	return ""
}

// Newer reports whether version a sorts after b, comparing runs of digits as numbers.
func Newer(a, b string) bool {
	return compareVersions(a, b) > 0
}

func compareVersions(a, b string) int {
	a, b = strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v")
	for a != "" && b != "" {
		da, ra := leadingDigits(a)
		db, rb := leadingDigits(b)
		if da != "" && db != "" {
			x, _ := strconv.Atoi(da)
			y, _ := strconv.Atoi(db)
			if x != y {
				return x - y
			}
			a, b = ra, rb
			continue
		}
		if a[0] != b[0] {
			return int(a[0]) - int(b[0])
		}
		a, b = a[1:], b[1:]
	}
	return len(a) - len(b)
}

func leadingDigits(s string) (string, string) {
	i := 0
	for i < len(s) && i < 9 && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i], s[i:]
}

// Instance is one container that runs an image, and where.
type Instance struct {
	Project   string
	Process   string
	Pod       string
	Namespace string
	Node      string
	Started   *time.Time
	Release   string
	Managed   bool
	Deleting  bool
	Container
}

// Drift is whether the digest a container runs differs from the one its pod spec pins.
func (i Instance) Drift() bool {
	pinned := ParseImage(i.Image).Digest
	return pinned != "" && i.RunningDigest != "" && pinned != i.RunningDigest
}

// Images is one image the estate runs, and every container that runs it.
type Images struct {
	Repository string
	Name       string
	Instances  []Instance
}

// Newest is the newest version any of its containers runs, and where that version comes from.
func (g Images) Newest() (string, VersionSource) {
	version, source := "", FromNowhere
	for _, i := range g.Instances {
		if i.Version != "" && (version == "" || Newer(i.Version, version)) {
			version, source = i.Version, i.VersionSource
		}
	}
	return version, source
}

// Platform is whether fleet-infra deploys every container of it.
func (g Images) Platform() bool {
	return !slices.ContainsFunc(g.Instances, func(i Instance) bool { return i.Managed })
}

// Drift is whether any of its containers drifts.
func (g Images) Drift() bool {
	return slices.ContainsFunc(g.Instances, Instance.Drift)
}

// ProjectOf is the Project a namespace belongs to: the namespace, less the `-system` the render adds.
func ProjectOf(namespace string) string {
	return strings.TrimSuffix(namespace, "-system")
}

// Group lays every container of pods out by image, by name and then repository, each image's
// containers by Project, Process, pod and container.
func Group(pods []Pod) []Images {
	byRepository := map[string]*Images{}
	for _, p := range pods {
		for _, c := range p.Containers {
			img := ParseImage(c.Image)
			g := byRepository[img.Repository]
			if g == nil {
				g = &Images{Repository: img.Repository, Name: img.Name()}
				byRepository[img.Repository] = g
			}
			g.Instances = append(g.Instances, Instance{
				Project: ProjectOf(p.Namespace), Process: p.Process, Pod: p.Name, Namespace: p.Namespace, Node: p.Node,
				Started: p.Started, Release: p.Version, Managed: p.Managed, Deleting: p.Deleting, Container: c,
			})
		}
	}
	out := make([]Images, 0, len(byRepository))
	for _, g := range byRepository {
		slices.SortFunc(g.Instances, func(a, b Instance) int {
			return strings.Compare(a.Project+"\x00"+a.Process+"\x00"+a.Pod+"\x00"+a.Name, b.Project+"\x00"+b.Process+"\x00"+b.Pod+"\x00"+b.Name)
		})
		out = append(out, *g)
	}
	slices.SortFunc(out, func(a, b Images) int {
		return strings.Compare(a.Name+"\x00"+a.Repository, b.Name+"\x00"+b.Repository)
	})
	return out
}

// Revision is one revision of a Deployment, as its ReplicaSet records its pod template.
type Revision struct {
	Namespace  string
	Deployment string
	// Release is the template's app.kubernetes.io/version label; empty without one.
	Release    string
	Created    time.Time
	Containers []Container
}

// VersionChange is one software version an image ran.
type VersionChange struct {
	Version string
	From    string
	Project string
	Release string
	At      time.Time
}

// maxHistory bounds a history to what one answer carries.
const maxHistory = 100

// History is every version of the image at repository the revisions record, newest first: in each
// Deployment, a revision whose version of it differs from the revision before. A revision that
// changed only the configuration around it is no change.
func History(repository string, revisions []Revision) []VersionChange {
	slices.SortFunc(revisions, func(a, b Revision) int { return a.Created.Compare(b.Created) })
	last := map[string]string{}
	var out []VersionChange
	for _, r := range revisions {
		for _, c := range r.Containers {
			if ParseImage(c.Image).Repository != repository || c.Version == "" {
				continue
			}
			key := r.Namespace + "/" + r.Deployment + "/" + c.Name
			before, seen := last[key]
			if seen && before == c.Version {
				continue
			}
			last[key] = c.Version
			out = append(out, VersionChange{Version: c.Version, From: before, Project: ProjectOf(r.Namespace), Release: r.Release, At: r.Created})
		}
	}
	slices.Reverse(out)
	return out[:min(len(out), maxHistory)]
}
