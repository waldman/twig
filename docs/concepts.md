# Concepts

The mental model. Read once — the rest of the docs assume it.

## Path is data

Your infrastructure lives in a fixed directory shape:

```
<project-root>/
  twig.yaml
  infra/<cloud>/<profile>/<region>/<environment>/<class>/<component>.yaml
```

Each `.yaml` file at the deepest level (called a **leaf**) declares one deployable component: which Terraform modules to call, what to pass them, and which other leaves' outputs to consume.

Twig does not template. It reads the leaf, reads the path segments as data, and writes a single `main.tf` into a cache directory. Terraform runs there.

## Seven path variables

Every module call twig generates receives seven variables. Six come from the file path; the seventh is the module's instance key inside the leaf.

| Variable | Source | Example |
|---|---|---|
| `cloud` | path segment 1 | `aws` |
| `profile` | path segment 2 | `waldman` |
| `region` | path segment 3 | `us-east-1` |
| `environment` | path segment 4 | `production` |
| `class` | path segment 5 | `services` |
| `component` | filename (no `.yaml`) | `ansible-anchor` |
| `module` | instance key in `modules:` | `iam_cicd` |

All seven are reserved — they may not be declared in a leaf's `vars:` block, and the ref namespace names (`module`, `remote`, `vars`) may not be used as module instance keys or `remote_state` aliases.

Modules that follow the convention take all seven as required input variables. Twig injects them automatically; you never type them in a leaf.

## Three reference namespaces

Values in a leaf's module vars can contain three kinds of references. The namespace prefix is required — unqualified `${x.y}` is emitted as a literal string, not a reference.

| Reference | Resolves to | Where it points |
|---|---|---|
| `${module.<instance>.<output>}` | `module.<instance>.<output>` | Another module in the same leaf |
| `${remote.<alias>.<output>}` | `data.terraform_remote_state.<alias>.outputs.<output>` | Another leaf's state |
| `${vars.<name>}` | inlined value from inherited `vars:` | A `vars.yaml` up the hierarchy |

Pure references (`value: ${module.x.y}`) become unquoted HCL expressions with the correct type. Embedded references (`value: "prefix-${module.x.y}-suffix"`) become interpolated strings.

## The inheritance model

A `vars.yaml` file may live at any level of the `infra/` tree — at the root, per cloud, per profile, per region, per environment, per class. Any leaf inherits every `vars.yaml` above it in the tree.

`vars.yaml` files carry three optional top-level sections:

- **`vars:`** — key/value store, referenced from leaves via `${vars.<name>}`. Not auto-injected into modules; reference-only.
- **`remote_state:`** — alias → leaf path. Extends the leaf's own `remote_state:` block so leaves don't have to redeclare shared upstream dependencies.
- **`module_defaults:`** — default vars scoped to a specific module source (e.g. `aws/5/vpc`). Injected into every module instance matching that source; leaf `vars:` overrides per key.

Merge rule everywhere: closer to the leaf wins per key. Map values are replaced wholesale — no deep merging inside a map value.

## Lazy emission and provenance

Two properties of the generated `main.tf` that flow from the model above:

- **Lazy emission of remote state:** `data "terraform_remote_state" "<alias>"` blocks appear in the generated output only for aliases actually referenced by some resolved module var. Aliases declared in a shared `vars.yaml` but not used by this particular leaf produce nothing.

- **Provenance comments:** every argument in a generated module block carries a trailing `# from: <origin>` comment identifying where that key was declared — the path, the leaf, or a specific `vars.yaml` file. Read `main.tf` and you can trace any value in one step without walking the hierarchy manually.

## What twig is not

- Not a templating engine — no Jinja, no HCL string interpolation beyond the three ref namespaces.
- Not a module registry — module sources are paths (local or git URL) resolved at Terraform init time.
- Not a state manager — beyond writing the S3 backend block into the generated file.
- Not a wrapper for `terraform state`, `terraform import`, or plan-file management.

See [specs/00_core_architecture.md](../specs/00_core_architecture.md) for the formal non-goals.

## Where to go next

- **Set up a project** → [project-config.md](project-config.md), [providers.md](providers.md)
- **Write your first leaf** → [leaves.md](leaves.md)
- **Share config across leaves** → [vars-yaml.md](vars-yaml.md)
- **Run twig** → [cli.md](cli.md)
- **Formal reference** → [`specs/`](../specs/)
