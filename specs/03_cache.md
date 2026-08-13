# TWIG - CACHE SPEC

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
