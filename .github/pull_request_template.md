<!--
Thanks for opening a PR! Please fill in the sections below.
See CONTRIBUTING.md for the full contributor guide.
-->

## Summary

<!-- 1-3 bullets describing what changed and why. -->

-
-

## Test plan

<!-- How did you verify this change? Commands, screenshots for UI, etc. -->

- [ ] `make test` passes locally
- [ ] Manually exercised the change (describe how):

## Checklist

- [ ] Branch is topic-specific (not reused across tasks)
- [ ] One logical change per PR
- [ ] Updated docs (`README.md`, `CLAUDE.md`, `CONTRIBUTING.md`) if behavior changed
- [ ] SQLite access uses WAL + `busy_timeout=5000` if this touches a new DB open
- [ ] Operational data (metrics, costs, run history) persists to SQLite, not memory only

## Related issues

<!-- Closes #123, Refs #456 -->
