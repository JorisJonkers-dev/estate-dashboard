package app_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/app"
	"github.com/JorisJonkers-dev/estate-dashboard/internal/delivery/domain"
)

func container(name, image, version string, source domain.VersionSource) domain.Container {
	return domain.Container{Name: name, Image: image, Version: version, VersionSource: source}
}

func images(t *testing.T, pods []domain.Pod, skipped ...string) []domain.Images {
	t.Helper()
	got, err := app.New(cluster{all: pods}, skipped...).Containers(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return got.Items
}

func TestAnImageSeveralProjectsRunIsOneEntryWithEveryContainerOfIt(t *testing.T) {
	backup := "ghcr.io/jorisjonkers-dev/delivery/file-backup@sha256:1"
	got := images(t, []domain.Pod{
		{Namespace: "qbittorrent-system", Name: "q-1", Process: "qbittorrent", Managed: true, Containers: []domain.Container{container("backup", backup, "0.9.2", domain.FromLock)}},
		{Namespace: "arr-system", Name: "radarr-1", Process: "radarr", Managed: true, Containers: []domain.Container{container("backup", backup, "0.9.2", domain.FromLock)}},
	})
	if len(got) != 1 || got[0].Name != "file-backup" || got[0].Repository != "ghcr.io/jorisjonkers-dev/delivery/file-backup" {
		t.Fatalf("images = %+v", got)
	}
	var where []string
	for _, i := range got[0].Instances {
		where = append(where, i.Project+"/"+i.Process)
	}
	if !slices.Equal(where, []string{"arr/radarr", "qbittorrent/qbittorrent"}) {
		t.Fatalf("instances = %v", where)
	}
}

func TestImagesAreListedByName(t *testing.T) {
	got := images(t, []domain.Pod{
		{Namespace: "a-system", Containers: []domain.Container{container("x", "ghcr.io/o/zeta:1.0.0", "", "")}},
		{Namespace: "a-system", Containers: []domain.Container{container("y", "ghcr.io/o/alpha:1.0.0", "", "")}},
	})
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "zeta" {
		t.Fatalf("images = %+v", got)
	}
}

func TestTheClustersOwnNamespacesAreLeftOut(t *testing.T) {
	got := images(t, []domain.Pod{
		{Namespace: "kube-system", Containers: []domain.Container{container("coredns", "coredns/coredns:1.12.0", "", "")}},
		{Namespace: "observability", Containers: []domain.Container{container("gatus", "twinproduction/gatus:v5.36.0", "", "")}},
	}, "kube-system")
	if len(got) != 1 || got[0].Repository != "docker.io/twinproduction/gatus" {
		t.Fatalf("images = %+v", got)
	}
}

func TestAnImagesVersionIsTheNewestAnyOfItsContainersRuns(t *testing.T) {
	got := images(t, []domain.Pod{
		{Namespace: "data", Containers: []domain.Container{container("vault", "hashicorp/vault:1.21.2", "1.21.2", domain.FromTag)}},
		{Namespace: "data", Containers: []domain.Container{container("agent", "hashicorp/vault:2.0.4", "2.0.4", domain.FromTag)}},
		{Namespace: "data", Containers: []domain.Container{container("renew", "hashicorp/vault:1.21.10", "1.21.10", domain.FromTag)}},
	})
	if v, source := got[0].Newest(); v != "2.0.4" || source != domain.FromTag {
		t.Fatalf("newest = %s %s", v, source)
	}
}

func TestVersionsCompareTheirNumbersAsNumbers(t *testing.T) {
	for _, c := range []struct{ a, b string }{
		{"1.21.10", "1.21.9"}, {"v3.7.12", "3.7.9"}, {"6.3.0.10514-ls314", "6.3.0.10514-ls30"}, {"1.0.0-rc2", "1.0.0-rc1"}, {"1.0.1", "1.0"}, {"2.0b", "2.0a"},
	} {
		t.Run(c.a+" after "+c.b, func(t *testing.T) {
			if !domain.Newer(c.a, c.b) || domain.Newer(c.b, c.a) {
				t.Fatalf("%s does not sort after %s", c.a, c.b)
			}
		})
	}
}

func TestAnImageNoContainerNamesAVersionOfHasNone(t *testing.T) {
	got := images(t, []domain.Pod{{Namespace: "data", Containers: []domain.Container{container("postgres", "pgvector/pgvector:pg17", "", domain.FromNowhere)}}})
	if v, source := got[0].Newest(); v != "" || source != domain.FromNowhere {
		t.Fatalf("newest = %q %s", v, source)
	}
}

func TestAnImageOnlyFleetInfraRunsIsThePlatforms(t *testing.T) {
	got := images(t, []domain.Pod{
		{Namespace: "edge", Containers: []domain.Container{container("traefik", "traefik:v3.7.12", "", "")}},
		{Namespace: "n8n-system", Managed: true, Containers: []domain.Container{container("n8n", "ghcr.io/o/n8n@sha256:1", "", "")}},
	})
	if !got[1].Platform() || got[0].Platform() {
		t.Fatalf("platform: n8n %v, traefik %v", got[0].Platform(), got[1].Platform())
	}
}

func TestAContainerDriftsWhenItRunsADigestItsPodDoesNotPin(t *testing.T) {
	pinned := container("jellyseerr", "ghcr.io/o/jellyseerr@sha256:5ab0", "3.5.0", domain.FromLock)
	running := pinned
	running.RunningDigest = "sha256:0c41"
	matching := pinned
	matching.RunningDigest = "sha256:5ab0"
	unpinned := container("x", "ghcr.io/o/jellyseerr:3.5.0", "", "")
	unpinned.RunningDigest = "sha256:0c41"
	got := images(t, []domain.Pod{
		{Namespace: "a", Name: "1", Containers: []domain.Container{matching}},
		{Namespace: "a", Name: "2", Containers: []domain.Container{running}},
		{Namespace: "a", Name: "3", Containers: []domain.Container{pinned}},
		{Namespace: "a", Name: "4", Containers: []domain.Container{unpinned}},
	})
	var drifts []bool
	for _, i := range got[0].Instances {
		drifts = append(drifts, i.Drift())
	}
	if !slices.Equal(drifts, []bool{false, true, false, false}) || !got[0].Drift() {
		t.Fatalf("drifts = %v", drifts)
	}
}

func TestAnImageWhoseContainersRunWhatTheyPinDoesNotDrift(t *testing.T) {
	c := container("n8n", "ghcr.io/o/n8n@sha256:1", "", "")
	c.RunningDigest = "sha256:1"
	if got := images(t, []domain.Pod{{Containers: []domain.Container{c}}}); got[0].Drift() {
		t.Fatalf("images = %+v", got)
	}
}

func TestAPodsProjectIsItsNamespaceWithoutTheSuffixTheRenderAdds(t *testing.T) {
	got := images(t, []domain.Pod{
		{Namespace: "arr-system", Containers: []domain.Container{container("a", "a:1", "", "")}},
		{Namespace: "observability", Containers: []domain.Container{container("b", "b:1", "", "")}},
	})
	if got[0].Instances[0].Project != "arr" || got[1].Instances[0].Project != "observability" {
		t.Fatalf("images = %+v", got)
	}
}

func TestAClusterThatDoesNotAnswerHasNoImages(t *testing.T) {
	if _, err := app.New(cluster{err: domain.ErrNoCluster}).Containers(t.Context()); !errors.Is(err, domain.ErrNoCluster) {
		t.Fatalf("error = %v", err)
	}
}

func TestAnImagesHistoryIsEachVersionItsDeploymentsRanNewestFirst(t *testing.T) {
	at := func(h int) time.Time { return time.Date(2026, 10, 20, h, 0, 0, 0, time.UTC) }
	sonarr := func(version string) domain.Container {
		return container("sonarr", "ghcr.io/o/sonarr@sha256:"+version, version, domain.FromLock)
	}
	revisions := []domain.Revision{
		{Namespace: "arr-system", Deployment: "sonarr", Release: "1.0.4", Created: at(9), Containers: []domain.Container{sonarr("4.0.19"), container("backup", "ghcr.io/o/file-backup@sha256:2", "0.9.2", domain.FromLock)}},
		{Namespace: "arr-system", Deployment: "sonarr", Release: "1.0.0", Created: at(1), Containers: []domain.Container{sonarr("4.0.18")}},
		{Namespace: "arr-system", Deployment: "sonarr", Release: "1.0.3", Created: at(5), Containers: []domain.Container{sonarr("4.0.18")}},
		{Namespace: "arr-system", Deployment: "radarr", Created: at(2), Containers: []domain.Container{container("radarr", "ghcr.io/o/radarr@sha256:1", "", domain.FromNowhere)}},
	}
	got, err := app.New(cluster{revisions: revisions}).History(t.Context(), "ghcr.io/o/sonarr")
	want := []domain.VersionChange{
		{Version: "4.0.19", From: "4.0.18", Project: "arr", Release: "1.0.4", At: at(9)},
		{Version: "4.0.18", Project: "arr", Release: "1.0.0", At: at(1)},
	}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("history = %+v, %v", got, err)
	}
}

func TestAHistoryHoldsAtMostOneHundredVersions(t *testing.T) {
	var revisions []domain.Revision
	for i := range 120 {
		v := time.Duration(i).String()
		revisions = append(revisions, domain.Revision{Namespace: "a", Deployment: "d", Created: time.Unix(int64(i), 0), Containers: []domain.Container{container("c", "r:"+v, v, domain.FromTag)}})
	}
	got, err := app.New(cluster{revisions: revisions}).History(t.Context(), "docker.io/library/r")
	if err != nil || len(got) != 100 || got[0].Version != "119ns" {
		t.Fatalf("history holds %d, first %+v, %v", len(got), got[0], err)
	}
}

func TestAClusterThatDoesNotAnswerHasNoHistory(t *testing.T) {
	if _, err := app.New(cluster{err: domain.ErrNoCluster}).History(t.Context(), "r"); !errors.Is(err, domain.ErrNoCluster) {
		t.Fatalf("error = %v", err)
	}
}

func TestAnImageReferenceSplitsTheWayTheRegistryNamesIt(t *testing.T) {
	for _, c := range []struct {
		ref  string
		want domain.Image
	}{
		{"nginx:alpine", domain.Image{Repository: "docker.io/library/nginx", Tag: "alpine"}},
		{"hashicorp/vault:1.21.4", domain.Image{Repository: "docker.io/hashicorp/vault", Tag: "1.21.4"}},
		{"ghcr.io/o/n8n:v1.0.0@sha256:ab", domain.Image{Repository: "ghcr.io/o/n8n", Tag: "v1.0.0", Digest: "sha256:ab"}},
		{"localhost/x", domain.Image{Repository: "localhost/x"}},
		{"registry:5000/x/y", domain.Image{Repository: "registry:5000/x/y"}},
	} {
		t.Run(c.ref, func(t *testing.T) {
			if got := domain.ParseImage(c.ref); got != c.want {
				t.Fatalf("ParseImage(%q) = %+v", c.ref, got)
			}
		})
	}
}

func TestAVersionComesFromTheAnnotationThenAVersionTagThenNowhere(t *testing.T) {
	for _, c := range []struct {
		name, annotated, ref, version string
		source                        domain.VersionSource
	}{
		{"annotated", "2.43.8", "ghcr.io/o/n8n@sha256:1", "2.43.8", domain.FromLock},
		{"a version tag", "", "traefik:v3.7.12", "v3.7.12", domain.FromTag},
		{"a suffixed version tag", "", "linuxserver/radarr:6.3.0.10514-ls314", "6.3.0.10514-ls314", domain.FromTag},
		{"a tag that names no version", "", "pgvector/pgvector:pg17", "", domain.FromNowhere},
		{"no tag", "", "ghcr.io/o/x@sha256:1", "", domain.FromNowhere},
	} {
		t.Run(c.name, func(t *testing.T) {
			if v, source := domain.VersionOf(c.annotated, domain.ParseImage(c.ref)); v != c.version || source != c.source {
				t.Fatalf("version = %q %s", v, source)
			}
		})
	}
}

func TestADigestIsWhatAnImageIDNamesAfterItsAt(t *testing.T) {
	if domain.DigestOf("docker-pullable://ghcr.io/o/x@sha256:1") != "sha256:1" || domain.DigestOf("sha256:1") != "" {
		t.Fatal("digest misread")
	}
}
