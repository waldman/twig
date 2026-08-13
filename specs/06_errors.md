# TWIG - ERRORS SPEC

## Error conditions

| Condition | Error |
|---|---|
| `<leaf-file>` does not exist or is not a `.yaml` file | fatal |
| `twig.yaml` not found walking up from `<leaf-file>` | fatal |
| Leaf file not exactly at `infra/<p>/<p>/<p>/<p>/<p>/<name>.yaml` below project root | fatal |
| `source` module directory does not exist | fatal |
| Cross-ref `${x.y}` where `x` is neither a declared module key nor a `remote_state` alias | fatal |
| `remote_state` alias conflicts with a module instance key | fatal |
| `remote_state` leaf path does not match the path convention | fatal |
| Reserved path variable used in `vars` | fatal |
