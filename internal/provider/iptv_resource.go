// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &iptvResource{}
	_ resource.ResourceWithConfigure   = &iptvResource{}
	_ resource.ResourceWithImportState = &iptvResource{}
)

func NewIPTVResource() resource.Resource { return &iptvResource{} }

type iptvResource struct{ data *providerData }

type iptvResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`

	IGMPProxyEnable types.Bool   `tfsdk:"igmp_proxy_enable"`
	IGMPVersion     types.String `tfsdk:"igmp_version"`
	IPTVEnable      types.Bool   `tfsdk:"enable"`
	EnabledPortIDs  types.List   `tfsdk:"enabled_port_ids"`
	AvailablePorts  types.List   `tfsdk:"available_ports"`
}

func (r *iptvResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iptv"
}

func (r *iptvResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the site's IPTV and IGMP-proxy settings.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> **Enabling IPTV on a port changes what that port is.** The ports listed in " +
			"`available_ports` are the gateway's WAN/LAN ports; switching one into IPTV mode takes it " +
			"out of ordinary service. Do not do it to the port carrying your WAN or the uplink to your " +
			"switch.\n\n" +
			"~> **`igmp_version` also exists on `omada_gateway`.** The controller keeps an IGMP version " +
			"in two documents — this one (site IGMP proxy) and the gateway device document " +
			"(`omada_gateway.igmp_version`). They read back independently and were observed holding " +
			"*different* values, so they are not the same setting, but managing both from Terraform " +
			"with conflicting values will produce a confusing result. Pick one unless you know why you " +
			"want both.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()},
				MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.",
			},
			"site_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"igmp_proxy_enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Whether the gateway proxies IGMP, so multicast can cross between " +
					"the WAN and the LAN.",
			},
			"igmp_version": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("2"),
				MarkdownDescription: "IGMP version, as a string — the controller stores `\"2\"`, not `2`. " +
					"See the note above about the gateway's copy of this setting.",
			},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Whether IPTV mode is active at all. With this off, `enabled_port_ids` " +
					"has no effect.",
			},
			"enabled_port_ids": schema.ListAttribute{
				ElementType: types.StringType,
				Optional:    true, Computed: true,
				MarkdownDescription: "Port ids switched into IPTV mode, in the controller's `<n>_<hex>` " +
					"form. Read the available ids from `available_ports`. The port list itself belongs to " +
					"the controller and is never rewritten — only these flags are.",
			},
			"available_ports": schema.ListNestedAttribute{
				Computed: true,
				MarkdownDescription: "The gateway's ports as the controller reports them. Read-only; use " +
					"it to find the id for `enabled_port_ids`.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":           schema.StringAttribute{Computed: true, MarkdownDescription: "Port id."},
						"name":         schema.StringAttribute{Computed: true, MarkdownDescription: "Port label, e.g. `WAN/LAN3`."},
						"support_iptv": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether the port can do IPTV at all."},
						"enabled":      schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether IPTV mode is currently on for this port."},
					},
				},
			},
		},
	}
}

func (r *iptvResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iptvResource) siteName(m iptvResourceModel) string {
	if !m.Site.IsNull() && !m.Site.IsUnknown() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

// iptvPortObjectType is the shape of one row of available_ports.
var iptvPortObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"id":           types.StringType,
	"name":         types.StringType,
	"support_iptv": types.BoolType,
	"enabled":      types.BoolType,
}}

func iptvPortsValue(ctx context.Context, ports []omada.IPTVPort) (types.List, diag.Diagnostics) {
	vals := make([]attr.Value, 0, len(ports))
	var diags diag.Diagnostics
	for _, p := range ports {
		obj, d := types.ObjectValue(iptvPortObjectType.AttrTypes, map[string]attr.Value{
			"id":           types.StringValue(p.Port),
			"name":         types.StringValue(p.Name),
			"support_iptv": types.BoolValue(p.SupportIPTV),
			"enabled":      types.BoolValue(p.Status),
		})
		diags.Append(d...)
		vals = append(vals, obj)
	}
	lv, d := types.ListValue(iptvPortObjectType, vals)
	diags.Append(d...)
	return lv, diags
}

func (r *iptvResource) refresh(ctx context.Context, s *omada.IPTVSettings, m *iptvResourceModel, siteID, siteName string, sink *diagSink) {
	m.ID = types.StringValue(siteID)
	m.SiteID = types.StringValue(siteID)
	m.Site = types.StringValue(siteName)
	m.IGMPProxyEnable = types.BoolValue(s.IGMPProxyEnable)
	m.IGMPVersion = types.StringValue(s.IGMPVersion)
	m.IPTVEnable = types.BoolValue(s.IPTVEnable)

	ids, d := stringListValue(ctx, s.EnabledPortIDs)
	if d.HasError() {
		sink.AddError("Unable to read enabled port ids", "converting the port id list failed")
	}
	m.EnabledPortIDs = ids

	ports, d2 := iptvPortsValue(ctx, s.Ports)
	if d2.HasError() {
		sink.AddError("Unable to read available ports", "converting the port list failed")
	}
	m.AvailablePorts = ports
}

func (r *iptvResource) apply(ctx context.Context, plan *iptvResourceModel, sink *diagSink) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(*plan))
	if err != nil {
		sink.AddError("Unable to resolve site", err.Error())
		return
	}
	// `site` is Computed, so record the *canonical* name the request resolved
	// to rather than what was asked for. Without this an unset `site` stays
	// empty in state while an import records "Default", and the two disagree.
	siteID, siteName := site.ID, site.Name
	ids, d := stringSlice(ctx, plan.EnabledPortIDs)
	if d.HasError() {
		sink.AddError("Invalid enabled_port_ids", "could not read the port id list")
		return
	}
	in := omada.IPTVSettings{
		IGMPProxyEnable: plan.IGMPProxyEnable.ValueBool(),
		IGMPVersion:     plan.IGMPVersion.ValueString(),
		IPTVEnable:      plan.IPTVEnable.ValueBool(),
		EnabledPortIDs:  ids,
	}
	if err := r.data.client.UpdateIPTV(ctx, siteID, in); err != nil {
		sink.AddError("Unable to update iptv settings", err.Error())
		return
	}
	cur, err := r.data.client.GetIPTV(ctx, siteID)
	if err != nil {
		sink.AddError("Unable to read iptv settings after update", err.Error())
		return
	}
	r.refresh(ctx, cur, plan, siteID, siteName, sink)
}

func (r *iptvResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan iptvResourceModel
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

func (r *iptvResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state iptvResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	site, err := r.data.client.ResolveSite(ctx, r.siteName(state))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	siteID, siteName := site.ID, site.Name
	cur, err := r.data.client.GetIPTV(ctx, siteID)
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.refresh(ctx, cur, &state, siteID, siteName, &diagSink{&resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *iptvResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan iptvResourceModel
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

// Delete drops the singleton from state; there is nothing to remove.
func (r *iptvResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the site name.
func (r *iptvResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), req.ID)...)
}
