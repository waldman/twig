# twig

Thin CLI that reads a named YAML file from a path-as-data directory tree, generates a single `main.tf`, and delegates to Terraform.

One thing done well: turn a declarative module list into runnable Terraform without templates, HCL authoring, or state management ceremony.

## Install

Download the latest release for your platform from the [releases page](https://github.com/waldman/twig/releases), extract, and place `twig` in your `$PATH`.

You also need [Terraform](https://developer.hashicorp.com/terraform/install) in `$PATH` and cloud credentials configured for whichever provider(s) your modules use.

## Quickstart

Project layout:

```
my-infra/
  twig.yaml
  infra/
    aws/
      providers.yaml
      waldman/
        us-east-1/
          production/
            services/
              app.yaml
```

`twig.yaml`:

```yaml
modules_path: ../terraform-modules/modules

backend:
  bucket: waldman-terraform-state
  region: us-east-1
```

`infra/aws/providers.yaml`:

```yaml
aws:
  source: hashicorp/aws
  config:
    profile: "${profile}"
    region:  "${region}"
```

`infra/aws/waldman/us-east-1/production/services/app.yaml`:

```yaml
modules:
  bucket:
    source: aws/5/s3-bucket
    vars:
      s3_bucket_name: my-app-assets
```

Preview the generated Terraform without running it:

```bash
twig show infra/aws/waldman/us-east-1/production/services/app.yaml
```

Run it:

```bash
twig apply infra/aws/waldman/us-east-1/production/services/app.yaml
```

## Where to go from here

- [docs/bootstrap.md](docs/bootstrap.md) — one-time state backend setup: S3 bucket + DynamoDB lock table
- [docs/concepts.md](docs/concepts.md) — the mental model: path-as-data, path variables, reference namespaces, inheritance
- [docs/project-config.md](docs/project-config.md) — `twig.yaml` reference (backend, local + git modules, `TWIG_MODULES_PATH`)
- [docs/providers.md](docs/providers.md) — `providers.yaml` (per-cloud, multi-cloud, path-var substitution)
- [docs/leaves.md](docs/leaves.md) — leaf file format, cross-refs, `remote_state`, worked examples
- [docs/vars-yaml.md](docs/vars-yaml.md) — share config across leaves: `vars:`, `remote_state:`, `module_defaults:`, lazy emission, provenance
- [docs/cli.md](docs/cli.md) — commands, pass-through flags, cache directory
- [specs/](specs/) — formal specification, one file per concern

## Example project

See [`examples/`](examples/) for a working project layout.

## License

MIT — see [LICENSE](LICENSE).
