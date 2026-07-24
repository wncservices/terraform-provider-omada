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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &portalResource{}
	_ resource.ResourceWithConfigure   = &portalResource{}
	_ resource.ResourceWithImportState = &portalResource{}
)

func NewPortalResource() resource.Resource { return &portalResource{} }

type portalResource struct{ data *providerData }

type portalResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Site        types.String `tfsdk:"site"`
	SiteID      types.String `tfsdk:"site_id"`
	Name        types.String `tfsdk:"name"`
	Enable      types.Bool   `tfsdk:"enable"`
	AuthType    types.Int64  `tfsdk:"auth_type"`
	Password    types.String `tfsdk:"password"`
	SSIDIDs     types.List   `tfsdk:"ssid_ids"`
	NetworkIDs  types.List   `tfsdk:"network_ids"`
	AuthTimeout types.Int64  `tfsdk:"auth_timeout"`
	HTTPSRedir  types.Bool   `tfsdk:"https_redirect"`
	LandingPage types.Int64  `tfsdk:"landing_page"`
}

func (r *portalResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_portal"
}

func (r *portalResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a captive portal on the Omada controller (Settings → Authentication → Portal).\n\n" +
			"Bind it to wireless clients with `ssid_ids` and/or to a whole wired network with `network_ids`. " +
			"The landing-page design (logo, colours, terms of service, background) is **not** modelled: it is preserved " +
			"on update via read-modify-write, so you can style the page in the controller UI without Terraform reverting it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.",
				Optional:            true,
				PlanModifiers:       []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"site_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"name": schema.StringAttribute{
				MarkdownDescription: "Portal name.",
				Required:            true,
			},
			"enable": schema.BoolAttribute{
				MarkdownDescription: "Whether the portal is active. While enabled, clients on the bound SSIDs/networks must authenticate before reaching the internet.",
				Optional:            true, Computed: true,
				Default: booldefault.StaticBool(true),
			},
			"auth_type": schema.Int64Attribute{
				MarkdownDescription: "Authentication type: `0` = no auth (click-through), `1` = simple password (one shared password).",
				Optional:            true, Computed: true,
				Default: int64default.StaticInt64(omada.PortalAuthNone),
			},
			"password": schema.StringAttribute{
				MarkdownDescription: "Shared password for `auth_type = 1`. **Write-only**: the controller never returns it, " +
					"so it is never read into Terraform state and drift on it cannot be detected. Supply it from a secret " +
					"store (e.g. Vault) rather than hard-coding it. Changing the value pushes the new password.",
				Optional:  true,
				Sensitive: true,
			},
			// The Optional+Computed attributes below carry UseStateForUnknown so
			// that leaving them out of the config keeps whatever the controller
			// already has, instead of planning a spurious "known after apply"
			// change on every run.
			"ssid_ids": schema.ListAttribute{
				MarkdownDescription: "IDs of the SSIDs this portal gates (see the `omada_wireless_network` resource).",
				ElementType:         types.StringType,
				Optional:            true, Computed: true,
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
			"network_ids": schema.ListAttribute{
				MarkdownDescription: "IDs of the LAN networks this portal gates (see the `omada_network` resource).",
				ElementType:         types.StringType,
				Optional:            true, Computed: true,
				PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
			},
			"auth_timeout": schema.Int64Attribute{
				MarkdownDescription: "Session timeout before a client must re-authenticate (controller units; the UI's preset selector).",
				Optional:            true, Computed: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
			"https_redirect": schema.BoolAttribute{
				MarkdownDescription: "Whether to redirect HTTPS requests to the portal page.",
				Optional:            true, Computed: true,
				PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()},
			},
			"landing_page": schema.Int64Attribute{
				MarkdownDescription: "Where the client lands after authenticating (`1` = the originally requested URL).",
				Optional:            true, Computed: true,
				PlanModifiers: []planmodifier.Int64{int64planmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *portalResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func (r *portalResource) siteName(m portalResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

// fieldsFrom builds the controller payload from the plan. Only attributes the
// user actually set are sent, so controller defaults and the landing-page design
// are left alone.
func (r *portalResource) fieldsFrom(ctx context.Context, m portalResourceModel) (map[string]any, diag.Diagnostics) {
	var diags diag.Diagnostics
	f := map[string]any{
		"name":     m.Name.ValueString(),
		"enable":   m.Enable.ValueBool(),
		"authType": int(m.AuthType.ValueInt64()),
	}
	if !m.SSIDIDs.IsNull() && !m.SSIDIDs.IsUnknown() {
		ids, d := stringSlice(ctx, m.SSIDIDs)
		diags.Append(d...)
		f["ssidList"] = nilToEmpty(ids)
	}
	if !m.NetworkIDs.IsNull() && !m.NetworkIDs.IsUnknown() {
		ids, d := stringSlice(ctx, m.NetworkIDs)
		diags.Append(d...)
		f["networkList"] = nilToEmpty(ids)
	}
	if !m.AuthTimeout.IsNull() && !m.AuthTimeout.IsUnknown() {
		f["authTimeout"] = map[string]any{"authTimeout": int(m.AuthTimeout.ValueInt64())}
	}
	if !m.HTTPSRedir.IsNull() && !m.HTTPSRedir.IsUnknown() {
		f["httpsRedirectEnable"] = m.HTTPSRedir.ValueBool()
	}
	if !m.LandingPage.IsNull() && !m.LandingPage.IsUnknown() {
		f["landingPage"] = int(m.LandingPage.ValueInt64())
	}
	return f, diags
}

// apply copies controller state back into the model. `password` is deliberately
// never touched — it is write-only and stays whatever the config says.
func (r *portalResource) apply(ctx context.Context, p *omada.Portal, m *portalResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Enable = types.BoolValue(p.Enable)
	m.AuthType = types.Int64Value(int64(p.AuthType))
	m.AuthTimeout = types.Int64Value(int64(p.AuthTimeout.AuthTimeout))
	m.HTTPSRedir = types.BoolValue(p.HTTPSRedir)
	m.LandingPage = types.Int64Value(int64(p.LandingPage))

	ssids, d := stringListValue(ctx, p.SSIDList)
	diags.Append(d...)
	m.SSIDIDs = ssids

	nets, d := stringListValue(ctx, p.NetworkList)
	diags.Append(d...)
	m.NetworkIDs = nets
	return diags
}

func (r *portalResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan portalResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if plan.AuthType.ValueInt64() == omada.PortalAuthSimplePassword && plan.Password.ValueString() == "" {
		resp.Diagnostics.AddAttributeError(path.Root("password"), "Missing portal password",
			"auth_type = 1 (simple password) requires `password` to be set, otherwise the portal would accept an empty password.")
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
	created, err := r.data.client.CreatePortal(ctx, siteID, fields, plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create portal", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portalResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state portalResourceModel
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
	p, err := r.data.client.GetPortal(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, p, &state)...)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *portalResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan portalResourceModel
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
	updated, err := r.data.client.UpdatePortal(ctx, siteID, plan.ID.ValueString(), fields, plan.Password.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update portal", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *portalResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state portalResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeletePortal(ctx, state.SiteID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete portal", err.Error())
	}
}

// ImportState accepts "<id>" or "<site_name>/<id>". Note that `password` cannot
// be imported (the controller never returns it) — set it in config after import.
func (r *portalResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.SplitN(req.ID, "/", 2)
	if len(parts) == 2 {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
}
