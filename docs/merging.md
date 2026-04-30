# Merging policy

Reliability Recovery P7. PR #285 squash-merged 63 commits into a single
commit on main — reverting one specific behavior change inside became an
editing exercise on the squashed diff rather than a `git revert`. This
document defines when to squash and when to merge-commit.

## Default: squash-merge

Use `gh pr merge --squash --delete-branch` for the common case:

- Single-purpose PRs (one feature, one fix, one refactor).
- Five or fewer commits.
- Under 500 lines changed (additions + deletions).
- All commits are review-noise (typo fixes, "address review",
  "wip" branches, etc).

Squash keeps `git log main` readable.

## Threshold: prefer merge-commit

Use `gh pr merge --merge --delete-branch` when ANY of:

- More than 5 commits and each is a meaningful step.
- More than 500 lines changed.
- The PR is a multi-stage migration (PR/M2, PR/M3 patterns) where
  each commit is a deliberate boundary that must remain individually
  revertable.
- The PR introduces a new product / track that you may want to revert
  selectively in the future.

Merge-commit preserves per-commit revertability at the cost of a noisier
`git log`. Use `git log --first-parent main` to read the squashed view.

## Process

1. Inspect commits before merging:

   ```bash
   gh pr view <num> --json commits --jq '.commits | length'
   gh pr view <num> --json additions,deletions
   ```

2. If `commits > 5` OR `additions + deletions > 500`, default to merge-commit.

3. Override only with explicit reasoning in the merge comment ("squash because
   commits are review-noise only", etc).

## Repo settings

The repo must have both merge methods enabled:

```bash
gh api -X PATCH repos/k8nstantin/OpenPraxis \
  -f allow_merge_commit=true \
  -f allow_squash_merge=true \
  -f allow_rebase_merge=false
```

(The above is a one-time config — verify with `gh api repos/k8nstantin/OpenPraxis | jq '.allow_merge_commit, .allow_squash_merge'`.)
