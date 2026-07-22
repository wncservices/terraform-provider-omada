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
	_ resource.Resource                = &mdnsResource{}
	_ resource.ResourceWithConfigure   = &mdnsResource{}
	_ resource.ResourceWithImportState = &mdnsResource{}
)

func NewMDNSReflectorResource() resource.Resource { return &mdnsResource{} }

type mdnsResource struct{ data *providerData }

type mdnsResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Site        types.String `tfsdk:"site"`
	SiteID      types.String `tfsdk:"site_id"`
	Name        types.String `tfsdk:"name"`
	Enable      types.Bool   `tfsdk:"enable"`
	ProfileIDs  types.List   `tfsdk:"profile_ids"`
	ServiceVlan types.String `tfsdk:"service_vlan"`
	ClientVlan  types.String `tfsdk:"client_vlan"`
}

func (r *mdnsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mdns_reflector"
}

func (r *mdnsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an mDNS reflection rule (bridges mDNS/Bonjour discovery between a service VLAN and a client VLAN).",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Rule name."},
			"enable":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether the rule is enabled."},
			"profile_ids": schema.ListAttribute{
				ElementType: types.StringType, Required: true,
				MarkdownDescription: "AP profile IDs the reflection applies to (e.g. `buildIn-1`).",
			},
			"service_vlan": schema.StringAttribute{Required: true, MarkdownDescription: "VLAN ID where services are advertised."},
			"client_vlan":  schema.StringAttribute{Required: true, MarkdownDescription: "VLAN ID where clients discover the services."},
		},
	}
}

func (r *mdnsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func (r *mdnsResource) siteName(m mdnsResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *mdnsResource) inputFrom(ctx context.Context, m mdnsResourceModel) (*omada.MDNSInput, diag.Diagnostics) {
	profiles, diags := stringSlice(ctx, m.ProfileIDs)
	return &omada.MDNSInput{
		Name: m.Name.ValueString(), Status: m.Enable.ValueBool(), Type: 0,
		AP: omada.MDNSAPCfg{ProfileIDs: nilToEmpty(profiles), ServiceVlan: m.ServiceVlan.ValueString(), ClientVlan: m.ClientVlan.ValueString()},
	}, diags
}

func (r *mdnsResource) apply(ctx context.Context, n *omada.MDNSReflector, m *mdnsResourceModel) diag.Diagnostics {
	m.ID = types.StringValue(n.ID)
	m.Name = types.StringValue(n.Name)
	m.Enable = types.BoolValue(n.Status)
	m.ServiceVlan = types.StringValue(n.AP.ServiceVlan)
	m.ClientVlan = types.StringValue(n.AP.ClientVlan)
	list, diags := stringListValue(ctx, n.AP.ProfileIDs)
	m.ProfileIDs = list
	return diags
}

func (r *mdnsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mdnsResourceModel
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
	created, err := r.data.client.CreateMDNS(ctx, siteID, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create mDNS rule", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mdnsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mdnsResourceModel
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
	n, err := r.data.client.GetMDNS(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, n, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mdnsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan mdnsResourceModel
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
	updated, err := r.data.client.UpdateMDNS(ctx, siteID, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update mDNS rule", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mdnsResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mdnsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteMDNS(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete mDNS rule", err.Error())
	}
}

func (r *mdnsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
