# terraform-provider-omada

A Terraform provider for the **TP-Link Omada** controller (v6 — OC200/OC300
hardware or Software Controller), managing network config as infrastructure-as-code.

It talks to the controller's own web API (`/{omadacId}/api/v2/…`) — the same API
the Omada UI uses. TP-Link publishes no documentation for it; endpoints and payload
shapes are derived from the UI. This is deliberate: it's the only surface with full
config coverage, including gateway/router settings that other providers omit.

> **Status: in progress.** Five resources with verified CRUD (see the table
> below) plus two data sources. More resources are being added.

## Resources & data sources

| Resource | CRUD contract |
|---|---|
| `omada_network` | import / read / update / delete verified live ✅ · **create: see limitation** |
| `omada_lan_dns` | full CRUD verified live ✅ |
| `omada_port_forward` | full CRUD verified live ✅ |
| `omada_ip_group` | full CRUD verified live ✅ |
| `omada_firewall_acl` | full CRUD verified live ✅ |
| `omada_wlan_group` | full CRUD verified live ✅ |
| `omada_mdns_reflector` | full CRUD verified live ✅ |
| `omada_port_profile` | full CRUD; managed field subset (rest preserved via read-modify-write) |
| `omada_wireless_network` | SSID; managed field subset; `psk` is write-only |
| `omada_vpn` | manages `name`/`enable` only; **write verbs inferred, not live-validated** |
| `omada_site_settings` | singleton; manages the device-LED toggle (subset) |
| data sources `omada_sites`, `omada_networks` | ✅ |

Every resource has mock-backed acceptance tests (create → import → update) that run
in CI. Resources marked "verified live" had their exact endpoint + verbs confirmed
against a real v6.2 controller with throwaway objects (created and deleted).

## Known limitations

- **Creating a brand-new network is not yet supported.** The controller's web UI
  creates networks through the official Omada **OpenAPI**
  (`/openapi/v1/.../networks/confirm`), which needs client-credentials auth (a
  separate token flow) — the `/api/v2` endpoint this provider uses rejects the
  create (it demands write-only fields like `proto`). **Importing, reading,
  updating and deleting** existing networks all work. Full create support needs
  the OpenAPI auth flow added (register an Open API app under *Controller →
  Settings → Platform Integration → Open API*).
- Firewall ACL `customAclPorts` / `customAclDevices` are sent empty (not yet
  modelled).
- `omada_vpn` manages only `name`/`enable` and its write verbs are **inferred**
  (the read shape is live-verified, but create/update/delete were not exercised on
  hardware). Prefer importing an existing VPN and toggling `enable`.
- `omada_port_profile`, `omada_wireless_network` and `omada_site_settings` manage a
  practical subset of fields; unmanaged fields are preserved on update
  (read-modify-write).

## Usage

```hcl
terraform {
  required_providers {
    omada = { source = "wncservices/omada" }
  }
}

provider "omada" {
  url      = "https://10.0.0.2:443" # or OMADA_URL
  username = var.omada_username     # or OMADA_USERNAME
  password = var.omada_password     # or OMADA_PASSWORD
  # skip_tls_verify defaults to true (self-signed controller cert)
  # site           defaults to the controller's primary site
}

data "omada_sites" "all" {}
```

## Local development

### Prerequisites

Toolchain versions are pinned in [`.tool-versions`](.tool-versions). With
[asdf](https://asdf-vm.com):

```sh
asdf install          # Go 1.26.5, Terraform 1.15.x, golangci-lint 2.12.2
```

…or install those manually. Then, once per clone:

```sh
go mod download       # fetch deps (go.sum is committed)
make tools            # install tfplugindocs + golangci-lint into GOPATH/bin
```

### Everyday commands

```sh
make build            # compile ./terraform-provider-omada
make test             # unit tests — no controller needed
make testacc          # acceptance tests (see below)
make lint             # golangci-lint
make fmt              # gofmt -s -w
make docs             # regenerate docs/ from schema + examples/
```

### Running it against a real controller (`dev_overrides`)

Before the provider is published you can't `terraform init` it, so tell the
Terraform CLI to use your local build directly. Add this to `~/.terraformrc`
(create it if absent):

```hcl
provider_installation {
  dev_overrides {
    # Point at the directory holding the built binary, e.g. `go env GOPATH`/bin.
    "wncservices/omada" = "/Users/you/go/bin"
  }
  # For everything else, use the normal registry.
  direct {}
}
```

Then:

```sh
make install                       # go install -> GOPATH/bin
export OMADA_URL="https://10.0.0.2:443"
export OMADA_USERNAME="tf-admin"
export OMADA_PASSWORD="…"

cd examples/data-sources/omada_sites
terraform plan                     # NB: no `terraform init` under dev_overrides
```

Terraform prints a warning that dev overrides are in effect — that's expected.

### Tests

- **Unit tests** (`make test`) — fast, exercise the client against an `httptest`
  mock. No controller needed.
- **Acceptance tests** (`make testacc`, `TF_ACC=1`) — drive the provider through a
  real Terraform binary against an in-process **mock controller**, so they run
  offline and in CI with no hardware or secrets.

Both run in CI on every PR (see [`.github/workflows/test.yml`](.github/workflows/test.yml)).
Real-controller validation is done manually via the `dev_overrides` flow above.

## How endpoints are reverse-engineered

Because the web API is undocumented, new resources are built by capturing what the
UI does:

1. Open the Omada UI with browser devtools, perform the action (e.g. add a
   port-forward), and record the request path, method, and JSON body.
2. Confirm by replaying it through the client.
3. Freeze a representative response as `internal/omada/testdata/*.json` and add a
   unit test.

> ⚠️ Some struct field mappings in `internal/omada/` (notably `networks.go`) are
> provisional and must be confirmed against a live controller during Phase 1.

## References

- HashiCorp [terraform-plugin-framework](https://developer.hashicorp.com/terraform/plugin/framework)
- `emanuelbesliu/terraform-provider-tplink-omada` — v6 client handshake reference
- `dougbw/go-omada`, `MarkGodwin/tplink-omada-api` (Python) — endpoint references

## License

[MPL-2.0](LICENSE)
