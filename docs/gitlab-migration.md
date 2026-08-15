# GitLab migration runbook

The GitHub repository became unavailable, so the project is mirrored to
[gitlab.com/teoritty/xQuakShell](https://gitlab.com/teoritty/xQuakShell). GitHub is expected back;
this is a mirror, not a replacement, and both platforms stay wired up.

What that means in practice: `.github/workflows/**` is untouched and still authoritative for
releases, and [`.gitlab-ci.yml`](../.gitlab-ci.yml) is a port of it that runs on GitLab. When a
gate changes it changes in both. A gate that is green on one platform and absent on the other is
worse than no gate, because the green check is read as coverage that does not exist.

## Remotes

`origin` still points at GitHub. GitLab is a second remote named `gitlab`.

```sh
git remote -v
# gitlab  https://gitlab.com/teoritty/xQuakShell.git
# origin  https://github.com/teoritty/xQuakShell
```

Pushing work to both:

```sh
git push gitlab <branch>
git push origin <branch>   # once GitHub is reachable again
```

`git push --mirror` is deliberately not used. It pushes `refs/remotes/*` as well and deletes any
ref on the target that is missing locally, which turns a stale checkout into data loss on the one
copy of the repository that currently exists. `--all` plus `--tags` carries the same content
without the delete.

## Runners

Two runners, because the two halves of this build have opposite requirements.

| Runner | Executor | Tags | Runs |
|---|---|---|---|
| Linux | `docker` | none | everything except the Windows jobs |
| Windows | `shell` (pwsh) | `xquakshell-windows` | `go:test:windows`, `release:windows` |

Install [GitLab Runner](https://docs.gitlab.com/runner/install/) 18.x — the official binary, not a
distribution package, which lags. Register each runner from **Settings - CI/CD - Runners - New
project runner**, which issues a `glrt-` authentication token.

The Windows runner uses the `shell` executor because a Wails Windows build needs the real Windows
SDK and the WebView2 fixed runtime, and containerising that buys nothing. That choice has a
consequence: a `shell` job is arbitrary code running as the runner's user on a physical machine,
with no isolation of any kind. Everything in the next section exists because of that sentence.

```sh
# Linux, docker executor
gitlab-runner register --non-interactive \
  --url https://gitlab.com/ \
  --token "glrt-REDACTED" \
  --executor docker \
  --docker-image alpine:3.20 \
  --access-level not_protected
```

```powershell
# Windows, shell executor. --access-level ref_protected is the load-bearing flag.
gitlab-runner.exe register --non-interactive --url https://gitlab.com/ --token "glrt-REDACTED" --executor shell --shell pwsh --tag-list "xquakshell-windows" --access-level ref_protected
```

## Keeping an untrusted merge request off the runner

Four independent controls. Any one of them failing open must not be enough, because the failure
mode is a stranger's code executing on a personal machine with whatever that machine can reach.

**1. The pipeline is refused outright.** The first rule in `workflow:rules` in
[`.gitlab-ci.yml`](../.gitlab-ci.yml) compares `CI_MERGE_REQUEST_SOURCE_PROJECT_PATH` against
`CI_PROJECT_PATH` and creates no pipeline at all when they differ. This costs a fork contributor
nothing: the pipeline still runs in their own fork, on their own runners, and the result is still
visible on the merge request.

**2. The project setting matching it.** **Settings - CI/CD - General pipelines**, *Run pipelines
for merge requests from forks*, stays **off**. This duplicates control 1 on purpose: a setting in a
web UI can be undone by a future click with no record, and a rule in git cannot.

**3. The runner is protected.** `--access-level ref_protected` means the runner only accepts jobs
from protected branches and protected tags. A tag alone says *which* runner a job wants; protected
says *which refs may reach it*. Without this flag, any branch anyone can push — including a future
collaborator's — executes on the Windows machine.

That makes protected refs the real security boundary, so set them accordingly under **Settings -
Repository**:

- **Protected branches**: `main`, allowed to merge and push: *Maintainers*, force push off.
- **Protected tags**: `v*`, allowed to create: *Maintainers*.

While the project is a solo mirror, protecting the working branches too (`security/*`, `feat/*`,
`fix/*`) is reasonable and keeps day-to-day pipelines on the local runner instead of burning
shared compute minutes. Reconsider it the moment a second person gets push access, because
protecting a branch pattern anyone can push to hands them the runner.

**4. The job token is scoped.** **Settings - CI/CD - Token Access**, *Limit access to this
project*, stays **on**. `CI_JOB_TOKEN` is what `release:publish` uses to upload to the package
registry; an unscoped token is a credential every job hands to whatever it runs.

Two more worth setting while in there:

- Every CI/CD variable is created **Protected** and **Masked**. A protected variable is not exposed
  to a job on an unprotected ref, which is the same boundary as control 3.
- If external contributions are not wanted at all right now, **Settings - General - Visibility**,
  *Merge requests*, *Only project members*. Blunt, and it makes controls 1 and 2 moot rather than
  redundant.

## CI/CD variables

| Variable | Value | Flags | Purpose |
|---|---|---|---|
| `ENABLE_WINDOWS_CI` | `true` | protected | Turns on `go:test:windows` and `release:windows`. Leave unset until the Windows runner is registered — the jobs skip cleanly rather than hanging on a tag no runner carries. |
| `SCHEDULE` | see below | — | Set per pipeline schedule, not project-wide. Picks which scheduled job runs. |
| `ONLY` | e.g. `domain/plugin` | — | Optional, on a mutation schedule: comma-separated target labels to measure. |

## Pipeline schedules

GitLab has no `on: schedule` in the config file, so the cron lives in **Build - Pipeline
schedules**. Each schedule sets `SCHEDULE` to select exactly one job; the times match the GitHub
workflows they replace.

| Description | Cron (UTC) | Variable | Replaces |
|---|---|---|---|
| Nightly build | `0 1 * * *` | `SCHEDULE=nightly` | `nightly.yml` |
| Mutation testing | `17 3 * * *` | `SCHEDULE=mutation` | `mutation.yml` |
| External links | `0 7 * * 1` | `SCHEDULE=links` | `links.yml` |

Re-recording a mutation baseline is a fourth schedule, kept **inactive** and run by hand from the
schedule list when a baseline needs updating: `SCHEDULE=mutation-update`. It uploads
`scripts/mutate/baseline.json` as an artifact; committing it is a manual step, because a job that
pushes to its own repository is a job that can rewrite the thing it was measuring.

## What did not survive the port

Recorded here rather than discovered later.

- **CodeQL has no GitLab equivalent.** The `sast` job runs GitLab's analysers (semgrep and gosec)
  instead. That is not parity: CodeQL's dataflow queries find things semgrep's patterns do not, and
  the `GOOS=windows` cross-compile pass that `security.yml` performs — the only thing that makes
  CodeQL see the Windows-only sandbox, job-object and syscall code — has no counterpart at all.
  Windows-only Go code is unanalysed on GitLab.
- **Nightly releases are build-only.** `nightly.yml` publishes a rolling `nightly` GitHub release,
  deleting the previous one each night. GitLab releases are immutable by tag, and the
  delete-and-recreate dance costs more than a mirror is worth. `nightly:build:linux` keeps the
  archives as job artifacts instead.
- **The Linux glibc floor moved.** GitHub builds release archives on `ubuntu-22.04` (glibc 2.35);
  the GitLab job builds in `golang:1.26-bookworm` (glibc 2.36). The build image sets the floor of
  what it produces, so a GitLab-built archive will not start on a system a GitHub-built one would.
  Until the two agree, GitHub stays the source of published Linux archives.
- **Dependabot is replaced by Renovate** ([`renovate.json5`](../renovate.json5)), reproducing the
  same groups and the same Monday schedule. Both configs are live, so while GitHub is also active
  the same bump can arrive twice. That is accepted: a duplicate merge request is visible, a missing
  dependency update is not.

## When GitHub comes back

Nothing has to be undone. `.github/**` was never modified, so the workflows resume on the first
push to `origin`. The decisions worth revisiting then are which platform publishes releases, and
whether to keep running Renovate and Dependabot side by side.
