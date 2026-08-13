# CLAUDE.md — twig

Working rules for AI assistants and human contributors alike.

## Spec-first (SDD)

The `specs/` directory is the source of truth for behavior. **Change the spec before you change the code.**

- Any behavioral change (new feature, changed semantics, new error condition, changed CLI, changed file format) requires a spec update in the same PR — the spec change lands first, then code and tests derive from it.
- Adding a new concern? Create a new numbered spec file (`NN_<concern>.md`). Numbers are chronological — do not renumber existing files.
- Pure refactors (no behavior change) do not require a spec update.

A `.claude/scripts/spec-check.sh` PreToolUse hook enforces this for AI sessions: editing `*.go`, `*.sh`, `*.toml`, `*.yaml`, or `*.yml` files is blocked unless a fresh `.claude/spec-ack` file names the spec that was read. The ack expires after 10 minutes — re-read the spec after that.

To ack: `printf 'spec: 02_leaf_file.md\n' > .claude/spec-ack`

## Test-first

- No new logic without tests. If you're adding a code path, add a test that exercises it.
- If you're changing a code path with existing tests, update the tests to reflect the new behavior in the same PR.
- Bug fixes: write a failing test that reproduces the bug, then fix.
- Tests live next to the code (`*_test.go`) — the Go convention.

## Docs sync

- User-visible changes (CLI, YAML schema, config file format, generated output) update `README.md` in the same PR.
- Non-user-visible changes (internals, refactors) do not.

## Working conventions

- Go module: `github.com/waldman/twig`
- One thing done well — see `specs/00_core_architecture.md` non-goals list before proposing scope expansion.
- Match the existing style in the file you're editing. Don't refactor adjacent code.
