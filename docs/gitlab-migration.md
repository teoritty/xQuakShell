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

<!-- Superseded: the Linux runner carried no tag here, which reads as "untagged runner" and is not
     what is wanted. It now carries one AND accepts untagged jobs.

| Runner | Executor | Tags | Runs |
|---|---|---|---|
| Linux | `docker` | none | everything except the Windows jobs |
| Windows | `shell` (pwsh) | `xquakshell-windows` | `go:test:windows`, `release:windows` |
-->

Only one tag appears in `.gitlab-ci.yml`, on the two Windows jobs. Everything else is untagged, so
the Linux runner needs `run_untagged` on regardless of whether it carries a tag of its own — and it
carries one anyway, so that jobs can be pinned to it later without re-registering.

| Runner | Executor | Tags | Untagged jobs | Access level | Runs |
|---|---|---|---|---|---|
| Linux | `docker` | `xquakshell-linux` | **yes** | `ref_protected` | the other 17 jobs |
| Windows | `shell` (pwsh) | `xquakshell-windows` | no | `ref_protected` | `go:test:windows`, `release:windows` |

<!-- Superseded: pinned 18.x, and the runners in use are 19.2.2.

Install [GitLab Runner](https://docs.gitlab.com/runner/install/) 18.x — the official binary, not a
distribution package, which lags. Register each runner from **Settings - CI/CD - Runners - New
project runner**, which issues a `glrt-` authentication token.
-->

Install [GitLab Runner](https://docs.gitlab.com/runner/install/) 19.x — the official binary, not a
distribution package, which lags. Create each runner in **Settings - CI/CD - Runners - New project
runner**: that form is where tags, protected, untagged jobs and the timeout are set, and it issues
the `glrt-` authentication token that `register` then consumes. The order is not interchangeable —
see the table below the install commands.

The Windows runner uses the `shell` executor because a Wails Windows build needs the real Windows
SDK and the WebView2 fixed runtime, and containerising that buys nothing. That choice has a
consequence: a `shell` job is arbitrary code running as the runner's user on a physical machine,
with no isolation of any kind. Everything in the next section exists because of that sentence.

<!-- Superseded. Kept for the record: the Linux runner was registered not_protected, and the
     section below called the in-file workflow rule the strongest control. Both were wrong; the
     corrected version follows.

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
-->

Both runners register `ref_protected`. Why that applies to the Linux runner too, and not only to
the Windows machine, is the subject of the next section.

<!-- Superseded a second time. These commands were written for the pre-16.0 registration flow and
     are rejected outright by a modern runner with:

       FATAL: Runner configuration other than name and executor configuration is reserved
       (specifically --locked, --access-level, --run-untagged, --maximum-timeout, --paused,
       --tag-list, and --maintenance-note) and cannot be specified when registering with a runner
       authentication token.

```sh
# Linux, docker executor.
#
# --run-untagged=true is not optional: 17 of the 19 jobs in .gitlab-ci.yml carry no tag, and a
# runner that has a tag stops accepting untagged jobs unless this says otherwise. Without it this
# runner sits idle while the jobs it exists for go to gitlab.com's shared runners.
#
# --maximum-timeout covers nightly:mutation:frontend, which declares timeout: 5h.
gitlab-runner register --non-interactive \
  --url https://gitlab.com/ \
  --token "glrt-REDACTED" \
  --executor docker \
  --docker-image alpine:3.20 \
  --tag-list "xquakshell-linux" \
  --run-untagged=true \
  --locked=true \
  --access-level ref_protected \
  --maximum-timeout 21600
```

```powershell
# Windows, shell executor. --run-untagged=false keeps this machine from grabbing Linux jobs and
# failing them; the tag must match .gitlab-ci.yml byte for byte.
gitlab-runner.exe register --non-interactive --url https://gitlab.com/ --token "glrt-REDACTED" --executor shell --shell pwsh --tag-list "xquakshell-windows" --run-untagged=false --locked=true --access-level ref_protected --maximum-timeout 7200
```
-->

**With a `glrt-` authentication token, everything except the executor is configured on the GitLab
server, before the token exists.** Tags, untagged jobs, protected, locking and the job timeout are
fields in the **Settings - CI/CD - Runners - New project runner** form; passing them to `register`
is a fatal error, not a deprecation warning. Fill the form first, then register with what is left.

| Form field | Linux | Windows |
|---|---|---|
| Platform | Linux | Windows |
| Tags | `xquakshell-linux` | `xquakshell-windows` |
| Run untagged jobs | **checked** | unchecked |
| Protected | **checked** | **checked** |
| Lock to current projects | checked | checked |
| Maximum job timeout | `21600` | `7200` |

`Run untagged jobs` differs between the two and is the field most easily got wrong. 17 of the 19
jobs in `.gitlab-ci.yml` carry no tag, and a runner with a tag stops accepting untagged jobs unless
this is on — so the Linux runner would sit idle while the jobs it exists for went to gitlab.com's
shared runners. The Windows runner has it off for the mirror-image reason: it must never pick up a
Linux job and fail it.

An existing runner does not need re-creating to change any of these — **Settings - CI/CD - Runners**,
pick the runner, **Edit**.

```sh
# Linux, docker executor.
gitlab-runner register --non-interactive \
  --url https://gitlab.com/ \
  --token "glrt-REDACTED" \
  --executor docker \
  --docker-image alpine:3.20
```

```powershell
# Windows, shell executor. --config is explicit because register writes config.toml to the CURRENT
# directory, not next to the executable, and a config that lands somewhere the service does not
# read looks exactly like a registration that never happened.
.\gitlab-runner.exe register --config C:\r\config.toml --non-interactive --url https://gitlab.com/ --token "glrt-REDACTED" --executor shell --shell pwsh
```

`concurrent` has no registration flag; set it in `config.toml` afterwards — `2` on the Linux
runner, `1` on the Windows one, where parallel `wails build -clean` runs fight over `build/bin`.

The Windows runner has no image, so its toolchain comes from the machine: Go 1.26, Node 20, Git,
PowerShell 7, and `%USERPROFILE%\go\bin` on `PATH` for the `wails.exe` that `go install` puts
there. No MSVC and no Windows SDK — the module has no cgo dependencies, which is the same fact
that lets `security.yml` cross-compile `GOOS=windows` from Linux. WebView2 is not preinstalled
either; `scripts/download_webview2.ps1` fetches the fixed runtime and verifies Microsoft signed it.

## Keeping an untrusted merge request off the runner

**Start from what GitLab already does, because it is most of the answer.** Unlike GitHub Actions, a
merge request from a fork runs its pipeline **in the fork's own project, on the fork's runners**.
It never touches this project's runners, and no configuration here is what stops it. The controls
below exist for the case where that default is changed.

The two that are actually boundaries — an author of a fork can reach neither:

**1. The project setting.** **Settings - CI/CD - General pipelines**, *Run pipelines for merge
requests from forks*, stays **off**. This is the switch that would opt into running fork code here
at all.

**2. Both runners are `ref_protected`.** A runner registered with `--access-level ref_protected`
accepts jobs only from protected branches and protected tags. A merge request pipeline runs on
`refs/merge-requests/N/head`, which is never a protected ref, so such a runner refuses it — and
refuses it while selecting a runner, before a line of the submitted YAML is read.

This is why the Linux runner is protected as well and not only the Windows machine. With shared
runners disabled, the Linux runner would otherwise be the one runner in the project willing to
take an untrusted job, which is the whole exposure moved rather than closed.

That makes protected refs the real security boundary, so set them under **Settings - Repository**:

- **Protected branches**: `*`, allowed to push and merge: *Maintainers*, **Allow force push: on**.
- **Protected tags**: `v*`, allowed to create: *Maintainers*.

The wildcard is deliberate and costs a solo project nothing: every ref that exists here is one the
owner pushed, and every pipeline then reaches the local runners instead of burning shared compute
minutes. Force push stays on so an ordinary rebase does not hit the protection. The price is that
protected CI/CD variables are exposed on every branch — acceptable while the only one is
`ENABLE_WINDOWS_CI`, and the reason to narrow the pattern to `main` and `security/*` the day a real
secret is added.

A second person changes this calculation, and in the right direction: a Developer cannot push to a
protected branch, so a Developer cannot reach the runners. Only a Maintainer can.

**3. A safety net, not a boundary: the rule in `workflow:rules`.** The first rule in
[`.gitlab-ci.yml`](../.gitlab-ci.yml) compares `CI_MERGE_REQUEST_SOURCE_PROJECT_PATH` against
`CI_PROJECT_PATH` and creates no pipeline when they differ.

Do not count this as security. A merge request pipeline is built from the `.gitlab-ci.yml` of the
**source branch**, which for a fork is a file the fork's author controls and can simply delete this
rule from. What it does buy is protection against the accident: control 1 flipped by a stray click,
or a future collaborator enabling fork pipelines without understanding the consequence. That is
worth having in git, where reverting it leaves a diff — it is just not what stops an attacker.

**4. The job token is scoped.** **Settings - CI/CD - Token Access**, *Limit access to this
project*, stays **on**. `CI_JOB_TOKEN` is what `release:publish` uses to upload to the package
registry; an unscoped token is a credential every job hands to whatever it runs.

Two more worth setting while in there:

- Every CI/CD variable is created **Protected** and **Masked**.
- If external contributions are not wanted at all right now, **Settings - General - Visibility**,
  *Merge requests*, *Only project members*. Blunt, and it makes controls 1 and 3 moot rather than
  redundant.

What the end state looks like when someone does open a fork merge request: the pipeline runs in
their fork on their minutes, the result shows on the merge request, and this project's machines are
not involved. If control 1 is ever switched on by mistake, the pipeline is created, finds no runner
willing to take it, and sits in `pending` — visible and harmless.

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
