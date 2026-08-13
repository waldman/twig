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

## See also

- [`specs/08_providers.md`](../specs/08_providers.md) — formal reference
