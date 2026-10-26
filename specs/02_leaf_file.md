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
    providers:                       # optional — see "Provider aliases"
      <module-provider-key>: <alias-name>
    vars:
      <variable_name>: <value>

provider_aliases:                    # optional — see "Provider aliases"
  <cloud>:
    - {account: <account>, region: <region>}
```

### Rules

- `instance_key` must be unique within the file. It becomes both the
  Terraform module label (`module "<instance_key>"`) and the `module`
  variable injected into that module call.
- `instance_key` must not be one of the reserved ref namespaces (`modules`,
  `remotes`, `vars`).
- `instance_key` must match `[a-zA-Z_][a-zA-Z0-9_]*`, the identifier
  charset of twig's `${...}` reference syntax. A key outside it (for example
  one containing a hyphen) could never be referenced, so it is rejected when
  the leaf is loaded.
- `source` is resolved against `modules_path` (see
  `specs/01_project_config.md`) — local directory for local `modules_path`,
  git URL for git `modules_path`.
- All seven path variables are injected automatically and cannot appear
  in `vars`.
- `vars` values are strings, numbers, booleans, lists, or maps.
- String values, at any depth of a list or map, may contain the path
  variables `${cloud}`, `${profile}`, `${region}`, `${environment}`,
  `${class}`, and `${component}`; they are replaced with the leaf's own
  values at generate time. The same applies to `module_defaults` values
  (see `specs/07_inherited_vars.md`).

### Remote state references

An optional `remotes:` block declares outputs consumed from other leaves.
The values are read from their S3 state files at plan/apply time — no data
copying, no hardcoding.

```yaml
remotes:
  <alias>: <path-to-leaf.yaml relative to project root>
```

- Aliases must not conflict with module instance keys.
- Aliases must not be reserved ref namespaces (`modules`, `remotes`, `vars`).
- Aliases must match `[a-zA-Z_][a-zA-Z0-9_]*`, for the same reason as
  instance keys.
- The same backend config (bucket, region, profile) is used; only the `key`
  changes to point at the target leaf's state file.
- The leaf's `remotes:` block merges with the inherited `remotes:` map from
  the `vars.yaml` hierarchy (see `specs/07_inherited_vars.md`). On
  alias-key collisions, the leaf wins.
- Only aliases actually referenced in the resolved module vars produce
  a `data "terraform_remote_state"` block in the output (lazy emission).
  Declaring an alias here or in a `vars.yaml` and never referencing it
  is not an error.

### Provider aliases

A leaf may declare extra, aliased provider blocks with `provider_aliases:`
and route them into module instances with `providers:`:

```yaml
provider_aliases:
  aws:
    - {account: waldman, region: us-west-2}
    - {account: shared,  region: us-east-1}

modules:
  peering:
    source: aws/5/vpc-peering
    providers:
      aws.peer: waldman_us_west_2
```

- Each `provider_aliases` entry is an `(account, region)` target; both keys
  are required. The alias name is `<account>_<region>`, with dashes replaced
  by underscores.
- `providers:` maps a module-side provider key (`aws` for the module's
  default provider, `aws.<name>` for one of its `configuration_aliases`) to
  an alias name declared in `provider_aliases`.

Generation and validation rules are in `specs/08_providers.md`.

### References

Values may contain `${<ns>.<segment>...}` references. The namespace prefix
is required — twig does not resolve unqualified `${x.y}` references. Three
namespaces exist, each with exactly one shape:

| Form | Resolves to |
|---|---|
| `${modules.<instance>.<output>}` | `module.<instance>.<output>` (intra-leaf) |
| `${remotes.<alias>.<instance>.<output>}` | `data.terraform_remote_state.<alias>.outputs.<instance>.<output>` (cross-leaf) |
| `${vars.<name>}` | value from the merged `vars:` section in the `vars.yaml` hierarchy (see `specs/07_inherited_vars.md`) |

In the `remotes` form, `<instance>` is a module instance key declared in the
remote leaf and `<output>` is one of that module's outputs — the remote leaf
exposes one root output per module instance (see "Root-level outputs").
twig does not open the remote leaf to check `<instance>` or `<output>`;
Terraform reports an unsupported attribute at plan time.

A reference with any other number of segments is a fatal error (see
`specs/06_errors.md`). The flat cross-leaf form `${remotes.<alias>.<output>}`
is rejected with an error that names the namespaced form.

A **pure** reference — the entire value string is one `${...}` token —
becomes an unquoted HCL expression whose type matches the underlying value.
A reference **embedded** in a longer string becomes an interpolated Terraform
string.

| YAML value | Generated HCL |
|---|---|
| `${modules.x.bucket_arn}` | `module.x.bucket_arn` |
| `${remotes.vpc.main.vpc_id}` | `data.terraform_remote_state.vpc.outputs.main.vpc_id` |
| `${modules.x.bucket_arn}/*` | `"${module.x.bucket_arn}/*"` |
| `arn:aws:s3:::my-bucket` | `"arn:aws:s3:::my-bucket"` |

### Root-level outputs

twig emits one root-level `output` per module instance, named after the
instance key, whose value is the whole module object (`module.<instance>`).
This is what makes `data "terraform_remote_state"` consumers work — without
root-level output blocks, a state file exposes no outputs. Instance keys
are unique within a leaf, so root output names never collide, and several
instances of the same module source (e.g. two `aws/5/iam-user` modules) are
valid in one leaf. Git-sourced modules are included. Sensitivity rules are
in `specs/04_generation.md` §5.

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
      iam_policy_user_name: ${modules.iam_cicd.user_name}
      iam_policy_statements:
        - effect: Allow
          actions: [s3:GetObject, s3:PutObject, s3:DeleteObject, s3:ListBucket]
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
        - effect: Allow
          actions: [s3:GetObject, s3:ListBucket]
          resources:
            - ${modules.s3_bucket.bucket_arn}
            - ${modules.s3_bucket.bucket_arn}/*
        - effect: Allow
          actions: [dynamodb:PutItem, dynamodb:GetItem, dynamodb:DescribeTable]
          resources:
            - ${modules.dynamodb.table_arn}
```

Note: `iam_cicd` and `iam_infra` require no `vars` — the `module` variable
(`iam_cicd` / `iam_infra`) already distinguishes them within the component.
Each instance also gets its own root output (`output "iam_cicd"`,
`output "iam_infra"`), so another leaf reads e.g.
`${remotes.<alias>.iam_cicd.user_name}`.

### Cross-leaf example

The producer leaf declares its VPC module as instance `main`:

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

twig generates a `data "terraform_remote_state" "vpc"` block and reads
`vpc_id` and `first_public_subnet_id` from the `main` root output in the vpc
leaf's S3 state file. No coordination required between the two leaf
operators beyond agreeing on the producer's instance key and output names.
