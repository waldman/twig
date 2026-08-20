# Project config

The `twig.yaml` file at the project root — the file that marks a directory as a twig project and tells twig where to find modules and how to configure the Terraform backend.

## Location

Placed at the directory containing `infra/`. Twig walks up from the leaf file at invocation time until it finds `twig.yaml`; that directory becomes the project root.

```
<project-root>/
  twig.yaml         ← here
  infra/
    <cloud>/
      providers.yaml
      <profile>/<region>/<env>/<class>/<component>.yaml
```

## Minimum

```yaml
modules_path: ../terraform-modules/modules

backend:
  bucket:  waldman-terraform-state
  region:  us-east-1
  profile: myprofile
```

`bucket` and `region` are required. `profile` is technically optional (twig
passes it through verbatim to the S3 backend), but in practice required for
any named-profile setup — omitting it makes Terraform fall back to the default
credential chain, which silently picks the wrong account in multi-account
environments. Include it unless you are deliberately relying on env-var or
instance-profile credentials.

## `modules_path`

Points at the Terraform modules referenced by leaves' `source:` fields. Accepts either a local path or a git URL.

### Local path

Relative to the `twig.yaml` file, or absolute:

```yaml
modules_path: ../terraform-modules/modules
```

Local sources are validated at generate time — twig errors if any leaf's `source:` resolves to a directory that does not exist on disk.

### Git URL

Passed through to Terraform's built-in git module fetcher. Twig does not clone; Terraform does that during `terraform init`.

| Format | Example |
|---|---|
| Bare hostname (recommended) | `github.com/org/repo//subdir` |
| Full HTTPS | `https://github.com/org/repo.git//subdir` |
| SSH | `git@github.com:org/repo.git//subdir` |
| With explicit `git::` prefix | `git::https://github.com/org/repo.git//subdir` |

The `//` separates the repo URL from a subdirectory within the repo. Omit `//subdir` if modules live at the repo root.

Example:

```yaml
modules_path: github.com/waldman/terraform-modules//modules
```

Produces module sources like:

```hcl
source = "git::https://github.com/waldman/terraform-modules.git//modules/aws/5/vpc"
```

## `modules_ref`

Optional git ref (tag, branch, or commit SHA) to pin when `modules_path` is a git URL. Ignored for local paths.

```yaml
modules_path: github.com/waldman/terraform-modules//modules
modules_ref:  v2.1.0
```

Produces:

```hcl
source = "git::https://github.com/waldman/terraform-modules.git//modules/aws/5/vpc?ref=v2.1.0"
```

Omit `modules_ref` and Terraform uses the default branch.

## `TWIG_MODULES_PATH` — runtime override

The environment variable `TWIG_MODULES_PATH` overrides `modules_path` for a single invocation. Same local-vs-git detection rules apply.

Useful for pointing twig at a working copy of the modules repo without editing `twig.yaml`:

```bash
TWIG_MODULES_PATH=~/work/terraform-modules/modules \
  twig plan infra/aws/waldman/us-east-1/dev/services/app.yaml
```

## `backend`

The S3 backend configuration for Terraform state. Everything you put here is emitted verbatim into the generated `terraform { backend "s3" { ... } }` block, with one exception.

```yaml
backend:
  bucket:         waldman-terraform-state
  region:         us-east-1
  dynamodb_table: waldman-terraform-locks   # optional
  profile:        default                    # optional
  # any other s3-backend key works
```

**`backend.key` must not be set.** Twig derives the state key from each leaf's file path:

```
infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>/terraform.tfstate
```

Setting `backend.key` in `twig.yaml` is a fatal error — it would defeat the path-as-data model.

## Full example

```yaml
modules_path: github.com/waldman/terraform-modules//modules
modules_ref:  v2.1.0

backend:
  bucket:         waldman-terraform-state
  region:         us-east-1
  dynamodb_table: waldman-terraform-locks
  profile:        prod-tf
```

## See also

- [providers.md](providers.md) — the `providers.yaml` files that go alongside `twig.yaml`
- [`specs/01_project_config.md`](../specs/01_project_config.md) — formal reference
