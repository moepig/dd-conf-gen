import assert from 'node:assert/strict'
import test from 'node:test'
import { findMergeCandidate } from './dependabot-auto-merge.mjs'

const repository = 'owner/repository'
const sha = '1234567890abcdef1234567890abcdef12345678'
const prefix = `repos/${repository}`
const workflows = [
  { file: 'test.yml', jobs: [{ name: 'test', step: 'Run tests' }] },
  { file: 'e2e.yml', jobs: [{ name: 'browser', step: 'Run browser tests' }] },
]

function fixture() {
  const pr = { number: 7, state: 'open', draft: false, user: { login: 'dependabot[bot]' },
    head: { sha, repo: { full_name: repository } }, base: { ref: 'main' },
    body: 'updated-dependencies:\n- dependency-name: example\n  update-type: version-update:semver-patch\n' }
  const run = { id: 10, path: '.github/workflows/test.yml', event: 'pull_request',
    head_sha: sha, status: 'completed', conclusion: 'success' }
  const responses = {
    [`${prefix}/actions/runs/10`]: run,
    [`${prefix}/pulls?state=open&per_page=100`]: [pr],
    [`${prefix}/pulls/7`]: structuredClone(pr),
    [`${prefix}/actions/workflows/test.yml/runs?event=pull_request&head_sha=${sha}&per_page=100`]: { workflow_runs: [run] },
    [`${prefix}/actions/workflows/e2e.yml/runs?event=pull_request&head_sha=${sha}&per_page=100`]: { workflow_runs: [{ ...run, id: 20 }] },
    [`${prefix}/actions/runs/10/jobs?per_page=100`]: { jobs: [{ name: 'test', status: 'completed', conclusion: 'success', steps: [{ name: 'Run tests', conclusion: 'success' }] }] },
    [`${prefix}/actions/runs/20/jobs?per_page=100`]: { jobs: [{ name: 'browser', status: 'completed', conclusion: 'success', steps: [{ name: 'Run browser tests', conclusion: 'success' }] }] },
  }
  const evaluate = () => findMergeCandidate({ repository, runId: 10, workflows, api(path) {
    assert.ok(Object.hasOwn(responses, path), `Unexpected API request: ${path}`)
    return responses[path]
  } })
  return { responses, pr, run, evaluate }
}

test('API fixtures with both test workflows passing identify the exact PR head', () => {
  assert.deepEqual(fixture().evaluate(), { number: 7, sha })
})

for (const [name, change] of [
  ['human author', f => { f.pr.user.login = 'someone' }],
  ['fork', f => { f.pr.head.repo.full_name = 'someone/repository' }],
  ['different target branch', f => { f.pr.base.ref = 'release' }],
  ['closed PR', f => { f.pr.state = 'closed' }],
  ['draft PR', f => { f.pr.draft = true }],
  ['different PR head', f => { f.pr.head.sha = 'new-head' }],
  ['missing update metadata', f => { f.pr.body = '' }],
  ['major update in a group', f => { f.pr.body += '  update-type: version-update:semver-major\n' }],
  ['push event', f => { f.run.event = 'push' }],
  ['failed triggering run', f => { f.run.conclusion = 'failure' }],
  ['unrelated workflow', f => { f.run.path = '.github/workflows/other.yml' }],
  ['missing E2E run', f => { f.responses[`${prefix}/actions/workflows/e2e.yml/runs?event=pull_request&head_sha=${sha}&per_page=100`].workflow_runs = [] }],
  ['pending E2E run', f => { f.responses[`${prefix}/actions/workflows/e2e.yml/runs?event=pull_request&head_sha=${sha}&per_page=100`].workflow_runs[0].status = 'in_progress' }],
  ['skipped test step', f => { f.responses[`${prefix}/actions/runs/10/jobs?per_page=100`].jobs[0].steps[0].conclusion = 'skipped' }],
  ['missing test job', f => { f.responses[`${prefix}/actions/runs/10/jobs?per_page=100`].jobs = [] }],
  ['failed test job', f => { f.responses[`${prefix}/actions/runs/10/jobs?per_page=100`].jobs[0].conclusion = 'failure' }],
  ['head changed after tests', f => { f.responses[`${prefix}/pulls/7`].head.sha = 'new-head' }],
]) {
  test(`API fixture with ${name} prevents merging`, () => {
    const f = fixture()
    change(f)
    assert.equal(f.evaluate(), null)
  })
}

test('a failed newer run blocks merging even when an older run passed', () => {
  const f = fixture()
  f.responses[`${prefix}/actions/workflows/e2e.yml/runs?event=pull_request&head_sha=${sha}&per_page=100`].workflow_runs
    .push({ id: 21, event: 'pull_request', head_sha: sha, status: 'completed', conclusion: 'failure' })
  assert.equal(f.evaluate(), null)
})
