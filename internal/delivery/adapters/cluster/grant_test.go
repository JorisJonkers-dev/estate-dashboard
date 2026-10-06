package cluster_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// The dashboard's Kubernetes API access is what its project file declares, which deploy-kit
// renders into its ClusterRole as written (deploy-kit spec/v1/16-dependencies.md). Read-only by
// design: every verb it holds reads, it holds the four kinds the adapter reads and no other, and
// a ConfigMap it may only get by name, never list or watch, since ConfigMaps hold anything.
func TestTheDeclaredGrantReadsTheFourKindsAndWritesNothing(t *testing.T) {
	raw, err := os.ReadFile("../../../../deploy/estate-dashboard.project.yml")
	if err != nil {
		t.Fatal(err)
	}
	var project struct {
		Applications []struct {
			Processes []struct {
				Name string `json:"name"`
				API  *struct {
					Rules []struct {
						Group   string   `json:"group"`
						Objects []string `json:"objects"`
						Verbs   []string `json:"verbs"`
					} `json:"rules"`
				} `json:"api"`
			} `json:"processes"`
		} `json:"applications"`
	}
	if err := yaml.Unmarshal(raw, &project); err != nil {
		t.Fatal(err)
	}
	reads := []string{"get", "list", "watch"}
	var granted []string
	for _, a := range project.Applications {
		for _, p := range a.Processes {
			if p.API == nil {
				continue
			}
			for _, rule := range p.API.Rules {
				for _, verb := range rule.Verbs {
					if !slices.Contains(reads, verb) {
						t.Errorf("%s may %s %s/%v", p.Name, verb, rule.Group, rule.Objects)
					}
				}
				for _, object := range rule.Objects {
					granted = append(granted, rule.Group+"/"+object)
					for _, verb := range rule.Verbs {
						granted = append(granted, rule.Group+"/"+object+":"+verb)
					}
				}
			}
		}
	}
	if !slices.Contains(granted, "core/configmaps:get") || slices.Contains(granted, "core/configmaps:list") || slices.Contains(granted, "core/configmaps:watch") {
		t.Errorf("ConfigMaps are got by name and never listed: %v", granted)
	}
	granted = slices.DeleteFunc(granted, func(g string) bool { return strings.Contains(g, ":") })
	slices.Sort(granted)
	want := []string{"core/configmaps", "flagger.app/canaries", "kustomize.toolkit.fluxcd.io/kustomizations", "source.toolkit.fluxcd.io/ocirepositories"}
	if !slices.Equal(granted, want) {
		t.Fatalf("granted %v, want %v", granted, want)
	}
}
