# TWIG - INHERITED VARIABLES SPEC

A `vars.yaml` file may be placed at any directory level under `infra/`
(from the `infra/` root down through each path segment above the leaf).

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

## Top-level structure

Three top-level sections are recognised. All are optional; any other
top-level key is rejected.

```yaml
vars:
  <variable_name>: <value>

remote_state:
  <alias>: <path-to-leaf.yaml relative to project root>

module_defaults:
  <cloud>/<major>/<module-name>:
    <variable_name>: <value>
```

Merge across the hierarchy is **per-key at the leaf of each section**,
never a deep merge inside a value. The lowest level (closest to the leaf)
wins on collision.

## `vars:` — reference-resolvable values

Values referenced from a leaf via `${vars.<name>}`.

```yaml
# infra/aws/vars.yaml
vars:
  vpn_cidr:         10.30.0.0/16
  internal_dns_zone: internal.example.com
```

Reference from a leaf:

```yaml
modules:
  sg:
    source: aws/5/security-group
    vars:
      sg_ingress_cidr: ${vars.vpn_cidr}
```

- A pure `${vars.x}` reference — the whole value string is exactly the
  reference token — resolves to the underlying value with its native type
  (string, bool, number, list, map).
- Embedded in a longer string, it is substituted as its string
  representation.
- Values are **not** auto-injected into module blocks. Reference-only.
- Reserved path variable names (`cloud`, `profile`, `region`,
  `environment`, `class`, `component`, `module`) may not be used as keys
  inside the `vars:` block.

## `remote_state:` — inherited remote-state aliases

Alias-to-leaf-path mapping merged into the effective remote-state map
that a leaf sees. A leaf's own `remote_state:` merges last (leaf wins on
alias collision).

```yaml
# infra/aws/<profile>/<region>/base/vars.yaml
remote_state:
  network: infra/aws/<profile>/<region>/base/network/main.yaml
```

A leaf below `base/` can then reference `${remote.network.<field>}`
without declaring the alias itself. The alias namespace is shared with
the leaf's own `remote_state:` block — see
`specs/02_leaf_file.md` for the leaf-level shape.

- Aliases must not be reserved ref namespaces (`module`, `remote`, `vars`).
- The `data "terraform_remote_state"` block for an inherited alias is
  emitted only if some resolved module var actually references it (see
  `specs/04_generation.md` — lazy emission).

## `module_defaults:` — scoped default vars per module source

Provides default `vars:` values that are injected into every module
instance in a leaf whose `source:` matches the top-level key exactly.

```yaml
# infra/aws/vars.yaml
module_defaults:
  aws/5/vpc:
    vpc_cidr_block:         10.0.0.0/16
    vpc_availability_zones: 2
  aws/5/ec2:
    ec2_instance_type: t3.small
    ec2_subnet_id:     ${remote.network.first_public_subnet_id}
```

### Matching

The key is the **full module source path** as it appears in a leaf's
`modules.<instance>.source` field. Matching is exact string equality:

- `module_defaults."aws/5/vpc"` matches any leaf module with
  `source: aws/5/vpc`.
- It does **not** match `source: aws/6/vpc` — different major versions
  may declare different variables, so defaults do not carry across.
- Two module instances in the same leaf that share the same source both
  receive the same defaults. To differentiate them, set explicit
  `vars:` per instance in the leaf.

### Injection

For each module instance in the consuming leaf:

1. Start with the merged `module_defaults[<mod.source>]` map (empty if
   no matching key exists).
2. Overlay the leaf's own `modules.<instance>.vars` per-key. Leaf wins
   on key collision.
3. The result is emitted as the module block's arguments (in addition to
   the seven path variables).

### References inside `module_defaults` values

Values may contain any of the three reference forms. They are resolved
at generate time against the consuming leaf's context:

| Reference | Resolved against |
|---|---|
| `${vars.<name>}` | merged `vars:` for this leaf |
| `${remote.<alias>.<field>}` | merged `remote_state:` for this leaf (inherited + leaf-declared) |
| `${module.<instance>.<field>}` | module instances declared in this leaf |

A `module_defaults` value that references `${module.sg.security_group_id}`
requires the consuming leaf to declare an `sg` module. If it does not, the
reference fails validation at generate time for that leaf.

### Restrictions

- Reserved path variable names may not be used as keys inside any
  `module_defaults.<source>` map.
- References (`${...}`) inside `vars:` values are still forbidden —
  the reference support described above applies only inside
  `module_defaults.<source>.<var>` values.

## Merge order (all sections)

For each file in the hierarchy order — `infra/vars.yaml` → `infra/<cloud>/vars.yaml`
→ ... → `infra/<cloud>/<profile>/<region>/<env>/<class>/vars.yaml`:

- `vars:` — per-variable-name; closer wins.
- `remote_state:` — per-alias; closer wins.
- `module_defaults:` — per-source, then per-variable-name; closer wins
  on the leaf-most key. Two vars.yaml files setting different variables
  for the same source contribute both.

Value replacement is always wholesale. A map value at one level is
replaced entirely by a map value at a lower level; no deep merge into
map contents.

The leaf's own `modules.<instance>.vars` and `remote_state:` merge last
(after all vars.yaml files), leaf wins per-key.

## Provenance

The generator tracks the origin of every value it emits (which
`vars.yaml` file supplied a `module_defaults` var, which leaf declared a
`modules.<instance>.vars` entry, whether a path variable came from the
leaf path). See `specs/04_generation.md` — every emitted argument is
annotated with a trailing `# from: <origin>` comment.
