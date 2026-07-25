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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &ipsWhitelistResource{}
	_ resource.ResourceWithConfigure   = &ipsWhitelistResource{}
	_ resource.ResourceWithImportState = &ipsWhitelistResource{}
)

func NewIPSWhitelistResource() resource.Resource { return &ipsWhitelistResource{} }

type ipsWhitelistResource struct{ data *providerData }

type ipsWhitelistResourceModel struct {
	ID            types.String `tfsdk:"id"`
	Site          types.String `tfsdk:"site"`
	SiteID        types.String `tfsdk:"site_id"`
	Direction     types.Int64  `tfsdk:"direction"`
	TrafficType   types.Int64  `tfsdk:"traffic_type"`
	TrafficSource types.String `tfsdk:"traffic_source"`
}

func (r *ipsWhitelistResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_ips_whitelist"
}

func (r *ipsWhitelistResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Exempts a traffic source from IPS/IDS inspection (the IPS allow list).\n\n" +
			"~> **This weakens a security control for whatever it covers.** An exempted source is no " +
			"longer inspected, so use it for a specific false positive rather than to quiet the log.\n\n" +
			"An entry is nothing but its three fields, so changing any of them is a different rule and " +
			"replaces the entry — the controller offers no update verb for it either.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"direction": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "Controller enum for the traffic direction the exemption applies to. " +
					"TP-Link does not document the values; the UI writes `1`. Check the controller before " +
					"choosing another.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"traffic_type": schema.Int64Attribute{
				Required: true,
				MarkdownDescription: "Controller enum selecting what `traffic_source` identifies. `1` pairs " +
					"with a network id.",
				PlanModifiers: []planmodifier.Int64{int64planmodifier.RequiresReplace()},
			},
			"traffic_source": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "ID of the exempted object — a network id when `traffic_type` is `1` " +
					"(see the `omada_networks` data source).",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
		},
	}
}

func (r *ipsWhitelistResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *ipsWhitelistResource) siteName(m ipsWhitelistResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *ipsWhitelistResource) resolveSite(ctx context.Context, m ipsWhitelistResourceModel, diags *diagSink) string {
	if id := m.SiteID.ValueString(); id != "" {
		return id
	}
	id, err := r.data.client.ResolveSiteID(ctx, r.siteName(m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return ""
	}
	return id
}

func (m *ipsWhitelistResourceModel) refresh(e *omada.IPSWhitelistEntry) {
	m.ID = types.StringValue(e.ID)
	m.Direction = types.Int64Value(int64(e.Direction))
	m.TrafficType = types.Int64Value(int64(e.TrafficType))
	m.TrafficSource = types.StringValue(e.TrafficSource)
}

func (r *ipsWhitelistResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan ipsWhitelistResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.data.client.CreateIPSWhitelistEntry(ctx, siteID, omada.IPSWhitelistEntry{
		Direction:     int(plan.Direction.ValueInt64()),
		TrafficType:   int(plan.TrafficType.ValueInt64()),
		TrafficSource: plan.TrafficSource.ValueString(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create ips whitelist entry", err.Error())
		return
	}
	cur, err := r.data.client.GetIPSWhitelistEntry(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read ips whitelist entry after create", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *ipsWhitelistResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state ipsWhitelistResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetIPSWhitelistEntry(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.refresh(cur)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Update is unreachable: every attribute forces replacement, and the
// controller exposes no update verb for these entries.
func (r *ipsWhitelistResource) Update(_ context.Context, _ resource.UpdateRequest, resp *resource.UpdateResponse) {
	resp.Diagnostics.AddError(
		"IPS whitelist entries cannot be updated in place",
		"every attribute of an entry forces replacement; this is a provider bug if you see it",
	)
}

func (r *ipsWhitelistResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state ipsWhitelistResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteIPSWhitelistEntry(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete ips whitelist entry", err.Error())
	}
}

// ImportState takes the entry id, or "<site>/<id>".
func (r *ipsWhitelistResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
