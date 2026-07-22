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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &portProfileResource{}
	_ resource.ResourceWithConfigure   = &portProfileResource{}
	_ resource.ResourceWithImportState = &portProfileResource{}
)

func NewPortProfileResource() resource.Resource { return &portProfileResource{} }

type portProfileResource struct{ data *providerData }

type portProfileResourceModel struct {
	ID              types.String `tfsdk:"id"`
	Site            types.String `tfsdk:"site"`
	SiteID          types.String `tfsdk:"site_id"`
	Name            types.String `tfsdk:"name"`
	POE             types.Int64  `tfsdk:"poe"`
	NativeNetworkID types.String `tfsdk:"native_network_id"`
	TaggedIDs       types.List   `tfsdk:"tagged_network_ids"`
	UntaggedIDs     types.List   `tfsdk:"untagged_network_ids"`
	VLANConfig      types.Bool   `tfsdk:"vlan_config_enable"`
}

func (r *portProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_port_profile"
}

func (r *portProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a switch port profile on the Omada controller. A managed subset of fields is exposed; other fields on the profile are preserved on update.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Profile name."},
			"poe":     schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(2), MarkdownDescription: "PoE mode: 0=off, 1=on, 2=keep-device-setting."},
			"native_network_id": schema.StringAttribute{
				Optional: true, Computed: true, MarkdownDescription: "Untagged/native network (VLAN) ID for the port.",
			},
			"tagged_network_ids":   schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true, MarkdownDescription: "Tagged (trunk) network IDs."},
			"untagged_network_ids": schema.ListAttribute{ElementType: types.StringType, Optional: true, Computed: true, MarkdownDescription: "Additional untagged network IDs."},
			"vlan_config_enable":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Whether custom VLAN tagging is enabled on the profile."},
		},
	}
}

func (r *portProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func (r *portProfileResource) siteName(m portProfileResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *portProfileResource) fieldsFrom(ctx context.Context, m portProfileResourceModel) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	tagged, d := stringSlice(ctx, m.TaggedIDs)
	diags.Append(d...)
	untagged, d := stringSlice(ctx, m.UntaggedIDs)
	diags.Append(d...)
	fields := map[string]any{
		"name":             m.Name.ValueString(),
		"poe":              m.POE.ValueInt64(),
		"tagNetworkIds":    nilToEmpty(tagged),
		"untagNetworkIds":  nilToEmpty(untagged),
		"vlanConfigEnable": m.VLANConfig.ValueBool(),
	}
	if !m.NativeNetworkID.IsNull() && !m.NativeNetworkID.IsUnknown() && m.NativeNetworkID.ValueString() != "" {
		fields["nativeNetworkId"] = m.NativeNetworkID.ValueString()
	}
	return fields, diags
}

func (r *portProfileResource) apply(ctx context.Context, p *omada.PortProfile, m *portProfileResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.POE = types.Int64Value(int64(p.POE))
	m.NativeNetworkID = types.StringValue(p.NativeNetworkID)
	m.VLANConfig = types.BoolValue(p.VLANConfigEnable)
	tagged, d := stringListValue(ctx, p.TagNetworkIDs)
	diags.Append(d...)
	m.TaggedIDs = tagged
	untagged, d := stringListValue(ctx, p.UntagNetworkIDs)
	diags.Append(d...)
	m.UntaggedIDs = untagged
	return diags
}

func (r *portProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	fields, diags := r.fieldsFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.data.client.CreatePortProfile(ctx, siteID, fields)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create port profile", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portProfileResourceModel
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
	p, err := r.data.client.GetPortProfile(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, p, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *portProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	fields, diags := r.fieldsFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.data.client.UpdatePortProfile(ctx, siteID, plan.ID.ValueString(), fields)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update port profile", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeletePortProfile(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete port profile", err.Error())
	}
}

func (r *portProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
