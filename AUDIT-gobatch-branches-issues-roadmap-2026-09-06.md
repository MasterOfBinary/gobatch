# gobatch audit — branches, issues, code, roadmap
**Status: IN PROGRESS.** Sections 1-4 are final (Phase 1 discovery, complete). Section 5 (verified findings) and section 6 (roadmap) are being filled by the verification and synthesis runs; this header is removed when they land.

Audit run 2026-09-05/06 against master @ 63ef757. Repo `/Users/vaughn/dev/gobatch`, module `github.com/MasterOfBinary/gobatch`.
Method: 55 agent investigations in Phase 1 (one per open PR, one per open issue, seven thematic clusters, five review lenses over four rounds), then adversarial verification of every new finding and every proposed issue disposition. Raw results, repro tests and issue/PR dumps are preserved under `.planning/audit-2026-09-05/` (gitignored).
**This audit took no outward action.** Nothing was pushed, merged, closed, commented on or deleted. Every command below is written to be pasted by hand.

---

## 1. Where the repository stands

Master is green on every gate: `gofmt -l`, `go vet ./...`, `go test -race -count=5 ./batch/`, and `golangci-lint v2.12.2`.

The last release is **v0.5.0 (2026-02-15)**. Six commits sit on master since that tag, including the breaking `Go()` signature change (#64) and a behaviour change to `Filter` (#66). `CHANGELOG.md` `[Unreleased]` is empty.

The most user-visible consequence: **the README on master documents the unreleased two-value `Go()`, while `go get` still resolves to v0.5.0, where that quickstart does not compile.** Anyone who finds the project today gets a broken first example. Tagging fixes it; editing code does not.

## 2. Branch and PR verdicts

| PR | Branch | Verdict | One-line reason |
|---|---|---|---|
| #65 | `fix/engine-hardening` | **Rework** | The context-aware `sendErr` silently drops 80-95% of post-cancel errors even with a live consumer draining. |
| #76 | `feat/cancel-mode` | **Rework** | `CancelStop` abandons the Source (goroutine leak, dropped buffered items), six review comments unaddressed, no docs, and it has never run CI. |
| #67 | `fix/config-semantics-docs` | **Rebase, then merge** | Docs plus tests, all green; one mechanical conflict in `batch/config.go`. |
| #68 | `fix/doc-corrections` | **Small rework** | Would land a factual error into the agent-instruction file; both review comments unaddressed; one manual conflict. |

### #65 fix/engine-hardening — rework
Three of the four fixes are sound: panic recovery, bounded pre-allocation, and the `Done()` mutex. The fourth is a regression. After cancellation, with the library's own `CollectErrors` actively draining, the branch delivered 404, 101 and 295 of 2000 per-item errors across three runs. Master delivered 2000 of 2000 every time. The `doReader` early return that the change enables abandons the Source: a Source written exactly as the Source godoc example leaks its goroutine, and items already buffered in `out` are discarded.

The change trades a documented deadlock for undocumented silent loss. `CollectErrors` and `RunBatchAndWait` both promise "all errors". The test suite stays green because the one test in that area caps its error count at 50, explicitly "well under the default error buffer of 100". There is no CHANGELOG entry and no public godoc for either behaviour change. The `sendErr` commits are interleaved through the branch, so the fix belongs on the branch rather than in a cherry-pick.

### #76 feat/cancel-mode — rework
Three independent grounds. First, the project rule that a public API change updates README, CHANGELOG, the doc.go files and the examples in the same change; this branch touches three files, all under `batch/`. Second, a demonstrated defect in the new opt-in path: on the stop signal the reader abandons the Source's channels without draining them, so the godoc-example Source leaked its goroutine in 250 of 300 cancelled runs, and post-cancel Source errors arrived in only 50 of 300. `CancelStop` also drops Source output that is already buffered and ready, 0 of 100 items processed against 100 of 100 under the default. Third, all six review comments are unaddressed, two of them about tests that leak four and one goroutines per run.

The feature is wanted and `CancelDrain` is the right default, so this is rework, not rejection.

**The rebase is not optional.** `feat/cancel-mode` forked from `fix/engine-hardening` at 027b25a and carries three commits. It does not contain master, nor the four commits #65 gained in July. Once #65 squash-merges, merging this branch as-is conflicts in `batch/batch.go` and `batch/hardening_test.go`. Replaying it works cleanly and reproduces a tree that passes vet, race tests and lint:

```
# do not run here — for the maintainer, after #65 lands on master
cd /Users/vaughn/dev/gobatch-wt/cancelmode
git fetch origin
git rebase --onto origin/master 027b25a feat/cancel-mode
```

### #67 fix/config-semantics-docs — rebase, then merge
Documentation plus additive tests, no behaviour change, all gates green. One content conflict, in the `Config.Get` doc block that #80 already rewrote on master. The resolution is mechanical: master's five-line block wins verbatim, and the branch's four `ConfigValues` field-doc hunks auto-merge. Every field-doc claim was checked against `fixConfig` and `waitForItems` and holds, with one imprecision worth folding into the rebase commit.

```
# do not run here
cd /Users/vaughn/dev/gobatch-wt/config
git fetch origin && git rebase origin/master
# CONFLICT in batch/config.go while replaying d869673:
# delete the branch-side 9-line "Get returns the values for configuration..." block
# and the conflict markers; keep master's 5-line block verbatim.
```

### #68 fix/doc-corrections — small rework
The content is 90% correct and was verified against the code, including the `errors.As` claim against the real engine. But merging as-is would write a factual error into the file that steers future agent edits: `CLAUDE.md:37` and `AGENTS.md:37` place `BufferConfig` in `batch/config.go`, and it is defined in `batch/batch.go`. Both review comments are unaddressed; the only commit after the review has an empty diff. A manual `CLAUDE.md` conflict resolution is needed regardless, because master added the end-of-file newline in #80 and this branch predates it. All three fit in one commit.

Resolve that conflict by hand. Never with `--ours` or `--theirs`: either side discards the other's hunks.

## 3. CI and repository settings

Five facts, all verified with `gh`:

- The workflow triggers only on pushes and pull requests **targeting master**, so the stacked PR #76 has never run CI. Its green checks come from an earlier base.
- The master ruleset requires a pull request but lists **no required status checks**. A red PR is mergeable.
- `checkout@v4`, `setup-go@v5` and `codecov-action@v5` are Node 20-era majors, two to three majors stale. Every run carries a deprecation annotation, and **Node 20 leaves the runner on 2026-09-16**, taking the opt-out with it. That is ten days out.
- Workflow permissions default to write, there is no `permissions:` key, no actions are SHA-pinned, Dependabot security updates are off, and there is no tag protection ruleset.
- The matrix tests Go 1.25, which is end-of-life, and never the current release. `go.mod` declares `go 1.18`, a floor CI cannot verify.

## 4. Local cleanup

- **Four worktrees**, one per open PR, all clean: `gobatch-wt/{engine,cancelmode,config,docs}`. Remove each after its PR merges with `git worktree remove <path>`. A stale entry for a deleted `/private/tmp/gobatch-pr65` was pruned during this audit.
- **`origin/fix/filter-error-passthrough` is dead.** Its patch landed squashed as #66; `git cherry` confirms the change is upstream. Delete with `git push origin --delete fix/filter-error-passthrough`.
- **Five orphan prototype commits** sit in no branch and are reachable only until garbage collection: a58bbf8 and e9a7c6f (logging and statistics, issue #70), 0fd80a9 (sync API, issue #71), 91b6ab1 (Redis test, issue #72), 690dbd0 (factory constructors, issue #74). Keeping them is a maintainer decision: `git tag archive/<name> <sha>` preserves them, doing nothing lets them go.
- **Two untracked files in the repo root** of a public repository: `ULTRA-BRAINSTORM-gobatch-10-projects-2026-06-12.md` and `.ultra-brainstorm/`. Move them out of the repo or add them to `.gitignore`, which already covers `.planning/`.

## 5. Code findings

_Pending verification run._

## 6. Issue dispositions

_Pending verification run._

## 7. Roadmap

_Pending synthesis run._
