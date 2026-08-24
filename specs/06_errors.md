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
| `env_files:` key present in `providers.yaml` (use `vars.yaml` at the same level) | fatal |

### Leaf syntax

| Condition | Error |
|---|---|
| Reserved path variable (`cloud`, `profile`, `region`, `environment`, `class`, `component`, `module`) used as a key in `vars` | fatal |
| Reserved ref namespace (`modules`, `remotes`, `vars`) used as a module instance key | fatal |
| Reserved ref namespace used as a `remotes` alias | fatal |
| `remotes` alias conflicts with a module instance key in the same leaf | fatal |
| `remotes` leaf path does not match the path convention | fatal |
| Module `source` missing for a declared module instance | fatal |

### References

Applied against the **effective** context — leaf modules, effective merged
`remotes:` (inherited + leaf), and effective merged `vars:` (inherited).

| Condition | Error |
|---|---|
| `${modules.<key>.<field>}` where `<key>` is not a declared module instance in this leaf | fatal |
| `${remotes.<alias>.<field>}` where `<alias>` is not present in the effective merged `remotes:` for this leaf | fatal |
| `${vars.<name>}` where `<name>` is not present in the effective merged `vars:` for this leaf | fatal |

### Inherited (vars.yaml)

| Condition | Error |
|---|---|
| Unknown top-level key in a `vars.yaml` file (only `vars:`, `remotes:`, `module_defaults:`, and `env_files:` are accepted) | fatal |
| Reserved path variable used as a key inside a `vars.yaml` file's `vars:` block | fatal |
| Reserved path variable used as a key inside any `module_defaults.<source>` map | fatal |
| Reserved ref namespace (`modules`, `remotes`, `vars`) used as a `remotes` alias in `vars.yaml` | fatal |
| Reference (`${modules.x.y}`, `${remotes.x.y}`, `${vars.x}`) appears inside a `vars:` value in `vars.yaml` (references are permitted only inside `module_defaults.<source>.<var>` values) | fatal |
| `remotes` alias in `vars.yaml` maps to a leaf path that does not match the path convention (checked lazily — only when the alias is actually referenced and the data block is emitted) | fatal |

### env_files

| Condition | Error |
|---|---|
| A path listed in `env_files:` does not exist on the filesystem | fatal |
| A line in an env file does not match `[export] KEY=value` format | fatal |
| A key in an env file does not match `[A-Za-z_][A-Za-z0-9_]*` | fatal |
