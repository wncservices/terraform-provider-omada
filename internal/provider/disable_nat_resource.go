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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &disableNATResource{}
	_ resource.ResourceWithConfigure   = &disableNATResource{}
	_ resource.ResourceWithImportState = &disableNATResource{}
)

func NewDisableNATResource() resource.Resource { return &disableNATResource{} }

type disableNATResource struct{ data *providerData }

type disableNATResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Site      types.String `tfsdk:"site"`
	SiteID    types.String `tfsdk:"site_id"`
	Name      types.String `tfsdk:"name"`
	Interface types.String `tfsdk:"interface"`
	Networks  types.List   `tfsdk:"network_ids"`
	Enable    types.Bool   `tfsdk:"enable"`
}

func (r *disableNATResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_disable_nat"
}

func (r *disableNATResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a disable-NAT rule: traffic from the listed LAN networks is **routed " +
			"rather than NATed** out of the given WAN interface.\n\n" +
			"~> **Enabling this removes internet access for those networks unless the upstream " +
			"router already has return routes for their subnets.** Without NAT, replies to those " +
			"private addresses have no way back. Set `enable = false` while you stage the rule, and " +
			"flip it only once upstream routing is in place.\n\n" +
			"The controller permits **one rule per WAN port**; a second create is rejected " +
			"(`-34247`).",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Rule name."},
			"interface": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "WAN interface id, in the controller's `1_<hex>` form (for example " +
					"`1_c967cf39292e474291e409b4dfe7f0cd`). Read it from an existing rule, or from the " +
					"controller UI's network request when adding one.",
			},
			"network_ids": schema.ListAttribute{
				ElementType:         types.StringType,
				Required:            true,
				MarkdownDescription: "IDs of the LAN networks this rule applies to (see the `omada_networks` data source).",
			},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Whether the rule is active. Defaults to `false` — see the warning above.",
			},
		},
	}
}

func (r *disableNATResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *disableNATResource) siteName(m disableNATResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *disableNATResource) toAPI(ctx context.Context, m disableNATResourceModel, diags *diagSink) omada.DisableNAT {
	nets, d := stringSlice(ctx, m.Networks)
	if d.HasError() {
		diags.AddError("Invalid network_ids", "could not read the network id list")
	}
	return omada.DisableNAT{
		Name:      m.Name.ValueString(),
		Interface: m.Interface.ValueString(),
		LanList:   nilToEmpty(nets),
		Status:    m.Enable.ValueBool(),
	}
}

func (r *disableNATResource) refresh(ctx context.Context, d *omada.DisableNAT, m *disableNATResourceModel) {
	m.ID = types.StringValue(d.ID)
	m.Name = types.StringValue(d.Name)
	m.Interface = types.StringValue(d.Interface)
	m.Enable = types.BoolValue(d.Status)
	lv, _ := stringListValue(ctx, d.LanList)
	m.Networks = lv
}

func (r *disableNATResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan disableNATResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	sink := &diagSink{&resp.Diagnostics}
	body := r.toAPI(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.data.client.CreateDisableNAT(ctx, siteID, body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create disable-nat rule", err.Error())
		return
	}
	cur, err := r.data.client.GetDisableNAT(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read disable-nat rule after create", err.Error())
		return
	}
	r.refresh(ctx, cur, &plan)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *disableNATResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state disableNATResourceModel
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
	cur, err := r.data.client.GetDisableNAT(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.refresh(ctx, cur, &state)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *disableNATResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state disableNATResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := state.SiteID.ValueString()
	if siteID == "" {
		var err error
		if siteID, err = r.data.client.ResolveSiteID(ctx, r.siteName(plan)); err != nil {
			resp.Diagnostics.AddError("Unable to resolve site", err.Error())
			return
		}
	}
	sink := &diagSink{&resp.Diagnostics}
	body := r.toAPI(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	if err := r.data.client.UpdateDisableNAT(ctx, siteID, id, body); err != nil {
		resp.Diagnostics.AddError("Unable to update disable-nat rule", err.Error())
		return
	}
	cur, err := r.data.client.GetDisableNAT(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read disable-nat rule after update", err.Error())
		return
	}
	r.refresh(ctx, cur, &plan)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *disableNATResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state disableNATResourceModel
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
	if err := r.data.client.DeleteDisableNAT(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete disable-nat rule", err.Error())
	}
}

// ImportState takes the rule id, or "<site>/<id>".
func (r *disableNATResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
