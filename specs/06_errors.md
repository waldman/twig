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
| Reserved ref namespace (`module`, `remote`, `var`) used as a module instance key | fatal |
| Reserved ref namespace used as a `remote_state` alias | fatal |
| `remote_state` alias conflicts with a module instance key in the same leaf | fatal |
| `remote_state` leaf path does not match the path convention | fatal |
| Module `source` missing for a declared module instance | fatal |

### References

| Condition | Error |
|---|---|
| `${module.<key>.<field>}` where `<key>` is not a declared module instance in this leaf | fatal |
| `${remote.<alias>.<field>}` where `<alias>` is not a declared `remote_state` alias in this leaf | fatal |
| `${var.<name>}` where `<name>` is not present in the merged inherited `vars.yaml` chain | fatal |

### Inherited vars

| Condition | Error |
|---|---|
| Reserved path variable used as a key in any `vars.yaml` | fatal |
| Reference (`${module.x.y}`, `${remote.x.y}`, `${var.x}`) appears inside a `vars.yaml` value | fatal |
