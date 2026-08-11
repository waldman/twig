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
- AWS credentials configured (profile or environment variables)

## Install

Download the latest release for your platform from the [releases page](https://github.com/waldman/twig/releases), extract, and place `twig` in your `$PATH`.

## Setup

Place a `twig.yaml` at the root of your project (beside the `infra/` directory):

```yaml
modules_path: ../terraform-modules/modules

backend:
  bucket: my-terraform-state
  region: us-east-1
  dynamodb_table: my-terraform-locks  # optional
```

- `modules_path` — path to your Terraform modules, relative to `twig.yaml`
- `backend` — S3 backend config; `key` is always derived from the leaf path and must not be set here
- Override `modules_path` at runtime with `TWIG_MODULES_PATH`

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

Pass flags through to Terraform after `--`:

```
twig apply infra/aws/myprofile/us-east-1/production/services/my-app.yaml -- -auto-approve
```

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
