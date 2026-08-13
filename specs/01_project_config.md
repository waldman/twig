# TWIG - PROJECT CONFIG SPEC

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
