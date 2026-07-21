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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &lanDNSResource{}
	_ resource.ResourceWithConfigure   = &lanDNSResource{}
	_ resource.ResourceWithImportState = &lanDNSResource{}
)

func NewLanDNSResource() resource.Resource {
	return &lanDNSResource{}
}

type lanDNSResource struct {
	data *providerData
}

type lanDNSResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Site          types.String `tfsdk:"site"`
	SiteID        types.String `tfsdk:"site_id"`
	Enable        types.Bool   `tfsdk:"enable"`
	Name          types.String `tfsdk:"name"`
	Domain        types.String `tfsdk:"domain"`
	Aliases       types.List   `tfsdk:"aliases"`
	IPAddresses   types.List   `tfsdk:"ip_addresses"`
	IPv6Addresses types.List   `tfsdk:"ipv6_addresses"`
	LanNetworkIDs types.List   `tfsdk:"lan_network_ids"`
	CustomTTL     types.Bool   `tfsdk:"custom_ttl"`
}

func (r *lanDNSResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_lan_dns"
}

func (r *lanDNSResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a local (LAN) DNS record on the Omada controller — a hostname/domain resolved to LAN IP(s) for the selected networks.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				MarkdownDescription: "Controller-assigned record ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Site name. Defaults to the controller's primary site. Changing this forces replacement.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Resolved site ID.",
				Computed:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enable": schema.BoolAttribute{
				MarkdownDescription: "Whether the record is enabled.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(true),
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Display name for the record.",
				Required:            true,
			},
			"domain": schema.StringAttribute{
				MarkdownDescription: "The domain/hostname to resolve, e.g. `nas.wilant.be`.",
				Required:            true,
			},
			"aliases": schema.ListAttribute{
				MarkdownDescription: "Additional domains that resolve to the same addresses.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"ip_addresses": schema.ListAttribute{
				MarkdownDescription: "IPv4 addresses the domain resolves to.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"ipv6_addresses": schema.ListAttribute{
				MarkdownDescription: "IPv6 addresses the domain resolves to.",
				ElementType:         types.StringType,
				Optional:            true,
				Computed:            true,
			},
			"lan_network_ids": schema.ListAttribute{
				MarkdownDescription: "IDs of the LAN networks this record is served on. Must reference existing networks.",
				ElementType:         types.StringType,
				Required:            true,
			},
			"custom_ttl": schema.BoolAttribute{
				MarkdownDescription: "Whether a custom TTL is used.",
				Optional:            true,
				Computed:            true,
				Default:             booldefault.StaticBool(false),
			},
		},
	}
}

func (r *lanDNSResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *lanDNSResource) siteName(m lanDNSResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *lanDNSResource) inputFrom(ctx context.Context, m lanDNSResourceModel) (*omada.LanDNSInput, diag.Diagnostics) {
	var diags diag.Diagnostics
	aliases, d := stringSlice(ctx, m.Aliases)
	diags.Append(d...)
	ips, d := stringSlice(ctx, m.IPAddresses)
	diags.Append(d...)
	ip6s, d := stringSlice(ctx, m.IPv6Addresses)
	diags.Append(d...)
	nets, d := stringSlice(ctx, m.LanNetworkIDs)
	diags.Append(d...)

	in := &omada.LanDNSInput{
		Enable:        m.Enable.ValueBool(),
		Name:          m.Name.ValueString(),
		Domain:        m.Domain.ValueString(),
		Aliases:       aliases,
		IPAddresses:   ips,
		IPv6Addresses: ip6s,
		LanNetworkIDs: nets,
		CustomTTL:     m.CustomTTL.ValueBool(),
	}
	return in, diags
}

func (r *lanDNSResource) apply(ctx context.Context, n *omada.LanDNS, m *lanDNSResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(n.ID)
	m.Enable = types.BoolValue(n.Enable)
	m.Name = types.StringValue(n.Name)
	m.Domain = types.StringValue(n.Domain)
	m.CustomTTL = types.BoolValue(n.CustomTTL)

	for _, p := range []struct {
		dst *types.List
		src []string
	}{
		{&m.Aliases, n.Aliases}, {&m.IPAddresses, n.IPAddresses},
		{&m.IPv6Addresses, n.IPv6Addresses}, {&m.LanNetworkIDs, n.LanNetworkIDs},
	} {
		list, d := stringListValue(ctx, p.src)
		diags.Append(d...)
		*p.dst = list
	}
	return diags
}

func (r *lanDNSResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan lanDNSResourceModel
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
	created, err := r.data.client.CreateLanDNS(ctx, siteID, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create LAN DNS record", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *lanDNSResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state lanDNSResourceModel
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
	rec, err := r.data.client.GetLanDNS(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, rec, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *lanDNSResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan lanDNSResourceModel
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
	updated, err := r.data.client.UpdateLanDNS(ctx, siteID, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update LAN DNS record", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *lanDNSResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state lanDNSResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteLanDNS(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete LAN DNS record", err.Error())
	}
}

// ImportState accepts "<id>" or "<site_name>/<id>".
func (r *lanDNSResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
