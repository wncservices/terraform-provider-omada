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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &radiusProfileResource{}
	_ resource.ResourceWithConfigure   = &radiusProfileResource{}
	_ resource.ResourceWithImportState = &radiusProfileResource{}
)

func NewRadiusProfileResource() resource.Resource { return &radiusProfileResource{} }

type radiusProfileResource struct{ data *providerData }

type radiusAuthServerModel struct {
	IP     types.String `tfsdk:"ip"`
	Port   types.Int64  `tfsdk:"port"`
	Secret types.String `tfsdk:"shared_secret"`
	RadSec types.Bool   `tfsdk:"radsec_enable"`
}

type radiusProfileResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`
	Name   types.String `tfsdk:"name"`

	AuthServers []radiusAuthServerModel `tfsdk:"auth_server"`

	Accounting     types.Bool `tfsdk:"accounting_enable"`
	InterimUpdate  types.Bool `tfsdk:"interim_update_enable"`
	VlanAssignment types.Bool `tfsdk:"wireless_vlan_assignment"`
	DomainEnable   types.Bool `tfsdk:"domain_enable"`
	CoaEnable      types.Bool `tfsdk:"coa_enable"`
	Proxy          types.Bool `tfsdk:"proxy"`
	RequireMsgAuth types.Bool `tfsdk:"require_message_authenticator"`
	TunnelReply    types.Bool `tfsdk:"tunnel_reply_enable"`

	BuiltInServer types.Bool `tfsdk:"built_in_server"`
}

func (r *radiusProfileResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_radius_profile"
}

func (r *radiusProfileResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a RADIUS profile: the servers that 802.1X, MAC authentication and " +
			"WPA-Enterprise SSIDs authenticate against. `omada_dot1x` is not usable without one.\n\n" +
			"~> **`shared_secret` is write-only.** The controller returns RADIUS secrets in " +
			"**plaintext**, which is exactly why this provider never reads them back into state — " +
			"the same rule the WiFi `psk` follows. A secret you do not re-supply is preserved on " +
			"update rather than blanked, and it cannot round-trip through `terraform import`.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Profile name."},

			"accounting_enable":             schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Enable RADIUS accounting."},
			"interim_update_enable":         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Send interim accounting updates."},
			"wireless_vlan_assignment":      schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Let RADIUS assign the wireless client's VLAN."},
			"domain_enable":                 schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Use a domain name rather than an address for the server."},
			"coa_enable":                    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Accept RADIUS Change-of-Authorization messages."},
			"proxy":                         schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Act as a RADIUS proxy."},
			"require_message_authenticator": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Require the Message-Authenticator attribute. Recommended: it mitigates the BlastRADIUS class of attack."},
			"tunnel_reply_enable":           schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Honour tunnel attributes in the RADIUS reply."},

			"built_in_server": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether this is the controller's built-in RADIUS server. Read-only."},
		},
		Blocks: map[string]schema.Block{
			"auth_server": schema.ListNestedBlock{
				MarkdownDescription: "Authentication servers, in priority order.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"ip":   schema.StringAttribute{Required: true, MarkdownDescription: "Server address."},
						"port": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1812), MarkdownDescription: "Authentication port. Defaults to `1812`."},
						"shared_secret": schema.StringAttribute{
							Optional:  true,
							Sensitive: true,
							// A genuine Terraform write-only attribute: the value is
							// available to the provider during apply but is never
							// persisted to state or plan. Marking it merely Sensitive
							// would still write it to the state file, since Terraform
							// stores configured values — which is exactly the leak
							// this resource must not have.
							WriteOnly: true,
							MarkdownDescription: "Shared secret. **Write-only**: supplied on apply and never " +
								"persisted to state or plan. Omit it on a later apply to keep the secret " +
								"already configured on the controller.",
						},
						"radsec_enable": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Wrap RADIUS in TLS (RadSec)."},
					},
				},
			},
		},
	}
}

func (r *radiusProfileResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *radiusProfileResource) siteName(m radiusProfileResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

// fields renders the plan as the controller's JSON shape.
//
// Secrets come from cfg, not from the plan: write-only attributes are null
// everywhere except the configuration. A server whose shared_secret is unset
// contributes no radiusPwd at all, which is what lets the client carry the
// existing one across an update.
func (m radiusProfileResourceModel) fields(cfg *radiusProfileResourceModel) map[string]any {
	servers := make([]any, 0, len(m.AuthServers))
	for i, s := range m.AuthServers {
		entry := map[string]any{
			"radiusServerIp": s.IP.ValueString(),
			"radiusPort":     s.Port.ValueInt64(),
			"radSecEnable":   s.RadSec.ValueBool(),
		}
		if cfg != nil && i < len(cfg.AuthServers) {
			if secret := cfg.AuthServers[i].Secret; !secret.IsNull() && secret.ValueString() != "" {
				entry["radiusPwd"] = secret.ValueString()
			}
		}
		servers = append(servers, entry)
	}
	return map[string]any{
		"name":                        m.Name.ValueString(),
		"authServer":                  servers,
		"radiusAccountingEnable":      m.Accounting.ValueBool(),
		"interimUpdateEnable":         m.InterimUpdate.ValueBool(),
		"wirelessVlanAssignment":      m.VlanAssignment.ValueBool(),
		"domainEnable":                m.DomainEnable.ValueBool(),
		"coaEnable":                   m.CoaEnable.ValueBool(),
		"proxy":                       m.Proxy.ValueBool(),
		"requireMessageAuthenticator": m.RequireMsgAuth.ValueBool(),
		"tunnelReplyEnable":           m.TunnelReply.ValueBool(),
	}
}

// refresh fills the model from the controller. Secrets are never returned by
// the client, so the values already in the model are carried forward by index.
func (m *radiusProfileResourceModel) refresh(p *omada.RadiusProfileSummary) {
	m.ID = types.StringValue(p.ID)
	m.Name = types.StringValue(p.Name)
	m.Accounting = types.BoolValue(p.Accounting)
	m.InterimUpdate = types.BoolValue(p.InterimUpd)
	m.VlanAssignment = types.BoolValue(p.VlanAssign)
	m.DomainEnable = types.BoolValue(p.DomainEnable)
	m.CoaEnable = types.BoolValue(p.CoaEnable)
	m.Proxy = types.BoolValue(p.Proxy)
	m.RequireMsgAuth = types.BoolValue(p.RequireMsgAu)
	m.TunnelReply = types.BoolValue(p.TunnelReply)
	m.BuiltInServer = types.BoolValue(p.BuiltIn)

	servers := make([]radiusAuthServerModel, 0, len(p.AuthServers))
	for _, s := range p.AuthServers {
		servers = append(servers, radiusAuthServerModel{
			IP:   types.StringValue(s.IP),
			Port: types.Int64Value(int64(s.Port)),
			// Always null: write-only attributes are never stored.
			Secret: types.StringNull(),
			RadSec: types.BoolValue(s.RadSecEnabl),
		})
	}
	m.AuthServers = servers
}

func (r *radiusProfileResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan, cfg radiusProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	// Write-only secrets live only in the configuration.
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	id, err := r.data.client.CreateRadiusProfile(ctx, siteID, plan.fields(&cfg))
	if err != nil {
		resp.Diagnostics.AddError("Unable to create radius profile", err.Error())
		return
	}
	cur, err := r.data.client.GetRadiusProfile(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read radius profile after create", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *radiusProfileResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state radiusProfileResourceModel
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
	cur, err := r.data.client.GetRadiusProfile(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.refresh(cur)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *radiusProfileResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state, cfg radiusProfileResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	// Write-only secrets live only in the configuration.
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
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
	id := state.ID.ValueString()
	if err := r.data.client.UpdateRadiusProfile(ctx, siteID, id, plan.fields(&cfg)); err != nil {
		resp.Diagnostics.AddError("Unable to update radius profile", err.Error())
		return
	}
	cur, err := r.data.client.GetRadiusProfile(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read radius profile after update", err.Error())
		return
	}
	plan.refresh(cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *radiusProfileResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state radiusProfileResourceModel
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
	if err := r.data.client.DeleteRadiusProfile(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete radius profile", err.Error())
	}
}

// ImportState takes the profile id, or "<site>/<id>". Shared secrets cannot be
// imported — the provider refuses to read them — so re-supply them in config.
func (r *radiusProfileResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
