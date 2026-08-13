# twig

Thin CLI that reads a named YAML file from a path-as-data directory tree, generates a single `main.tf`, and delegates to Terraform.

One thing done well: turn a declarative module list into runnable Terraform without templates, HCL authoring, or state management ceremony.

## How it works

Your infrastructure lives in a directory tree where the path encodes context:

```
infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>.yaml
```

Each `.yaml` file (a _leaf_) lists the Terraform modules to call. twig reads the path, reads the file, generates `main.tf` in a local cache directory, and runs Terraform there. Your codebase stays clean.

Seven variables are automatically derived and injected into every module call:

| Variable      | Source              | Example            |
|---------------|---------------------|--------------------|
| `cloud`       | path segment        | `aws`              |
| `profile`     | path segment        | `myprofile`        |
| `region`      | path segment        | `us-east-1`        |
| `environment` | path segment        | `production`       |
| `class`       | path segment        | `services`         |
| `component`   | filename (no `.yaml`) | `my-app`         |
| `module`      | instance key        | `app_user`         |

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/install) in `$PATH`
- Cloud credentials configured (see [Supported clouds](#supported-clouds))

## Install

Download the latest release for your platform from the [releases page](https://github.com/waldman/twig/releases), extract, and place `twig` in your `$PATH`.

## Setup

Place a `twig.yaml` at the root of your project (beside the `infra/` directory):

```yaml
modules_path: github.com/your-org/terraform-modules//modules
modules_ref:  v1.0.0

backend:
  bucket: my-terraform-state
  region: us-east-1
  dynamodb_table: my-terraform-locks  # optional
```

- `modules_path` — path to your Terraform modules. Accepts a local path (relative to `twig.yaml` or absolute) or a git URL (see below).
- `modules_ref` — git ref (tag, branch, or commit SHA) to pin when `modules_path` is a git URL. Omit to use HEAD.
- `backend` — S3 backend config; `key` is always derived from the leaf path and must not be set here.
- Override `modules_path` at runtime with `TWIG_MODULES_PATH`.

### modules_path: local vs git

**Local path** — modules live on disk alongside (or near) your project:

```yaml
modules_path: ../terraform-modules/modules
```

twig validates that each module source path exists before generating `main.tf`.

**Git URL** — modules are fetched from a remote repository by `terraform init`. twig constructs the correct `git::` source URL and passes it through; no local clone is required.

Supported formats:

| Format | Example |
|---|---|
| Bare hostname (recommended) | `github.com/org/repo//subdir` |
| Full HTTPS | `https://github.com/org/repo.git//subdir` |
| SSH | `git@github.com:org/repo.git//subdir` |
| With `git::` prefix | `git::https://github.com/org/repo.git//subdir` |

The `//` separates the repository URL from the subdirectory within the repo. Omit `//subdir` if modules live at the repo root.

Pin to a specific release with `modules_ref`:

```yaml
modules_path: github.com/your-org/terraform-modules//modules
modules_ref:  v2.1.0
```

This generates module sources like:

```hcl
source = "git::https://github.com/your-org/terraform-modules.git//modules/aws/5/vpc?ref=v2.1.0"
```

Terraform caches fetched modules in `.terraform/` inside the twig cache directory — subsequent runs do not re-download unless the ref or source changes.

## Usage

```
twig <command> <leaf-file> [-- <terraform-flags>...]
```

| Command | What it does |
|---|---|
| `twig show <leaf>` | Print the generated `main.tf` — no Terraform |
| `twig init <leaf>` | Generate + `terraform init` |
| `twig plan <leaf>` | Generate + auto-init + `terraform plan` |
| `twig apply <leaf>` | Generate + auto-init + `terraform apply` |
| `twig destroy <leaf>` | Generate + auto-init + `terraform destroy` |
| `twig output <leaf>` | Generate + auto-init + `terraform output` |
| `twig state <leaf>` | Generate + auto-init + `terraform state <subcmd>` |

Pass flags through to Terraform after `--`:

```
twig apply infra/aws/myprofile/us-east-1/production/services/my-app.yaml -- -auto-approve
twig output infra/aws/myprofile/us-east-1/production/services/my-app.yaml -- -json
twig state  infra/aws/myprofile/us-east-1/dev/ec2/web.yaml -- mv module.ec2.aws_security_group.this module.sg.aws_security_group.this
```

## Providers

twig reads `infra/<cloud>/providers.yaml` alongside your leaf tree to generate the `required_providers` block and `provider {}` blocks. The version constraint is derived from the `<major>` segment of each module's source path (`aws/5/vpc` → `~> 5.0`). The HCL provider name is derived from the last segment of the registry source URL (`hashicorp/google` → `google`).

```yaml
# infra/aws/providers.yaml
aws:
  source: hashicorp/aws
  config:
    profile: "${profile}"
    region:  "${region}"

datadog:
  source: datadog/datadog
  config:
    api_key: "my-api-key"
```

Keys match the `<cloud>` segment used in module source paths (`aws/5/vpc`, `datadog/1/monitor`). Values in `config` may reference path segments via `${profile}`, `${region}`, `${environment}`, `${cloud}`, `${class}`, or `${component}`. Multiple providers can coexist in the same file — a leaf that uses both `aws` and `datadog` modules will generate both blocks from a single `infra/aws/providers.yaml`.

twig errors if `providers.yaml` is missing for a leaf's cloud.

## Inherited variables (`vars.yaml`)

Place a `vars.yaml` at any directory level under `infra/` to share configuration with leaves below that point:

```
infra/vars.yaml                                          ← all clouds
infra/aws/vars.yaml                                      ← all AWS leaves
infra/aws/myprofile/us-east-1/vars.yaml                  ← all leaves in this region
infra/aws/myprofile/us-east-1/production/vars.yaml       ← all production leaves
infra/aws/myprofile/us-east-1/production/services/vars.yaml  ← all service leaves
```

Three top-level sections, all optional:

```yaml
vars:
  # reference-only values, resolved via ${vars.<name>} in a leaf
  vpn_cidr:          10.30.0.0/16
  internal_dns_zone: internal.example.com

remote_state:
  # alias → leaf path; merges with the leaf's own remote_state (leaf wins per alias)
  network: infra/aws/myprofile/us-east-1/base/network/main.yaml

module_defaults:
  # default vars for modules whose source matches the top-level key exactly
  aws/5/vpc:
    vpc_cidr_block:         10.0.0.0/16
    vpc_availability_zones: 2
  aws/5/ec2:
    ec2_instance_type: t3.small
    ec2_subnet_id:     ${remote.network.first_public_subnet_id}
```

### `vars:` — reference-resolvable values

Reference from a leaf with `${vars.<name>}`:

```yaml
modules:
  sg:
    source: aws/5/security-group
    vars:
      sg_ingress_cidr: ${vars.vpn_cidr}
```

A pure `${vars.x}` expands to the correct HCL for its type (string, bool, number, list, map). Embedded in a string it is interpolated as its string representation.

Values under `vars:` are **not** auto-injected into modules — they are available only through explicit references. This means a `vars.yaml` may hold values used by only some leaves without breaking others.

### `remote_state:` — inherited remote-state aliases

Aliases declared here are added to the effective remote-state map that every leaf below sees, alongside any aliases the leaf declares itself. A leaf's own `remote_state:` wins on alias-key collision.

The corresponding `data "terraform_remote_state"` block is emitted **only when referenced**. Aliases that no module var references produce no data block — declare freely without worrying about unnecessary state reads in leaves that don't need them.

### `module_defaults:` — scoped default vars per module source

The top-level key is the **full module source path** (e.g. `aws/5/vpc`), matched exactly against a leaf's `modules.<instance>.source`. Different major versions (`aws/5/vpc` vs `aws/6/vpc`) may declare different variables, so defaults do not carry across versions.

For each module instance in a leaf:
1. Twig starts with the merged `module_defaults[<mod.source>]` map from the hierarchy.
2. Overlays the leaf's own `modules.<instance>.vars` per-key. Leaf wins.
3. Emits the result as the module block's arguments.

Values in `module_defaults` may reference `${vars.<name>}`, `${remote.<alias>.<field>}`, or `${module.<instance>.<field>}` — all resolved against the consuming leaf's context.

### Merge rules

- All three sections merge per-key across the hierarchy — closer to the leaf wins.
- `module_defaults.<source>` merges per source, then per variable inside that source.
- Values are replaced wholesale — no deep merge inside map values.
- The leaf's own `modules.<instance>.vars` and `remote_state:` merge last (leaf wins).

### Restrictions

- Only `vars:`, `remote_state:`, and `module_defaults:` are accepted as top-level keys in `vars.yaml`.
- Reserved path variable names (`cloud`, `profile`, `region`, `environment`, `class`, `component`, `module`) may not be used as keys inside `vars:` or inside any `module_defaults.<source>` map.
- Reserved ref namespaces (`module`, `remote`, `vars`) may not be used as module instance keys or `remote_state` aliases.
- References (`${...}`) are permitted inside `module_defaults.<source>.<var>` values but **not** inside `vars:` values.

## Provenance

Every argument in a generated module block carries a trailing `# from: <origin>` comment showing where it came from — `path` for path variables, `leaf: modules.<instance>.vars` for leaf-declared vars, `<path>: module_defaults."<source>"` for inherited defaults. Makes debugging generated output a one-step lookup instead of an inheritance-trace.

## Leaf file format

```yaml
modules:
  <instance_key>:
    source: <cloud>/<version>/<module-name>
    vars:
      <variable>: <value>
```

References use explicit namespace prefixes:

| Form | Resolves to |
|---|---|
| `${module.ec2.vpc_id}` | `module.ec2.vpc_id` |
| `${remote.vpc.vpc_id}` | `data.terraform_remote_state.vpc.outputs.vpc_id` |
| `${vars.vpn_cidr}` | value from the merged `vars:` section in `vars.yaml` |

```yaml
modules:
  bucket:
    source: aws/5/s3-bucket
    vars:
      s3_bucket_name: my-bucket

  app_policy:
    source: aws/5/iam-policy
    vars:
      iam_policy_user_name: ${module.app_user.user_name}
      iam_policy_statements:
        - effect: Allow
          actions: [s3:GetObject, s3:PutObject]
          resources:
            - ${module.bucket.bucket_arn}
            - ${module.bucket.bucket_arn}/*
```

A pure `${ns.key.field}` reference becomes an unquoted Terraform expression. A reference embedded in a string becomes an interpolated string (`"${module.x.y}/*"`).

## Cache

Generated files live in `~/.twig-cache/<hash>/` — never in your codebase. The cache persists across runs so providers are not re-downloaded on every invocation.

## Example

See the [`examples/`](examples/) directory for a working project layout.

## License

MIT — see [LICENSE](LICENSE).
