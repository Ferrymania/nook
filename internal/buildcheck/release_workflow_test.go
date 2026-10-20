package buildcheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflow struct {
	On   map[string]any         `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowJob struct {
	Uses     string            `yaml:"uses"`
	If       string            `yaml:"if"`
	Needs    any               `yaml:"needs"`
	With     map[string]string `yaml:"with"`
	Env      map[string]string `yaml:"env"`
	Services map[string]any    `yaml:"services"`
	Steps    []struct {
		Uses string            `yaml:"uses"`
		Run  string            `yaml:"run"`
		With map[string]string `yaml:"with"`
	} `yaml:"steps"`
}

func readWorkflow(t *testing.T, name string) workflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var w workflow
	if err := yaml.Unmarshal(data, &w); err != nil {
		t.Fatal(err)
	}
	return w
}

// A failed prerequisite must block publishing. Merely running CI alongside a
// publisher, or adding an always() condition, does not establish this guarantee.
func blockedBy(w workflow, jobID, gate string) bool {
	if jobID == gate {
		return true
	}
	job, ok := w.Jobs[jobID]
	if !ok || (job.If != "" && job.If != "success()") {
		return false
	}
	var needs []string
	switch value := job.Needs.(type) {
	case string:
		needs = []string{value}
	case []any:
		for _, item := range value {
			needs = append(needs, item.(string))
		}
	}
	for _, parent := range needs {
		if blockedBy(w, parent, gate) {
			return true
		}
	}
	return false
}

func TestReleaseWorkflowsRequireSuccessfulCIForTestedCommit(t *testing.T) {
	for _, tc := range []struct {
		file    string
		ref     string
		publish []string
	}{
		{"Auto-docker-publish.yml", "${{ github.sha }}", []string{"build-image", "build-frontend", "publish-create-release", "build-binaries"}},
		{"docker-publish.yml", "${{ needs.resolve-ref.outputs.sha }}", []string{"docker"}},
	} {
		t.Run(tc.file, func(t *testing.T) {
			w := readWorkflow(t, tc.file)
			verify := w.Jobs["verify"]
			if verify.Uses != "./.github/workflows/ci.yml" || verify.If != "" {
				t.Fatal("publisher must unconditionally call the complete CI workflow")
			}
			if ref := verify.With["ref"]; ref != tc.ref && !(ref == "" && tc.ref == "${{ github.sha }}") {
				t.Fatalf("CI ref=%q does not match the published commit", ref)
			}
			for _, id := range tc.publish {
				if !blockedBy(w, id, "verify") {
					t.Errorf("%s can publish after failed/skipped CI", id)
				}
				checkouts := 0
				for _, step := range w.Jobs[id].Steps {
					if strings.HasPrefix(step.Uses, "actions/checkout@") {
						checkouts++
						if step.With["ref"] != tc.ref {
							t.Errorf("%s builds a different/moving ref: %q", id, step.With["ref"])
						}
					}
				}
				if checkouts != 1 {
					t.Errorf("%s needs one checkout of the tested commit", id)
				}
			}
		})
	}
}

func TestReusableCIIncludesDatabaseTestsAndSmokeCheck(t *testing.T) {
	w := readWorkflow(t, "ci.yml")
	if _, ok := w.On["workflow_call"]; !ok {
		t.Fatal("release CI is not reusable")
	}
	if _, ok := w.On["pull_request"]; !ok {
		t.Fatal("PR checks were disabled")
	}
	backend := w.Jobs["backend"]
	if backend.Services["postgres"] == nil || backend.Env["NOOK_TEST_POSTGRES_DSN"] == "" {
		t.Fatal("release CI would skip PostgreSQL integration tests")
	}
	fullTests := false
	for _, step := range backend.Steps {
		if strings.TrimSpace(step.Run) == "go test ./..." {
			fullTests = true
		}
	}
	if !fullTests {
		t.Fatal("release CI must include the full Go test suite")
	}
	for _, id := range []string{"backend", "frontend", "smoke"} {
		if !blockedBy(w, "ci-success", id) {
			t.Errorf("CI summary does not require successful %s", id)
		}
		for _, step := range w.Jobs[id].Steps {
			if strings.HasPrefix(step.Uses, "actions/checkout@") && step.With["ref"] != "${{ inputs.ref || github.sha }}" {
				t.Errorf("%s does not check the selected immutable commit", id)
			}
		}
	}
}
