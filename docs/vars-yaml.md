# vars.yaml

Files that share configuration across leaves. Optional at every level of the `infra/` tree.

## Where they live

```
infra/vars.yaml                                              # all leaves
infra/<cloud>/vars.yaml                                      # all leaves in cloud
infra/<cloud>/<profile>/vars.yaml                            # all leaves in profile
infra/<cloud>/<profile>/<region>/vars.yaml                   # all leaves in region
infra/<cloud>/<profile>/<region>/<env>/vars.yaml             # all leaves in env
infra/<cloud>/<profile>/<region>/<env>/<class>/vars.yaml     # all leaves in class
```

Any leaf inherits every `vars.yaml` above it. Missing files are silently skipped.

## Three top-level sections

All optional; any other top-level key is a fatal error.

```yaml
vars:
  <name>: <value>

remote_state:
  <alias>: <path-to-leaf.yaml relative to project root>

module_defaults:
  <cloud>/<major>/<module-name>:
    <var>: <value>
```

## `vars:` — reference-resolvable values

Values referenced by leaves via `${vars.<name>}`.

```yaml
# infra/aws/vars.yaml
vars:
  vpn_cidr:          10.30.0.0/16
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

Semantics:

- A **pure** `${vars.x}` reference (the whole value is exactly the reference token) resolves to the underlying value with its native type — string, bool, number, list, or map.
- **Embedded** in a longer string, `${vars.x}` is substituted as its string representation.
- Values under `vars:` are **not** auto-injected into modules. Reference-only. A `vars.yaml` may hold values used by only some leaves without breaking others.
- Reserved path variable names (`cloud`, `profile`, `region`, `environment`, `class`, `component`, `module`) may not be used as keys inside `vars:`.
- References (`${...}`) are **not** allowed inside `vars:` values. Values must be literal.

## `remote_state:` — inherited alias-to-leaf-path

Aliases declared here extend the effective remote-state map that every leaf below sees, on top of whatever the leaf declares itself.

```yaml
# infra/aws/waldman/us-east-1/base/vars.yaml
remote_state:
  network: infra/aws/waldman/us-east-1/base/network/main.yaml
```

A leaf anywhere below `infra/aws/waldman/us-east-1/base/` can then reference `${remote.network.<field>}` without declaring the alias itself.

Semantics:

- Merges with the leaf's own `remote_state:` — leaf wins per alias on collision.
- Aliases must not be reserved ref namespaces (`module`, `remote`, `vars`).
- Emission is lazy (see [Lazy emission](#lazy-emission) below).

## `module_defaults:` — defaults scoped to a module source

Default vars injected into every module instance in a leaf whose `source:` matches the top-level key exactly.

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

For each module instance in a leaf, twig:

1. Starts with `module_defaults[<mod.source>]` from the merged hierarchy (empty if no matching key).
2. Overlays the leaf's own `modules.<instance>.vars` per key. Leaf wins.
3. Emits the result as the module block's arguments.

### Full source path — no cross-version pollution

The key is the **full source path** (`aws/5/vpc`), not just the module name (`vpc`). Different major versions may declare different variables — sharing defaults across `aws/5/vpc` and `aws/6/vpc` would silently break the older one. Full-path matching prevents that.

Two module instances in the same leaf that share the same source both receive the same defaults. To differentiate them, set explicit `vars:` per instance in the leaf.

### References inside `module_defaults` values

Values may contain any of the three reference forms — resolved at generate time against the consuming leaf's context:

| Reference | Resolved against |
|---|---|
| `${vars.<name>}` | merged inherited `vars:` for this leaf |
| `${remote.<alias>.<field>}` | merged effective `remote_state:` (inherited + leaf) |
| `${module.<instance>.<field>}` | modules declared in this leaf |

A `module_defaults` value that references `${module.sg.security_group_id}` requires the consuming leaf to declare an `sg` module. If it does not, validation fails at generate time — for that leaf only.

Restrictions:

- Reserved path variable names may not be used as keys inside any `module_defaults.<source>` map.
- Unlike `vars:` values, references **are** permitted inside `module_defaults.<source>.<var>` values.

## Merge order

For each file walked in hierarchy order (root of `infra/` → deepest ancestor of the leaf):

- **`vars:`** — per key, closer wins.
- **`remote_state:`** — per alias, closer wins.
- **`module_defaults:`** — per source key, then per variable inside. Two `vars.yaml` files setting different variables for the same source contribute both. Same-key collisions: closer wins.

All merges are shallow — a map value at one level is replaced entirely by a map value at a lower level. No deep merge into map contents. If you want to compose two maps, do it explicitly in one place.

The leaf's own `modules.<instance>.vars` and `remote_state:` merge last (after every `vars.yaml`), leaf wins per key or per alias.

## Lazy emission

A `data "terraform_remote_state" "<alias>"` block appears in the generated `main.tf` **only when the alias is actually referenced** by some resolved module var in that leaf.

Twig walks every module's effective vars (module_defaults + leaf overlay), collects every `${remote.<alias>.<field>}` token, and emits data blocks only for aliases in that set. Emit order is alphabetical.

Consequence: a shared `vars.yaml` may declare many `remote_state:` aliases without imposing unnecessary state reads on leaves that don't consume them.

## Provenance

Every argument in a generated module block carries a trailing `# from: <origin>` comment identifying where the key was declared. Three forms:

| Origin | Meaning |
|---|---|
| `path` | one of the seven path variables |
| `leaf: modules.<instance>.vars` | declared directly in the leaf |
| `<path/to/vars.yaml>: module_defaults."<source>"` | inherited from a `module_defaults` entry in the named file |

Origin tracks where the **key** was declared, not what the value resolves to. A leaf var `ec2_subnet_id: ${remote.network.first_public_subnet_id}` still shows leaf origin — the ref target is orthogonal.

For multi-line values (lists, maps), the provenance comment appears at the end of the closing bracket line:

```hcl
  ec2_security_group_ids = [
    module.sg.security_group_id,
  ]  # from: leaf: modules.ec2.vars
```

## Full example

```yaml
# infra/aws/vars.yaml

vars:
  vpn_cidr:          10.30.0.0/16
  internal_dns_zone: internal.example.com

remote_state:
  network: infra/aws/waldman/us-east-1/base/network/main.yaml

module_defaults:
  aws/5/ec2:
    ec2_instance_type: t3.small
    ec2_subnet_id:     ${remote.network.first_public_subnet_id}
```

```yaml
# infra/aws/waldman/us-east-1/dev/ec2/web.yaml

modules:
  web:
    source: aws/5/ec2
    vars:
      ec2_ami: ami-abc123
```

Generated (simplified, ellided):

```hcl
data "terraform_remote_state" "network" {
  backend = "s3"
  config = {
    bucket = "waldman-terraform-state"
    region = "us-east-1"
    key    = "infra/aws/waldman/us-east-1/base/network/main/terraform.tfstate"
  }
}

module "web" {
  # ... path vars with # from: path ...
  ec2_ami           = "ami-abc123"                                                     # from: leaf: modules.web.vars
  ec2_instance_type = "t3.small"                                                       # from: /.../infra/aws/vars.yaml: module_defaults."aws/5/ec2"
  ec2_subnet_id     = data.terraform_remote_state.network.outputs.first_public_subnet_id  # from: /.../infra/aws/vars.yaml: module_defaults."aws/5/ec2"
}
```

## See also

- [leaves.md](leaves.md) — how leaves reference these values
- [`specs/07_inherited_vars.md`](../specs/07_inherited_vars.md) — formal reference
- [`specs/04_generation.md`](../specs/04_generation.md) — generation details (lazy emission, provenance)
