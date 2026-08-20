# Leaves

The `.yaml` files at the deepest level of the `infra/` tree — one per deployable component.

## Location

```
infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>.yaml
```

Where `<component>` becomes the leaf's `component` path variable. Everything above the filename is derived from directory segments.

## Format

```yaml
remotes:                                    # optional
  <alias>: <path-to-another-leaf-relative-to-project-root>

modules:                                    # required
  <instance_key>:
    source: <cloud>/<version>/<module-name>
    vars:                                   # optional
      <variable_name>: <value>
```

Rules:

- Instance keys are unique within the file, and become both the Terraform module label (`module "<instance_key>"`) and the value of the `module` path variable injected into that module call.
- Instance keys and `remotes` aliases may not be `modules`, `remotes`, or `vars` (reserved ref namespaces).
- The seven path variables (`cloud`, `profile`, `region`, `environment`, `class`, `component`, `module`) are injected automatically and may not be declared in `vars:`.
- `vars:` values are strings, numbers, booleans, lists, or maps.

## References

Values in `vars:` can contain references — three namespaces, all requiring the namespace prefix:

| Form | Resolves to |
|---|---|
| `${modules.<instance>.<output>}` | `module.<instance>.<output>` — another module in the same leaf |
| `${remotes.<alias>.<output>}` | `data.terraform_remote_state.<alias>.outputs.<output>` — another leaf's state |
| `${vars.<name>}` | inlined value from the inherited `vars:` map (see [vars-yaml.md](vars-yaml.md)) |

A **pure** reference — the whole value is one `${...}` token — becomes an unquoted HCL expression with the underlying type. An **embedded** reference becomes an interpolated string.

| YAML | Generated HCL |
|---|---|
| `${modules.x.bucket_arn}` | `module.x.bucket_arn` |
| `${remotes.vpc.vpc_id}` | `data.terraform_remote_state.vpc.outputs.vpc_id` |
| `${modules.x.bucket_arn}/*` | `"${module.x.bucket_arn}/*"` |
| `arn:aws:s3:::my-bucket` | `"arn:aws:s3:::my-bucket"` |

Unqualified `${x.y}` is not a reference — it emits as a literal string. All three namespace prefixes are required.

## `remotes:` — depend on another leaf

Declares outputs consumed from other leaves. Values are read from their S3 state files at plan/apply time — no data copying, no hardcoding.

```yaml
remotes:
  vpc: infra/aws/waldman/us-east-1/base/vpc/main.yaml
```

- Aliases must not conflict with module instance keys.
- Aliases must not be reserved ref namespaces.
- The leaf's `remotes:` merges with the inherited `remotes:` from the `vars.yaml` hierarchy — leaf wins per alias on collision.
- Only aliases actually referenced in resolved module vars produce a `data "terraform_remote_state"` block in the output (lazy emission — see [vars-yaml.md](vars-yaml.md#lazy-emission)).

## Worked example — intra-leaf refs

Six modules in one component, wired together by `${modules.<instance>.<output>}` references:

```yaml
# infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml

modules:
  s3_bucket:
    source: aws/5/s3-bucket
    vars:
      s3_bucket_name: anchor-automation

  dynamodb:
    source: aws/5/dynamodb
    vars:
      dynamodb_table_name:    ansible-anchor
      dynamodb_hash_key:      node
      dynamodb_ttl_enabled:   true
      dynamodb_ttl_attribute: ttl

  iam_cicd:
    source: aws/5/iam-user

  iam_cicd_policy:
    source: aws/5/iam-policy
    vars:
      iam_policy_user_name: ${modules.iam_cicd.user_name}
      iam_policy_statements:
        - effect:    Allow
          actions:   [s3:GetObject, s3:PutObject, s3:DeleteObject, s3:ListBucket]
          resources:
            - ${modules.s3_bucket.bucket_arn}
            - ${modules.s3_bucket.bucket_arn}/*

  iam_infra:
    source: aws/5/iam-user

  iam_infra_policy:
    source: aws/5/iam-policy
    vars:
      iam_policy_user_name: ${modules.iam_infra.user_name}
      iam_policy_statements:
        - effect:    Allow
          actions:   [s3:GetObject, s3:ListBucket]
          resources:
            - ${modules.s3_bucket.bucket_arn}
            - ${modules.s3_bucket.bucket_arn}/*
        - effect:    Allow
          actions:   [dynamodb:PutItem, dynamodb:GetItem, dynamodb:DescribeTable]
          resources:
            - ${modules.dynamodb.table_arn}
```

`iam_cicd` and `iam_infra` need no `vars:` — the injected `module` path variable (`iam_cicd` / `iam_infra`) already distinguishes them within the component.

## Worked example — cross-leaf refs

```yaml
# infra/aws/waldman/us-east-1/dev/ec2/app.yaml

remotes:
  vpc: infra/aws/waldman/us-east-1/base/vpc/main.yaml

modules:
  app:
    source: aws/5/ec2
    vars:
      ec2_vpc_id:    ${remotes.vpc.vpc_id}
      ec2_subnet_id: ${remotes.vpc.first_public_subnet_id}
```

Generates a `data "terraform_remote_state" "vpc"` block that reads the two outputs from the vpc leaf's state file. No coordination beyond agreeing on output names.

## See also

- [vars-yaml.md](vars-yaml.md) — sharing config across leaves via `vars.yaml`
- [`specs/02_leaf_file.md`](../specs/02_leaf_file.md) — formal reference
