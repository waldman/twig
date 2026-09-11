# Providers

The `providers.yaml` files that live under `infra/<cloud>/` — one per cloud your leaves use. They tell twig how to render the Terraform `required_providers` and `provider` blocks.

## Location

```
infra/
  aws/providers.yaml       ← required if any leaf below uses aws/*/... modules
  gcp/providers.yaml       ← required if any leaf below uses gcp/*/... modules
  ...
```

If a leaf uses modules from a cloud with no matching `providers.yaml`, twig errors at generate time.

## Format

```yaml
<cloud>:
  source: <terraform-registry-source>
  config:
    <provider-config-key>: <value>
```

The top-level key matches the `<cloud>` segment in module source paths (e.g. `aws` matches modules with source `aws/5/vpc`).

### Example — single provider

```yaml
# infra/aws/providers.yaml
aws:
  source: hashicorp/aws
  config:
    profile: "${profile}"
    region:  "${region}"
```

Generates:

```hcl
terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
  ...
}

provider "aws" {
  profile = "waldman"
  region  = "us-east-1"
}
```

The version constraint (`~> 5.0`) is derived from the `<major>` segment of the leaf's module sources — `aws/5/vpc` yields `~> 5.0`. All modules for a given cloud within one leaf must agree on the major version.

The HCL provider name (`aws`) is derived from the last segment of `source:` — `hashicorp/aws` → `aws`, `hashicorp/google` → `google`, `datadog/datadog` → `datadog`.

## Path-variable substitution

String values inside `config:` can reference any of the six path variables:

`${cloud}`, `${profile}`, `${region}`, `${environment}`, `${class}`, `${component}`

They are substituted at generate time for the leaf being generated. The example above uses `${profile}` and `${region}` to make one `providers.yaml` work across every profile and region without duplication.

Only strings are substituted. Numeric and boolean values pass through unchanged.

## Multi-cloud in a single file

A single `providers.yaml` can declare multiple clouds. Twig emits one `required_providers` entry and one `provider {}` block per distinct cloud actually used by the leaf:

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

A leaf that mixes `aws/5/ec2` and `datadog/1/monitor` modules gets both `provider "aws" {}` and `provider "datadog" {}` blocks.

Note: the file lives at `infra/<cloud>/providers.yaml` where `<cloud>` matches the leaf's first path segment. If a leaf at `infra/aws/...` uses a `datadog/1/monitor` module, twig reads its `providers.yaml` from `infra/aws/providers.yaml` — not from `infra/datadog/`. Declare all providers a given `infra/<cloud>/` subtree needs in that subtree's `providers.yaml`.

## Provider aliases (cross-account / cross-region)

A leaf can declare additional aliased provider blocks — useful for
cross-region workloads (e.g. a VPN hub peering with spokes across regions
in the same account) or cross-account modules. Declare them at the leaf
level with `provider_aliases:`:

```yaml
# leaf.yaml
provider_aliases:
  aws:
    - {account: waldman, region: us-west-2}
    - {account: waldman, region: eu-north-1}
    - {account: marvelx, region: us-east-1}

modules:
  ...
```

Both `account` and `region` are required in every entry. The alias name
is auto-derived as `<account>_<region>` (dashes become underscores).
Given the example above and a leaf at `infra/aws/waldman/us-east-1/...`,
twig generates:

```hcl
provider "aws" {                    # default, from path
  profile = "waldman"
  region  = "us-east-1"
}

provider "aws" {
  alias   = "waldman_us_west_2"
  profile = "waldman"
  region  = "us-west-2"
}

provider "aws" {
  alias   = "waldman_eu_north_1"
  profile = "waldman"
  region  = "eu-north-1"
}

provider "aws" {
  alias   = "marvelx_us_east_1"
  profile = "marvelx"
  region  = "us-east-1"
}
```

Validation:

- Both `account` and `region` are required non-empty strings.
- The directory `infra/<cloud>/<account>/<region>/` must exist.
- An entry equal to the leaf's own `(profile, region)` is rejected —
  that is the default provider.
- Duplicate `(account, region)` tuples are rejected.
- The cloud must be declared in `providers.yaml`.

The alias block reuses the primary provider's `source:` and version
constraint. Its `config:` inherits from the template, then overrides
`profile` and `region` with the entry's literal values. Other config
keys (e.g. `default_tags`) are still substituted using the leaf's
own path variables.

Aliases are opt-in per leaf. Unreferenced alias blocks are lazy in
Terraform (no API calls until a resource references them), so declaring
aliases you don't yet consume costs only lines of HCL.

## See also

- [`specs/08_providers.md`](../specs/08_providers.md) — formal reference
