# Contributing to terraform-provider-omada

Thanks for considering a contribution. This document covers the mechanics of
sending a change; for the architecture, the coverage matrix, and the full
"add a resource" recipe, see [`DESIGN.md`](DESIGN.md) — read it first if
you're adding or changing a resource, it's the single reference for that.

## Before you start

- **Bug fix or small change:** open a PR directly.
- **New resource, data source, or non-trivial change:** open an issue first
  (or check [`DESIGN.md`](DESIGN.md) §3/§5 — the gap may already be scoped,
  or blocked on something like missing test hardware). This avoids duplicate
  work and lets us agree on the shape of the change before you write it.

## Development setup

Toolchain versions are pinned in [`.tool-versions`](.tool-versions); with
[asdf](https://asdf-vm.com), `asdf install` gets Go, Terraform, and
golangci-lint. Then, once per clone:

```sh
go mod download
make tools    # installs tfplugindocs + golangci-lint into GOPATH/bin
```

```sh
make build    # compile the provider binary
make test     # unit tests
make testacc  # acceptance tests (TF_ACC=1)
make lint     # golangci-lint
make docs     # regenerate docs/ from schema + examples/
make fmt      # gofmt -s -w
```

**Neither test suite needs a real controller.** Unit tests exercise the
client against an `httptest` mock; acceptance tests drive a real Terraform
binary against an in-process mock controller
(`internal/provider/provider_test.go`). Both run in CI on every PR, offline
and without secrets.

To try a local build against a real controller (only needed to discover an
endpoint or confirm a verb), see README → Local development, or
[`DESIGN.md` §4.1](DESIGN.md#41-running-against-a-real-controller).

## Code organization

- `internal/omada/` — all HTTP/controller client logic. Pure Go, no
  Terraform types. Never import `terraform-plugin-*` here.
- `internal/provider/` — maps between the Terraform schema and the client.
  One file per resource/data source, each with a matching `_test.go`
  acceptance test run against the mock controller.
- `examples/` and schema `MarkdownDescription`s are the source for `docs/`.
  Edit those, run `make docs`, and never hand-edit `docs/` — CI fails on a
  stale diff.

## Adding or changing a resource

The short version (full recipe in [`DESIGN.md` §4.2](DESIGN.md#42-the-recipe)):

1. Capture the real request from the Omada UI (browser devtools) — method,
   path, JSON body.
2. Confirm the verbs against a real controller with a disposable object
   (create → update → delete), and note the exact payload shape.
3. Add the client layer in `internal/omada/<domain>.go`.
4. Add the provider layer in `internal/provider/<name>_resource.go`, mirroring
   an existing resource.
5. Register it in `internal/provider/provider.go`.
6. Add handlers to the mock controller and a `<name>_resource_test.go` that
   does create → import → update.
7. Add an example under `examples/resources/omada_<name>/`, write the schema
   descriptions, and run `make docs`.

**Every new endpoint needs mock coverage and an acceptance test.** The mock
in `newMockController` isn't a stub that says yes — it reproduces the
controller's real quirks (wrong-verb errors, read-only keys, etc). When you
discover a controller quirk, teach the mock about it so the next regression
fails in CI instead of on someone's network.

## Commit and PR guidelines

- Keep PRs focused — one resource, fix, or piece of coverage per PR.
- Write commit messages that explain *why*, not just what changed.
- Fill in the PR template; the checklist mirrors the gate below.
- Before opening a PR, make sure this all passes locally:

  ```sh
  make build && make test && TF_ACC=1 make testacc && make lint && make docs
  ```

  CI runs the same checks (build, unit tests, acceptance tests, lint,
  `gofmt`, docs freshness, CodeQL, `govulncheck`) plus a documentation-drift
  check — a stale `docs/` fails the build.

## Security

- Never commit controller credentials, `.env*` (other than `.env.example`),
  or `*.tfstate` / `*.asc` key material. `.gitignore` covers these — don't
  defeat it.
- Never log secrets: controller passwords, the CSRF token, or auth cookies.
- If you find a security issue, please **do not open a public issue** —
  report it privately via
  [GitHub Security Advisories](https://github.com/wncservices/terraform-provider-omada/security/advisories/new).

## Questions

Open an issue — there's no mailing list or chat for this project.
