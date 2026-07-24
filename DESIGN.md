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

### 2.5 Singleton settings documents

Several controller settings are **flat singleton documents**: one JSON object per
site behind a fixed path, with no id, that can only be read and updated — never
created or deleted. SSH, ALG and attack defense are all this shape.

They are implemented once, not per endpoint:

- `internal/omada/settings.go` — a `SettingDoc{Path, Verb}` per endpoint, plus
  `GetSetting` / `UpdateSetting` (read-modify-write).
- `internal/provider/settings_singleton.go` — a generic table-driven resource.
  Adding one is then just a `settingsSpec` listing `attr → controller key → kind`
  (see `alg_resource.go`, ~40 lines including docs).

Two behaviours are baked in because every one of these endpoints shares them:

- **Reads carry controller-owned metadata** — a `resource` counter and the
  `support*` / `exist*` capability flags the UI uses to decide what to render.
  These must be stripped before writing back; `controllerOwnedKey` does it, and a
  mock handler fails the test if any leaks into a write.
- **The update verb varies.** `/setting/ssh`, `/setting/transmission/alg` and
  `/setting/firewall/attackdefense` reject `PATCH` with `-1600` and need `PUT`;
  `/setting/dot1x` and `/setting/accessControl` are the reverse. Each `SettingDoc`
  records the verb confirmed on hardware.

Unlike list-backed resources, the singleton resource addresses plan/state by
`path.Root(attr)` instead of a struct with `tfsdk` tags, because the field set
differs per spec and one Go type cannot cover them all.

### 2.6 Invariants you must not break

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
| `omada_port_group` | CRUD | live | type-1 group; ACL ref via `*_type = 2` |
| `omada_firewall_acl` | CRUD | live | ACL type auto-discovered on import; custom ports/devices sent empty (§5.3) |
| `omada_wlan_group` | CRUD | live | |
| `omada_mdns_reflector` | CRUD | live | |
| `omada_port_profile` | CRUD | live · subset | STP block deep-merged |
| `omada_wireless_network` | CRUD | live · subset | `psk` write-only |
| `omada_static_route` | CRUD | live | update is `PUT` (`PATCH` → `-1600`) |
| `omada_portal` | CRUD | live · subset | write-only `password`; bare-array list; PATCH RMW |
| `omada_vpn` | CRUD | **read live, writes inferred** | see §5.2 |
| `omada_attack_defense` | R/U (singleton) | live · subset | flood defense / packet anomaly / IP options; update is `PUT` |
| `omada_alg` | R/U (singleton) | live | FTP/H.323/PPTP/IPsec/SIP ALGs; update is `PUT` |
| `omada_ssh_settings` | R/U (singleton) | live | device SSH; update is `PUT` |
| `omada_dot1x` | R/U (singleton) | live | site-wide 802.1X; update is `PATCH` |
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

### Finding an undocumented endpoint

Step 1 of the recipe assumes you can capture the UI's request. When you can't
(or want to check first whether an endpoint exists at all), these work well and
need only a read-only session:

- **Probe paths directly.** An unknown path returns `-1600 Unsupported request
  path.`; a real one returns data or a *different* error (`-1001` invalid
  parameters, `-34326` object does not exist). That distinction makes a
  brute-force sweep cheap and unambiguous. Watch the casing: paths are
  **camelCase** (`/setting/radiusProfiles`, `/setting/accessControl`,
  `/setting/transmission/portForwardings`) even though a few are all-lower
  (`/setting/firewall/attackdefense`).
- **Ask the controller what the gateway supports.**
  `GET /sites/{id}/setting/capacity` returns a feature→bool map (`oneToOneNat`,
  `disableNat`, `customAcl`, `policyRouting`, `ipsec`, …). Use it to tell "this
  gateway can't do that" apart from "I haven't found the path yet", and to
  explain empty payload fields.
- **Determine the update verb without changing anything.** Read the document,
  strip controller-owned keys, and `PUT`/`PATCH` its own values straight back.
  A wrong verb answers `-1600` and a right one answers `0` — and because the
  payload is what was already there, nothing is modified. This is how every verb
  in `settings.go` was confirmed on live hardware.
- **Mine the web app.** `js/app/*.js` and `js/su/*.js` hold a handful of paths,
  but the settings pages are lazy-loaded and not reachable by URL guessing, so
  this is a weaker source than the three techniques above.

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

### 5.3 Firewall ACL port/device scoping 🟢

**Status:** the common case is covered — port-scoped rules use a reusable
**port group** (`omada_port_group`, a type-1 profile group) referenced from the
ACL via `source_type`/`destination_type = 2`. Both shipped and verified live
(zero-diff import of a real port group + the ACL that references it).

Still open: the ACL's **inline** `customAclPorts` / `customAclDevices` fields —
ports/devices specified on the rule itself rather than through a group — are sent
empty. On a v6.2 controller the UI populates these only in specific modes; every
live rule tested had them empty, so the populated payload shape is still
uncaptured. It may not be capturable on all hardware: the ER707-M2 used for
development reports **`customAcl: false`** in `/setting/capacity`, i.e. the
gateway does not offer inline ACL ports at all. **To implement:** on a gateway
whose capacity reports `customAcl: true`, capture the shape from a rule that
uses inline ports (not a group), then model both as nested lists on
`omada_firewall_acl`.

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

### 5.6a Captive-portal landing page / background image 🟢

**Status:** `omada_portal` covers the functional settings. The landing-page design
(`portalCustomize`: logo, colours, terms of service, background) is deep-merged and
therefore preserved, but not manageable from Terraform.
**To implement a background image:** the controller keeps portal images in a media
library — `portalCustomize.background`, `backgroundPictureIndex` and the
`bgPicCoordinatesOfLibrary` / `mobileBgPicCoordinatesOfLibrary` crop rectangles
reference it, and `/setting/portals/media` exists (a bare GET returns `-34326`,
i.e. the path is valid but wants parameters). Needs the multipart upload captured
from the UI, then a resource (or a `background_image` attribute taking a local file
path + hash) that uploads and references the picture index.

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

### 5.7 Client-level: pagination — **done**

Shipped. `internal/omada/pagination.go` provides a generic `listAll[T]` that
follows `totalRows` across pages; every `List*` method and `RawList` routes
through it. Endpoints that return a **bare JSON array** rather than a paging
envelope (`/devices`, `/setting/portals`, `/setting/radiusProfiles`) are decoded
directly and deliberately bypass the pager.

### 5.8 Endpoints discovered but not yet modelled 🟢

All of these were located and their shape read on a live v6.2 controller
(ER707-M2 gateway); none is implemented yet. With §2.5's singleton scaffold most
are an afternoon each — write a `settingsSpec` and a mock handler.

| Endpoint | Verb | Would become | Notes |
|---|---|---|---|
| `/setting/radiusProfiles` | POST / `PATCH` / DELETE | `omada_radius_profile` | **list, not singleton**; full CRUD confirmed with a throwaway. Secret is `authServer[].radiusPwd` — see the warning below. Needed to make `omada_dot1x` genuinely usable |
| `/setting/accessControl` | `PATCH` | `omada_portal_access_control` | Captive-portal pre-auth + free-auth policies (`preAuthAccessPolicies`, `freeAuthClientPolicies` — nested lists, so not a plain `settingsSpec`) |
| `/setting/macAuth` | `PATCH` | `omada_mac_auth` | MAC-based authentication, incl. `ssids` binding |
| `/setting/upnp` | `PUT` | `omada_upnp` | single `enable` |
| `/setting/snmp` | `PUT` | `omada_snmp` | v1/v2c/v3, security level, auth/privacy mode |
| `/setting/firewall/macfilter` | — | `omada_mac_filter` | |
| `/setting/service/ddns` | — | `omada_ddns` | paginated list |
| `/setting/service/rebootSchedules`, `/setting/service/poeSchedules` | — | schedules | paginated lists |
| `/setting/transmission/sessionLimits`, `/setting/transmission/bandwidthControls` | — | QoS-ish | |
| `/setting/transmission/policyRoutings` | — | `omada_policy_route` | paginated list; complements `omada_static_route` |

⚠️ **`radiusPwd` must be write-only.** The controller returns the RADIUS shared
secret in **plaintext** on read, exactly like the WiFi `psk`. Per §2.6 that means
it is never read into state — model it write-only, deep-merge it on update, and
add it to `ImportStateVerifyIgnore`.

### 5.9 Wanted but *not located* — needs a UI capture

Probing found no path for these, so they need step 1 of the recipe (browser
devtools against the UI) rather than more guessing:

- **One-to-One NAT** and **Disable NAT**. `/setting/capacity` reports
  `oneToOneNat: true` and `disableNat: true` on the ER707-M2, so the gateway
  supports them and an endpoint must exist — but nothing was found under
  `/setting/transmission/*`, `/setting/firewall/*`, `/setting/nat/*` or the WAN
  document across ~40 name spellings each. DMZ and port triggering were equally
  absent, which suggests this whole group lives somewhere unguessed. One-to-One
  NAT also needs multiple WAN IPs (`supportWanMultipleIp`), so the page may only
  materialise once the WAN is configured for it.
- **WLAN optimization** — endpoints found, but it is an **action, not config**.
  It lives outside `/setting/*` entirely, under `/sites/{id}/rfPlanning`:

  | Call | Behaviour |
  |---|---|
  | `GET /rfPlanning` | returns the parameter document: `channelDeployEnable*`, `powerAdjustEnable*`, `chanWidth{2,5,6}g`, `mode`, `excludeAps`, `scheduleEnable`, `occurrence{timingType,hour,minute}` |
  | `GET /rfPlanning/result` | `{"status": N}` — the state of an optimization *run* |
  | `PUT /rfPlanning/config` | a real route (bogus siblings answer `-1600`) that **validates** the full document — partial or wrapped bodies are rejected `-1001`, and `mode: 1` is rejected — but **nothing written through it is reflected by `GET /rfPlanning`** |

  Every field tested (`excludeAps`, `chanWidth2g`, `channelDeployEnable6g`,
  `occurrence.minute`) round-tripped as accepted-but-unpersisted. So
  `/rfPlanning/config` appears to *stage* parameters for an optimization run
  that a separate call then starts — a wizard, not durable state.

  **This is a poor fit for a Terraform resource** as it stands: Terraform
  reconciles desired state, and there is no state here to reconcile, only a job
  to trigger. Before building anything, capture the UI's **Save** and **Run**
  requests from Tools → WLAN Optimization to find whether any of it persists.
  If only the *schedule* persists, that part alone could be a small resource.

  (Note: the run-triggering call was deliberately never fired during this
  investigation — starting an optimization re-channels live APs.)

### 5.10 Already covered — don't re-implement

Two things commonly asked for are **already modelled** and just aren't obvious
from the resource names:

- **Isolation.** `omada_network.isolation` (LAN-to-LAN isolation) and
  `omada_wireless_network.guest_net` (guest/client isolation on an SSID).
- **Multicast.** `omada_network` carries `igmp_snoop_enable`, `mld_snoop_enable`
  and `fast_leave_enable`; `omada_wireless_network` carries the
  `multicast_*` family.

---

## 6. Release & versioning

- Semver via signed tags. On a `v*` tag, GoReleaser builds multi-platform archives,
  **GPG-signs** the checksums, and publishes a GitHub Release; the Terraform Registry
  ingests it. Current line: `v0.4.x`.
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
