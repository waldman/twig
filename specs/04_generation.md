# TWIG - GENERATION SPEC

## Generated main.tf structure

Blocks are emitted in this order: `terraform`, `provider` (one per cloud,
alphabetical), `data "terraform_remote_state"` (referenced aliases only,
alphabetical), `module` (one per `modules:` entry, declaration order),
`output` (one per output in each local module's `outputs.tf`, sorted).

### 1. terraform block

```hcl
terraform {
  required_version = ">= 1.10"
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

### 3. remote state blocks — lazy emission

Emitted only for aliases actually referenced in the leaf's resolved module
vars. The effective remote-state map for a leaf is the merge of the
`remotes:` sections in the inherited `vars.yaml` hierarchy overlaid with
the leaf's own `remotes:` block (see `specs/07_inherited_vars.md` — merge
order). Aliases that exist in the effective map but are not referenced by
any module var produce no data block.

Referenced-alias collection: after `module_defaults` injection and leaf
`vars:` overlay, twig walks every module's resolved var values and
collects every `${remotes.<alias>.<field>}` token. That set is the emit
set.

Aliases in the emit set that are missing from the effective remotes map
fail generation with an unresolved-reference error.

Emit order: alphabetical among referenced aliases.

```hcl
data "terraform_remote_state" "<alias>" {
  backend = "s3"
  config = {
    # all backend fields from twig.yaml except dynamodb_table and key, plus derived key:
    bucket  = "<bucket>"
    region  = "<region>"
    key     = "infra/<cloud>/.../<component>/terraform.tfstate"
  }
}
```

### 4. module blocks

One block per module entry in the leaf, in declaration order.

For each module instance, the emitted arguments are, in this order:

1. The seven path variables (`cloud`, `profile`, `region`, `environment`,
   `class`, `component`, `module`), always present.
2. The effective vars for this instance — the merge of
   `module_defaults[<mod.source>]` from the inherited hierarchy overlaid
   by `modules.<instance>.vars` from the leaf. Leaf wins per-key.
   Emitted in alphabetical order.

```hcl
module "<instance_key>" {
  source = "<module-source>"        # absolute filesystem path or git:: URL

  cloud       = "<cloud>"                  # from: path
  profile     = "<profile>"                # from: path
  region      = "<region>"                 # from: path
  environment = "<environment>"            # from: path
  class       = "<class>"                  # from: path
  component   = "<component>"              # from: path
  module      = "<instance_key>"           # from: path

  <var_from_leaf>     = <value>            # from: leaf: modules.<instance>.vars
  <var_from_defaults> = <value>            # from: <path/to/vars.yaml>: module_defaults."<source>"
}
```

### 5. root-level output blocks

For local (non-git) module sources, twig reads each module's `outputs.tf`
and emits a root-level `output` block for every output declared there.
These blocks expose the leaf's outputs so that other leaves can consume them
via `data "terraform_remote_state"`. Without them, a state file has no
outputs for remote consumers to read.

Emitted after all module blocks, sorted alphabetically per module (in
module declaration order), then alphabetically within each module's outputs.

```hcl
output "<output_name>" {
  value = module.<instance_key>.<output_name>
}
```

Git-sourced modules are skipped — their `outputs.tf` is not locally
readable at generate time.

## Provenance comments

Every emitted argument in a module block carries a trailing
`# from: <origin>` comment identifying where the value came from. Three
origin forms:

| Origin | Meaning |
|---|---|
| `path` | one of the seven path variables derived from the leaf's file path |
| `leaf: modules.<instance>.vars` | declared directly in the leaf's `modules.<instance>.vars` block |
| `<path/to/vars.yaml>: module_defaults."<source>"` | inherited from a `module_defaults` entry in the named `vars.yaml` file |

Reference resolution does not affect provenance: a value that started as
`${vars.x}` in the leaf's vars still shows `# from: leaf: modules.<instance>.vars`
— origin tracks where the key-value pair was declared, not what the value
resolves to.

For multi-line values (lists, maps), the provenance comment appears at
the end of the closing bracket line:

```hcl
  ec2_security_group_ids = [
    module.sg.security_group_id,
  ]  # from: leaf: modules.ec2.vars
```
