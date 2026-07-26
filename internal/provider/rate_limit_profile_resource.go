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
	_ resource.Resource                   = &rateLimitProfileResource{}
	_ resource.ResourceWithConfigure      = &rateLimitProfileResource{}
	_ resource.ResourceWithImportState    = &rateLimitProfileResource{}
	_ resource.ResourceWithValidateConfig = &rateLimitProfileResource{}
)

func NewRateLimitProfileResource() resource.Resource { return &rateLimitProfileResource{} }

type rateLimitProfileResource struct{ data *providerData }

type rateLimitProfileModel struct {
	ID         types.String `tfsdk:"id"`
	Site       types.String `tfsdk:"site"`
	SiteID     types.String `tfsdk:"site_id"`
	Name       types.String `tfsdk:"name"`
	DownEnable types.Bool   `tfsdk:"download_limit_enable"`
	DownLimit  types.Int64  `tfsdk:"download_limit"`
	UpEnable   types.Bool   `tfsdk:"upload_limit_enable"`
	UpLimit    types.Int64  `tfsdk:"upload_limit"`
	IsBuiltin  types.Bool   `tfsdk:"is_builtin"`
}

func (r *rateLimitProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_rate_limit_profile"
}

func (r *rateLimitProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a rate-limit profile: a per-client throughput cap that SSIDs and portal " +
			"authentication can reference.\n\n" +
			"The controller ships a built-in `Default` profile, marked `is_builtin`. Manage custom " +
			"profiles here rather than the built-in one.\n\n" +
			"A limit only exists on the controller while its enable flag is set — the document omits " +
			"`download_limit` entirely when `download_limit_enable` is false — so setting a limit " +
			"without enabling it has no effect, and the provider rejects that at plan time rather " +
			"than writing a value the controller will drop.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Profile name."},

			"download_limit_enable": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Cap download throughput."},
			"download_limit":        schema.Int64Attribute{Optional: true, MarkdownDescription: "Download cap, in the controller's units (the UI presents Kbps). Only meaningful with `download_limit_enable`."},
			"upload_limit_enable":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Cap upload throughput."},
			"upload_limit":          schema.Int64Attribute{Optional: true, MarkdownDescription: "Upload cap, in the controller's units (the UI presents Kbps). Only meaningful with `upload_limit_enable`."},

			"is_builtin": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the controller's built-in profile. Read-only."},
		},
	}
}

// ValidateConfig rejects a limit set without its enable flag. The controller
// silently drops such a value, which would otherwise show up as a permanent
// diff between what was configured and what came back.
func (r *rateLimitProfileResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg rateLimitProfileModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	check := func(enable types.Bool, limit types.Int64, enableAttr, limitAttr string) {
		if limit.IsNull() || limit.IsUnknown() {
			return
		}
		if enable.IsNull() || !enable.ValueBool() {
			resp.Diagnostics.AddAttributeError(path.Root(limitAttr),
				fmt.Sprintf("%s requires %s", limitAttr, enableAttr),
				fmt.Sprintf("the controller stores %s only while %s is true, and drops it otherwise", limitAttr, enableAttr))
		}
	}
	check(cfg.DownEnable, cfg.DownLimit, "download_limit_enable", "download_limit")
	check(cfg.UpEnable, cfg.UpLimit, "upload_limit_enable", "upload_limit")
}

func (r *rateLimitProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *rateLimitProfileResource) siteName(m rateLimitProfileModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *rateLimitProfileResource) resolveSite(ctx context.Context, m rateLimitProfileModel, diags *diagSink) string {
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

func (m rateLimitProfileModel) toAPI() omada.RateLimitProfile {
	return omada.RateLimitProfile{
		Name:       m.Name.ValueString(),
		DownEnable: m.DownEnable.ValueBool(),
		DownLimit:  intPtrFrom(m.DownLimit),
		UpEnable:   m.UpEnable.ValueBool(),
		UpLimit:    intPtrFrom(m.UpLimit),
	}
}

func (m *rateLimitProfileModel) refresh(p *omada.RateLimitProfile) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.DownEnable = types.BoolValue(p.DownEnable)
	m.UpEnable = types.BoolValue(p.UpEnable)
	m.IsBuiltin = types.BoolValue(p.Default)
	// Absent on the controller means null here, not zero: a disabled limit has
	// no value at all.
	m.DownLimit = types.Int64Null()
	if p.DownLimit != nil {
		m.DownLimit = types.Int64Value(int64(*p.DownLimit))
	}
	m.UpLimit = types.Int64Null()
	if p.UpLimit != nil {
		m.UpLimit = types.Int64Value(int64(*p.UpLimit))
	}
}

func (r *rateLimitProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan rateLimitProfileModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.data.client.CreateRateLimitProfile(ctx, siteID, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create rate limit profile", err.Error())
		return
	}
	cur, err := r.data.client.GetRateLimitProfile(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read rate limit profile after create", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *rateLimitProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state rateLimitProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetRateLimitProfile(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.refresh(cur)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *rateLimitProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state rateLimitProfileModel
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
	if err := r.data.client.UpdateRateLimitProfile(ctx, siteID, id, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Unable to update rate limit profile", err.Error())
		return
	}
	cur, err := r.data.client.GetRateLimitProfile(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read rate limit profile after update", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *rateLimitProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state rateLimitProfileModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteRateLimitProfile(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete rate limit profile", err.Error())
	}
}

// ImportState takes the profile id, or "<site>/<id>".
func (r *rateLimitProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
