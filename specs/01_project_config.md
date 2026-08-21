# TWIG - PROJECT CONFIG SPEC

## twig.yaml (project root config)

Placed at the directory containing `infra/`. Marks the project root and
configures the modules path and Terraform backend.

```yaml
modules_path: ../terraform-modules/modules
modules_ref:  v1.0.0                        # optional, only for git modules_path

backend:
  bucket:       waldman-terraform-state
  region:       us-east-1
  use_lockfile: true                        # requires Terraform >= 1.10
```

Both `modules_path` and `backend` are required.

### modules_path

May be a **local path** (relative to `twig.yaml` or absolute) or a **git URL**.

Local path:

```yaml
modules_path: ../terraform-modules/modules
```

Local sources are validated at generation time — twig errors if a module's
source subdirectory does not exist on disk.

Git URL — supported formats:

| Format | Example |
|---|---|
| Bare hostname (recommended) | `github.com/org/repo//subdir` |
| Full HTTPS | `https://github.com/org/repo.git//subdir` |
| SSH | `git@github.com:org/repo.git//subdir` |
| With explicit `git::` prefix | `git::https://github.com/org/repo.git//subdir` |

The `//` separates the repository URL from the subdirectory within the repo.
Omit `//subdir` if modules live at the repo root. Git sources are passed
through to Terraform as `git::` URLs; twig does not clone the repository
itself.

### modules_ref

Optional git ref (tag, branch, or commit SHA) to pin when `modules_path` is
a git URL. Omit to use the default branch. Ignored for local `modules_path`.

```yaml
modules_path: github.com/waldman/terraform-modules//modules
modules_ref:  v2.1.0
```

Produces module sources like:

```hcl
source = "git::https://github.com/waldman/terraform-modules.git//modules/aws/5/vpc?ref=v2.1.0"
```

### TWIG_MODULES_PATH override

The environment variable `TWIG_MODULES_PATH` overrides `modules_path` at
runtime. It follows the same local-vs-git detection rules. Useful for
pointing a single twig invocation at a working-copy of the modules repo
without editing `twig.yaml`.

### backend

Emitted verbatim into the generated `terraform { backend "s3" {} }` block.
`key` is derived from the leaf path and **must not** appear in `twig.yaml`.
Any other key/value pair from the map is passed through unchanged.

`use_lockfile: true` is the standard locking mechanism (Terraform >= 1.10,
native S3 object locking). The deprecated `dynamodb_table` key is silently
excluded from `data "terraform_remote_state"` config blocks but still
passed through to the backend block if present. New deployments should use
`use_lockfile` instead.
