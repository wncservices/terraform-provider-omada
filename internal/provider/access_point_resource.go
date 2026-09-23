// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &accessPointResource{}
	_ resource.ResourceWithConfigure   = &accessPointResource{}
	_ resource.ResourceWithImportState = &accessPointResource{}
)

func NewAccessPointResource() resource.Resource { return &accessPointResource{} }

type accessPointResource struct{ data *providerData }

type accessPointResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`
	MAC    macValue     `tfsdk:"mac"`

	Model types.String `tfsdk:"model"`
	IP    types.String `tfsdk:"ip"`

	Name         types.String `tfsdk:"name"`
	LEDSetting   types.Int64  `tfsdk:"led_setting"`
	LLDPEnable   types.Int64  `tfsdk:"lldp_enable"`
	SNMPLocation types.String `tfsdk:"snmp_location"`
	SNMPContact  types.String `tfsdk:"snmp_contact"`
	L3Access     types.Bool   `tfsdk:"l3_access_enable"`

	OFDMA2G types.Bool `tfsdk:"ofdma_enable_2g"`
	OFDMA5G types.Bool `tfsdk:"ofdma_enable_5g"`

	// Radios. channel is Computed-only: the controller accepts a write, reports
	// success, and ignores it.
	Radio2GEnable       types.Bool   `tfsdk:"radio_2g_enable"`
	Radio2GChannelWidth types.String `tfsdk:"radio_2g_channel_width"`
	Radio2GTXPower      types.Int64  `tfsdk:"radio_2g_tx_power"`
	Radio2GTXPowerLevel types.Int64  `tfsdk:"radio_2g_tx_power_level"`
	Radio2GChannel      types.String `tfsdk:"radio_2g_channel"`

	Radio5GEnable       types.Bool   `tfsdk:"radio_5g_enable"`
	Radio5GChannelWidth types.String `tfsdk:"radio_5g_channel_width"`
	Radio5GTXPower      types.Int64  `tfsdk:"radio_5g_tx_power"`
	Radio5GTXPowerLevel types.Int64  `tfsdk:"radio_5g_tx_power_level"`
	Radio5GChannel      types.String `tfsdk:"radio_5g_channel"`

	LoadBalance2GEnable types.Bool  `tfsdk:"load_balance_2g_enable"`
	LoadBalance2GMax    types.Int64 `tfsdk:"load_balance_2g_max_clients"`
	LoadBalance5GEnable types.Bool  `tfsdk:"load_balance_5g_enable"`
	LoadBalance5GMax    types.Int64 `tfsdk:"load_balance_5g_max_clients"`

	RSSI2GEnable    types.Bool  `tfsdk:"rssi_2g_enable"`
	RSSI2GThreshold types.Int64 `tfsdk:"rssi_2g_threshold"`
	RSSI5GEnable    types.Bool  `tfsdk:"rssi_5g_enable"`
	RSSI5GThreshold types.Int64 `tfsdk:"rssi_5g_threshold"`
}

func (r *accessPointResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_access_point"
}

func (r *accessPointResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages device-level settings on an adopted access point (EAP).\n\n" +
			"~> **An access point cannot be created or destroyed.** Terraform *adopts* the existing " +
			"device. Removing this resource from your configuration leaves it exactly as last " +
			"applied — Terraform forgets it, the AP does not.\n\n" +
			"~> **Changing a radio setting drops that radio's clients.** The controller restarts the " +
			"radio whenever a radio document is written, so `radio_*_channel_width`, " +
			"`radio_*_tx_power` and `radio_*_enable` are not free to change. This resource sends a " +
			"radio only when one of its values actually differs, so a no-op apply is silent — but a " +
			"real change is a brief outage on that band. Plan it accordingly.\n\n" +
			"Every settable attribute is optional and only what you set is sent — the controller's " +
			"update is a genuine partial PATCH. Anything you leave out keeps its current value and " +
			"will never report drift.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The AP MAC, normalised.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace(),
				},
			},
			"site_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				CustomType: macType{},
				Required:   true,
				MarkdownDescription: "MAC address of the access point, in any common spelling. " +
					"See the `omada_devices` data source.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"model": schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model. Read-only."},
			"ip":    schema.StringAttribute{Computed: true, MarkdownDescription: "Management IP. Read-only."},

			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Device name shown in the controller. A freshly adopted AP is named " +
					"after its MAC, which makes a multi-AP site hard to read.",
			},
			"led_setting": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Status LED: `0` off, `1` on, `2` follow the site setting.",
			},
			"lldp_enable": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "LLDP: `0` off, `1` on, `2` follow the site setting. An integer on APs, " +
					"unlike the gateway's boolean, and null on models without LLDP.\n\n" +
					"~> LLDP advertises the device's identity and model to anything on the link.",
			},
			"snmp_location": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "SNMP sysLocation string. Readable by anything permitted to poll " +
					"SNMP (see `omada_snmp`).",
			},
			"snmp_contact": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "SNMP sysContact string. Treat it as public to whoever can poll — " +
					"a personal email address here is an information leak.",
			},
			"l3_access_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Allow the controller to manage this AP across a layer-3 boundary.",
			},

			"ofdma_enable_2g": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "OFDMA on 2.4GHz. Unlike the radio attributes below, this applies " +
					"without restarting the radio.",
			},
			"ofdma_enable_5g": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "OFDMA on 5GHz. Applies without restarting the radio.",
			},

			"radio_2g_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Whether the 2.4GHz radio is on. **Changing this restarts the radio.**",
			},
			"radio_2g_channel_width": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "2.4GHz channel width, as a string: `\"2\"` 20MHz, `\"3\"` 40MHz, `\"4\"` auto. " +
					"**Changing this restarts the radio.**",
			},
			"radio_2g_tx_power": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "2.4GHz transmit power in dBm. **Changing this restarts the radio.**\n\n" +
					"~> More power is not more coverage: it makes the AP shout further than clients can " +
					"answer, and raises the noise floor for the neighbours you share the band with.",
			},
			"radio_2g_tx_power_level": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "2.4GHz power level: `0` low, `1` medium, `2` high, `3` custom, `4` auto; " +
					"`radio_2g_tx_power` applies only at `3`. " +
					"**Changing this restarts the radio.**",
			},
			"radio_2g_channel": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "2.4GHz channel, `\"0\"` for automatic. **Read-only.** The controller " +
					"accepts a write, reports success, and leaves the channel unchanged — via the device " +
					"PATCH *and* via `PUT /eaps/{mac}/config/radios` — so modelling it as settable would " +
					"produce a clean apply followed by permanent drift. See the resource notes.",
			},

			"radio_5g_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Whether the 5GHz radio is on. **Changing this restarts the radio.**",
			},
			"radio_5g_channel_width": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "5GHz channel width, as a string: `\"5\"` 80MHz, `\"6\"` auto (80/40/20), `\"7\"` 160MHz. " +
					"**Changing this restarts the radio.**",
			},
			"radio_5g_tx_power": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "5GHz transmit power in dBm. **Changing this restarts the radio.**",
			},
			"radio_5g_tx_power_level": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "5GHz power level: `0` low, `1` medium, `2` high, `3` custom, `4` auto; " +
					"`radio_5g_tx_power` applies only at `3`. " +
					"**Changing this restarts the radio.**",
			},
			"radio_5g_channel": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "5GHz channel, `\"0\"` for automatic. **Read-only**, for the same " +
					"reason as `radio_2g_channel`.",
			},

			"load_balance_2g_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Refuse new 2.4GHz associations past `load_balance_2g_max_clients`.",
			},
			"load_balance_2g_max_clients": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Client ceiling for 2.4GHz load balancing.",
			},
			"load_balance_5g_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Refuse new 5GHz associations past `load_balance_5g_max_clients`.",
			},
			"load_balance_5g_max_clients": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Client ceiling for 5GHz load balancing.",
			},

			"rssi_2g_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Disassociate 2.4GHz clients below `rssi_2g_threshold`.\n\n" +
					"~> A threshold set too high evicts clients that were working perfectly well.",
			},
			"rssi_2g_threshold": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "2.4GHz RSSI floor in dBm, negative. `-95` observed live.",
			},
			"rssi_5g_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Disassociate 5GHz clients below `rssi_5g_threshold`.",
			},
			"rssi_5g_threshold": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "5GHz RSSI floor in dBm, negative. `-95` observed live.",
			},
		},
	}
}

func (r *accessPointResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.data = data
}

func (r *accessPointResource) siteName(m accessPointResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *accessPointResource) resolveSite(ctx context.Context, m accessPointResourceModel, diags *diagSink) (string, string) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return "", ""
	}
	return site.ID, site.Name
}

// channel is carried through from the device and never from the plan: it is
// read-only (see omada.AccessPoint), and UpdateAccessPoint refuses a body that
// sets it.
//
// cur is nil when the device reports no radio for this band at all (e.g.
// single-band hardware with no 5GHz radio). Silently returning nil there, the
// same as the no-op case, would drop a practitioner's configured value from
// the PATCH without error; the subsequent refresh then writes state back with
// that value false/empty, contradicting the config. So a configured value
// with cur == nil is an error instead.
func radioBody(band string, cur *omada.RadioSetting, enable types.Bool, width types.String, power, level types.Int64) (map[string]any, error) {
	if cur == nil {
		if known(enable) || known(width) || known(power) || known(level) {
			return nil, fmt.Errorf("radio_%s_* is configured, but the device reports no %s radio "+
				"at all (single-band hardware, or that band is not adopted) — remove it from the "+
				"configuration", band, band)
		}
		return nil, nil
	}
	next := *cur
	if known(enable) {
		next.RadioEnable = enable.ValueBool()
	}
	if known(width) {
		next.ChannelWidth = width.ValueString()
	}
	if known(power) {
		next.TXPower = int(power.ValueInt64())
	}
	if known(level) {
		next.TXPowerLevel = int(level.ValueInt64())
	}
	if next == *cur {
		return nil, nil
	}
	return map[string]any{
		"radioEnable":  next.RadioEnable,
		"channelWidth": next.ChannelWidth,
		"txPower":      next.TXPower,
		"txPowerLevel": next.TXPowerLevel,
		"freq":         next.Freq,
		"wirelessMode": next.WirelessMode,
	}, nil
}

// changed builds the PATCH body from the attributes the practitioner set.
func (r *accessPointResource) changed(plan accessPointResourceModel, cur *omada.AccessPoint) (map[string]any, error) {
	out := map[string]any{}

	if known(plan.Name) {
		out["name"] = plan.Name.ValueString()
	}
	if known(plan.LEDSetting) {
		out["ledSetting"] = plan.LEDSetting.ValueInt64()
	}
	if known(plan.LLDPEnable) {
		out["lldpEnable"] = plan.LLDPEnable.ValueInt64()
	}
	if known(plan.L3Access) {
		out["l3AccessSetting"] = map[string]any{"enable": plan.L3Access.ValueBool()}
	}
	if known(plan.OFDMA2G) {
		out["ofdmaEnable2g"] = plan.OFDMA2G.ValueBool()
	}
	if known(plan.OFDMA5G) {
		out["ofdmaEnable5g"] = plan.OFDMA5G.ValueBool()
	}

	if known(plan.SNMPLocation) || known(plan.SNMPContact) {
		loc, contact := "", ""
		if cur.SNMP != nil {
			loc, contact = cur.SNMP.Location, cur.SNMP.Contact
		}
		if known(plan.SNMPLocation) {
			loc = plan.SNMPLocation.ValueString()
		}
		if known(plan.SNMPContact) {
			contact = plan.SNMPContact.ValueString()
		}
		out["snmp"] = map[string]any{"location": loc, "contact": contact}
	}

	b2g, err := radioBody("2g", cur.RadioSetting2G, plan.Radio2GEnable, plan.Radio2GChannelWidth,
		plan.Radio2GTXPower, plan.Radio2GTXPowerLevel)
	if err != nil {
		return nil, err
	}
	if b2g != nil {
		out["radioSetting2g"] = b2g
	}
	b5g, err := radioBody("5g", cur.RadioSetting5G, plan.Radio5GEnable, plan.Radio5GChannelWidth,
		plan.Radio5GTXPower, plan.Radio5GTXPowerLevel)
	if err != nil {
		return nil, err
	}
	if b5g != nil {
		out["radioSetting5g"] = b5g
	}

	if known(plan.LoadBalance2GEnable) || known(plan.LoadBalance2GMax) {
		out["lbSetting2g"] = lbBody(cur.LoadBalance2G, plan.LoadBalance2GEnable, plan.LoadBalance2GMax)
	}
	if known(plan.LoadBalance5GEnable) || known(plan.LoadBalance5GMax) {
		out["lbSetting5g"] = lbBody(cur.LoadBalance5G, plan.LoadBalance5GEnable, plan.LoadBalance5GMax)
	}
	if known(plan.RSSI2GEnable) || known(plan.RSSI2GThreshold) {
		out["rssiSetting2g"] = rssiBody(cur.RSSI2G, plan.RSSI2GEnable, plan.RSSI2GThreshold)
	}
	if known(plan.RSSI5GEnable) || known(plan.RSSI5GThreshold) {
		out["rssiSetting5g"] = rssiBody(cur.RSSI5G, plan.RSSI5GEnable, plan.RSSI5GThreshold)
	}

	return out, nil
}

func lbBody(cur *omada.LoadBalanceSetting, enable types.Bool, max types.Int64) map[string]any {
	out := map[string]any{"lbEnable": false, "maxClients": 1}
	if cur != nil {
		out["lbEnable"], out["maxClients"] = cur.LBEnable, cur.MaxClients
	}
	if known(enable) {
		out["lbEnable"] = enable.ValueBool()
	}
	if known(max) {
		out["maxClients"] = max.ValueInt64()
	}
	return out
}

func rssiBody(cur *omada.RSSISetting, enable types.Bool, threshold types.Int64) map[string]any {
	out := map[string]any{"rssiEnable": false, "threshold": -95}
	if cur != nil {
		out["rssiEnable"], out["threshold"] = cur.RSSIEnable, cur.Threshold
	}
	if known(enable) {
		out["rssiEnable"] = enable.ValueBool()
	}
	if known(threshold) {
		out["threshold"] = threshold.ValueInt64()
	}
	return out
}

func (r *accessPointResource) refresh(ap *omada.AccessPoint, m *accessPointResourceModel) {
	m.ID = types.StringValue(omada.NormalizeMAC(ap.MAC))
	m.Model = types.StringValue(ap.Model)
	m.IP = types.StringValue(ap.IP)
	m.Name = types.StringValue(ap.Name)
	m.LEDSetting = types.Int64Value(int64(ap.LEDSetting))
	m.LLDPEnable = types.Int64Value(int64(ap.LLDPEnable))
	m.OFDMA2G = types.BoolValue(ap.OFDMAEnable2G)
	m.OFDMA5G = types.BoolValue(ap.OFDMAEnable5G)

	loc, contact := "", ""
	if ap.SNMP != nil {
		loc, contact = ap.SNMP.Location, ap.SNMP.Contact
	}
	m.SNMPLocation = types.StringValue(loc)
	m.SNMPContact = types.StringValue(contact)

	l3 := false
	if ap.L3Access != nil {
		l3 = ap.L3Access.Enable
	}
	m.L3Access = types.BoolValue(l3)

	refreshRadio(ap.RadioSetting2G, &m.Radio2GEnable, &m.Radio2GChannelWidth,
		&m.Radio2GTXPower, &m.Radio2GTXPowerLevel, &m.Radio2GChannel)
	refreshRadio(ap.RadioSetting5G, &m.Radio5GEnable, &m.Radio5GChannelWidth,
		&m.Radio5GTXPower, &m.Radio5GTXPowerLevel, &m.Radio5GChannel)

	refreshLB(ap.LoadBalance2G, &m.LoadBalance2GEnable, &m.LoadBalance2GMax)
	refreshLB(ap.LoadBalance5G, &m.LoadBalance5GEnable, &m.LoadBalance5GMax)
	refreshRSSI(ap.RSSI2G, &m.RSSI2GEnable, &m.RSSI2GThreshold)
	refreshRSSI(ap.RSSI5G, &m.RSSI5GEnable, &m.RSSI5GThreshold)
}

func refreshRadio(cur *omada.RadioSetting, enable *types.Bool, width *types.String, power, level *types.Int64, channel *types.String) {
	if cur == nil {
		*enable, *width = types.BoolValue(false), types.StringValue("")
		*power, *level = types.Int64Value(0), types.Int64Value(0)
		*channel = types.StringValue("")
		return
	}
	*enable = types.BoolValue(cur.RadioEnable)
	*width = types.StringValue(cur.ChannelWidth)
	*power = types.Int64Value(int64(cur.TXPower))
	*level = types.Int64Value(int64(cur.TXPowerLevel))
	*channel = types.StringValue(cur.Channel)
}

func refreshLB(cur *omada.LoadBalanceSetting, enable *types.Bool, max *types.Int64) {
	if cur == nil {
		*enable, *max = types.BoolValue(false), types.Int64Value(0)
		return
	}
	*enable, *max = types.BoolValue(cur.LBEnable), types.Int64Value(int64(cur.MaxClients))
}

func refreshRSSI(cur *omada.RSSISetting, enable *types.Bool, threshold *types.Int64) {
	if cur == nil {
		*enable, *threshold = types.BoolValue(false), types.Int64Value(0)
		return
	}
	*enable, *threshold = types.BoolValue(cur.RSSIEnable), types.Int64Value(int64(cur.Threshold))
}

// apply is the shared body of Create and Update.
func (r *accessPointResource) apply(ctx context.Context, plan *accessPointResourceModel, diags *diagSink) {
	siteID, siteName := r.resolveSite(ctx, *plan, diags)
	if siteID == "" {
		return
	}
	mac := plan.MAC.ValueString()

	cur, err := r.data.client.GetAccessPoint(ctx, siteID, mac)
	if err != nil {
		diags.AddError("Unable to read access point", err.Error())
		return
	}
	body, err := r.changed(*plan, cur)
	if err != nil {
		diags.AddError("Invalid access point configuration", err.Error())
		return
	}
	if len(body) > 0 {
		if err := r.data.client.UpdateAccessPoint(ctx, siteID, mac, body); err != nil {
			diags.AddError("Unable to update access point", err.Error())
			return
		}
		if cur, err = r.data.client.GetAccessPoint(ctx, siteID, mac); err != nil {
			diags.AddError("Unable to read access point after update", err.Error())
			return
		}
	}
	r.refresh(cur, plan)
	plan.SiteID = types.StringValue(siteID)
	plan.Site = types.StringValue(siteName)
}

// Create adopts the existing access point rather than making one.
func (r *accessPointResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan accessPointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *accessPointResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state accessPointResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, siteName := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetAccessPoint(ctx, siteID, state.MAC.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.refresh(cur, &state)
	state.SiteID = types.StringValue(siteID)
	state.Site = types.StringValue(siteName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *accessPointResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state accessPointResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteID = state.SiteID
	r.apply(ctx, &plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete drops the AP from state without touching the device.
func (r *accessPointResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Access point left as configured",
		"An access point cannot be deleted, so Terraform has only stopped managing this one. Its "+
			"settings are unchanged. Resetting them on destroy would mean restarting the radios of a "+
			"live AP to reach this provider's guess at a default, which is not a guess worth making.",
	)
}

// ImportState takes the AP MAC, or "<site>/<mac>".
func (r *accessPointResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), omada.NormalizeMAC(id))...)
}
