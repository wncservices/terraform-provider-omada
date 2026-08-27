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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &clientAliasResource{}
	_ resource.ResourceWithConfigure   = &clientAliasResource{}
	_ resource.ResourceWithImportState = &clientAliasResource{}
)

func NewClientAliasResource() resource.Resource { return &clientAliasResource{} }

type clientAliasResource struct{ data *providerData }

type clientAliasResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`
	MAC    macValue     `tfsdk:"mac"`
	Alias  types.String `tfsdk:"alias"`
}

func (r *clientAliasResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_client_alias"
}

func (r *clientAliasResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the friendly display name (alias) of a known wired or wireless client.\n\n" +
			"The alias is attached to the client's MAC address and is independent of DHCP reservations. " +
			"The client may be online or offline, but it must already be known to the controller.\n\n" +
			"~> **A client cannot be created or destroyed.** Removing this resource from configuration " +
			"leaves the alias exactly as last applied; Terraform only stops managing it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "The client MAC address, normalised.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace(),
				},
			},
			"site_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"mac": schema.StringAttribute{
				CustomType: macType{},
				Required:   true,
				MarkdownDescription: "Client MAC address. Accepts `:`, `-` or `.` separators in any case. " +
					"Changing the address replaces the resource because aliases are keyed by MAC.",
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.RequiresReplaceIf(macChanged,
						"Replaces the alias resource when the MAC actually changes.",
						"Replaces the alias resource when the MAC actually changes."),
				},
			},
			"alias": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Friendly display name shown for the client in the controller.",
			},
		},
	}
}

func (r *clientAliasResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *clientAliasResource) siteName(m clientAliasResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *clientAliasResource) resolveSite(ctx context.Context, m clientAliasResourceModel, diags *diagSink) (string, string) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return "", ""
	}
	return site.ID, site.Name
}

func (m *clientAliasResourceModel) refresh(client *omada.ClientAlias) {
	m.ID = types.StringValue(omada.NormalizeMAC(client.MAC))
	if omada.NormalizeMAC(m.MAC.ValueString()) != omada.NormalizeMAC(client.MAC) {
		m.MAC = newMACValue(client.MAC)
	}
	m.Alias = types.StringValue(client.Name)
}

func (r *clientAliasResource) apply(ctx context.Context, plan *clientAliasResourceModel, diags *diagSink) {
	siteID, siteName := r.resolveSite(ctx, *plan, diags)
	if siteID == "" {
		return
	}
	mac := plan.MAC.ValueString()
	cur, err := r.data.client.GetClientAlias(ctx, siteID, mac)
	if err != nil {
		diags.AddError("Unable to read client", err.Error())
		return
	}
	if cur.Name != plan.Alias.ValueString() {
		if err := r.data.client.UpdateClientAlias(ctx, siteID, mac, plan.Alias.ValueString()); err != nil {
			diags.AddError("Unable to update client alias", err.Error())
			return
		}
		cur, err = r.data.client.GetClientAlias(ctx, siteID, mac)
		if err != nil {
			diags.AddError("Unable to read client after alias update", err.Error())
			return
		}
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	plan.Site = types.StringValue(siteName)
}

// Create adopts an existing known client and applies its alias.
func (r *clientAliasResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan clientAliasResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *clientAliasResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state clientAliasResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, siteName := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetClientAlias(ctx, siteID, state.MAC.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.refresh(cur)
	state.SiteID = types.StringValue(siteID)
	state.Site = types.StringValue(siteName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *clientAliasResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan clientAliasResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	r.apply(ctx, &plan, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

// Delete forgets the client without clearing its persistent alias.
func (r *clientAliasResource) Delete(_ context.Context, _ resource.DeleteRequest, resp *resource.DeleteResponse) {
	resp.Diagnostics.AddWarning(
		"Client alias left as configured",
		"A network client cannot be deleted, so Terraform has only stopped managing its alias. "+
			"The display name remains unchanged in the controller.",
	)
}

// ImportState takes the client MAC, or "<site>/<mac>".
func (r *clientAliasResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("mac"), newMACValue(omada.NormalizeMAC(id)))...)
}
