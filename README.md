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

## Supported clouds

The `cloud` path segment determines the provider block twig generates. Modules own their own `required_providers` version constraint via `versions.tf`; twig only wires up credentials and region.

| `cloud` | Provider | Credential source |
|---|---|---|
| `aws` | `hashicorp/aws` | `profile` path segment → `~/.aws/credentials` section |
| `gcp` | `hashicorp/google` | `profile` path segment → GCP project ID; creds via `GOOGLE_CREDENTIALS` env var or ADC |
| `digitalocean` | `digitalocean/digitalocean` | `DIGITALOCEAN_TOKEN` env var |

## Leaf file format

```yaml
modules:
  <instance_key>:
    source: <cloud>/<version>/<module-name>
    vars:
      <variable>: <value>
```

Cross-module references use `${instance_key.output_name}`:

```yaml
modules:
  bucket:
    source: aws/5/s3-bucket
    vars:
      s3_bucket_name: my-bucket

  app_policy:
    source: aws/5/iam-policy
    vars:
      iam_policy_user_name: ${app_user.user_name}
      iam_policy_statements:
        - effect: Allow
          actions: [s3:GetObject, s3:PutObject]
          resources:
            - ${bucket.bucket_arn}
            - ${bucket.bucket_arn}/*
```

A pure `${x.y}` reference becomes an unquoted Terraform expression (`module.x.y`). A reference embedded in a string becomes an interpolated string (`"${module.x.y}/*"`).

## Cache

Generated files live in `~/.twig-cache/<hash>/` — never in your codebase. The cache persists across runs so providers are not re-downloaded on every invocation.

## Example

See the [`examples/`](examples/) directory for a working project layout.

## License

MIT — see [LICENSE](LICENSE).
