# TWIG - CACHE SPEC

## Cache directory

Generated files and Terraform working state live in:

```
~/.twig-cache/<sha256-of-absolute-leaf-file-path>/
```

| Path | Description |
|---|---|
| `main.tf` | Generated on every twig invocation |
| `.terraform-version` | Copied from the project root on every invocation except `show`, when present. tfenv looks for this file from the working directory upward, and Terraform runs in the cache dir, so without the copy tfenv would not see the project's pinned version |
| `.terraform/` | Provider and module cache, populated by `terraform init` |
| `.terraform.lock.hcl` | Provider lock file |

The cache directory persists across runs. twig runs `terraform init
-reconfigure -upgrade` on every invocation (see `specs/05_cli.md`):
providers already in `.terraform/` are reused, and a newer release is
downloaded, and recorded in `.terraform.lock.hcl`, only when one matches the
version constraints.
