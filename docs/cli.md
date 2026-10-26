# CLI

```
twig <command> <leaf-file> [-- <terraform-flags>...]
```

`<leaf-file>` is required — a relative or absolute path to a leaf `.yaml` file. Twig walks up from that file to find `twig.yaml` and derives the project root, path variables, and effective inherited state.

## Commands

| Command | Behavior |
|---|---|
| `twig show <leaf>` | Print the generated `main.tf` to stdout. No Terraform invocation. |
| `twig init <leaf>` | Generate `main.tf` → `terraform init -reconfigure -upgrade` in the cache dir. |
| `twig plan <leaf>` | Generate → init → `terraform plan`. |
| `twig apply <leaf>` | Generate → init → `terraform apply`. |
| `twig destroy <leaf>` | Generate → init → `terraform destroy`. |
| `twig output <leaf>` | Generate → init → `terraform output`. |
| `twig state <leaf>` | Generate → init → `terraform state <subcmd>`. |

**Init.** `plan`, `apply`, `destroy`, `output`, and `state` run `terraform init -reconfigure -upgrade` before the requested command, every time. `-reconfigure` picks up backend changes without a migration prompt; `-upgrade` re-resolves module and provider versions against their constraints. Flags after `--` go to the requested command, not to this init; `twig init` appends them to its own init.

**Outputs.** `twig output` lists one entry per module instance. Instances whose module has a sensitive output — and every git-sourced instance — show as `<sensitive>`; `twig output <leaf> -- <instance>` or `twig output <leaf> -- -json` prints their values.

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
| `.terraform-version` | Copied from the project root when present, so tfenv picks the project's Terraform version (Terraform runs inside the cache dir). |
| `.terraform/` | Provider cache, populated by `terraform init`. |
| `.terraform.lock.hcl` | Provider lock file. |

The cache persists across runs — providers already downloaded are reused, and `init -upgrade` fetches a provider only when a newer release matches the constraints. Deleting a leaf's cache dir is safe; the next invocation re-initialises it.

## See also

- [`specs/05_cli.md`](../specs/05_cli.md) — formal reference
- [`specs/03_cache.md`](../specs/03_cache.md) — cache directory contract
