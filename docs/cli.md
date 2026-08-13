# CLI

```
twig <command> <leaf-file> [-- <terraform-flags>...]
```

`<leaf-file>` is required — a relative or absolute path to a leaf `.yaml` file. Twig walks up from that file to find `twig.yaml` and derives the project root, path variables, and effective inherited state.

## Commands

| Command | Behavior |
|---|---|
| `twig show <leaf>` | Print the generated `main.tf` to stdout. No Terraform invocation. |
| `twig init <leaf>` | Generate `main.tf` → `terraform init` in the cache dir. |
| `twig plan <leaf>` | Generate → auto-init if needed → `terraform plan`. |
| `twig apply <leaf>` | Generate → auto-init if needed → `terraform apply`. |
| `twig destroy <leaf>` | Generate → auto-init if needed → `terraform destroy`. |
| `twig output <leaf>` | Generate → auto-init if needed → `terraform output`. |
| `twig state <leaf>` | Generate → auto-init if needed → `terraform state <subcmd>`. |

**Auto-init.** `plan`, `apply`, `destroy`, `output`, and `state` run `terraform init` automatically the first time (when `.terraform/` is absent from the cache dir).

## Pass-through flags

Flags after `--` are forwarded verbatim to the underlying Terraform command:

```bash
twig apply  infra/aws/waldman/us-east-1/production/services/app.yaml -- -auto-approve
twig output infra/aws/waldman/us-east-1/production/services/app.yaml -- -json
twig state  infra/aws/waldman/us-east-1/dev/ec2/web.yaml \
  -- mv module.ec2.aws_security_group.this module.sg.aws_security_group.this
```

## `twig show` — inspect without running

`twig show` prints the generated `main.tf` — every provider block, every `data "terraform_remote_state"` block, every `module` block with fully-resolved arguments and provenance comments — without touching Terraform. Use it to:

- Verify what a leaf will produce before running `plan`.
- Trace where any argument came from (the `# from: <origin>` comments).
- Debug reference resolution.

## Cache directory

Generated files and Terraform working state live in a persistent cache directory keyed by the absolute leaf path:

```
~/.twig-cache/<sha256-of-absolute-leaf-file-path>/
```

| Path inside | Contents |
|---|---|
| `main.tf` | Regenerated on every twig invocation. |
| `.terraform/` | Provider cache, populated by `terraform init`. |
| `.terraform.lock.hcl` | Provider lock file. |

The cache persists across runs — providers are not re-downloaded on every invocation. Deleting a leaf's cache dir is safe; the next invocation will re-init.

## See also

- [`specs/05_cli.md`](../specs/05_cli.md) — formal reference
- [`specs/03_cache.md`](../specs/03_cache.md) — cache directory contract
