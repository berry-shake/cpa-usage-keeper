package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type releaseWorkflow struct {
	Jobs map[string]struct {
		Steps []struct {
			Run string `yaml:"run"`
		} `yaml:"steps"`
	} `yaml:"jobs"`
}

func readReleaseWorkflow(t *testing.T, name string) releaseWorkflow {
	t.Helper()
	path := filepath.Join("..", "..", "..", ".github", "workflows", name)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var workflow releaseWorkflow
	if err := yaml.Unmarshal(content, &workflow); err != nil {
		t.Fatalf("decode %s: %v", name, err)
	}
	return workflow
}

func TestCIGateRejectsIncompleteReleaseChecks(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required to exercise the GitHub Actions gate")
	}
	workflow := readReleaseWorkflow(t, "ci.yml")
	gate := workflow.Jobs["ci-gate"]
	if len(gate.Steps) != 1 || gate.Steps[0].Run == "" {
		t.Fatal("missing CI gate script")
	}
	runGate := func(t *testing.T, full bool, results [4]string, wantPass bool) {
		t.Helper()
		command := exec.Command(bash, "-e", "-c", strings.ReplaceAll(gate.Steps[0].Run, "\r\n", "\n"))
		fullValue := "false"
		if full {
			fullValue = "true"
		}
		command.Env = append(os.Environ(),
			"FULL_CHECK="+fullValue,
			"CHANGES_RESULT="+results[0],
			"BACKEND_RESULT="+results[1],
			"FRONTEND_RESULT="+results[2],
			"CONTAINER_RESULT="+results[3],
		)
		output, err := command.CombinedOutput()
		if (err == nil) != wantPass {
			t.Fatalf("full=%t results=%v: want pass=%t, err=%v\n%s", full, results, wantPass, err, output)
		}
	}
	for _, full := range []bool{false, true} {
		runGate(t, full, [4]string{"success", "success", "success", "success"}, true)
		for i, job := range []string{"changes", "backend", "frontend", "container"} {
			for _, result := range []string{"failure", "cancelled", "skipped"} {
				t.Run(job+"/"+result+"/full="+map[bool]string{false: "false", true: "true"}[full], func(t *testing.T) {
					results := [4]string{"success", "success", "success", "success"}
					results[i] = result
					runGate(t, full, results, !full && i > 0 && result == "skipped")
				})
			}
		}
	}
}
