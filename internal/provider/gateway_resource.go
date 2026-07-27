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
	_ resource.Resource                = &gatewayResource{}
	_ resource.ResourceWithConfigure   = &gatewayResource{}
	_ resource.ResourceWithImportState = &gatewayResource{}
)

func NewGatewayResource() resource.Resource { return &gatewayResource{} }

type gatewayResource struct{ data *providerData }

type gatewayResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`
	MAC    macValue     `tfsdk:"mac"`

	Model types.String `tfsdk:"model"`
	IP    types.String `tfsdk:"ip"`

	Name            types.String `tfsdk:"name"`
	LEDSetting      types.Int64  `tfsdk:"led_setting"`
	LLDPEnable      types.Bool   `tfsdk:"lldp_enable"`
	LLDPSetting     types.Int64  `tfsdk:"lldp_setting"`
	HWOffloadEnable types.Bool   `tfsdk:"hw_offload_enable"`
	IPPT            types.Bool   `tfsdk:"ip_passthrough"`
	SupportIPPT     types.Bool   `tfsdk:"supports_ip_passthrough"`
	SNMPLocation    types.String `tfsdk:"snmp_location"`
	SNMPContact     types.String `tfsdk:"snmp_contact"`
	IGMPEnable      types.Bool   `tfsdk:"igmp_enable"`
	IGMPVersion     types.String `tfsdk:"igmp_version"`
}

func (r *gatewayResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_gateway"
}

func (r *gatewayResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages device-level settings on the adopted gateway.\n\n" +
			"~> **A gateway cannot be created or destroyed.** Terraform *adopts* the existing device. " +
			"Removing this resource from your configuration leaves it exactly as last applied — " +
			"Terraform forgets it, the gateway does not.\n\n" +
			"~> **The gateway's physical ports are deliberately out of scope.** That is where the WAN " +
			"lives, and a wrong write there does not misconfigure a feature — it takes the site off the " +
			"internet. Use `omada_wan` (read), `omada_port_forward`, `omada_disable_nat` and the " +
			"firewall resources, which are scoped and reviewable. This resource refuses to send " +
			"`portConfigs` even if asked.\n\n" +
			"Every settable attribute is optional and only what you set is sent — the controller's " +
			"update is a genuine partial PATCH. Anything you leave out keeps its current value and " +
			"will never report drift.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The gateway MAC, normalised.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				Optional:            true,
				MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"site_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				CustomType: macType{},
				Required:   true,
				MarkdownDescription: "MAC address of the gateway, in any common spelling. " +
					"See the `omada_devices` data source.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"model": schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model. Read-only."},
			"ip":    schema.StringAttribute{Computed: true, MarkdownDescription: "Management IP. Read-only."},

			"name": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Device name shown in the controller.",
			},
			"led_setting": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Controller enum for the status LED. `2` — follow the site setting — " +
					"is what was observed live; the controller does not publish the rest.",
			},
			"lldp_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Whether the gateway sends LLDP.\n\n" +
					"~> LLDP advertises the device's identity and model to anything on the link. That is " +
					"useful for topology and unhelpful on a segment you do not fully trust.",
			},
			"lldp_setting": schema.Int64Attribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Controller enum for LLDP behaviour. `1` observed live.",
			},
			"hw_offload_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "Hardware acceleration of forwarding.\n\n" +
					"~> Offloaded flows bypass parts of the software path, so features that need to see " +
					"every packet — deep inspection, some statistics — can quietly stop applying to " +
					"them. If a security feature seems not to be working, this is worth checking.",
			},
			"ip_passthrough": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "IP passthrough, handing the WAN address to a single LAN client.\n\n" +
					"~> Only settable on hardware that supports it — check `supports_ip_passthrough`. " +
					"The ER707-M2 reports `false`.",
			},
			"supports_ip_passthrough": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the hardware offers IP passthrough at all. Read-only.",
			},
			"snmp_location": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "SNMP sysLocation string. Descriptive only; readable by anything " +
					"permitted to poll SNMP (see `omada_snmp`).",
			},
			"snmp_contact": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "SNMP sysContact string. As with `snmp_location`, treat it as public " +
					"to whoever can poll — a personal email address here is an information leak.",
			},
			"igmp_enable": schema.BoolAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "IGMP snooping/proxy for IPTV multicast.",
			},
			"igmp_version": schema.StringAttribute{
				Optional: true, Computed: true,
				MarkdownDescription: "IGMP version, as a string — the controller stores `\"2\"`, not `2`.",
			},
		},
	}
}

func (r *gatewayResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *gatewayResource) siteName(m gatewayResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *gatewayResource) resolveSite(ctx context.Context, m gatewayResourceModel, diags *diagSink) string {
	if s := m.SiteID.ValueString(); s != "" {
		return s
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return ""
	}
	return siteID
}

// changed builds the PATCH body from the attributes the practitioner actually
// set, leaving everything else untouched.
//
// The nested documents are sent whole because the controller takes them that
// way, so both halves are read from the plan — and where only one half is
// configured, the other is taken from what the gateway currently has rather
// than being blanked.
func (r *gatewayResource) changed(plan gatewayResourceModel, cur *omada.Gateway) map[string]any {
	out := map[string]any{}
	set := func(key string, v any) { out[key] = v }

	if known(plan.Name) {
		set("name", plan.Name.ValueString())
	}
	if known(plan.LEDSetting) {
		set("ledSetting", plan.LEDSetting.ValueInt64())
	}
	if known(plan.LLDPEnable) {
		set("lldpEnable", plan.LLDPEnable.ValueBool())
	}
	if known(plan.LLDPSetting) {
		set("lldpSetting", plan.LLDPSetting.ValueInt64())
	}
	if known(plan.HWOffloadEnable) {
		set("hwOffloadEnable", plan.HWOffloadEnable.ValueBool())
	}
	if known(plan.IPPT) {
		set("ippt", plan.IPPT.ValueBool())
	}

	if known(plan.SNMPLocation) || known(plan.SNMPContact) {
		loc, contact := "", ""
		if cur.SNMPSetting != nil {
			loc, contact = cur.SNMPSetting.Location, cur.SNMPSetting.Contact
		}
		if known(plan.SNMPLocation) {
			loc = plan.SNMPLocation.ValueString()
		}
		if known(plan.SNMPContact) {
			contact = plan.SNMPContact.ValueString()
		}
		set("snmpSeting", map[string]any{"location": loc, "contact": contact})
	}

	if known(plan.IGMPEnable) || known(plan.IGMPVersion) {
		enable, version := false, "2"
		if cur.IPTVSetting != nil {
			enable, version = cur.IPTVSetting.IGMPEnable, cur.IPTVSetting.IGMPVersion
		}
		if known(plan.IGMPEnable) {
			enable = plan.IGMPEnable.ValueBool()
		}
		if known(plan.IGMPVersion) {
			version = plan.IGMPVersion.ValueString()
		}
		set("iptvSetting", map[string]any{"igmpEnable": enable, "igmpVersion": version})
	}

	return out
}

// known reports whether an attribute holds a value the practitioner supplied,
// as opposed to null (unset) or unknown (Computed, to be filled in).
func known(v interface{ IsNull() bool }) bool {
	type unknowable interface{ IsUnknown() bool }
	if u, ok := v.(unknowable); ok && u.IsUnknown() {
		return false
	}
	return !v.IsNull()
}

func (r *gatewayResource) refresh(g *omada.Gateway, m *gatewayResourceModel) {
	m.ID = types.StringValue(omada.NormalizeMAC(g.MAC))
	m.Model = types.StringValue(g.Model)
	m.IP = types.StringValue(g.IP)
	m.Name = types.StringValue(g.Name)
	m.LEDSetting = types.Int64Value(int64(g.LEDSetting))
	m.LLDPEnable = types.BoolValue(g.LLDPEnable)
	m.LLDPSetting = types.Int64Value(int64(g.LLDPSetting))
	m.HWOffloadEnable = types.BoolValue(g.HWOffloadEnable)
	m.IPPT = types.BoolValue(g.IPPT)
	m.SupportIPPT = types.BoolValue(g.SupportIPPT)

	loc, contact := "", ""
	if g.SNMPSetting != nil {
		loc, contact = g.SNMPSetting.Location, g.SNMPSetting.Contact
	}
	m.SNMPLocation = types.StringValue(loc)
	m.SNMPContact = types.StringValue(contact)

	enable, version := false, ""
	if g.IPTVSetting != nil {
		enable, version = g.IPTVSetting.IGMPEnable, g.IPTVSetting.IGMPVersion
	}
	m.IGMPEnable = types.BoolValue(enable)
	m.IGMPVersion = types.StringValue(version)
}

// apply is the shared body of Create and Update.
func (r *gatewayResource) apply(ctx context.Context, plan *gatewayResourceModel, diags *diagSink) {
	siteID := r.resolveSite(ctx, *plan, diags)
	if siteID == "" {
		return
	}
	mac := plan.MAC.ValueString()

	cur, err := r.data.client.GetGateway(ctx, siteID, mac)
	if err != nil {
		diags.AddError("Unable to read gateway", err.Error())
		return
	}
	if body := r.changed(*plan, cur); len(body) > 0 {
		if err := r.data.client.UpdateGateway(ctx, siteID, mac, body); err != nil {
			diags.AddError("Unable to update gateway", err.Error())
			return
		}
		if cur, err = r.data.client.GetGateway(ctx, siteID, mac); err != nil {
			diags.AddError("Unable to read gateway after update", err.Error())
			return
		}
	}
	r.refresh(cur, plan)
	plan.SiteID = types.StringValue(siteID)
}

// Create adopts the existing gateway rather than making one.
func (r *gatewayResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan gatewayResourceModel
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

func (r *gatewayResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state gatewayResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetGateway(ctx, siteID, state.MAC.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.refresh(cur, &state)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *gatewayResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state gatewayResourceModel
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

// Delete drops the gateway from state without touching the device.
func (r *gatewayResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Gateway left as configured",
		"A gateway cannot be deleted, so Terraform has only stopped managing this one. Its settings "+
			"are unchanged. Resetting them on destroy would mean this provider guessing at defaults "+
			"for the device the whole site routes through, which is not a guess worth making.",
	)
}

// ImportState takes the gateway MAC, or "<site>/<mac>".
func (r *gatewayResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), omada.NormalizeMAC(id))...)
}
