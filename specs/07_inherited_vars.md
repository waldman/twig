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

Four top-level sections are recognised. All are optional; any other
top-level key is rejected.

```yaml
vars:
  <variable_name>: <value>

remotes:
  <alias>: <path-to-leaf.yaml relative to project root>

module_defaults:
  <cloud>/<major>/<module-name>:
    <variable_name>: <value>

env_files:
  - <path>   # shell-expandable; supports ${cloud}, ${profile}, etc. and ~/
```

Merge across the hierarchy is **per-key at the leaf of each section**,
never a deep merge inside a value. The lowest level (closest to the leaf)
wins on collision — except `env_files:`, which concatenates (see below).

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

## `remotes:` — inherited remote-state aliases

Alias-to-leaf-path mapping merged into the effective remote-state map
that a leaf sees. A leaf's own `remotes:` merges last (leaf wins on
alias collision).

```yaml
# infra/aws/<profile>/<region>/base/vars.yaml
remotes:
  network: infra/aws/<profile>/<region>/base/network/main.yaml
```

A leaf below `base/` can then reference `${remotes.network.<field>}`
without declaring the alias itself. The alias namespace is shared with
the leaf's own `remotes:` block — see `specs/02_leaf_file.md` for the
leaf-level shape.

- Aliases must not be reserved ref namespaces (`modules`, `remotes`, `vars`).
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
    ec2_subnet_id:     ${remotes.network.first_public_subnet_id}
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
| `${remotes.<alias>.<field>}` | merged `remotes:` for this leaf (inherited + leaf-declared) |
| `${modules.<instance>.<field>}` | module instances declared in this leaf |

A `module_defaults` value that references `${modules.sg.security_group_id}`
requires the consuming leaf to declare an `sg` module. If it does not, the
reference fails validation at generate time for that leaf.

### Restrictions

- Reserved path variable names may not be used as keys inside any
  `module_defaults.<source>` map.
- References (`${...}`) inside `vars:` values are still forbidden —
  the reference support described above applies only inside
  `module_defaults.<source>.<var>` values.

## `env_files:` — credential file loader

A list of file paths whose `KEY=value` contents are exported into the
Terraform subprocess environment before each invocation. The primary use
case is cloud-provider credential injection for providers that have no
native credentials-file mechanism equivalent to `~/.aws/credentials`.

```yaml
# infra/azure/vars.yaml
env_files:
  - ~/.azure/profiles/${profile}
```

### Path expansion

Paths support:
- `${cloud}`, `${profile}`, `${region}`, `${environment}`, `${class}`,
  `${component}` — replaced with the leaf's path segment values.
- Leading `~/` — expanded to the user's home directory.

No other substitution is performed. `env_files` paths are not twig refs
(`${remotes.*}`, `${modules.*}`, `${vars.*}` are not resolved here).

### File format

Standard shell-sourceable `KEY=value` format:

```
# comment lines are ignored
export ARM_CLIENT_ID=abc123          # export prefix is stripped
ARM_CLIENT_SECRET="my secret"        # double-quoted values are unquoted
ARM_TENANT_ID='00000000-...'         # single-quoted values are unquoted
ARM_SUBSCRIPTION_ID=00000000-...     # bare values are taken as-is
```

Key must match `[A-Za-z_][A-Za-z0-9_]*`. A line without `=` is a hard
error. A missing file is a hard error.

### Scope

Loaded values are **environment variables only** — they are passed to the
Terraform subprocess but are not available as `${vars.*}` or any other
twig reference. They do not appear in the generated `main.tf`.

### Merge semantics

- Lists **concatenate** across inheritance levels — `env_files:` at
  `infra/azure/vars.yaml` and `env_files:` at the leaf both contribute,
  shallowest first.
- Later files in the combined list override earlier ones for duplicate keys.
- File values override the operator's process environment for the same key
  (file wins).

### CI/CD pattern

CI writes the credential file to the same path before calling twig.
The operator experience is identical locally and in CI — no cloud is a
special case, no per-environment documentation needed.

```
~/.azure/profiles/my-subscription      ← local credential file (not in repo)
~/.azure/profiles/my-subscription-prod ← prod credentials (written by CI)
```

## Merge order (all sections)

For each file in the hierarchy order — `infra/vars.yaml` → `infra/<cloud>/vars.yaml`
→ ... → `infra/<cloud>/<profile>/<region>/<env>/<class>/vars.yaml`:

- `vars:` — per-variable-name; closer wins.
- `remotes:` — per-alias; closer wins.
- `module_defaults:` — per-source, then per-variable-name; closer wins
  on the leaf-most key. Two vars.yaml files setting different variables
  for the same source contribute both.
- `env_files:` — lists concatenate; shallowest first, deepest appended
  last. Within the combined list, later files override earlier ones for
  duplicate keys.

Value replacement is always wholesale. A map value at one level is
replaced entirely by a map value at a lower level; no deep merge into
map contents.

The leaf's own `modules.<instance>.vars` and `remotes:` merge last
(after all vars.yaml files), leaf wins per-key. The leaf's own
`env_files:` is appended last to the combined list.

## Provenance

The generator tracks the origin of every value it emits (which
`vars.yaml` file supplied a `module_defaults` var, which leaf declared a
`modules.<instance>.vars` entry, whether a path variable came from the
leaf path). See `specs/04_generation.md` — every emitted argument is
annotated with a trailing `# from: <origin>` comment.
