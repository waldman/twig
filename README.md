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

## Inherited variables

Place a `vars.yaml` at any directory level under `infra/` to inject variables into every module in leaves below that point:

```
infra/vars.yaml                                          ← all clouds
infra/aws/vars.yaml                                      ← all AWS leaves
infra/aws/myprofile/us-east-1/vars.yaml                  ← all leaves in this region
infra/aws/myprofile/us-east-1/production/vars.yaml       ← all production leaves
infra/aws/myprofile/us-east-1/production/services/vars.yaml  ← all service leaves
```

Example:

```yaml
# infra/aws/vars.yaml
cost_center: engineering
vpn_cidr: 10.30.0.0/16
default_tags:
  ManagedBy: twig
```

Reference inherited vars in leaf files with `${var.<name>}`:

```yaml
modules:
  sg:
    source: aws/5/security-group
    vars:
      sg_ingress_cidr: ${var.vpn_cidr}
```

A pure `${var.x}` expands to the correct HCL for its type (string, bool, number, list, map). Embedded in a string it is interpolated as its string representation.

Merge rules:
- Lower levels (closer to the leaf) override higher levels.
- Module-level `vars:` in the leaf always override inherited values.
- Reserved path variable names (`cloud`, `profile`, `region`, `environment`, `class`, `component`, `module`) are rejected at load time.
- The ref namespace names (`module`, `remote`, `var`) are reserved and cannot be used as module instance keys or remote_state aliases.
- References (`${module.x.y}`, `${remote.x.y}`, `${var.x}`) are not supported inside `vars.yaml` files.

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
| `${var.vpn_cidr}` | value from inherited `vars.yaml` |

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
