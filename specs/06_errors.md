# TWIG - ERRORS SPEC

## Error conditions

All errors below are fatal — twig exits non-zero without invoking Terraform.

### Project layout

| Condition | Error |
|---|---|
| `<leaf-file>` does not exist or is not a `.yaml` file | fatal |
| `twig.yaml` not found walking up from `<leaf-file>` | fatal |
| Leaf file not exactly at `infra/<p>/<p>/<p>/<p>/<p>/<name>.yaml` below project root | fatal |

### twig.yaml

| Condition | Error |
|---|---|
| `modules_path` missing or empty | fatal |
| `backend` block missing or empty | fatal |
| `backend.key` set (twig derives it from the leaf path) | fatal |

### Modules

| Condition | Error |
|---|---|
| `source` module directory does not exist (local `modules_path` only; git sources are validated by Terraform at init time) | fatal |
| Module `source` does not match the `<cloud>/<major>/...` format | fatal |
| Two modules in the same leaf use the same cloud with conflicting `<major>` versions | fatal |

### Providers

| Condition | Error |
|---|---|
| `infra/<cloud>/providers.yaml` missing for a cloud used by the leaf | fatal |
| `providers.yaml` present but missing an entry for a cloud used by the leaf | fatal |

### Leaf syntax

| Condition | Error |
|---|---|
| Reserved path variable (`cloud`, `profile`, `region`, `environment`, `class`, `component`, `module`) used as a key in `vars` | fatal |
| Reserved ref namespace (`module`, `remote`, `vars`) used as a module instance key | fatal |
| Reserved ref namespace used as a `remote_state` alias | fatal |
| `remote_state` alias conflicts with a module instance key in the same leaf | fatal |
| `remote_state` leaf path does not match the path convention | fatal |
| Module `source` missing for a declared module instance | fatal |

### References

Applied against the **effective** context — leaf modules, effective merged
`remote_state:` (inherited + leaf), and effective merged `vars:` (inherited).

| Condition | Error |
|---|---|
| `${module.<key>.<field>}` where `<key>` is not a declared module instance in this leaf | fatal |
| `${remote.<alias>.<field>}` where `<alias>` is not present in the effective merged `remote_state:` for this leaf | fatal |
| `${vars.<name>}` where `<name>` is not present in the effective merged `vars:` for this leaf | fatal |

### Inherited (vars.yaml)

| Condition | Error |
|---|---|
| Unknown top-level key in a `vars.yaml` file (only `vars:`, `remote_state:`, and `module_defaults:` are accepted) | fatal |
| Reserved path variable used as a key inside a `vars.yaml` file's `vars:` block | fatal |
| Reserved path variable used as a key inside any `module_defaults.<source>` map | fatal |
| Reserved ref namespace (`module`, `remote`, `vars`) used as a `remote_state` alias in `vars.yaml` | fatal |
| Reference (`${module.x.y}`, `${remote.x.y}`, `${vars.x}`) appears inside a `vars:` value in `vars.yaml` (references are permitted only inside `module_defaults.<source>.<var>` values) | fatal |
| `remote_state` alias in `vars.yaml` maps to a leaf path that does not match the path convention (checked lazily — only when the alias is actually referenced and the data block is emitted) | fatal |
