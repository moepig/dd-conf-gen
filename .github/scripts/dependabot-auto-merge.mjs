import { execFileSync } from 'node:child_process'
import { readFileSync } from 'node:fs'
import { pathToFileURL } from 'node:url'

export const requiredWorkflows = [
  {
    "file": "test.yml",
    "jobs": [
      {
        "name": "test",
        "step": "Run tests"
      }
    ]
  }
]

function eligiblePullRequest(pr, repository, sha) {
  const updateTypes = [...(pr.body ?? '').matchAll(/^\s*update-type:\s*(\S+)\s*$/gm)].map(match => match[1])
  return pr.state === 'open' && !pr.draft && pr.user?.login === 'dependabot[bot]'
    && pr.head?.repo?.full_name === repository && pr.base?.ref === 'main'
    && pr.head?.sha === sha && updateTypes.length > 0
    && updateTypes.every(type => ['version-update:semver-minor', 'version-update:semver-patch'].includes(type))
}

/**
 * Find a current Dependabot PR whose required workflows and test steps passed.
 * @param {{api: Function, repository: string, runId: number, workflows: Array}} options GitHub API reader and required checks.
 * @returns {{number: number, sha: string} | null} The tested PR, or null when any eligibility check fails.
 */
export function findMergeCandidate({ api, repository, runId, workflows = requiredWorkflows }) {
  const prefix = `repos/${repository}`
  const trigger = api(`${prefix}/actions/runs/${runId}`)
  if (trigger.event !== 'pull_request' || trigger.status !== 'completed' || trigger.conclusion !== 'success'
    || !workflows.some(workflow => trigger.path === `.github/workflows/${workflow.file}`)) return null

  const sha = trigger.head_sha
  const pulls = api(`${prefix}/pulls?state=open&per_page=100`)
  const pr = pulls.find(pull => eligiblePullRequest(pull, repository, sha))
  if (!pr) return null

  for (const workflow of workflows) {
    const runs = api(`${prefix}/actions/workflows/${workflow.file}/runs?event=pull_request&head_sha=${sha}&per_page=100`).workflow_runs
      .filter(run => run.event === 'pull_request' && run.head_sha === sha)
      .sort((a, b) => b.id - a.id)
    const latest = runs[0]
    if (!latest || latest.status !== 'completed' || latest.conclusion !== 'success') return null
    const jobs = api(`${prefix}/actions/runs/${latest.id}/jobs?per_page=100`).jobs
    if (!workflow.jobs.every(required => jobs.some(job => job.name === required.name
      && job.status === 'completed' && job.conclusion === 'success'
      && job.steps?.some(step => step.name === required.step && step.conclusion === 'success')))) return null
  }

  const current = api(`${prefix}/pulls/${pr.number}`)
  return eligiblePullRequest(current, repository, sha) ? { number: pr.number, sha } : null
}

if (process.argv[1] && import.meta.url === pathToFileURL(process.argv[1]).href) {
  const repository = process.env.GITHUB_REPOSITORY
  const event = JSON.parse(readFileSync(process.env.GITHUB_EVENT_PATH, 'utf8'))
  const runId = event.workflow_run?.id
  if (!/^[\w.-]+\/[\w.-]+$/.test(repository ?? '') || !Number.isSafeInteger(runId)) {
    throw new Error('A repository and workflow_run ID are required')
  }
  const api = path => JSON.parse(execFileSync('gh', ['api', '--method', 'GET', path], { encoding: 'utf8' }))
  const candidate = findMergeCandidate({ api, repository, runId })
  if (candidate) {
    execFileSync('gh', ['pr', 'merge', String(candidate.number), '--repo', repository,
      '--merge', '--match-head-commit', candidate.sha], { stdio: 'inherit' })
  } else {
    console.log('No current Dependabot PR has all required tests passing; merge skipped.')
  }
}
