# TWIG - CLI SPEC

## CLI commands

```
twig <command> <leaf-file> [-- <terraform-flags>...]
```

`<leaf-file>` is required: a relative or absolute path to a leaf `.yaml` file.

| Command | Behavior |
|---|---|
| `twig show <leaf>` | Print generated `main.tf` to stdout. No Terraform invocation. |
| `twig init <leaf>` | Generate `main.tf` → `terraform init -reconfigure -upgrade` in cache dir |
| `twig plan <leaf>` | Generate `main.tf` → init → `terraform plan` |
| `twig apply <leaf>` | Generate `main.tf` → init → `terraform apply` |
| `twig destroy <leaf>` | Generate `main.tf` → init → `terraform destroy` |
| `twig output <leaf>` | Generate `main.tf` → init → `terraform output` |
| `twig state <leaf>` | Generate `main.tf` → init → `terraform state <subcmd>` |

**Init**: every command except `show` writes `main.tf` and
`.terraform-version` into the cache dir (see `specs/03_cache.md`). `plan`,
`apply`, `destroy`, `output`, and `state` then run
`terraform init -reconfigure -upgrade` before the requested command, on
every invocation. `-reconfigure` adopts backend-block changes without a
state-migration prompt; `-upgrade` re-resolves module and provider versions
against their constraints. Pass-through flags go to the requested command,
not to this init. `twig init` runs the same init and appends its
pass-through flags.

**Outputs**: `twig output` lists one entry per module instance (see
`specs/04_generation.md` §5). Instances marked sensitive appear as
`<sensitive>` in the listing; `twig output <leaf> -- <instance>` or
`twig output <leaf> -- -json` prints their values.

**Pass-through flags**: flags after `--` are forwarded verbatim to Terraform.

```
twig apply  infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml -- -auto-approve
twig output infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml -- -json
twig state  infra/aws/waldman/us-east-1/dev/ec2/web.yaml -- mv module.ec2.aws_security_group.this module.sg.aws_security_group.this
```
