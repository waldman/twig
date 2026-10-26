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

provider_aliases:                           # optional — see providers.md
  <cloud>:
    - {account: <account>, region: <region>}

modules:                                    # required
  <instance_key>:
    source: <cloud>/<version>/<module-name>
    providers:                              # optional — see providers.md
      <module-provider-key>: <alias-name>
    vars:                                   # optional
      <variable_name>: <value>
```

Rules:

- Instance keys are unique within the file, and become both the Terraform module label (`module "<instance_key>"`) and the value of the `module` path variable injected into that module call.
- Instance keys and `remotes` aliases may not be `modules`, `remotes`, or `vars` (reserved ref namespaces).
- Instance keys and `remotes` aliases must match `[a-zA-Z_][a-zA-Z0-9_]*`. Twig's `${...}` references cannot address a key containing a hyphen or any other character, so such keys are rejected.
- The seven path variables (`cloud`, `profile`, `region`, `environment`, `class`, `component`, `module`) are injected automatically and may not be declared in `vars:`.
- `vars:` values are strings, numbers, booleans, lists, or maps.
- String values may contain `${cloud}`, `${profile}`, `${region}`, `${environment}`, `${class}`, and `${component}`, replaced with the leaf's own values.

## References

Values in `vars:` can contain references — three namespaces, all requiring the namespace prefix:

| Form | Resolves to |
|---|---|
| `${modules.<instance>.<output>}` | `module.<instance>.<output>` — another module in the same leaf |
| `${remotes.<alias>.<instance>.<output>}` | `data.terraform_remote_state.<alias>.outputs.<instance>.<output>` — an output of module `<instance>` in another leaf |
| `${vars.<name>}` | inlined value from the inherited `vars:` map (see [vars-yaml.md](vars-yaml.md)) |

A **pure** reference — the whole value is one `${...}` token — becomes an unquoted HCL expression with the underlying type. An **embedded** reference becomes an interpolated string.

| YAML | Generated HCL |
|---|---|
| `${modules.x.bucket_arn}` | `module.x.bucket_arn` |
| `${remotes.vpc.main.vpc_id}` | `data.terraform_remote_state.vpc.outputs.main.vpc_id` |
| `${modules.x.bucket_arn}/*` | `"${module.x.bucket_arn}/*"` |
| `arn:aws:s3:::my-bucket` | `"arn:aws:s3:::my-bucket"` |

Unqualified `${x.y}` is not a reference — it emits as a literal string. All three namespace prefixes are required.

Each namespace has exactly one shape — any other number of segments is an error. The flat form `${remotes.<alias>.<output>}` is rejected with an error that shows the namespaced form.

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

`iam_cicd` and `iam_infra` need no `vars:` — the injected `module` path variable (`iam_cicd` / `iam_infra`) already distinguishes them within the component. Each also gets its own root output, so another leaf can read `${remotes.<alias>.iam_cicd.user_name}`.

## What a leaf exposes to other leaves

Twig emits one root output per module instance, named after the instance key, whose value is the whole module object:

```hcl
output "main" {
  value = module.main
}
```

Other leaves read it as `${remotes.<alias>.<instance>.<output>}`. Instance keys are unique within a leaf, so two modules of the same source (two `iam-user` instances, say) never collide.

When any output of the module is `sensitive`, the instance's root output is marked `sensitive = true` (Terraform requires it). Git-sourced modules can't be inspected at generate time, so their instances are always marked. Marking only changes the producing leaf's display — `terraform_remote_state` does not carry sensitivity across leaves, so consumers get plain values.

## Worked example — cross-leaf refs

The vpc leaf declares its VPC module as instance `main`:

```yaml
# infra/aws/waldman/us-east-1/base/vpc/main.yaml

modules:
  main:
    source: aws/5/vpc
```

The consumer reads that instance's outputs:

```yaml
# infra/aws/waldman/us-east-1/dev/ec2/app.yaml

remotes:
  vpc: infra/aws/waldman/us-east-1/base/vpc/main.yaml

modules:
  app:
    source: aws/5/ec2
    vars:
      ec2_vpc_id:    ${remotes.vpc.main.vpc_id}
      ec2_subnet_id: ${remotes.vpc.main.first_public_subnet_id}
```

Generates a `data "terraform_remote_state" "vpc"` block that reads the two outputs from the `main` root output in the vpc leaf's state file. No coordination beyond agreeing on the instance key and output names.

## See also

- [vars-yaml.md](vars-yaml.md) — sharing config across leaves via `vars.yaml`
- [`specs/02_leaf_file.md`](../specs/02_leaf_file.md) — formal reference
