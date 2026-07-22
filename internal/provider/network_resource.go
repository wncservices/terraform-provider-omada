// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &networkResource{}
	_ resource.ResourceWithConfigure   = &networkResource{}
	_ resource.ResourceWithImportState = &networkResource{}
)

func NewNetworkResource() resource.Resource {
	return &networkResource{}
}

type networkResource struct {
	data *providerData
}

type networkResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Site          types.String `tfsdk:"site"`
	SiteID        types.String `tfsdk:"site_id"`
	Name          types.String `tfsdk:"name"`
	Purpose       types.String `tfsdk:"purpose"`
	VLANID        types.Int64  `tfsdk:"vlan_id"`
	GatewaySubnet types.String `tfsdk:"gateway_subnet"`
	InterfaceIDs  types.List   `tfsdk:"interface_ids"`
	DHCPEnabled   types.Bool   `tfsdk:"dhcp_enabled"`
	DHCPStart     types.String `tfsdk:"dhcp_start"`
	DHCPEnd       types.String `tfsdk:"dhcp_end"`
}

func (r *networkResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_network"
}

func (r *networkResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a LAN network (VLAN) on the Omada controller.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Controller-assigned network ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Site name this network belongs to. Defaults to the controller's primary site. Changing this forces replacement.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Resolved site ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Network name.",
				Required:            true,
			},
			"purpose": schema.StringAttribute{
				MarkdownDescription: "`interface` for a routed VLAN with a gateway + DHCP, or `vlan` for an L2-only VLAN.",
				Optional:            true,
				Computed:            true,
				Default:             stringdefault.StaticString("interface"),
			},
			"vlan_id": schema.Int64Attribute{
				MarkdownDescription: "VLAN ID (1-4094).",
				Required:            true,
			},
			"gateway_subnet": schema.StringAttribute{
				MarkdownDescription: "Gateway IP + subnet in CIDR, e.g. `10.10.30.1/24`. Only for `interface` networks.",
				Optional:            true,
				Computed:            true,
			},
			"interface_ids": schema.ListAttribute{
				MarkdownDescription: "Gateway LAN interface IDs this network attaches to. Required by the controller for `interface` networks; populated automatically on import.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
				PlanModifiers:       []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
			"dhcp_enabled": schema.BoolAttribute{
				MarkdownDescription: "Enable the DHCP server on this network.",
				Optional:            true,
				Computed:            true,
			},
			"dhcp_start": schema.StringAttribute{
				MarkdownDescription: "First address of the DHCP pool. Only when `dhcp_enabled`.",
				Optional:            true,
				Computed:            true,
			},
			"dhcp_end": schema.StringAttribute{
				MarkdownDescription: "Last address of the DHCP pool. Only when `dhcp_enabled`.",
				Optional:            true,
				Computed:            true,
			},
		},
	}
}

func (r *networkResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
		return
	}
	r.data = data
}

// siteName returns the configured site name, or the provider default (which may
// be empty, meaning the primary site).
func (r *networkResource) siteName(m networkResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *networkResource) inputFrom(ctx context.Context, m networkResourceModel) (*omada.NetworkInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	in := &omada.NetworkInput{
		Name:          m.Name.ValueString(),
		Purpose:       m.Purpose.ValueString(),
		VLANID:        int(m.VLANID.ValueInt64()),
		GatewaySubnet: m.GatewaySubnet.ValueString(),
	}
	if !m.InterfaceIDs.IsNull() && !m.InterfaceIDs.IsUnknown() {
		var ids []string
		diags.Append(m.InterfaceIDs.ElementsAs(ctx, &ids, false)...)
		in.InterfaceIDs = ids
	}
	if !m.DHCPEnabled.IsNull() && !m.DHCPEnabled.IsUnknown() {
		in.DHCPSettings = &omada.DHCPInput{
			Enable:      m.DHCPEnabled.ValueBool(),
			IPAddrStart: m.DHCPStart.ValueString(),
			IPAddrEnd:   m.DHCPEnd.ValueString(),
		}
	}
	return in, diags
}

func (r *networkResource) apply(ctx context.Context, n *omada.Network, m *networkResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(n.ID)
	m.Name = types.StringValue(n.Name)
	m.Purpose = types.StringValue(n.Purpose)
	m.VLANID = types.Int64Value(int64(n.VLANID))
	m.GatewaySubnet = types.StringValue(n.GatewaySubnet)
	m.DHCPEnabled = types.BoolValue(n.DHCPSettings.Enable)
	m.DHCPStart = types.StringValue(n.DHCPSettings.IPAddrStart)
	m.DHCPEnd = types.StringValue(n.DHCPSettings.IPAddrEnd)

	ids := n.InterfaceIDs
	if ids == nil {
		ids = []string{}
	}
	list, d := types.ListValueFrom(ctx, types.StringType, ids)
	diags.Append(d...)
	m.InterfaceIDs = list
	return diags
}

// Create attempts to create a network via the /api/v2 endpoint.
//
// KNOWN LIMITATION: on v6.2 controllers, creating a *new* "interface" (routed)
// network is not supported here. The controller's web UI creates networks
// through the official Omada OpenAPI (`/openapi/v1/.../networks/confirm`), which
// requires client-credentials auth (a separate token flow) rather than the web
// session this provider uses; the /api/v2 endpoint rejects the create (it
// demands write-only fields like `proto` and ultimately returns a generic
// error). Import, read, update and delete of existing networks all work.
// Creating brand-new networks needs OpenAPI support to be added — see the
// provider README. VLAN-only (purpose="vlan") networks may still create.
func (r *networkResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}

	in, diags := r.inputFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.data.client.CreateNetwork(ctx, siteID, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create network", err.Error())
		return
	}

	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID := state.SiteID.ValueString()
	if siteID == "" {
		var err error
		if siteID, err = r.data.client.ResolveSiteID(ctx, r.siteName(state)); err != nil {
			resp.Diagnostics.AddError("Unable to resolve site", err.Error())
			return
		}
	}

	net, err := r.data.client.GetNetwork(ctx, siteID, state.ID.ValueString())
	if err != nil {
		// Network gone on the controller — drop it from state so TF recreates it.
		resp.State.RemoveResource(ctx)
		return
	}

	resp.Diagnostics.Append(r.apply(ctx, net, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *networkResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan networkResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}

	in, diags := r.inputFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.data.client.UpdateNetwork(ctx, siteID, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update network", err.Error())
		return
	}

	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *networkResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state networkResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteNetwork(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete network", err.Error())
	}
}

// ImportState accepts either "<network_id>" (primary/default site) or
// "<site_name>/<network_id>".
func (r *networkResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
