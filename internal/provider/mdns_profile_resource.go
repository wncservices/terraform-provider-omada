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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &mdnsProfileResource{}
	_ resource.ResourceWithConfigure   = &mdnsProfileResource{}
	_ resource.ResourceWithImportState = &mdnsProfileResource{}
)

func NewMDNSProfileResource() resource.Resource { return &mdnsProfileResource{} }

type mdnsProfileResource struct{ data *providerData }

type mdnsProfileResourceModel struct {
	ID         types.String `tfsdk:"id"`
	Site       types.String `tfsdk:"site"`
	SiteID     types.String `tfsdk:"site_id"`
	Name       types.String `tfsdk:"name"`
	ServiceIDs types.List   `tfsdk:"service_ids"`
	IsDefault  types.Bool   `tfsdk:"is_builtin"`
}

func (r *mdnsProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_mdns_profile"
}

func (r *mdnsProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a custom mDNS service profile: a named bundle of raw mDNS service strings " +
			"(e.g. `_matter._tcp.local`) that `omada_mdns_reflector.profile_ids` can reference alongside the " +
			"controller's read-only built-ins (`buildIn-1` .. `buildIn-10`).\n\n" +
			"The controller caps the number of custom profiles per site — 5 on the firmware this was developed " +
			"against. Exceeding it fails at apply time with the controller's own error, not a Terraform-side check.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Profile name."},
			"service_ids": schema.ListAttribute{
				ElementType: types.StringType, Required: true,
				MarkdownDescription: "Raw mDNS service strings this profile bundles, e.g. " +
					"`[\"_matter._tcp.local\", \"_matterc._udp.local\"]`. A single profile may bundle several " +
					"service strings — useful given the per-site cap above.",
			},
			"is_builtin": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is one of the controller's built-in profiles. Read-only."},
		},
	}
}

func (r *mdnsProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *mdnsProfileResource) siteName(m mdnsProfileResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *mdnsProfileResource) resolveSite(ctx context.Context, m mdnsProfileResourceModel, diags *diagSink) string {
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

func (m mdnsProfileResourceModel) toAPI(ctx context.Context) (omada.MDNSProfile, diag.Diagnostics) {
	ids, diags := stringSlice(ctx, m.ServiceIDs)
	return omada.MDNSProfile{
		Name:       m.Name.ValueString(),
		ServiceIDs: nilToEmpty(ids),
	}, diags
}

func (m *mdnsProfileResourceModel) refresh(ctx context.Context, p *omada.MDNSProfile) diag.Diagnostics {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.IsDefault = types.BoolValue(p.Default)
	list, diags := stringListValue(ctx, p.ServiceIDs)
	m.ServiceIDs = list
	return diags
}

func (r *mdnsProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan mdnsProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.data.client.CreateMDNSProfile(ctx, siteID, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create mdns profile", err.Error())
		return
	}
	cur, err := r.data.client.GetMDNSProfile(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read mdns profile after create", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.refresh(ctx, cur)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mdnsProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state mdnsProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetMDNSProfile(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(state.refresh(ctx, cur)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *mdnsProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state mdnsProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	in, diags := plan.toAPI(ctx)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.UpdateMDNSProfile(ctx, siteID, id, in); err != nil {
		resp.Diagnostics.AddError("Unable to update mdns profile", err.Error())
		return
	}
	cur, err := r.data.client.GetMDNSProfile(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read mdns profile after update", err.Error())
		return
	}
	resp.Diagnostics.Append(plan.refresh(ctx, cur)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *mdnsProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state mdnsProfileResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteMDNSProfile(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete mdns profile", err.Error())
	}
}

// ImportState takes the mdns-profile id, or "<site>/<id>".
func (r *mdnsProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
