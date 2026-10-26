# TWIG - PROVIDERS SPEC

## providers.yaml

Each cloud used in a leaf's modules must have a `providers.yaml` file
alongside its subtree:

```
infra/<cloud>/providers.yaml
```

This file declares the Terraform provider source, version, and configuration
for that cloud. twig reads it during generation and emits the corresponding
`required_providers` entries and `provider {}` blocks.

### Format

```yaml
<cloud>:
  source: <registry-source>
  config:
    <provider-config-key>: <value>
```

- The top-level key must match the `<cloud>` segment used in module source
  paths (e.g. `aws` for a module sourced from `aws/5/vpc`).
- `source` is a Terraform registry source (`hashicorp/aws`,
  `datadog/datadog`, etc.). The HCL provider name is derived from the last
  segment of the source (`hashicorp/aws` → `aws`).
- `config` values may be strings, numbers, booleans, or maps. String values
  may reference path variables via `${cloud}`, `${profile}`, `${region}`,
  `${environment}`, `${class}`, `${component}` and are substituted at
  generation time. Map values are rendered as nested HCL blocks (e.g.
  `features: {}` becomes `features {}`). Nesting is recursive.
- `env_files:` is **not** valid in `providers.yaml` — use `vars.yaml` at the
  same level instead. twig will error if `env_files:` is present.

### Multi-cloud

A single `providers.yaml` may declare multiple clouds:

```yaml
# infra/aws/providers.yaml
aws:
  source: hashicorp/aws
  config:
    profile: "${profile}"
    region:  "${region}"

datadog:
  source: datadog/datadog
  config:
    api_key: "my-api-key"
```

If a leaf uses modules from more than one cloud (e.g. `aws/5/vpc` and
`datadog/1/monitor`), twig emits one `provider` block and one
`required_providers` entry per distinct cloud. The version constraint for
each provider is derived from the `<major>` segment of that cloud's module
sources — `aws/5/vpc` yields `version = "~> 5.0"`. All modules for a given
cloud within one leaf must agree on the major version.

### Example (single cloud)

```yaml
# infra/aws/providers.yaml
aws:
  source: hashicorp/aws
  config:
    profile: "${profile}"
    region:  "${region}"
```

For a leaf at `infra/aws/waldman/us-east-1/production/services/app.yaml`
with modules using `aws/5/*`, this generates:

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  # ...
}

provider "aws" {
  profile = "waldman"
  region  = "us-east-1"
}
```

## Provider aliases

A leaf can declare additional, aliased provider blocks for cross-region or
cross-account modules, with a top-level `provider_aliases:` key:

```yaml
provider_aliases:
  aws:
    - {account: waldman, region: us-west-2}
    - {account: shared,  region: us-east-1}
```

- Each entry is a strict `{account, region}` map; both keys are required.
- The alias name is derived as `<account>_<region>`, with dashes replaced by
  underscores: `waldman_us_west_2`, `shared_us_east_1`.
- `infra/<cloud>/<account>/<region>/` must exist below the project root.
- An entry equal to the leaf's own `(profile, region)` is rejected, because
  that is the default provider.
- `(account, region)` pairs and derived alias names must be unique per cloud.
- The cloud must be declared in `providers.yaml`.

Each entry renders as one more `provider` block for that cloud, after the
default blocks and in declaration order. Its config is the cloud's
`providers.yaml` template with `profile` set to `<account>` and `region` set
to `<region>`; every other key is substituted with the leaf's own path
variables. The block shares the cloud's `required_providers` entry.

```hcl
provider "aws" {
  alias   = "waldman_us_west_2"
  profile = "waldman"
  region  = "us-west-2"
}
```

### Routing aliases into modules

An aliased provider block is used only where a module is told to use it.
`modules.<instance>.providers:` maps a module-side provider key to an alias
name:

```yaml
modules:
  peering:
    source: aws/5/vpc-peering
    providers:
      aws.peer: waldman_us_west_2
```

renders as

```hcl
module "peering" {
  source = "..."

  providers = {
    aws.peer = aws.waldman_us_west_2
  }
  # path variables and vars follow
}
```

- The key is the module's provider key: `aws` replaces the module's default
  provider, and `aws.<name>` fills one of the module's
  `configuration_aliases`. twig does not read the module's
  `required_providers`; Terraform reports a mismatch at plan time.
- The key's cloud (the part before the first dot) must have a default
  provider block in this leaf.
- The value must be an alias name declared in `provider_aliases` for that
  cloud.
