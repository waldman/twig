# TWIG - GENERATION SPEC

## Generated main.tf structure

Blocks are emitted in this order: `terraform`, `provider` (one per cloud),
`data "terraform_remote_state"` (one per `remote_state:` alias, declaration
order), `module` (one per `modules:` entry, declaration order).

### 1. terraform block

```hcl
terraform {
  required_version = ">= 1.1"
  required_providers {
    # one entry per distinct cloud used by the leaf's modules
    <provider-hcl-name> = {
      source  = "<registry-source>"
      version = "~> <major>.0"
    }
  }
  backend "s3" {
    # all fields from twig.yaml backend block, plus:
    key = "infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>/terraform.tfstate"
  }
}
```

`required_providers` is populated from `infra/<cloud>/providers.yaml` for
each cloud used by the leaf (see `specs/08_providers.md`). The `<major>`
segment in the version constraint comes from the module source path — e.g.
a module sourced from `aws/5/vpc` yields `version = "~> 5.0"`.

### 2. provider blocks

One block per distinct cloud used by the leaf's modules. Config values are
taken from `infra/<cloud>/providers.yaml`, with `${cloud|profile|region|environment|class|component}`
path-variable substitutions applied.

```hcl
provider "<provider-hcl-name>" {
  <key> = <value>
}
```

### 3. remote state blocks (one per `remote_state:` entry, in declaration order)

```hcl
data "terraform_remote_state" "<alias>" {
  backend = "s3"
  config = {
    # all backend fields from twig.yaml except dynamodb_table, plus derived key:
    bucket  = "<bucket>"
    region  = "<region>"
    key     = "infra/<cloud>/.../<component>/terraform.tfstate"
  }
}
```

### 4. module blocks

One block per module entry, in declaration order:

```hcl
module "<instance_key>" {
  source = "<module-source>"       # absolute filesystem path or git:: URL

  cloud       = "<cloud>"
  profile     = "<profile>"
  region      = "<region>"
  environment = "<environment>"
  class       = "<class>"
  component   = "<component>"
  module      = "<instance_key>"

  # inherited vars from vars.yaml hierarchy (see specs/07_inherited_vars.md)
  # and user vars from the leaf's modules.<instance>.vars, with references resolved
  <variable_name> = <value>
}
```
