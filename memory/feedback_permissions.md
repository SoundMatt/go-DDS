---
name: feedback_permissions
description: Broad permission allowlist is configured in .claude/settings.json — do not ask Matt to approve git, gh, go, or standard Unix tool invocations.
metadata:
  type: feedback
---

`.claude/settings.json` grants blanket allow for all dev-tool Bash commands. Do not prompt for approval for any of the following:

- `git *`, `gh *`
- `go *`, `gofmt *`, `goimports *`, `golangci-lint *`
- Standard Unix: `ls`, `find`, `grep`, `rg`, `cat`, `head`, `tail`, `wc`, `sort`, `uniq`, `diff`, `awk`, `sed`, `echo`, `printf`, `jq`, `xargs`
- File ops: `mkdir`, `rm`, `cp`, `mv`, `touch`, `chmod`, `chown`, `ln`, `tar`, `zip`, `unzip`
- Network/tools: `curl`, `make`, `docker`, `docker-compose`, `brew`, `python3`
- Claude tools: `Read`, `Edit`, `Write`, `Glob`, `Grep`
- WebFetch: github.com, raw.githubusercontent.com, pkg.go.dev, sum.golang.org

**Why:** Matt explicitly requested maximum permission suppression on 2026-06-08.

**How to apply:** Never ask for confirmation before running any of these. If a new tool comes up that isn't listed, add it to `.claude/settings.json` proactively rather than prompting.
