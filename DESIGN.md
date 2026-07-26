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
- **A settings document can carry secrets.** `/setting/snmp` returns the SNMP
  v3 `password` in **plaintext** once v3 is enabled, and the v1/v2c
  `communityString` likewise. The singleton scaffold models such keys with
  `kindStringWO`: `WriteOnly` in the schema, read from `req.Config` (write-only
  values are null in the plan), and never written to state. The read-modify-write
  then preserves a secret that was not re-supplied, because `cur` still holds it.
- **A document can mix settings with reference data.** `/setting/ips` returns
  `lowCategories` / `mediumCategories` / `highCategories` / `allCategories`
  describing what each protection level covers. Those are not configuration:
  the controller keeps them whether or not they are sent. Declare such keys in
  the `SettingDoc`'s `ReadOnlyKeys` so the read-modify-write drops them, and
  model them with `kindIntListRO` so they are reported as Computed-only.
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

- **Secrets the controller returns in plaintext are never read back.** The SSID
  list returns the WiFi `psk` in plaintext and RADIUS profiles return
  `authServer[].radiusPwd` the same way, so neither is ever decoded into state
  from the API, and updates preserve the stored value when a new one isn't
  supplied (`pskSetting` deep-merge; `carryRadiusSecrets`).
- **Not reading a secret back is only half the job.** Terraform persists
  *configured* values in state regardless of what the provider reads, so
  `Sensitive: true` alone still writes the secret to the state file. Only
  `WriteOnly: true` (Terraform ≥ 1.11, framework ≥ 1.14) keeps it out of state
  and plan entirely. `omada_radius_profile.shared_secret` uses it, and an
  acceptance test greps the whole state for the secret to prove it.
  ⚠️ `omada_wireless_network.psk` and `omada_portal.password` are **still only
  `Sensitive`**, so their configured values *do* land in state despite what
  their descriptions imply — converting them is a small, worthwhile follow-up.
- **`deviceAccount` is never sent.** Site-settings updates must never include the
  device-credential object. A mock test asserts it survives untouched.
- **Null is not false.** Some controller fields come back `null`; writing `false`
  over a null is a *change*, not a no-op, and shows up as an unwanted diff. Model
  such fields as `Optional` + `Computed` and leave them unset unless the user sets
  them. (Seen on port-profile `dhcpL2RelaySettings` and several SSID toggles.)
- **Verbs vary per endpoint.** Some items update with `PATCH`, some with `PUT`;
  some delete at `/{id}`, some at `/{type}/{id}`. Static routes reject `PATCH`
  outright (`-1600`). Never assume — confirm against the live controller.

### 2.7 The Open API is a second, separately-authenticated surface

Two capabilities live only on TP-Link's documented **Open API** under
`/openapi/`: creating a network (§5.1) and per-device configuration (§5.5).

It does **not** accept the web session — a request carrying a valid
`Csrf-Token` and cookie is refused `-44116`. The controller UI only reaches it
because its requests are proxied through TP-Link's cloud connector, which
authenticates on the operator's behalf. Locally the only way in is a
client-credentials grant against an application registered under *Settings →
Platform Integration → Open API*:

```
POST /openapi/authorize/token?grant_type=client_credentials
{"omadacId": …, "client_id": …, "client_secret": …}
-> {"result": {"accessToken": …, "expiresIn": 7200}}
```

then `Authorization: AccessToken=<token>` on every call.
`internal/omada/openapi.go` implements that: the token is cached until shortly
before expiry, and a call refused with `-44112`/`-44113`/`-44116` refreshes once
and retries, so a token lapsing mid-apply is not a failed apply.

Credentials are provider-level and **optional** (`openapi_client_id` /
`openapi_client_secret`). When absent, operations needing them fail with a
message naming the setting and the UI page — not a bare "unauthorized", which
would send a practitioner to check the wrong credentials entirely.

### 2.8 Sparse keyed collections

Some documents carry a long list of keyed entries where most of each entry is
description rather than configuration. `/logs/notification` has 63 alert and 68
event notifications, each `{key, shortMsg, module, level, deviceTypes, email,
webhook, enable}` — only the last three are settings.

Modelling that as a list would force a configuration to restate 131 entries and
would let a stale config silently revert entries it did not mean to touch. The
pattern used instead, worth copying for anything similar:

- expose a **map keyed by the controller's own key**, marked `Optional`;
- patch entries **in place by key** during the read-modify-write, leaving every
  other entry and every unmodelled field untouched;
- refresh **only the declared keys**, so a sparse config stays sparse;
- make the per-entry toggles `Optional` and **not** `Computed`, and refresh only
  the ones actually set. Nested `Computed` attributes inside an `Optional` map
  are planned as null rather than unknown, so filling them fails as an
  inconsistent apply.

The cost is that `terraform import` cannot populate the map — which of the 131
you intend to manage is not discoverable — so an import is followed by a
one-time diff adding the declared entries. That is inherent, and the acceptance
tests use `ImportStateVerifyIgnore` for those attributes.

---

## 3. Coverage matrix

Legend: **live** = exact endpoint + verbs confirmed against a real v6.2 controller
with a throwaway object; **mock** = has acceptance-test coverage against the
in-process mock; **subset** = a practical field subset is modelled, the rest
preserved via read-modify-write.

| Resource / data source | CRUD | Verified | Notes |
|---|---|---|---|
| `omada_network` | C/I/R/U/D | live | create goes through the Open API — see §5.1 |
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
| `omada_mac_filter` | R/U (singleton) | live | master toggle only; entries live elsewhere |
| `omada_attack_defense` | R/U (singleton) | live · subset | flood defense / packet anomaly / IP options; update is `PUT` |
| `omada_ips_whitelist` | C/R/D | live | read at `/grid/`, write one level up; no update verb |
| `omada_snmp` | R/U (singleton) | live | update is `PUT`; v3 password returned in plaintext, so write-only |
| `omada_ips` | R/U (singleton) | live | update is `PATCH`; `*Categories` are controller-owned reference data |
| `omada_upnp` | R/U (singleton) | live | update is `PUT` |
| `omada_gateway_bandwidth_control` | R/U (singleton) | live | reads nested, writes flat — dotted keys (§2.5) |
| `omada_portal_access_control` | R/U (singleton) | live | switches only; policy lists preserved |
| `omada_session_limit` | R/U (singleton) | live | `PUT`; the per-host `table` is dropped before write |
| `omada_alg` | R/U (singleton) | live | FTP/H.323/PPTP/IPsec/SIP ALGs; update is `PUT` |
| `omada_ssh_settings` | R/U (singleton) | live | device SSH; update is `PUT` |
| `omada_mac_auth` | R/U (singleton) | live | update is `PATCH` |
| `omada_dot1x` | R/U (singleton) | live | site-wide 802.1X; update is `PATCH` |
| `omada_rate_limit_profile` | CRUD | live | bare-array list; a limit is absent while its enable flag is false |
| `omada_service_type` | CRUD | live | create returns the id as a **bare string**; update is `PUT` |
| `omada_qos_bandwidth_control` | CRUD | live | one rule per WAN port (`-43310`); create returns null, resolved by WAN |
| `omada_time_range` | CRUD | live | schedule profile; create returns `profileId`; list has no `totalRows` |
| `omada_radius_profile` | CRUD | live | bare-array list; `radiusPwd` write-only, carried across updates |
| `omada_dhcp_reservation` | CRUD | live | item path keyed on **MAC**; unknown key still answers 0 |
| `omada_disable_nat` | CRUD | live (delete inferred) | plural list / singular item paths; update is `PUT`; one rule per WAN port |
| `omada_notification_settings` | R/U (singleton) | live | outside `/setting/`; `PATCH` full doc; sparse keyed maps |
| `omada_audit_notification` | R/U (singleton) | live | as above; entries carry only `webhook` |
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
  (`/setting/firewall/attackdefense`) — **and some are kebab-case and
  abbreviated**: `/setting/wan-ports`,
  `/setting/wired-networks/disable-nats`, and one-to-one NAT at
  `/setting/transmission/otonats` ("oto" = one-to-one). A sweep over
  camelCase full words alone will miss those entirely, which is exactly how
  they went unfound for a while. Sweep all three casings, and try
  abbreviations.
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

## 5. What's left — the road to "complete"

"Complete" here means: **every configuration surface the Omada v6 web UI
exposes for a site can be managed declaratively.** Read-only telemetry, one-shot
actions and per-client runtime state are out of scope (§5.7).

Everything below was established against a live v6.2.14 controller
(ER707-M2 gateway, ES205GP switches, EAP610 APs). An endpoint's absence from
this list means it was not found — not that it does not exist.

### 5.0 Coverage audit

Every configuration endpoint found on the controller, and where it stands.

| Endpoint | Status |
|---|---|
| `/setting/lan/networks` | ✅ `omada_network` (**create blocked**, §5.1) |
| `/setting/lan/dns` | ✅ `omada_lan_dns` |
| `/setting/lan/profiles` | ✅ `omada_port_profile` (subset) |
| `/setting/wan/networks` | ⚠️ read-only data source by design (§5.3) |
| `/setting/wan-ports` | ⚠️ query parameter unknown — but **not needed**, see §5.8 |
| `/setting/wired-networks/disable-nats` | ✅ `omada_disable_nat` |
| `/setting/wlans` + `/wlans/{id}/ssids` | ✅ `omada_wlan_group`, `omada_wireless_network` (subset) |
| `/setting/transmission/portForwardings` | ✅ `omada_port_forward` |
| `/setting/transmission/staticRoutings` | ✅ `omada_static_route` |
| `/setting/transmission/alg` | ✅ `omada_alg` |
| `/setting/transmission/otonats` | 🚫 **blocked** — needs a static-IP WAN (§5.1) |
| `/setting/transmission/policyRoutings` | ❌ §5.2 |
| `/setting/transmission/sessionLimits` | ✅ `omada_session_limit` (per-host table not modelled) |
| `/setting/transmission/bandwidthControls` | ✅ `omada_gateway_bandwidth_control` |
| `/setting/qos/gateway/bwc` | ✅ `omada_qos_bandwidth_control` |
| `/setting/firewall/acls` | ✅ `omada_firewall_acl` (inline ports: §5.6) |
| `/setting/firewall/attackdefense` | ✅ `omada_attack_defense` |
| `/setting/firewall/macfilter` | ✅ `omada_mac_filter` (toggle only) |
| `/setting/firewall/urlfilterings` | ❌ §5.2 — needs its query parameter |
| `/setting/ips` | ✅ `omada_ips` |
| `/setting/ips/whitelist` | ✅ `omada_ips_whitelist` |
| `/setting/ips/grid/blacklist`, `/setting/ips/signature` | ⚠️ read-only and empty (§5.4) |
| `/setting/portals` | ✅ `omada_portal` (landing page: §5.3) |
| `/setting/accessControl` | ✅ `omada_portal_access_control` (switches; policy lists not modelled) |
| `/setting/dot1x` | ✅ `omada_dot1x` |
| `/setting/radiusProfiles` | ✅ `omada_radius_profile` |
| `/setting/macAuth` | ✅ `omada_mac_auth` |
| `/setting/profiles/groups` | ✅ `omada_ip_group`, `omada_port_group` |
| `/setting/profiles/timeranges` | ✅ `omada_time_range` |
| `/setting/profiles/service-type` | ✅ `omada_service_type` |
| `/setting/profiles/rateLimits` | ✅ `omada_rate_limit_profile` |
| `/setting/profiles/apns` | ❌ §5.2 — cellular APNs |
| `/setting/service/mdns` | ✅ `omada_mdns_reflector` |
| `/setting/service/dhcp` | ✅ `omada_dhcp_reservation` |
| `/setting/service/ddns` | ❌ §5.2 |
| `/setting/service/rebootSchedules`, `/poeSchedules` | ❌ §5.2 |
| `/setting/snmp` | ✅ `omada_snmp` |
| `/setting/ssh` | ✅ `omada_ssh_settings` |
| `/setting/upnp` | ✅ `omada_upnp` |
| `/setting/vpns` | ✅ `omada_vpn` (**writes inferred**, §5.3) |
| `/setting` (site settings) | ✅ `omada_site_settings` (~45 of many fields, §5.3) |
| `/logs/notification` | ✅ `omada_notification_settings` |
| `/site/audit-notification` | ✅ `omada_audit_notification` |
| `/rfPlanning` | ⚠️ an action, not config (§5.4) |
| per-device configuration | ❌ §5.5 |

Not found despite looking, and so presumably unsupported on this hardware or
named unlike anything tried: DMZ, port triggering, multi-nets NAT, IPTV,
IP-MAC binding, switch-side QoS, standalone WLAN schedules and MAC filters.

### 5.1 Blocked on something outside the provider

These cannot be finished by writing code alone.

1. **Network create** — **done.** It lives on the Open API at
   `POST /openapi/v2/{omadacId}/sites/{site}/lan-networks`; the web API rejects
   the POST outright, and there is no `/networks/confirm` two-step (that path
   answers `-1600`). The required field set is small — `name`, `purpose`,
   `vlan`, `igmpSnoopEnable` — and plus `interfaceIds` — was derived from the
   endpoint's own validation errors without creating anything, by seeding a body
   with
   `vlan: 99999`: out of the 1–4094 range, so the request could never succeed
   no matter which other fields were filled in. Worth copying whenever a create
   contract has to be mapped on live hardware.

   One constraint hides behind the range check and is worth calling out,
   because it is the kind a probe designed to never succeed cannot find: with a
   *valid* vlan the controller answers **`-33515 LAN interfaces could not be
   none`**. A network must be bound to at least one LAN interface, so
   `interfaceIds` is required at create and cannot be deferred to the follow-up
   update the way the rest of the configuration is. The unsatisfiable-body
   technique is still the right first move — it maps the contract for free —
   but it finds only the checks that run *before* the one deliberately failed.
   Expect a second round of discovery once the body is otherwise valid.

   `CreateNetwork` sends those five and then applies the rest of the
   configuration through the ordinary web-API update. Translating every field
   into Open API shape would mean maintaining a second mapping (the surfaces
   disagree — `dhcpSettingsVO` vs `dhcpSettings`) that only create would
   exercise, and a mistake in it lands on a live VLAN. The cost of the split is
   that create is two calls, so an interruption between them leaves a network
   that exists but is not fully configured; the next apply reconciles it.

2. **One-to-one NAT** — the field set is complete
   (`name`, `status`, `externalIp`, `internalIp`, `dmz`, `interfaceIds`), but
   `-34282` says it requires a **WAN on a static-IP connection**, which the
   development site does not have. No write path can be exercised, and shipping
   an untestable NAT write path is how you take someone's internet down.
3. *(resolved — see §5.8. `/setting/wan-ports` still rejects every query
   parameter tried, but it turned out not to be needed.)*
4. **WLAN optimization** (`/rfPlanning`) — an *action*, not configuration:
   `PUT /rfPlanning/config` validates the document and persists nothing, and
   `/rfPlanning/result` reports a job status. Poor fit for Terraform unless the
   schedule turns out to persist; needs a UI capture of **Save** to tell.

### 5.2 Mapped, unblocked, ready to implement 🟢

Each was found and read on hardware. Most are a `settingsSpec` on the singleton
scaffold (§2.5) or a small CRUD resource; the shapes below are what the live
controller returned.

| Would become | Endpoint | Shape / notes |
|---|---|---|
| `omada_policy_route` | `/setting/transmission/policyRoutings` | paginated, empty on the dev site — needs one entry or a capture for the item shape |
| `omada_ddns` | `/setting/service/ddns` | paginated, empty; `support*` flags indicate TP-Link DDNS + custom providers |
| `omada_reboot_schedule`, `omada_poe_schedule` | `/setting/service/rebootSchedules`, `/poeSchedules` | paginated, empty; pair naturally with `omada_time_range` |
| `omada_url_filter` | `/setting/firewall/urlfilterings` | **needs its query parameter** — answers `-1001` to every one tried |
| `omada_apn_profile` | `/setting/profiles/apns` | cellular APNs; only relevant with an LTE/5G WAN |

The empty ones share a trap worth naming: with no stored row, the item shape is
guesswork. Either create one entry in the UI first, or capture the `POST` — see
§4 on why probing alone stalls on endpoints whose validation names no fields.

### 5.3 Breadth inside resources that already exist

- **`omada_site_settings`** models ~45 fields of a large object. Add more the
  same table-driven way. Note `remoteLog` holds only `{enable}` while remote
  logging is off — the syslog server fields appear once enabled, so pinning
  `remote_log_port` today can diff against a controller that omits it.
- **`omada_wireless_network`** and **`omada_port_profile`** model practical
  subsets. Unmodelled fields are preserved by read-modify-write, never blanked.
- **`omada_vpn`** manages only `name`/`enable`, and its write verbs are
  **inferred** — the read shape is live-verified but create/update/delete were
  never exercised, because the dev site's only VPN was removed. Prefer importing
  and toggling `enable` until someone validates the verbs on hardware.
- **`omada_portal`** covers the functional settings; the landing-page design
  (logo, colours, terms, **background image**) is preserved but not manageable.
  A background needs a multipart upload to `/setting/portals/media`, captured
  from the UI.
- **Writable WAN** stays a deliberate non-goal: `/setting/wan/networks` mixes
  config with read-only `support*` flags, its write verbs are undocumented, and
  unlike every other endpoint the write path **cannot be validated with a
  throwaway** — the only WAN is the live one. If attempted, do it with
  out-of-band access, in a window, modelling a narrow subset.

### 5.4 Read-only surfaces worth a data source

Straightforward, but each needs one real row before the item shape is known:

- `/setting/ips/grid/blacklist` and `/setting/ips/signature` — both read-only
  (every write verb `-1600`) and empty on the dev site.
- `omada_clients` — per-client runtime state.
- `omada_service_types`, `omada_wan_ports` — listings that would make the opaque
  ids in §5.1 and §5.8 usable by name.

### 5.5 Per-device configuration — blocked on the OpenAPI, same as §5.1

`omada_devices` covers read-only inventory. Per-device *config* is not started,
and the reason is now known rather than assumed.

Reads are on the familiar surface:

| Call | Returns |
|---|---|
| `GET /api/v2/sites/{site}/switches/{mac}` | switch detail |
| `GET /api/v2/sites/{site}/eaps/{mac}` | AP detail |
| `GET /api/v2/sites/{site}/gateways/{mac}` | gateway detail |
| `GET /api/v2/sites/{site}/switches/{mac}/ports` | full per-port config |
| `GET /api/v2/sites/{site}/setting/lan/profileSummary` | port profiles as `{id, name, type}` |
| `GET /api/v2/sites/{site}/setting/lan/networks-split` | networks with their `interfaceIds` |

**Writes are not.** A UI capture of saving a switch port shows it goes to the
**OpenAPI**:

```
PATCH /openapi/v1/{omadacId}/sites/{site}/switches/{mac}/ports/4
{duplex, linkSpeed, name, nativeNetworkId, networkTagsSetting,
 profileId, profileOverrideEnable, profileVlanOverrideEnable, tagIds}
```

and the OpenAPI refuses a web session — both `/openapi/v1/...` and
`/openapi/v2/...` answer `-44116 Open API Authorized failed` with a valid
`Csrf-Token` and cookie. The UI only gets away with it because its requests go
through TP-Link's cloud connector, which authenticates to the OpenAPI on the
user's behalf.

The `/api/v2` route at `.../switches/{mac}/ports/{port}` does exist — it answers
`-1001` rather than `-1600`, and the id form answers `-39701 This port does not
exist`, so the port *number* is its key — but it rejects the whole document,
every small subset tried, **and the UI's exact field set above**. No `/api/v2`
write path has been found.

**So per-device config and network create share one blocker.** Implementing the
OpenAPI client-credentials flow (§5.1) unlocks both, which makes it clearly the
highest-leverage remaining work rather than merely the largest.

### 5.6 Firewall ACL inline ports — likely unbuildable on this hardware

Port-scoped rules work today through `omada_port_group` (referenced via
`source_type`/`destination_type = 2`). The ACL's **inline** `customAclPorts` /
`customAclDevices` are sent empty, and the ER707-M2 reports
**`customAcl: false`** in `/setting/capacity` — so the populated shape cannot be
captured on it at all. Needs hardware that reports `customAcl: true`.

### 5.7 Explicitly out of scope

Not gaps, and not planned: statistics and telemetry, log retrieval, one-shot
actions (reboot, upgrade, RF optimization runs, speed tests), client
block/unblock, and controller-level (as opposed to site-level) administration
such as users, roles and cloud access.

### 5.8 The `1_<hex>` WAN interface id — resolved

Three features identify a WAN port by an opaque
`1_c967cf39292e474291e409b4dfe7f0cd` string: `omada_disable_nat.interface`,
`omada_qos_bandwidth_control.wan`, and one-to-one NAT's `interfaceIds`.

**It is the `portUuid`, and the provider already exposes it.** The `omada_wan`
data source flattens `/setting/wan/networks → wanPortSettings[]` and reports
`port_uuid` alongside `port_name`, so a configuration can reference a WAN by
its human name instead of hard-coding the id:

```hcl
data "omada_wan" "this" {}

locals {
  wan = { for p in data.omada_wan.this.ports : p.port_name => p.port_uuid }
}

resource "omada_qos_bandwidth_control" "wan" {
  wan = local.wan["2.5G WAN1"]
  # ...
}
```

Earlier revisions of this document claimed no endpoint listed these ids and
that `/setting/wan-ports` had to be cracked first. That was wrong twice over:
the ids are in a document the provider already reads, and the data source
already surfaced them. The cause was a probe whose regex was mangled by shell
quoting; it returned nothing and the empty result was taken as evidence rather
than checked. Worth keeping as a rule: when a probe reports "not found",
confirm the probe works before concluding anything about the API.

The full physical port inventory — including LAN-capable ports not currently
serving as a WAN — is at `osgPortInfo.wanLanPortSettings[]` in the same
document and is not surfaced. It would only matter for referencing a port that
is not yet a WAN.

### 5.9 Already covered — don't re-implement

Two things are commonly asked for and are already modelled, just not obvious
from the resource names:

- **Isolation** — `omada_network.isolation` (LAN-to-LAN) and
  `omada_wireless_network.guest_net` (guest/client isolation on an SSID).
- **Multicast** — `omada_network` carries `igmp_snoop_enable`,
  `mld_snoop_enable`, `fast_leave_enable`; `omada_wireless_network` carries the
  `multicast_*` family.

Also done and occasionally re-proposed: list pagination
(`internal/omada/pagination.go`), and discovery data sources for port forwards,
firewall ACLs and devices.

## 6. Release & versioning

- Semver via signed tags. On a `v*` tag, GoReleaser builds multi-platform archives
  and **GPG-signs** the checksums; the workflow then creates a **draft** release,
  uploads all 16 artifacts, and publishes it. The Terraform Registry ingests it on
  publication. Current line: `v0.6.x`.
- **Cut releases by pushing a tag, not from the GitHub UI.** The UI creates the
  tag *and* an empty published release; the tag push then starts the workflow,
  which used to add a second release object (drafts are not tag-bound), leaving
  a draft and a published release with the same name. The workflow now clears
  any pre-existing **empty** release for the tag first — and refuses to touch
  one that already has assets, so a re-run can never destroy a shipped release.
- **Why the draft dance:** with GitHub *immutable releases* enabled, a release is
  sealed the moment it is published and then rejects asset uploads with
  `422 Cannot upload assets to an immutable release`. GoReleaser publishes before
  uploading, so releasing directly produced a *published but empty* release — which
  looks like success. That silently shipped nothing for **v0.4.0, v0.5.0 and
  v0.5.1**, and the Registry stayed on 0.3.0 the whole time. Setting GoReleaser's
  own `release.draft: true` did **not** help (it still created the release
  published), so the workflow runs it with `--skip=publish` and does the
  draft → upload → publish sequence itself. The final step asserts at least 16
  assets are attached, so an empty release fails the build instead of passing
  quietly.
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
| add a flat settings singleton (cheapest new resource) | `internal/omada/settings.go` + `internal/provider/alg_resource.go` — a spec is ~40 lines |
| handle a secret correctly | `internal/provider/radius_profile_resource.go` (`WriteOnly`, read from `req.Config`) and `internal/omada/radiusprofile.go` (carry it across an update) |
| model a long keyed list sparsely | `internal/provider/notification_settings_resource.go` (§2.7) |
| deal with a natural key that is not the id | `internal/provider/dhcp_reservation_resource.go` (keyed on MAC, `RequiresReplaceIf`) |
| see deep-merge + read-only reference data | `internal/omada/wireless.go`, `internal/provider/ips_resource.go` |
| see import type-discovery | `internal/provider/firewall_acl_resource.go` |
| add mock endpoints / assertions | `internal/provider/provider_test.go` |
| prove a secret never reaches state | `internal/provider/secret_leak_test.go` |
| understand a data source | `internal/provider/networks_data_source.go` |
| find out what is left to build | §5 — start at the audit table in §5.0 |
