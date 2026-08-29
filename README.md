# terraform-provider-omada

Manage a **TP-Link Omada** controller with Terraform: networks and VLANs, DHCP,
firewall rules, NAT and port forwarding, WiFi, switch ports, and gateway and site
settings.

Targets Omada controller **v6** — OC200/OC300 hardware or the Software
Controller. It talks to a controller on your own network; no TP-Link cloud
account is involved.

```hcl
terraform {
  required_providers {
    omada = {
      source  = "wncservices/omada"
      version = "~> 0.10"
    }
  }
}

provider "omada" {
  url = "https://omada.example.com"
  # username / password from OMADA_USERNAME and OMADA_PASSWORD
}

resource "omada_network" "iot" {
  name           = "IOT"
  vlan_id        = 30
  gateway_subnet = "192.0.2.1/24"
  interface_ids  = ["2_e5f6a7b8c9d04e1fa2b3c4d5e6f70819"]

  dhcp_enabled = true
  dhcp_start   = "192.0.2.100"
  dhcp_end     = "192.0.2.250"

  isolation = true
}
```

Per-resource reference documentation lives in the
[Terraform Registry](https://registry.terraform.io/providers/wncservices/omada/latest/docs)
and in [`docs/`](docs/).

## Requirements

| | |
|---|---|
| Terraform | **≥ 1.11** — required for write-only attributes; see [Secrets](#secrets) |
| Omada controller | v6.x (validated against v6.2) |

## Authentication

The provider signs in with a controller admin account:

```sh
export OMADA_URL="https://omada.example.com"
export OMADA_USERNAME="terraform"
export OMADA_PASSWORD="…"
```

Creating a dedicated admin account is worth the two minutes: it keeps the
controller's audit log readable, and it lets you revoke Terraform's access
without disturbing anyone else's.

Controllers usually serve a self-signed certificate, so `skip_tls_verify`
defaults to **`true`**. That is a pragmatic default rather than a good one — it
means the connection is encrypted but not authenticated. If you can install the
controller's certificate on the machine running Terraform, set it to `false`.

### Open API credentials (needed by a few resources)

Some capabilities are served only by TP-Link's separate **Open API**, which the
controller authenticates independently of the admin login:

| Needs Open API credentials | Notes |
|---|---|
| `omada_network` **create** | import, read, update and delete work without them |
| `omada_switch_port` | the web API has no writable per-port route |
| `omada_iot_radio` | served *only* on the Open API, so even a refresh needs them |

Register an application under *Settings → Platform Integration → Open API* on the
controller, then:

```hcl
provider "omada" {
  openapi_client_id     = var.omada_openapi_client_id
  openapi_client_secret = var.omada_openapi_client_secret
}
```

or `OMADA_OPENAPI_CLIENT_ID` / `OMADA_OPENAPI_CLIENT_SECRET`. The admin username
and password do **not** grant Open API access — the controller refuses a web
session there with `-44116`. Everything else works without them.

## Adopting an existing controller

Most controllers are already configured before Terraform arrives, so every
resource supports import. The approach that keeps this safe: declare the
resource with the values the controller currently holds, add an `import` block,
and confirm the plan proposes **nothing**. Make changes afterwards, as their own
reviewable step.

```hcl
import {
  to = omada_network.iot
  id = "Default/6a64c365bb62a10bd62c3e08"
}
```

Each resource documents its import id form. Where a bare id is accepted, prefer
the `<site>/<id>` form anyway — the site is part of a resource's identity, and
omitting it leaves that attribute unset in the imported state.

## Secrets

Every secret this provider accepts — the WiFi `psk`, the captive-portal
`password`, a RADIUS `shared_secret`, the SNMP community string and v3 password,
and the IoT radio `passcode` — is a Terraform **write-only** attribute. Values
are sent to the controller on apply and never written to state or plan files.

That matters more here than it might elsewhere: the controller returns WiFi keys
and RADIUS secrets in **plaintext** on read, so a provider that stored what it
read would copy them into every state file and remote backend that touches this
configuration.

`Sensitive = true` would not be enough on its own — Terraform persists
*configured* values regardless of what a provider reads back. Only write-only
attributes keep them out, which is why Terraform ≥ 1.11 is required.

One consequence worth knowing: because these values are never read back, a
secret changed outside Terraform will not show up as drift.

## Limitations

What this provider deliberately does not do, or cannot yet:

- **WAN settings are mostly read-only** (the `omada_wan` data source). That
  document mixes configuration with read-only capability flags, its write
  verbs are undocumented, and it is the one object that cannot be validated
  with a throwaway — the only WAN is the live one, and a bad write disconnects
  the site. Read it here; change it in the controller UI. The one exception is
  `omada_wan_ipv6` — a narrowly-scoped write path for a WAN port's IPv6
  connection settings, whose write verb is inferred rather than validated
  live; see its own docs before using it.
- **A gateway's physical ports are not managed** by `omada_gateway`, for the same
  reason. Use the scoped resources — port forwarding, disable-NAT, firewall —
  which change one thing at a time and are legible in a plan.
- **One-to-one NAT** is not managed. Its field set is known, but the controller
  requires a static-IP WAN (`-34282`), which the validation environment does not
  have, so the write path has never been exercised.
- **Captive-portal page design** — logo, colours, terms, background image — is
  preserved on update but not manageable. A background image also needs a
  multipart upload the provider does not implement.
- **Some resources model a practical subset** of very large objects
  (`omada_wireless_network`, `omada_port_profile`, `omada_site_settings`).
  Unmodelled fields are preserved on update, never blanked.
- `omada_vpn` manages only `name` and `enable`, and its write verbs are
  **inferred** rather than confirmed against hardware. Prefer importing an
  existing VPN and toggling `enable`.
- `omada_iot_beacon` create and delete are **unverified** — they need an access
  point with a BLE radio, which the validation environment does not have.

A complete audit — every configuration endpoint found on a controller, and
whether it is managed, blocked, or out of scope — is in
[`DESIGN.md`](DESIGN.md#5-whats-left).

## How this provider talks to the controller

It uses the controller's own web API (`/{omadacId}/api/v2/…`) — the same one the
Omada UI uses — plus the Open API where that is the only surface offering a
capability.

TP-Link publishes no documentation for the web API. Endpoints and payload shapes
are derived from the UI and confirmed against a real controller. That is
deliberate: it is the only surface with full configuration coverage, including
the gateway and firewall settings that other providers omit.

The practical consequence is that behaviour here is established empirically
rather than from a specification. Resources are validated against a live v6.2
controller in a dedicated test environment, and **anything that could not be
exercised there is called out** — in [Limitations](#limitations) above, and in
the affected resource's own documentation. Where the provider says a write verb
is "inferred", it means exactly that.

## Contributing

[`DESIGN.md`](DESIGN.md) is the contributor's guide: the architecture and the
patterns worth copying, how to build and test locally, how undocumented
endpoints are reverse-engineered without breaking live equipment, and a
prioritised list of what still needs building.

## License

MPL-2.0. See [`LICENSE`](LICENSE).
