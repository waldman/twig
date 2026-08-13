# TWIG - LEAF FILE SPEC

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
- `instance_key` must not be one of the reserved ref namespaces (`module`,
  `remote`, `vars`).
- `source` is resolved against `modules_path` (see
  `specs/01_project_config.md`) — local directory for local `modules_path`,
  git URL for git `modules_path`.
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

- Aliases must not conflict with module instance keys.
- Aliases must not be reserved ref namespaces (`module`, `remote`, `vars`).
- The same backend config (bucket, region, profile) is used; only the `key`
  changes to point at the target leaf's state file.

### References

Values may contain `${<ns>.<key>[.<field>]}` references. The namespace prefix
is required — twig does not resolve unqualified `${x.y}` references. Three
namespaces exist:

| Form | Resolves to |
|---|---|
| `${module.<instance>.<output>}` | `module.<instance>.<output>` (intra-leaf) |
| `${remote.<alias>.<output>}` | `data.terraform_remote_state.<alias>.outputs.<output>` (cross-leaf) |
| `${vars.<name>}` | value from the merged `vars:` section in the `vars.yaml` hierarchy (see `specs/07_inherited_vars.md`) |

A **pure** reference — the entire value string is one `${...}` token —
becomes an unquoted HCL expression whose type matches the underlying value.
A reference **embedded** in a longer string becomes an interpolated Terraform
string.

| YAML value | Generated HCL |
|---|---|
| `${module.x.bucket_arn}` | `module.x.bucket_arn` |
| `${remote.vpc.vpc_id}` | `data.terraform_remote_state.vpc.outputs.vpc_id` |
| `${module.x.bucket_arn}/*` | `"${module.x.bucket_arn}/*"` |
| `arn:aws:s3:::my-bucket` | `"arn:aws:s3:::my-bucket"` |

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
      iam_policy_user_name: ${module.iam_cicd.user_name}
      iam_policy_statements:
        - effect: Allow
          actions: [s3:GetObject, s3:PutObject, s3:DeleteObject, s3:ListBucket]
          resources:
            - ${module.s3_bucket.bucket_arn}
            - ${module.s3_bucket.bucket_arn}/*

  iam_infra:
    source: aws/5/iam-user

  iam_infra_policy:
    source: aws/5/iam-policy
    vars:
      iam_policy_user_name: ${module.iam_infra.user_name}
      iam_policy_statements:
        - effect: Allow
          actions: [s3:GetObject, s3:ListBucket]
          resources:
            - ${module.s3_bucket.bucket_arn}
            - ${module.s3_bucket.bucket_arn}/*
        - effect: Allow
          actions: [dynamodb:PutItem, dynamodb:GetItem, dynamodb:DescribeTable]
          resources:
            - ${module.dynamodb.table_arn}
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
      ec2_vpc_id:    ${remote.vpc.vpc_id}
      ec2_subnet_id: ${remote.vpc.first_public_subnet_id}
```

twig generates a `data "terraform_remote_state" "vpc"` block that reads
`vpc_id` and `first_public_subnet_id` from the vpc leaf's S3 state file.
No coordination required between the two leaf operators beyond agreeing on
output names.
