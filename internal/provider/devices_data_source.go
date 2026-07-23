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

var _ datasource.DataSource = &devicesDataSource{}
var _ datasource.DataSourceWithConfigure = &devicesDataSource{}

func NewDevicesDataSource() datasource.DataSource { return &devicesDataSource{} }

type devicesDataSource struct {
	data *providerData
}

type deviceModel struct {
	Name            types.String `tfsdk:"name"`
	Type            types.String `tfsdk:"type"`
	Model           types.String `tfsdk:"model"`
	MAC             types.String `tfsdk:"mac"`
	SN              types.String `tfsdk:"sn"`
	IP              types.String `tfsdk:"ip"`
	Status          types.Int64  `tfsdk:"status"`
	StatusCategory  types.Int64  `tfsdk:"status_category"`
	FirmwareVersion types.String `tfsdk:"firmware_version"`
	Version         types.String `tfsdk:"version"`
	NeedUpgrade     types.Bool   `tfsdk:"need_upgrade"`
	UptimeSeconds   types.Int64  `tfsdk:"uptime_seconds"`
	ClientNum       types.Int64  `tfsdk:"client_num"`
	HWVersion       types.String `tfsdk:"hw_version"`
}

type devicesDataSourceModel struct {
	SiteID   types.String  `tfsdk:"site_id"`
	SiteName types.String  `tfsdk:"site"`
	Devices  []deviceModel `tfsdk:"devices"`
}

func (d *devicesDataSource) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_devices"
}

func (d *devicesDataSource) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Lists the adopted devices (gateways, switches, APs) on a site — an inventory view, and the foundation for future per-device config resources.",
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
			"devices": schema.ListNestedAttribute{
				MarkdownDescription: "The adopted devices on the site.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":             schema.StringAttribute{Computed: true, MarkdownDescription: "Device name."},
						"type":             schema.StringAttribute{Computed: true, MarkdownDescription: "Device type: `gateway`, `switch`, or `ap`."},
						"model":            schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware model, e.g. `ES205GP`."},
						"mac":              schema.StringAttribute{Computed: true, MarkdownDescription: "MAC address (the device's controller identity)."},
						"sn":               schema.StringAttribute{Computed: true, MarkdownDescription: "Serial number."},
						"ip":               schema.StringAttribute{Computed: true, MarkdownDescription: "Management IP address."},
						"status":           schema.Int64Attribute{Computed: true, MarkdownDescription: "Raw controller status code."},
						"status_category":  schema.Int64Attribute{Computed: true, MarkdownDescription: "Status category: 0=disconnected, 1=connected, 2=pending."},
						"firmware_version": schema.StringAttribute{Computed: true, MarkdownDescription: "Full firmware version string."},
						"version":          schema.StringAttribute{Computed: true, MarkdownDescription: "Short firmware version."},
						"need_upgrade":     schema.BoolAttribute{Computed: true, MarkdownDescription: "Whether a firmware upgrade is available."},
						"uptime_seconds":   schema.Int64Attribute{Computed: true, MarkdownDescription: "Uptime in seconds."},
						"client_num":       schema.Int64Attribute{Computed: true, MarkdownDescription: "Number of connected clients."},
						"hw_version":       schema.StringAttribute{Computed: true, MarkdownDescription: "Hardware version."},
					},
				},
			},
		},
	}
}

func (d *devicesDataSource) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
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

func (d *devicesDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var config devicesDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &config)...)
	if resp.Diagnostics.HasError() {
		return
	}

	siteID, diags := d.resolveSiteID(ctx, config)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	devices, err := d.data.client.ListDevices(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to list Omada devices", err.Error())
		return
	}

	state := devicesDataSourceModel{
		SiteID:   types.StringValue(siteID),
		SiteName: config.SiteName,
	}
	for _, dev := range devices {
		state.Devices = append(state.Devices, deviceModel{
			Name:            types.StringValue(dev.Name),
			Type:            types.StringValue(dev.Type),
			Model:           types.StringValue(dev.Model),
			MAC:             types.StringValue(dev.MAC),
			SN:              types.StringValue(dev.SN),
			IP:              types.StringValue(dev.IP),
			Status:          types.Int64Value(int64(dev.Status)),
			StatusCategory:  types.Int64Value(int64(dev.StatusCategory)),
			FirmwareVersion: types.StringValue(dev.FirmwareVersion),
			Version:         types.StringValue(dev.Version),
			NeedUpgrade:     types.BoolValue(dev.NeedUpgrade),
			UptimeSeconds:   types.Int64Value(dev.UptimeSeconds),
			ClientNum:       types.Int64Value(int64(dev.ClientNum)),
			HWVersion:       types.StringValue(dev.HWVersion),
		})
	}
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (d *devicesDataSource) resolveSiteID(ctx context.Context, config devicesDataSourceModel) (string, diag.Diagnostics) {
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
