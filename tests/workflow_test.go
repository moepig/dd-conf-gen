package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// Executes the CI test gate with a mocked GitHub CLI, rejecting failed or skipped tests and API errors.
func TestDependabotTestGate(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../.github/workflows/dependabot-auto-merge.yml")
	require.NoError(t, err)
	var workflow struct {
		On   map[string]interface{} `yaml:"on"`
		Jobs map[string]struct {
			Steps []struct{ Name, Run, Uses string }
		}
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	require.Contains(t, workflow.On, "pull_request_target")
	var gate, merge string
	for _, step := range workflow.Jobs["dependabot"].Steps {
		assert.NotContains(t, step.Uses, "actions/checkout")
		switch step.Name {
		case "Wait for PR tests":
			gate = step.Run
		case "Merge Dependabot PR":
			merge = step.Run
		}
	}
	require.NotEmpty(t, gate)
	assert.Contains(t, merge, `--auto --merge --match-head-commit "$PR_SHA"`)
	for _, scenario := range []string{"success", "failed", "skipped", "api-error"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			gh := `#!/bin/bash
set -euo pipefail
if [[ "$1" == "run" ]]; then
  [[ "$*" == "run watch 42 --exit-status --interval 10" ]]
  [[ "$SCENARIO" != "failed" ]]
elif [[ "$*" == *"/jobs"* ]]; then
  if [[ "$SCENARIO" == "skipped" ]]; then echo 0; else echo 1; fi
else
  [[ "$*" == *"event=pull_request"* && "$*" == *"head_sha=test-sha"* ]]
  [[ "$SCENARIO" != "api-error" ]]
  echo 42
fi
`
			require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(gh), 0700))
			cmd := exec.Command("bash", "-c", gate)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "GH_REPO=owner/repo", "PR_SHA=test-sha", "SCENARIO="+scenario)
			output, err := cmd.CombinedOutput()
			if scenario == "success" {
				require.NoError(t, err, string(output))
			} else {
				require.Error(t, err, string(output))
			}
		})
	}
}
