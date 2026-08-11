# twig

Thin CLI that reads a named YAML file from a path-as-data directory tree,
generates a single `main.tf`, and delegates to Terraform.

One thing done well: turn a declarative module list into runnable Terraform
without templates, HCL authoring, or state management ceremony.

---

## Non-goals

- No templating (Jinja2, HCL templates, string interpolation beyond cross-refs)
- No module registry or version enforcement across modules in a leaf
- No plan file management (`-out` / saved plans)
- No state management beyond configuring the S3 backend in the generated file
- No wrapping of `terraform state`, `terraform import`, or other subcommands

---

## Path convention

```
infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>.yaml
```

twig accepts the leaf file as a positional argument. It walks up from that
file to locate the project root (identified by a `twig.yaml` config file).
Path variables are parsed from the relative path between the config file and
the leaf file.

| Source | Variable | Example |
|---|---|---|
| path segment 1 | `cloud` | `aws` |
| path segment 2 | `profile` | `waldman` |
| path segment 3 | `region` | `us-east-1` |
| path segment 4 | `environment` | `production` |
| path segment 5 | `class` | `services` |
| filename (no `.yaml`) | `component` | `ansible-anchor` |
| module instance key | `module` | `iam_cicd` |

All seven variables are injected into every module call. All seven are
protected — none may appear in `vars`.

---

## twig.yaml (project root config)

Placed at the directory containing `infra/`. Marks the project root and
configures the modules path and Terraform backend.

```yaml
modules_path: ../terraform-modules/modules

backend:
  bucket:         waldman-terraform-state
  region:         us-east-1
  dynamodb_table: waldman-terraform-locks  # optional
```

`modules_path` is resolved relative to the `twig.yaml` file.

Override with env var: `TWIG_MODULES_PATH` (takes precedence over `twig.yaml`).

The `backend` block is emitted verbatim into the generated
`terraform { backend "s3" {} }` block. `key` is derived from the leaf path
and must not appear in `twig.yaml`.

---

## Leaf file

One `.yaml` file per deployable component:

```
infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml
```

The filename (without `.yaml`) becomes the `component` variable. The file
declares which modules to call and the variable values to pass.

### Format

```yaml
modules:
  <instance_key>:
    source: <cloud>/<version>/<module-name>
    vars:
      <variable_name>: <value>
```

### Rules

- `instance_key` must be unique within the file. It becomes both the
  Terraform module label (`module "<instance_key>"`) and the `module`
  variable injected into that module call.
- `source` is resolved against `modules_path` to an absolute directory.
- All seven path variables are injected automatically and cannot appear
  in `vars`.
- `vars` values are strings, numbers, booleans, lists, or maps.

### Remote state references

An optional `remote_state:` block declares outputs consumed from other leaves.
The values are read from their S3 state files at plan/apply time — no data
copying, no hardcoding.

```yaml
remote_state:
  <alias>: <path-to-leaf.yaml relative to project root>
```

Aliases must not conflict with module instance keys. The same backend config
(bucket, region, profile) is used; only the `key` changes to point at the
target leaf's state file.

### Cross-module references

A value containing `${alias.output_name}` is a cross-ref. The alias can be
either a module instance key (intra-leaf) or a `remote_state` alias
(cross-leaf). The syntax is identical; twig resolves them differently:

| YAML value | Context | Generated HCL |
|---|---|---|
| `${x.bucket_arn}` | `x` is a module key | `module.x.bucket_arn` |
| `${vpc.vpc_id}` | `vpc` is a remote state alias | `data.terraform_remote_state.vpc.outputs.vpc_id` |
| `${x.bucket_arn}/*` | either | `"${module.x.bucket_arn}/*"` (interpolated string) |
| `arn:aws:s3:::my-bucket` | — | `"arn:aws:s3:::my-bucket"` (string literal) |

### Example

```
infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml
```

```yaml
modules:
  s3_bucket:
    source: aws/5/s3-bucket
    vars:
      s3_bucket_name: anchor-automation

  dynamodb:
    source: aws/5/dynamodb
    vars:
      dynamodb_table_name: ansible-anchor
      dynamodb_hash_key: node
      dynamodb_ttl_enabled: true
      dynamodb_ttl_attribute: ttl

  iam_cicd:
    source: aws/5/iam-user

  iam_cicd_policy:
    source: aws/5/iam-policy
    vars:
      iam_policy_user_name: ${iam_cicd.user_name}
      iam_policy_statements:
        - effect: Allow
          actions: [s3:GetObject, s3:PutObject, s3:DeleteObject, s3:ListBucket]
          resources:
            - ${s3_bucket.bucket_arn}
            - ${s3_bucket.bucket_arn}/*

  iam_infra:
    source: aws/5/iam-user

  iam_infra_policy:
    source: aws/5/iam-policy
    vars:
      iam_policy_user_name: ${iam_infra.user_name}
      iam_policy_statements:
        - effect: Allow
          actions: [s3:GetObject, s3:ListBucket]
          resources:
            - ${s3_bucket.bucket_arn}
            - ${s3_bucket.bucket_arn}/*
        - effect: Allow
          actions: [dynamodb:PutItem, dynamodb:GetItem, dynamodb:DescribeTable]
          resources:
            - ${dynamodb.table_arn}
```

Note: `iam_cicd` and `iam_infra` require no `vars` — the `module` variable
(`iam_cicd` / `iam_infra`) already distinguishes them within the component.

### Cross-leaf example

```
infra/aws/waldman/us-east-1/dev/ec2/app.yaml
```

```yaml
remote_state:
  vpc: infra/aws/waldman/us-east-1/base/vpc/main.yaml

modules:
  app:
    source: aws/5/ec2
    vars:
      ec2_vpc_id:    ${vpc.vpc_id}
      ec2_subnet_id: ${vpc.first_public_subnet_id}
```

twig generates a `data "terraform_remote_state" "vpc"` block that reads
`vpc_id` and `first_public_subnet_id` from the vpc leaf's S3 state file.
No coordination required between the two leaf operators beyond agreeing on
output names.

---

## Cache directory

Generated files and Terraform working state live in:

```
~/.twig-cache/<sha256-of-absolute-leaf-file-path>/
```

| Path | Description |
|---|---|
| `main.tf` | Generated on every twig invocation |
| `.terraform/` | Provider cache, populated by `terraform init` |
| `.terraform.lock.hcl` | Provider lock file |

The cache directory persists across runs so providers are not re-downloaded.

---

## Generated main.tf structure

### 1. terraform block

```hcl
terraform {
  required_version = ">= 1.1"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  backend "s3" {
    # all fields from twig.yaml backend block, plus:
    key = "infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>/terraform.tfstate"
  }
}
```

### 2. provider block

```hcl
provider "aws" {
  profile = "<profile>"
  region  = "<region>"
}
```

### 3. remote state blocks (one per `remote_state:` entry, in declaration order)

```hcl
data "terraform_remote_state" "<alias>" {
  backend = "s3"
  config = {
    # all backend fields from twig.yaml except dynamodb_table, plus derived key:
    bucket  = "<bucket>"
    region  = "<region>"
    key     = "infra/<cloud>/.../<component>/terraform.tfstate"
  }
}
```

### 4. module blocks

One block per module entry, in declaration order:

```hcl
module "<instance_key>" {
  source = "<absolute-path-to-module>"

  cloud       = "<cloud>"
  profile     = "<profile>"
  region      = "<region>"
  environment = "<environment>"
  class       = "<class>"
  component   = "<component>"
  module      = "<instance_key>"

  # user vars with cross-refs resolved
  <variable_name> = <value>
}
```

---

## CLI commands

```
twig <command> <leaf-file> [-- <terraform-flags>...]
```

`<leaf-file>` is required: a relative or absolute path to a leaf `.yaml` file.

| Command | Behavior |
|---|---|
| `twig init <leaf>` | Generate `main.tf` → `terraform init` in cache dir |
| `twig plan <leaf>` | Generate `main.tf` → auto-init if needed → `terraform plan` |
| `twig apply <leaf>` | Generate `main.tf` → auto-init if needed → `terraform apply` |
| `twig destroy <leaf>` | Generate `main.tf` → auto-init if needed → `terraform destroy` |
| `twig show <leaf>` | Print generated `main.tf` to stdout. No Terraform invocation. |

**Auto-init**: `plan`, `apply`, and `destroy` run `terraform init` automatically
if `.terraform/` is absent from the cache dir.

**Pass-through flags**: flags after `--` are forwarded verbatim to Terraform.

```
twig apply infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml -- -auto-approve
```

---

## Error conditions

| Condition | Error |
|---|---|
| `<leaf-file>` does not exist or is not a `.yaml` file | fatal |
| `twig.yaml` not found walking up from `<leaf-file>` | fatal |
| Leaf file not exactly at `infra/<p>/<p>/<p>/<p>/<p>/<name>.yaml` below project root | fatal |
| `source` module directory does not exist | fatal |
| Cross-ref `${x.y}` where `x` is neither a declared module key nor a `remote_state` alias | fatal |
| `remote_state` alias conflicts with a module instance key | fatal |
| `remote_state` leaf path does not match the path convention | fatal |
| Reserved path variable used in `vars` | fatal |
