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
	_ resource.Resource                = &vpnResource{}
	_ resource.ResourceWithConfigure   = &vpnResource{}
	_ resource.ResourceWithImportState = &vpnResource{}
)

func NewVPNResource() resource.Resource { return &vpnResource{} }

type vpnResource struct{ data *providerData }

type vpnResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Site    types.String `tfsdk:"site"`
	SiteID  types.String `tfsdk:"site_id"`
	Name    types.String `tfsdk:"name"`
	Enable  types.Bool   `tfsdk:"enable"`
	Purpose types.Int64  `tfsdk:"purpose"`
}

func (r *vpnResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_vpn"
}

func (r *vpnResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a VPN on the Omada gateway. **Only a small subset (name, enable) is managed** via read-modify-write; all other VPN settings are preserved. Its write verbs are inferred (not live-validated) — prefer importing an existing VPN and toggling `enable` over creating/deleting.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "VPN name."},
			"enable":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether the VPN is enabled."},
			"purpose": schema.Int64Attribute{Computed: true, MarkdownDescription: "VPN purpose code (read-only)."},
		},
	}
}

func (r *vpnResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func (r *vpnResource) siteName(m vpnResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *vpnResource) fields(m vpnResourceModel) map[string]any {
	return map[string]any{"name": m.Name.ValueString(), "status": m.Enable.ValueBool()}
}

func (r *vpnResource) apply(v *omada.VPN, m *vpnResourceModel) {
	m.ID = types.StringValue(v.ID)
	m.Name = types.StringValue(v.Name)
	m.Enable = types.BoolValue(v.Status)
	m.Purpose = types.Int64Value(int64(v.Purpose))
}

func (r *vpnResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan vpnResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	created, err := r.data.client.CreateVPN(ctx, siteID, r.fields(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create VPN", err.Error())
		return
	}
	r.apply(created, &plan)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vpnResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state vpnResourceModel
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
	v, err := r.data.client.GetVPN(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.apply(v, &state)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *vpnResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan vpnResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	updated, err := r.data.client.UpdateVPN(ctx, siteID, plan.ID.ValueString(), r.fields(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to update VPN", err.Error())
		return
	}
	r.apply(updated, &plan)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *vpnResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state vpnResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteVPN(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete VPN", err.Error())
	}
}

func (r *vpnResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
