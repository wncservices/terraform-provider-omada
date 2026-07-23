# terraform-provider-omada

## Overview

A Terraform provider for the **TP-Link Omada** controller (v6 — OC200/OC300 or
Software Controller), managing network/router config as IaC. It drives the
controller's reverse-engineered web API (`/{omadacId}/api/v2/…`) — the same API
the Omada UI uses — which is the only surface with full config coverage, including
the gateway settings other providers omit. Consumed by the homelab `lab/omada/`
Terraform config; published to the public Terraform Registry as `wncservices/omada`.

For the architecture, the coverage matrix, the "add a resource" recipe, and the
prioritised list of what still needs building, read [`DESIGN.md`](DESIGN.md) — the
single reference for contributors and agents.

## Technology Stack

- **Language:** Go 1.26.5 (`terraform-plugin-framework`, not SDKv2)
- **Terraform:** 1.15.x for local/acceptance runs (min protocol 6.0)
- **Docker:** no — this is a Go module; tooling is local
- **Lint:** golangci-lint v2 (`.golangci.yml`)
- **Docs:** `tfplugindocs` (generated into `docs/`, never hand-edited)
- **Release:** GoReleaser + GPG-signed archives → GitHub Releases → Registry
- **Tests:** Go `testing` + `terraform-plugin-testing` acceptance harness

## Quick Start (local development)

Pin toolchain versions with [asdf](https://asdf-vm.com) (`.tool-versions`), or
install Go 1.26.5 + Terraform 1.15.x manually.

```sh
asdf install          # installs Go, Terraform, golangci-lint per .tool-versions
make tools            # installs tfplugindocs + golangci-lint into GOPATH/bin
make build            # compile the provider binary
make test             # unit tests (no controller needed)
```

### Trying it against a real controller before publishing

Point Terraform at your local build via `dev_overrides` (see README → Local
development). Then set `OMADA_URL` / `OMADA_USERNAME` / `OMADA_PASSWORD` and run
`terraform plan` in a scratch config — no `terraform init` under dev_overrides.

### Cloud agent notes

- Don't assume global tools exist; run `make tools` / `asdf install` first.
- Never hardcode controller credentials or the GPG key to "fix" a missing secret —
  they come from env vars (`OMADA_*`) and CI secrets.

## Commands

```sh
make build     # go build -> ./terraform-provider-omada
make test      # unit tests
make testacc   # acceptance tests (TF_ACC=1). Uses the in-test mock controller;
               # set OMADA_* to point at a real controller instead.
make lint      # golangci-lint run
make docs      # regenerate docs/ from schema + examples/ (tfplugindocs)
make fmt       # gofmt -s -w
```

## Code Conventions (beyond linters)

- **Client vs provider split:** all HTTP/controller logic lives in
  `internal/omada/` (pure Go, no Terraform types). `internal/provider/` only maps
  between the framework schema and the client. Never import `terraform-plugin-*`
  into `internal/omada/`.
- **One file per resource/data source** in `internal/provider/`, each with a
  matching `_test.go` acceptance test that runs against the mock controller.
- **Every new endpoint gets mock coverage + an acceptance test.** Reverse-engineer
  payloads from the UI (browser devtools), add handlers to the stateful mock
  controller in `internal/provider/provider_test.go` (`newMockController`), and
  drive a create → import → update acceptance test against it. Testing is
  mock-based, not fixture-file based — there is no `internal/omada/testdata/`.
- **Docs are generated.** Edit `examples/` and schema `MarkdownDescription`s, then
  `make docs`; never hand-edit `docs/`. CI fails if `docs/` is stale.
- Struct field mappings flagged "provisional" in `internal/omada/` must be
  confirmed against a live controller before they're relied on.

## CRITICAL Security Rules

- **DON'T** commit controller credentials, `.env*` (except `.env.example`), or any
  `*.tfstate` / `*.asc` GPG key material. `.gitignore` covers these; don't defeat it.
- **DON'T** log secrets — controller passwords, the CSRF token, or auth cookies.
- `skip_tls_verify` defaults to `true` because controllers ship self-signed certs;
  that is an intentional, documented default — don't silently change it.
