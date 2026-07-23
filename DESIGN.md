# Design & contribution guide

This document is the map for contributors — human or agent — working on
`terraform-provider-omada`. It explains **how the provider is built**, **what is
already covered**, and **what still needs doing**, with enough per-item detail to
pick something up and ship it without reverse-engineering the whole repo first.

For toolchain/commands and the security rules, see [`AGENTS.md`](AGENTS.md). For
the user-facing feature list, see [`README.md`](README.md). This file is the
"why" and the "what next".

---

## 1. What this provider is

Terraform provider for the **TP-Link Omada** controller (v6 — OC200/OC300 or
Software Controller). It drives the controller's **reverse-engineered web API**
(`/{omadacId}/api/v2/…`) — the same API the Omada web UI calls. TP-Link publishes
no documentation for it; every endpoint and payload shape here was learned from
the UI and confirmed against a live controller.

We use the web API rather than the official Omada **OpenAPI** because the web API
is the only surface with full config coverage, including gateway/router settings.
The one place that hurts us is network *creation* — see §5.1.

---

## 2. Architecture

### 2.1 Two layers, one rule

```
internal/omada/       ← the client: pure Go, HTTP + JSON, zero Terraform types
internal/provider/    ← the provider: framework schema <-> client, per resource
```

**The rule: never import `terraform-plugin-*` into `internal/omada/`.** The client
is a plain Go SDK that could stand alone; the provider layer is the only place that
knows about `types.String`, diagnostics, schema, etc. This keeps the client
unit-testable and the mapping logic obvious.

Each resource is one file in `internal/provider/<name>_resource.go` with a matching
`<name>_resource_test.go`, and its controller calls live in one
`internal/omada/<domain>.go`.

### 2.2 The controller handshake (`internal/omada/auth.go`, `client.go`)

1. `GET /api/info` → `omadacId`, `controllerVer`, `apiVer`.
2. `POST /{omadacId}/api/v2/login` with username/password → a **`token`**.
3. Every subsequent call sends that token as the **`Csrf-Token`** header **and**
   carries the session cookie (the client holds a cookie jar).
4. All responses use the envelope `{ errorCode, msg, result }`. `errorCode == 0`
   is success; anything else becomes an `APIError` (`client.go`).
5. On an expired-session error code the client **re-logs in once and retries**
   (`isSessionExpired` + the `retry` arg in `do`). Callers never handle this.
6. `skip_tls_verify` (default **true**) installs a permissive TLS transport —
   controllers ship self-signed certs. This default is intentional; don't change it.

### 2.3 Sites (`internal/omada/sites.go`)

Everything is site-scoped. `ResolveSiteID(ctx, name)` maps a site *name* to its ID,
and an **empty name resolves to the controller's primary site** — real sites are
often named `Home`, not `Default`, so we never hard-code a name. Resources cache
the resolved `site_id` in state and accept both `<id>` and `<site>/<id>` on import.

### 2.4 Read-modify-write (the most important pattern)

Most controller objects carry dozens of fields the provider doesn't model
(derived values, capability flags, sub-objects the UI manages). A naive `PATCH`
with only our fields would **blank the rest**. So updates do read-modify-write:

- `RawByID(ctx, listPath, idKey, id)` fetches the current object as a
  `map[string]any` (`client.go`).
- `mergeInto(cur, fields, deepKeys...)` overlays our fields onto it, **deep-merging**
  the named sub-objects instead of replacing them (`portprofile.go:141`).
- The merged map is sent back.

This is why, e.g., a port-profile update preserves the STP `instances` list and
`prohibitModify`, and an SSID update preserves the existing `pskSetting.securityKey`.
When you add fields to an existing resource, decide whether each new nested object
needs to be in that resource's `deepKeys` list.

### 2.5 Invariants you must not break

These are enforced by tests and/or matter for safety. Read before touching the
relevant resource.

- **`psk` is write-only.** The WiFi pre-shared key is never read into state and
  never written to any file. (The controller's SSID list endpoint returns keys in
  **plaintext** — that is exactly why we refuse to store them.) Updates deep-merge
  `pskSetting` so the key survives an update that doesn't set a new one.
- **`deviceAccount` is never sent.** Site-settings updates must never include the
  device-credential object. A mock test asserts it survives untouched.
- **Null is not false.** Some controller fields come back `null`; writing `false`
  over a null is a *change*, not a no-op, and shows up as an unwanted diff. Model
  such fields as `Optional` + `Computed` and leave them unset unless the user sets
  them. (Seen on port-profile `dhcpL2RelaySettings` and several SSID toggles.)
- **Verbs vary per endpoint.** Some items update with `PATCH`, some with `PUT`;
  some delete at `/{id}`, some at `/{type}/{id}`. Static routes reject `PATCH`
  outright (`-1600`). Never assume — confirm against the live controller.

---

## 3. Coverage matrix

Legend: **live** = exact endpoint + verbs confirmed against a real v6.2 controller
with a throwaway object; **mock** = has acceptance-test coverage against the
in-process mock; **subset** = a practical field subset is modelled, the rest
preserved via read-modify-write.

| Resource / data source | CRUD | Verified | Notes |
|---|---|---|---|
| `omada_network` | I/R/U/D | live | **create unsupported** — see §5.1 |
| `omada_lan_dns` | CRUD | live | |
| `omada_port_forward` | CRUD | live | |
| `omada_ip_group` | CRUD | live | delete path is `/groups/{type}/{id}` |
| `omada_firewall_acl` | CRUD | live | ACL type auto-discovered on import; custom ports/devices sent empty (§5.3) |
| `omada_wlan_group` | CRUD | live | |
| `omada_mdns_reflector` | CRUD | live | |
| `omada_port_profile` | CRUD | live · subset | STP block deep-merged |
| `omada_wireless_network` | CRUD | live · subset | `psk` write-only |
| `omada_static_route` | CRUD | live | update is `PUT` (`PATCH` → `-1600`) |
| `omada_vpn` | CRUD | **read live, writes inferred** | see §5.2 |
| `omada_site_settings` | R/U (singleton) | live · subset | ~45 fields; large object |
| `omada_sites` (data) | R | live | |
| `omada_networks` (data) | R | live | |
| `omada_wan` (data) | R | live | **read-only by design** — see §5.4 |
| `omada_port_forwards` (data) | R | mock | discovery — list rules + IDs |
| `omada_firewall_acls` (data) | R | mock | discovery — lists all ACL types |
| `omada_devices` (data) | R | live | inventory — gateways/switches/APs |

---

## 4. Adding a resource — the recipe

This is the exact loop the existing resources were built with. An agent can follow
it end to end.

1. **Capture the API.** In the Omada UI, perform the action (add the object, change
   a field) with browser devtools open. Record the request path, method, and JSON
   body. Where the UI builds the path dynamically, mine the web-app JS bundle.
2. **Confirm with a throwaway.** Replay create → update → delete against a **real
   controller** using a disposable object, and note the exact verbs and the delete
   path shape. Leave nothing behind. This is what "verified live" means.
3. **Client layer** (`internal/omada/<domain>.go`): a typed struct for the fields
   you model, plus `List` / `Get` / `Create` / `Update` / `Delete`. Use `RawByID`
   + `mergeInto` for updates if the object has fields you don't model (almost all
   do). Follow an existing file — `staticroute.go` is a small clean example,
   `wireless.go` shows deep-merge + a write-only field.
4. **Provider layer** (`internal/provider/<name>_resource.go`): the schema, the
   model struct, and `Create/Read/Update/Delete/ImportState`. Mirror an existing
   resource; keep the site-resolution and import boilerplate identical.
5. **Register** it in `internal/provider/provider.go` (`Resources` or
   `DataSources`).
6. **Mock + test.** Add handlers to the in-process mock in
   `internal/provider/provider_test.go` (`newMockController`) and a
   `<name>_resource_test.go` that does create → import (`ImportStateVerify`) →
   update. Assert that any unmodelled keys you rely on survive the update (see the
   port-profile test's `checkPreserved`).
7. **Example + docs.** Add `examples/resources/omada_<name>/resource.tf` (and
   `import.sh` if it imports), write good `MarkdownDescription`s in the schema, then
   `make docs`. **Never hand-edit `docs/`** — CI fails on a stale diff.
8. **Gate.** `make build && make test && TF_ACC=1 make testacc && make lint && make docs`
   all clean.

### Testing model — note

Testing is **mock-controller based**, not fixture-file based. `newMockController`
in `provider_test.go` is a stateful `httptest` server that emulates the handshake
and each endpoint; acceptance tests (`TF_ACC=1`) drive a real Terraform binary
against it, so CI needs no hardware or secrets. There is **no `internal/omada/testdata/`
fixture directory** despite what older notes imply. Live-controller validation is
done by hand via the `dev_overrides` flow (README → Local development).

---

## 5. What's missing — the roadmap

Ordered roughly by value. Each item has enough detail to start. "Good first
contribution" items are marked 🟢.

### 5.1 Network **create** (the big one)

**Status:** import/read/update/delete work; create does not.
**Why:** the UI creates networks through the official Omada **OpenAPI**
(`/openapi/v1/{omadacId}/sites/{siteId}/networks` → `…/confirm`), which needs
**client-credentials auth** — a separate OAuth-style token flow, distinct from the
web-API session handshake. The `/api/v2` create endpoint rejects the call (it
demands write-only fields like `proto`).
**To implement:**
- Add an OpenAPI auth path to the client (register an Open API app under *Controller
  → Settings → Platform Integration → Open API*, exchange client id/secret for a
  bearer token, refresh on expiry).
- Wire `omada_network`'s `Create` to the OpenAPI create+confirm; keep read/update/
  delete on `/api/v2` (they work).
- Gate it so existing import-only users are unaffected.
This is the single most-requested capability and the main reason the provider
exists. Largest task on the list.

### 5.2 VPN write verbs (`omada_vpn`)

**Status:** the read shape is live-verified, but create/update/delete were
**never exercised on hardware** (the homelab had its only VPN removed). The verbs
in `vpn.go` are inferred.
**To implement:** on a controller with a VPN configured, run a throwaway
create/update/delete for each VPN type the controller supports (IPsec / WireGuard;
OpenVPN is gone in v6.2), confirm the verbs and payloads, then flip the README/matrix
note to "live" and widen the modelled field set beyond `name`/`enable`.

### 5.3 Firewall ACL custom ports/devices 🟢

**Status:** `customAclPorts` / `customAclDevices` are sent **empty** — the ACL
works but you can't express port- or device-scoped rules through Terraform.
**To implement:** model both as nested lists on `omada_firewall_acl`, capture their
payload shape from the UI, and add them to the client struct + mock + test. Small,
self-contained, no new auth. Good starter task.

### 5.4 Writable WAN (`omada_wan`) — deliberately deferred

**Status:** read-only data source, on purpose.
**Why deferred:** `/setting/wan/networks` is one large document mixing config with
read-only `support*` capability flags, and its write verbs are undocumented. Unlike
every other endpoint, the write path **cannot be validated with a throwaway** — the
only WAN object is the live one, and a bad write drops the site's internet
(including the controller you'd fix it from).
**If someone takes it on:** do it against a controller you have console/out-of-band
access to, in a maintenance window; model a *narrow* writable subset (e.g. MTU,
VLAN tag) rather than the whole document; keep the data source as-is.

### 5.5 Device-level resources — `omada_device_switch`, `omada_device_ap` 🟢 (per field)

**Status:** read-only inventory shipped — the `omada_devices` data source over
`GET /api/v2/sites/{site}/devices` lists gateways/switches/APs (name, type, model,
mac, ip, firmware, uptime, client count, upgrade flag). Per-device *config*
(individual switch-port overrides, AP radio/power settings, per-device names) is
still not started.
**To implement:** add per-device config resources on top of the data source one
capability at a time. Each capability is a small task; the umbrella is large.

### 5.6 Smaller gaps 🟢

- **SSID sub-features:** captive **portal**, **WLAN schedules**, **MAC filters** are
  referenced by fields but not fully modelled on `omada_wireless_network`.
- **Site settings breadth:** only ~45 fields of a large object are modelled; add
  more the same table-driven way (`site_settings_resource.go`).
- **Policy routes / UPnP:** not modelled at all (static routes are). Capture and add
  like any other transmission-setting resource.
- **More data sources:** `omada_port_forwards` and `omada_firewall_acls` shipped
  (discovery — list objects + IDs for import). `omada_clients` and a
  device-discovery source (§5.5) are still open.

### 5.7 Client-level: pagination

**Status:** list calls request `?currentPage=1&currentPageSize=1000` — a single big
page, **not** a real pagination loop. Fine for a homelab; wrong for a site with
>1000 objects of one type.
**To implement:** a paging helper in `client.go` that follows `totalRows` across
pages, used by the `List*` methods. Purely internal; mock already returns the
paging envelope fields.

---

## 6. Release & versioning

- Semver via signed tags. On a `v*` tag, GoReleaser builds multi-platform archives,
  **GPG-signs** the checksums, and publishes a GitHub Release; the Terraform Registry
  ingests it. Current line: `v0.1.x`.
- Breaking schema changes wait for `v1.0.0`. Until then, additive field coverage and
  new resources are the normal cadence.
- CI (`.github/workflows/test.yml`) runs build, unit + acceptance tests, lint, and a
  `tfplugindocs` staleness check on every PR. Keep all green.

---

## 7. Where to look first

| You want to… | Start in |
|---|---|
| understand auth / retry / the envelope | `internal/omada/client.go`, `auth.go` |
| copy a small clean resource | `internal/omada/staticroute.go` + `internal/provider/static_route_resource.go` |
| see deep-merge + a write-only field | `internal/omada/wireless.go` |
| see import type-discovery | `internal/provider/firewall_acl_resource.go` |
| add mock endpoints / assertions | `internal/provider/provider_test.go` |
| understand a data source | `internal/provider/networks_data_source.go` |
