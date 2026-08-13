# TWIG - INHERITED VARIABLES SPEC

## vars.yaml

A `vars.yaml` file may be placed at any directory level under `infra/`
(from the `infra/` root down through each path segment above the leaf).
Values declared under the `vars:` key are merged into a single map and made
available to the leaf via `${vars.<name>}` references.

```
infra/vars.yaml                                              # all leaves
infra/aws/vars.yaml                                          # all AWS leaves
infra/aws/<profile>/vars.yaml                                # all leaves in profile
infra/aws/<profile>/<region>/vars.yaml                       # all leaves in region
infra/aws/<profile>/<region>/<env>/vars.yaml                 # all leaves in env
infra/aws/<profile>/<region>/<env>/<class>/vars.yaml         # all leaves in class
```

`vars.yaml` files are optional at every level. Missing files are silently
skipped.

### Format

```yaml
vars:
  <variable_name>: <value>
```

- The top-level `vars:` key holds all inherited variables.
- Values are strings, numbers, booleans, lists, or maps.
- Any other top-level key is rejected. This reserves the top-level
  namespace for future sections; PR 2 will add `module_defaults:` and
  `remote_state:`.

### Merge order

vars.yaml files are merged from the most general level (`infra/vars.yaml`)
down to the most specific level closest to the leaf. Within each file the
`vars:` map is merged into the running result; on key collisions, **closer
to the leaf wins**. Values are replaced wholesale — no deep merge into
map-typed values.

### Referencing inherited vars

Leaves reference inherited vars with `${vars.<name>}`:

```yaml
modules:
  sg:
    source: aws/5/security-group
    vars:
      sg_ingress_cidr: ${vars.vpn_cidr}
```

A pure `${vars.x}` reference — the whole value string is exactly the
reference token — resolves to the underlying value with its native type
(string, bool, number, list, map). Embedded in a longer string, it is
substituted as its string representation.

### No auto-injection

Inherited variables are **not** automatically injected as arguments into
module blocks. They are available only through explicit `${vars.<name>}`
references in a leaf's module `vars:` section.

This is the important difference from the pre-PR-1 behavior: previously
every key in `vars.yaml` was emitted as an argument on every module block
in the generated `main.tf`, which restricted `vars.yaml` to variables
declared by every module (e.g. `default_tags`). Reference-only resolution
lifts that restriction — a `vars.yaml` may hold values used by only some
leaves without breaking others.

### Restrictions

- The seven reserved path variable names (`cloud`, `profile`, `region`,
  `environment`, `class`, `component`, `module`) may not be used as keys
  inside the `vars:` block.
- References (`${module.x.y}`, `${remote.x.y}`, `${vars.x}`) may not appear
  inside `vars.yaml` values. Values must be literal.
