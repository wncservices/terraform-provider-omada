// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
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
	_ resource.Resource                = &dnsProxyResource{}
	_ resource.ResourceWithConfigure   = &dnsProxyResource{}
	_ resource.ResourceWithImportState = &dnsProxyResource{}
)

func NewDNSProxyResource() resource.Resource { return &dnsProxyResource{} }

type dnsProxyResource struct{ data *providerData }

type dnsProxyResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`

	Enable              types.Bool `tfsdk:"enable"`
	EnabledDefaultTypes types.List `tfsdk:"enabled_default_server_types"`
	CustomServers       types.List `tfsdk:"custom_server"`

	AvailableDefaultServers types.List  `tfsdk:"available_default_servers"`
	DoHServerLimit          types.Int64 `tfsdk:"doh_server_limit"`
	DoTServerLimit          types.Int64 `tfsdk:"dot_server_limit"`
	SupportsDNSOverride     types.Bool  `tfsdk:"supports_dns_override"`
}

func (r *dnsProxyResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_dns_proxy"
}

func (r *dnsProxyResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages the site's DNS proxy and DNS-over-HTTPS resolvers.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"~> **This decides who sees every DNS query the network makes.** DoH stops your ISP and " +
			"anyone on the path from reading them — and hands that same visibility to whoever runs the " +
			"resolver you point at. Choosing a resolver is choosing who to trust with that log, so " +
			"prefer one whose retention policy you have actually read, or run your own.\n\n" +
			"The controller keeps two server lists and they are **not** the same kind of thing. " +
			"`enabled_default_server_types` selects from a fixed list built into the firmware, which " +
			"this resource never rewrites — see `available_default_servers` for what it offers. " +
			"`custom_server` blocks are yours and are managed in full.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				Optional: true, Computed: true,
				PlanModifiers: []planmodifier.String{
					stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace(),
				},
				MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.",
			},
			"site_id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Whether the gateway's DNS proxy is active. With this off, the " +
					"resolver lists below have no effect.",
			},
			"enabled_default_server_types": schema.ListAttribute{
				ElementType: types.Int64Type,
				Optional:    true, Computed: true,
				MarkdownDescription: "Controller `type` values of the firmware-provided DoH resolvers to " +
					"enable. The list of available types is fixed by the firmware and reported in " +
					"`available_default_servers`; a type that is not in it is an error, because the " +
					"controller would accept the write and silently drop it.",
			},
			"available_default_servers": schema.ListNestedAttribute{
				Computed: true,
				MarkdownDescription: "The firmware's DoH resolvers as the controller reports them. " +
					"Read-only; use it to find the `type` values for `enabled_default_server_types`.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"type":    schema.Int64Attribute{Computed: true, MarkdownDescription: "Controller enum identifying the resolver."},
						"enabled": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether it is currently on."},
					},
				},
			},
			"doh_server_limit": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximum customised DoH servers this controller accepts. Read-only.",
			},
			"dot_server_limit": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Maximum DNS-over-TLS servers. Read-only — DoT itself has no endpoint on this firmware.",
			},
			"supports_dns_override": schema.BoolAttribute{
				Computed:            true,
				MarkdownDescription: "Whether the gateway can override client-specified DNS. Read-only.",
			},
		},
		Blocks: map[string]schema.Block{
			"custom_server": schema.ListNestedBlock{
				MarkdownDescription: "A customised DoH resolver. Declared in order; the controller stores " +
					"the list as given.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"name": schema.StringAttribute{
							Required:            true,
							MarkdownDescription: "Label for this resolver.",
						},
						"enable": schema.BoolAttribute{
							Optional: true, Computed: true, Default: booldefault.StaticBool(true),
							MarkdownDescription: "Whether this resolver is used.",
						},
						"urls": schema.ListAttribute{
							ElementType:         types.StringType,
							Required:            true,
							MarkdownDescription: "DoH endpoint URLs, e.g. `https://dns.example.com/dns-query`. May not be empty.",
						},
					},
				},
			},
		},
	}
}

func (r *dnsProxyResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *dnsProxyResource) siteName(m dnsProxyResourceModel) string {
	if !m.Site.IsNull() && !m.Site.IsUnknown() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

var dohDefaultObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"type":    types.Int64Type,
	"enabled": types.BoolType,
}}

var dohCustomObjectType = types.ObjectType{AttrTypes: map[string]attr.Type{
	"name":   types.StringType,
	"enable": types.BoolType,
	"urls":   types.ListType{ElemType: types.StringType},
}}

func dohDefaultsValue(servers []omada.DoHDefaultServer) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	vals := make([]attr.Value, 0, len(servers))
	for _, s := range servers {
		obj, d := types.ObjectValue(dohDefaultObjectType.AttrTypes, map[string]attr.Value{
			"type":    types.Int64Value(int64(s.Type)),
			"enabled": types.BoolValue(s.Enable),
		})
		diags.Append(d...)
		vals = append(vals, obj)
	}
	lv, d := types.ListValue(dohDefaultObjectType, vals)
	diags.Append(d...)
	return lv, diags
}

func dohCustomValue(ctx context.Context, servers []omada.DoHCustomServer) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	vals := make([]attr.Value, 0, len(servers))
	for _, s := range servers {
		urls, d := stringListValue(ctx, s.Servers)
		diags.Append(d...)
		obj, d2 := types.ObjectValue(dohCustomObjectType.AttrTypes, map[string]attr.Value{
			"name":   types.StringValue(s.Name),
			"enable": types.BoolValue(s.Enable),
			"urls":   urls,
		})
		diags.Append(d2...)
		vals = append(vals, obj)
	}
	lv, d := types.ListValue(dohCustomObjectType, vals)
	diags.Append(d...)
	return lv, diags
}

// customServersFrom reads the custom_server blocks out of the plan.
func customServersFrom(ctx context.Context, l types.List, sink *diagSink) []omada.DoHCustomServer {
	if l.IsNull() || l.IsUnknown() {
		return nil
	}
	var rows []struct {
		Name   types.String `tfsdk:"name"`
		Enable types.Bool   `tfsdk:"enable"`
		URLs   types.List   `tfsdk:"urls"`
	}
	if d := l.ElementsAs(ctx, &rows, false); d.HasError() {
		sink.AddError("Invalid custom_server", "could not read the customised server blocks")
		return nil
	}
	out := make([]omada.DoHCustomServer, 0, len(rows))
	for _, row := range rows {
		urls, d := stringSlice(ctx, row.URLs)
		if d.HasError() {
			sink.AddError("Invalid custom_server urls", "could not read the url list")
			return nil
		}
		out = append(out, omada.DoHCustomServer{
			Name:    row.Name.ValueString(),
			Enable:  row.Enable.ValueBool(),
			Servers: urls,
		})
	}
	return out
}

func (r *dnsProxyResource) refresh(ctx context.Context, s *omada.DNSProxy, m *dnsProxyResourceModel, siteID, siteName string, sink *diagSink) {
	m.ID = types.StringValue(siteID)
	m.SiteID = types.StringValue(siteID)
	m.Site = types.StringValue(siteName)
	m.Enable = types.BoolValue(s.Enable)
	m.DoHServerLimit = types.Int64Value(int64(s.DoHServerLimit))
	m.DoTServerLimit = types.Int64Value(int64(s.DoTServerLimit))
	m.SupportsDNSOverride = types.BoolValue(s.SupportDNSOverride)

	types_, d := intListValue(ctx, s.EnabledDefaultTypes)
	if d.HasError() {
		sink.AddError("Unable to read enabled default server types", "converting the type list failed")
	}
	m.EnabledDefaultTypes = types_

	defaults, d2 := dohDefaultsValue(s.DefaultServers)
	if d2.HasError() {
		sink.AddError("Unable to read available default servers", "converting the server list failed")
	}
	m.AvailableDefaultServers = defaults

	custom, d3 := dohCustomValue(ctx, s.CustomServers)
	if d3.HasError() {
		sink.AddError("Unable to read customised servers", "converting the server list failed")
	}
	m.CustomServers = custom
}

func (r *dnsProxyResource) apply(ctx context.Context, plan *dnsProxyResourceModel, sink *diagSink) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(*plan))
	if err != nil {
		sink.AddError("Unable to resolve site", err.Error())
		return
	}
	siteID, siteName := site.ID, site.Name

	typeList, d := intSlice(ctx, plan.EnabledDefaultTypes)
	if d.HasError() {
		sink.AddError("Invalid enabled_default_server_types", "could not read the type list")
		return
	}
	custom := customServersFrom(ctx, plan.CustomServers, sink)
	if sink.d.HasError() {
		return
	}

	in := omada.DNSProxy{
		Enable:              plan.Enable.ValueBool(),
		EnabledDefaultTypes: typeList,
		CustomServers:       custom,
	}
	if err := r.data.client.UpdateDNSProxy(ctx, siteID, in); err != nil {
		sink.AddError("Unable to update dns proxy settings", err.Error())
		return
	}
	cur, err := r.data.client.GetDNSProxy(ctx, siteID)
	if err != nil {
		sink.AddError("Unable to read dns proxy settings after update", err.Error())
		return
	}
	r.refresh(ctx, cur, plan, siteID, siteName, sink)
}

func (r *dnsProxyResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan dnsProxyResourceModel
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

func (r *dnsProxyResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state dnsProxyResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	site, err := r.data.client.ResolveSite(ctx, r.siteName(state))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	cur, err := r.data.client.GetDNSProxy(ctx, site.ID)
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.refresh(ctx, cur, &state, site.ID, site.Name, &diagSink{&resp.Diagnostics})
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *dnsProxyResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan dnsProxyResourceModel
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

// Delete drops the singleton from state; there is nothing to remove.
func (r *dnsProxyResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the site name.
func (r *dnsProxyResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), req.ID)...)
}
