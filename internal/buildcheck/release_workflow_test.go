package buildcheck

import (
	"fmt"
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
	Outputs  map[string]string `yaml:"outputs"`
	Steps    []workflowStep    `yaml:"steps"`
}

type workflowStep struct {
	ID   string            `yaml:"id"`
	Uses string            `yaml:"uses"`
	Run  string            `yaml:"run"`
	With map[string]string `yaml:"with"`
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
		{"Auto-docker-publish.yml", "${{ github.sha }}", []string{"prepare-version", "build-image", "publish-create-release", "build-binaries"}},
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

func actionStep(t *testing.T, job workflowJob, action string) workflowStep {
	t.Helper()
	for _, step := range job.Steps {
		if strings.HasPrefix(step.Uses, action+"@") {
			return step
		}
	}
	t.Fatalf("missing %s step", action)
	return workflowStep{}
}

func TestTestedFrontendArtifactIsReused(t *testing.T) {
	ci := readWorkflow(t, "ci.yml")
	frontend := ci.Jobs["frontend"]
	builds := 0
	for id, job := range ci.Jobs {
		for _, step := range job.Steps {
			if strings.Contains(step.Run, "npm run build") {
				builds++
				if id != "frontend" {
					t.Errorf("%s rebuilds rather than reuses the tested SPA", id)
				}
			}
		}
	}
	if builds != 1 {
		t.Errorf("CI must build the SPA once, got %d builds", builds)
	}
	upload := actionStep(t, frontend, "actions/upload-artifact")
	if upload.ID == "" || upload.With["path"] != "web/dist" || upload.With["if-no-files-found"] != "error" {
		t.Fatal("frontend must export a nonempty SPA artifact")
	}
	if frontend.Outputs["artifact-id"] != fmt.Sprintf("${{ steps.%s.outputs.artifact-id }}", upload.ID) {
		t.Fatal("frontend must expose the immutable artifact ID from its upload step")
	}
	call := ci.On["workflow_call"].(map[string]any)
	outputs := call["outputs"].(map[string]any)
	output := outputs["frontend-artifact-id"].(map[string]any)
	if output["value"] != "${{ jobs.frontend.outputs.artifact-id }}" {
		t.Fatal("callers must receive the same tested SPA artifact ID")
	}
	backend := ci.Jobs["backend"]
	download := actionStep(t, backend, "actions/download-artifact")
	if !blockedBy(ci, "backend", "frontend") || download.With["artifact-ids"] != "${{ needs.frontend.outputs.artifact-id }}" || download.With["path"] != "web/dist" {
		t.Fatal("backend must embed the successful frontend job's artifact")
	}

	for _, tc := range []struct{ file, job string }{
		{"Auto-docker-publish.yml", "build-image"},
		{"Auto-docker-publish.yml", "build-binaries"},
		{"docker-publish.yml", "docker"},
	} {
		t.Run(tc.file+"/"+tc.job, func(t *testing.T) {
			w := readWorkflow(t, tc.file)
			job := w.Jobs[tc.job]
			download := actionStep(t, job, "actions/download-artifact")
			if download.With["artifact-ids"] != "${{ needs.verify.outputs.frontend-artifact-id }}" {
				t.Fatal("release must consume its own CI artifact by immutable ID")
			}
			for _, step := range job.Steps {
				if strings.Contains(step.Run, "npm ci") || strings.Contains(step.Run, "npm run build") || strings.HasPrefix(step.Uses, "actions/setup-node@") {
					t.Fatal("publisher must not rebuild the SPA")
				}
			}
			if tc.job == "build-binaries" {
				if download.With["path"] != "web/dist" {
					t.Fatal("binary must embed the downloaded SPA")
				}
				return
			}
			build := actionStep(t, job, "docker/build-push-action")
			root := strings.TrimSuffix(download.With["path"], "/app/web/dist")
			if root == download.With["path"] || strings.TrimSpace(build.With["build-contexts"]) != "frontend="+root {
				t.Fatal("Docker frontend override must point at the downloaded /app/web/dist tree")
			}
			if build.With["context"] != "." {
				t.Fatal("Docker must build the checked-out source rather than a moving Git context")
			}
		})
	}
}

func TestReleaseImageAndBinaryBuildsAreIndependent(t *testing.T) {
	w := readWorkflow(t, "Auto-docker-publish.yml")
	for _, id := range []string{"build-image", "build-binaries"} {
		if !blockedBy(w, id, "prepare-version") {
			t.Errorf("%s must wait for the shared version", id)
		}
	}
	if blockedBy(w, "build-image", "build-binaries") || blockedBy(w, "build-binaries", "build-image") || blockedBy(w, "build-binaries", "publish-create-release") {
		t.Fatal("image and binary compilation must be able to run concurrently")
	}
	for _, id := range []string{"build-image", "build-binaries"} {
		if !blockedBy(w, "publish-create-release", id) {
			t.Errorf("Release must wait for all %s results", id)
		}
	}
	build := w.Jobs["build-binaries"]
	actionStep(t, build, "actions/upload-artifact")
	for _, step := range build.Steps {
		if strings.Contains(step.Run, "gh release") {
			t.Fatal("binary compilation must stage packages without requiring an existing Release")
		}
	}
	actionStep(t, w.Jobs["publish-create-release"], "actions/download-artifact")
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
