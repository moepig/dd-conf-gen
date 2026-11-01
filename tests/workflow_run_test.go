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

// Checks the trusted workflow reference and executes the merge CLI with API fixtures; failed or skipped tests never merge.
func TestDependabotTestGate(t *testing.T) {
	t.Parallel()
	data, err := os.ReadFile("../.github/workflows/dependabot-auto-merge.yml")
	require.NoError(t, err)
	var workflow struct {
		On   map[string]interface{} `yaml:"on"`
		Jobs map[string]struct {
			Steps []struct {
				Name, Run, Uses string
				With            map[string]interface{}
			}
		}
	}
	require.NoError(t, yaml.Unmarshal(data, &workflow))
	require.Contains(t, workflow.On, "workflow_run")
	assert.NotContains(t, workflow.On, "pull_request_target")
	var merge string
	for _, step := range workflow.Jobs["dependabot"].Steps {
		if step.Name == "Checkout trusted workflow code" {
			assert.Equal(t, "${{ github.sha }}", step.With["ref"])
			assert.Equal(t, false, step.With["persist-credentials"])
		}
		if step.Name == "Merge a tested Dependabot PR" {
			merge = step.Run
		}
	}
	require.Equal(t, "node .github/scripts/dependabot-auto-merge.mjs", merge)
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node.js is required to execute the automation CLI fixtures")
	}
	script, err := filepath.Abs("../.github/scripts/dependabot-auto-merge.mjs")
	require.NoError(t, err)
	for _, scenario := range []string{"success", "failed", "skipped", "api-error"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			gh := `#!/bin/bash
set -euo pipefail
if [[ "$1" == "pr" ]]; then
  [[ "$*" == "pr merge 7 --repo owner/repo --merge --match-head-commit test-sha" ]]
  echo merged > "$MERGE_LOG"
  exit 0
fi
[[ "$1 $2 $3" == "api --method GET" ]]
case "$4" in
  repos/owner/repo/actions/runs/42)
    echo '{"event":"pull_request","path":".github/workflows/test.yml","head_sha":"test-sha","status":"completed","conclusion":"success"}' ;;
  repos/owner/repo/pulls\?*)
    echo '[{"number":7,"state":"open","user":{"login":"dependabot[bot]"},"head":{"sha":"test-sha","repo":{"full_name":"owner/repo"}},"base":{"ref":"main"},"body":"update-type: version-update:semver-patch"}]' ;;
  repos/owner/repo/pulls/7)
    echo '{"number":7,"state":"open","user":{"login":"dependabot[bot]"},"head":{"sha":"test-sha","repo":{"full_name":"owner/repo"}},"base":{"ref":"main"},"body":"update-type: version-update:semver-patch"}' ;;
  repos/owner/repo/actions/workflows/test.yml/runs\?*)
    if [[ "$SCENARIO" == failed ]]; then conclusion=failure; else conclusion=success; fi
    echo '{"workflow_runs":[{"id":42,"event":"pull_request","head_sha":"test-sha","status":"completed","conclusion":"'"$conclusion"'"}]}' ;;
  repos/owner/repo/actions/runs/42/jobs\?*)
    [[ "$SCENARIO" != api-error ]]
    if [[ "$SCENARIO" == skipped ]]; then conclusion=skipped; else conclusion=success; fi
    echo '{"jobs":[{"name":"test","status":"completed","conclusion":"success","steps":[{"name":"Run tests","conclusion":"'"$conclusion"'"}]}]}' ;;
  *) exit 99 ;;
esac
`
			require.NoError(t, os.WriteFile(filepath.Join(dir, "gh"), []byte(gh), 0700))
			event := filepath.Join(dir, "event.json")
			require.NoError(t, os.WriteFile(event, []byte(`{"workflow_run":{"id":42}}`), 0600))
			mergeLog := filepath.Join(dir, "merge.log")
			cmd := exec.Command(node, script)
			cmd.Env = append(os.Environ(), "PATH="+dir+":"+os.Getenv("PATH"), "GITHUB_REPOSITORY=owner/repo", "GITHUB_EVENT_PATH="+event, "SCENARIO="+scenario, "MERGE_LOG="+mergeLog)
			output, err := cmd.CombinedOutput()
			if scenario == "api-error" {
				require.Error(t, err, string(output))
			} else {
				require.NoError(t, err, string(output))
			}
			_, err = os.Stat(mergeLog)
			if scenario == "success" {
				require.NoError(t, err, string(output))
			} else {
				require.True(t, os.IsNotExist(err), string(output))
			}
		})
	}
}
