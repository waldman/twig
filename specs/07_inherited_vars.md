# TWIG - INHERITED VARIABLES SPEC

## vars.yaml

A `vars.yaml` file may be placed at any directory level under `infra/` (from
the `infra/` root down through each path segment above the leaf). Variables
declared in these files are merged and made available to the leaf.

```
infra/vars.yaml                                              # all leaves
infra/aws/vars.yaml                                          # all AWS leaves
infra/aws/<profile>/vars.yaml                                # all leaves in profile
infra/aws/<profile>/<region>/vars.yaml                       # all leaves in region
infra/aws/<profile>/<region>/<env>/vars.yaml                 # all leaves in env
infra/aws/<profile>/<region>/<env>/<class>/vars.yaml         # all leaves in class
```

`vars.yaml` files are optional at every level. Missing files are silently skipped.

### Format

```yaml
<variable_name>: <value>
```

Top-level keys are variable names; values are strings, numbers, booleans,
lists, or maps.

### Merge order

vars.yaml files are merged from the most general level (`infra/vars.yaml`)
down to the most specific level closest to the leaf. On key collisions,
**closer to the leaf wins**. The merged map is the leaf's inherited-vars
namespace.

### Referencing inherited vars

Leaves reference inherited vars with `${var.<name>}`:

```yaml
modules:
  sg:
    source: aws/5/security-group
    vars:
      sg_ingress_cidr: ${var.vpn_cidr}
```

A pure `${var.x}` reference resolves to the underlying value with its native
type (string, bool, number, list, map). Embedded in a longer string, it is
substituted as its string representation.

### Injection into modules

**Current behavior:** every inherited variable is injected as an argument
into every module block in the generated `main.tf`. Module-level `vars:`
entries override inherited values for the same key.

This means `vars.yaml` must only contain variables that every module in every
leaf below it declares in its Terraform `variables.tf` — otherwise Terraform
will error with "argument named X is not expected" on modules that don't
declare the variable. In practice this restricts `vars.yaml` to truly
cross-cutting values (e.g. `default_tags`).

### Restrictions

- The seven reserved path variable names (`cloud`, `profile`, `region`,
  `environment`, `class`, `component`, `module`) may not be used as keys in
  `vars.yaml`.
- References (`${module.x.y}`, `${remote.x.y}`, `${var.x}`) may not appear
  inside `vars.yaml` files. Values must be literal.
