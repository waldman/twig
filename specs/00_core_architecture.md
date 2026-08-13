# TWIG - CORE ARCHITECTURE SPEC

Thin CLI that reads a named YAML file from a path-as-data directory tree,
generates a single `main.tf`, and delegates to Terraform.

One thing done well: turn a declarative module list into runnable Terraform
without templates, HCL authoring, or state management ceremony.

---

## Non-goals

- No templating (Jinja2, HCL templates, string interpolation beyond cross-refs)
- No module registry or version enforcement across modules in a leaf
- No plan file management (`-out` / saved plans)
- No state management beyond configuring the S3 backend in the generated file
- No wrapping of `terraform state`, `terraform import`, or other subcommands

---

## Path convention

```
infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>.yaml
```

twig accepts the leaf file as a positional argument. It walks up from that
file to locate the project root (identified by a `twig.yaml` config file).
Path variables are parsed from the relative path between the config file and
the leaf file.

| Source | Variable | Example |
|---|---|---|
| path segment 1 | `cloud` | `aws` |
| path segment 2 | `profile` | `waldman` |
| path segment 3 | `region` | `us-east-1` |
| path segment 4 | `environment` | `production` |
| path segment 5 | `class` | `services` |
| filename (no `.yaml`) | `component` | `ansible-anchor` |
| module instance key | `module` | `iam_cicd` |

All seven variables are injected into every module call. All seven are
protected — none may appear in `vars`.
