# Releasing Cogent

## Prerequisites

```bash
git checkout main
git pull --ff-only
make test-go && make test-python   # must be clean
```

## One-command release

The `no-commit-to-branch` pre-commit hook blocks direct commits to `main`.
Release commits are intentional, so skip that one hook:

```bash
SKIP=no-commit-to-branch make release VERSION=0.2.0
```

This does, in order:
1. Rewrites `version = "..."` in `pyproject.toml`
2. `git add pyproject.toml` + `git commit -m "chore: release v0.2.0"`
3. `git tag v0.2.0`
4. `git push origin HEAD` (pushes the commit)
5. `git push origin v0.2.0` (pushes the tag → triggers GitHub Actions)

## Individual steps (if you need to separate commit from tag)

```bash
make version                  # print current version
make bump VERSION=0.2.0       # update pyproject.toml only (no commit, no tag)
SKIP=no-commit-to-branch git add pyproject.toml && git commit -m "chore: release v0.2.0"
make tag                      # tag the current version + push the tag
```

## Verify

```bash
git log --oneline -3          # confirm the commit
git tag | tail -5             # confirm the tag
```

GitHub Actions picks up the tag push and runs the release pipeline (build, test, publish binaries).
