// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

var _ datasource.DataSource = &wanDataSource{}
var _ datasource.DataSourceWithConfigure = &wanDataSource{}

func NewWANDataSource() datasource.DataSource { return &wanDataSource{} }

type wanDataSource struct {
	data *providerData
}

type wanPortModel struct {
	PortUUID     types.String `tfsdk:"port_uuid"`
	PortName     types.String `tfsdk:"port_name"`
	Proto        types.String `tfsdk:"proto"`
	VLANID       types.Int64  `tfsdk:"vlan_id"`
	QosTagEnable types.Bool   `tfsdk:"qos_tag_enable"`
	MTU          types.Int64  `tfsdk:"mtu"`
	MacMethod    types.String `tfsdk:"mac_method"`
	MacAddress   types.String `tfsdk:"mac_address"`
	IPv6Enable   types.Int64  `tfsdk:"ipv6_enable"`
}

type wanDataSourceModel struct {
	SiteID   types.String   `tfsdk:"site_id"`
	SiteName types.String   `tfsdk:"site"`
	Ports    []wanPortModel `tfsdk:"ports"`
}

func (d *wanDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wan"
}

func (d *wanDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Reads the gateway's WAN port configuration.\n\n" +
			"This is intentionally **read-only**. The controller's WAN endpoint is a single " +
			"large document mixing configuration with read-only capability flags, and a bad " +
			"write drops the site's internet connection — which makes the write path unsafe " +
			"to validate against a live gateway. Use this data source to assert on or " +
			"reference the current WAN settings; change them in the Omada UI.",
		Attributes: map[string]schema.Attribute{
			"site_id": schema.StringAttribute{
				MarkdownDescription: "Site ID to query. Provide this or `site`; if neither is set, the provider's default site is used.",
				Optional:            true,
				Computed:            true,
			},
			"site": schema.StringAttribute{
				MarkdownDescription: "Site name to query (resolved to an ID). Ignored if `site_id` is set.",
				Optional:            true,
			},
			"ports": schema.ListNestedAttribute{
				MarkdownDescription: "The gateway's WAN ports.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"port_uuid":      schema.StringAttribute{Computed: true, MarkdownDescription: "Controller UUID of the physical port."},
						"port_name":      schema.StringAttribute{Computed: true, MarkdownDescription: "Port name, e.g. `WAN/LAN1`."},
						"proto":          schema.StringAttribute{Computed: true, MarkdownDescription: "IPv4 connection type: `dhcp`, `pppoe`, `static`, ..."},
						"vlan_id":        schema.Int64Attribute{Computed: true, MarkdownDescription: "VLAN tag applied to the WAN link (0 = untagged)."},
						"qos_tag_enable": schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether an 802.1p QoS tag is set on the WAN link."},
						"mtu":            schema.Int64Attribute{Computed: true, MarkdownDescription: "MTU of the IPv4 connection."},
						"mac_method":     schema.StringAttribute{Computed: true, MarkdownDescription: "MAC address mode: `recover` (factory), `clone`, or `custom`."},
						"mac_address":    schema.StringAttribute{Computed: true, MarkdownDescription: "MAC address in use on the WAN port."},
						"ipv6_enable":    schema.Int64Attribute{Computed: true, MarkdownDescription: "Whether IPv6 is enabled on the port (0 = off)."},
					},
				},
			},
		},
	}
}

func (d *wanDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data",
			fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
		return
	}
	d.data = data
}

func (d *wanDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config wanDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID, diags := d.resolveSiteID(ctx, config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	ports, err := d.data.client.GetWANPorts(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read Omada WAN settings", err.Error())
		return
	}

	state := wanDataSourceModel{
		SiteID:   types.StringValue(siteID),
		SiteName: config.SiteName,
	}
	for _, p := range ports {
		state.Ports = append(state.Ports, wanPortModel{
			PortUUID:     types.StringValue(p.PortUUID),
			PortName:     types.StringValue(p.PortName),
			Proto:        types.StringValue(p.Proto),
			VLANID:       types.Int64Value(int64(p.VLANID)),
			QosTagEnable: types.BoolValue(p.QosTagEnable),
			MTU:          types.Int64Value(int64(p.MTU)),
			MacMethod:    types.StringValue(p.MacMethod),
			MacAddress:   types.StringValue(p.MacAddress),
			IPv6Enable:   types.Int64Value(int64(p.IPv6Enable)),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// resolveSiteID picks the site to query: explicit site_id, else site name,
// else the provider's default site.
func (d *wanDataSource) resolveSiteID(ctx context.Context, config wanDataSourceModel) (string, diag.Diagnostics) {
	var diags diag.Diagnostics
	if !config.SiteID.IsNull() && config.SiteID.ValueString() != "" {
		return config.SiteID.ValueString(), diags
	}
	name := d.data.defaultSite
	if !config.SiteName.IsNull() && config.SiteName.ValueString() != "" {
		name = config.SiteName.ValueString()
	}
	id, err := d.data.client.ResolveSiteID(ctx, name)
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return "", diags
	}
	return id, diags
}
