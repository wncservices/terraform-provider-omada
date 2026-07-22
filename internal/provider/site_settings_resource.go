// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var (
	_ resource.Resource                = &siteSettingsResource{}
	_ resource.ResourceWithConfigure   = &siteSettingsResource{}
	_ resource.ResourceWithImportState = &siteSettingsResource{}
)

func NewSiteSettingsResource() resource.Resource { return &siteSettingsResource{} }

type siteSettingsResource struct{ data *providerData }

type siteSettingsResourceModel struct {
	ID        types.String `tfsdk:"id"`
	Site      types.String `tfsdk:"site"`
	LEDEnable types.Bool   `tfsdk:"led_enable"`
}

func (r *siteSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_settings"
}

func (r *siteSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages site-level settings (a singleton). Currently exposes the device status-LED toggle; other site settings are preserved (read-modify-write). Destroying this resource does not reset anything.",
		Attributes: map[string]schema.Attribute{
			"id":         schema.StringAttribute{Computed: true, MarkdownDescription: "The site ID.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":       schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"led_enable": schema.BoolAttribute{Required: true, MarkdownDescription: "Whether device status LEDs are on."},
		},
	}
}

func (r *siteSettingsResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func (r *siteSettingsResource) siteName(m siteSettingsResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *siteSettingsResource) write(ctx context.Context, m *siteSettingsResourceModel, resp *diagSink) {
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(*m))
	if err != nil {
		resp.AddError("Unable to resolve site", err.Error())
		return
	}
	if err := r.data.client.SetLEDEnable(ctx, siteID, m.LEDEnable.ValueBool()); err != nil {
		resp.AddError("Unable to update site settings", err.Error())
		return
	}
	m.ID = types.StringValue(siteID)
}

func (r *siteSettingsResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan siteSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *siteSettingsResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state siteSettingsResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := state.ID.ValueString()
	if siteID == "" {
		var err error
		if siteID, err = r.data.client.ResolveSiteID(ctx, r.siteName(state)); err != nil {
			resp.Diagnostics.AddError("Unable to resolve site", err.Error())
			return
		}
	}
	led, err := r.data.client.GetLEDEnable(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read site settings", err.Error())
		return
	}
	state.ID = types.StringValue(siteID)
	state.LEDEnable = types.BoolValue(led)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *siteSettingsResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan siteSettingsResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.write(ctx, &plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete is a no-op: site settings always exist and are not reset on destroy.
func (r *siteSettingsResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the site name, e.g.
//
//	terraform import omada_site_settings.this Home
//
// A singleton has no controller-side id of its own, so the site name is the
// import key; Read resolves it to the site id.
func (r *siteSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected the site name, e.g. `terraform import omada_site_settings.this Home`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), req.ID)...)
}
