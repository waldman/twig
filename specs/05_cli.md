# TWIG - CLI SPEC

## CLI commands

```
twig <command> <leaf-file> [-- <terraform-flags>...]
```

`<leaf-file>` is required: a relative or absolute path to a leaf `.yaml` file.

| Command | Behavior |
|---|---|
| `twig show <leaf>` | Print generated `main.tf` to stdout. No Terraform invocation. |
| `twig init <leaf>` | Generate `main.tf` → `terraform init` in cache dir |
| `twig plan <leaf>` | Generate `main.tf` → auto-init if needed → `terraform plan` |
| `twig apply <leaf>` | Generate `main.tf` → auto-init if needed → `terraform apply` |
| `twig destroy <leaf>` | Generate `main.tf` → auto-init if needed → `terraform destroy` |
| `twig output <leaf>` | Generate `main.tf` → auto-init if needed → `terraform output` |
| `twig state <leaf>` | Generate `main.tf` → auto-init if needed → `terraform state <subcmd>` |

**Auto-init**: `plan`, `apply`, `destroy`, `output`, and `state` run
`terraform init` automatically if `.terraform/` is absent from the cache dir.

**Pass-through flags**: flags after `--` are forwarded verbatim to Terraform.

```
twig apply  infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml -- -auto-approve
twig output infra/aws/waldman/us-east-1/production/services/ansible-anchor.yaml -- -json
twig state  infra/aws/waldman/us-east-1/dev/ec2/web.yaml -- mv module.ec2.aws_security_group.this module.sg.aws_security_group.this
```
