// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &iotServerResource{}
	_ resource.ResourceWithConfigure   = &iotServerResource{}
	_ resource.ResourceWithImportState = &iotServerResource{}
)

func NewIoTServerResource() resource.Resource { return &iotServerResource{} }

type iotServerResource struct{ data *providerData }

type iotServerResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`

	Name                 types.String `tfsdk:"name"`
	Enable               types.Bool   `tfsdk:"enable"`
	ServerURL            types.String `tfsdk:"server_url"`
	ServerType           types.Int64  `tfsdk:"server_type"`
	FormatType           types.Int64  `tfsdk:"format_type"`
	DeviceClasses        types.List   `tfsdk:"device_classes"`
	ReportInterval       types.Int64  `tfsdk:"report_interval"`
	Authentication       types.Int64  `tfsdk:"authentication"`
	RSSIFormat           types.Int64  `tfsdk:"rssi_format"`
	CountOnly            types.Bool   `tfsdk:"count_only"`
	BLEPeriodicTelemetry types.Bool   `tfsdk:"ble_periodic_telemetry"`
	RawData              types.Bool   `tfsdk:"raw_data"`
	SSLTLSEnable         types.Bool   `tfsdk:"ssl_tls_enable"`
}

func (r *iotServerResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iot_server"
}

func (r *iotServerResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an IoT telemetry server: an external endpoint the access points " +
			"push BLE/IoT observations to (Settings -> IoT -> Telemetry Servers).\n\n" +
			"~> **This streams presence data off your network.** BLE telemetry is a record of which " +
			"devices — and so, in practice, which people — were near which access point and when. " +
			"Send it only somewhere you control, keep `ssl_tls_enable` on, and prefer `count_only` " +
			"when aggregate numbers are enough: it is the difference between counting visitors and " +
			"tracking individuals.\n\n" +
			"~> **Servers needing authentication are not fully supported yet.** The controller stores " +
			"`authentication = 99` for a server with none, and exposes no credential field on such a " +
			"server, so the rest of the enum and whatever fields accompany it have not been observed. " +
			"Configure an authenticated server in the controller UI and import it; the credential " +
			"itself will not be managed here.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"site": schema.StringAttribute{
				Optional: true,
				// Computed as well as Optional, and recorded as the *canonical*
				// resolved name. Optional-only looks simpler but sets a trap:
				// importing without a "<site>/" prefix leaves this null, so a
				// configuration that names the site reads as a change — and
				// since the attribute forces replacement, the plan proposes
				// replacing hardware it had just adopted.
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
			"name": schema.StringAttribute{
				Required:            true,
				MarkdownDescription: "Server name, as shown in the controller.",
			},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Whether telemetry is actively pushed to this server. Defaults to " +
					"`false` so a server can be staged before it starts receiving data.",
			},
			"server_url": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "Destination URL, for example `https://telemetry.example.com/ingest`. " +
					"Prefer `https` — see `ssl_tls_enable`.",
			},
			"ssl_tls_enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Whether the connection to the server uses TLS. Defaults to `true`; " +
					"turning it off sends presence data across the network in clear.",
			},
			"server_type": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				MarkdownDescription: "Controller enum for the server protocol. `0` is the value observed " +
					"on live hardware; the controller does not publish the rest.",
			},
			"format_type": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				MarkdownDescription: "Controller enum for the payload format. `0` observed live.",
			},
			"device_classes": schema.ListAttribute{
				ElementType: types.Int64Type,
				Optional:    true, Computed: true,
				Default: listdefault.StaticValue(types.ListValueMust(types.Int64Type, []attr.Value{
					types.Int64Value(0), types.Int64Value(1), types.Int64Value(2), types.Int64Value(3),
				})),
				MarkdownDescription: "Controller enums for the device classes reported on. Defaults to " +
					"`[0, 1, 2, 3]`, which is what the UI sets when all classes are selected.\n\n" +
					"The list may not be empty — the controller rejects that with " +
					"*The parameter Device Class is required*.",
			},
			"report_interval": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(1),
				MarkdownDescription: "How often observations are reported. A shorter interval means " +
					"finer-grained tracking data leaving the site, as well as more load.",
			},
			"authentication": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(99),
				MarkdownDescription: "Controller enum for the authentication method. `99` means none, " +
					"and is the only value observed — see the note above.",
			},
			"rssi_format": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				MarkdownDescription: "Controller enum for how signal strength is reported. `0` observed live.",
			},
			"count_only": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Report only device counts rather than per-device observations. " +
					"The privacy-preserving option where it is sufficient.",
			},
			"ble_periodic_telemetry": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(true),
				MarkdownDescription: "Whether BLE telemetry is sent periodically.",
			},
			"raw_data": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Include raw advertisement payloads. Turning this on sends " +
					"considerably more, and less filtered, data off-box.",
			},
		},
	}
}

func (r *iotServerResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iotServerResource) siteName(m iotServerResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *iotServerResource) resolveSite(ctx context.Context, m iotServerResourceModel, diags *diagSink) (string, string) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return "", ""
	}
	return site.ID, site.Name
}

func (r *iotServerResource) toAPI(ctx context.Context, m iotServerResourceModel, diags *diagSink) omada.IoTServer {
	classes, d := intSlice(ctx, m.DeviceClasses)
	if d.HasError() {
		diags.AddError("Invalid device_classes", "could not read the device class list")
	}
	return omada.IoTServer{
		Name:                  m.Name.ValueString(),
		Enable:                m.Enable.ValueBool(),
		ServerURL:             m.ServerURL.ValueString(),
		ServerType:            int(m.ServerType.ValueInt64()),
		FormatType:            int(m.FormatType.ValueInt64()),
		DeviceClasses:         classes,
		ReportIntervalSeconds: int(m.ReportInterval.ValueInt64()),
		Authentication:        int(m.Authentication.ValueInt64()),
		RSSIFormat:            int(m.RSSIFormat.ValueInt64()),
		CountOnly:             m.CountOnly.ValueBool(),
		BLEPeriodicTelemetry:  m.BLEPeriodicTelemetry.ValueBool(),
		RawData:               m.RawData.ValueBool(),
		SSLTLSEnable:          m.SSLTLSEnable.ValueBool(),
	}
}

func (r *iotServerResource) refresh(ctx context.Context, s *omada.IoTServer, m *iotServerResourceModel) {
	m.ID = types.StringValue(s.ID)
	m.Name = types.StringValue(s.Name)
	m.Enable = types.BoolValue(s.Enable)
	m.ServerURL = types.StringValue(s.ServerURL)
	m.ServerType = types.Int64Value(int64(s.ServerType))
	m.FormatType = types.Int64Value(int64(s.FormatType))
	m.ReportInterval = types.Int64Value(int64(s.ReportIntervalSeconds))
	m.Authentication = types.Int64Value(int64(s.Authentication))
	m.RSSIFormat = types.Int64Value(int64(s.RSSIFormat))
	m.CountOnly = types.BoolValue(s.CountOnly)
	m.BLEPeriodicTelemetry = types.BoolValue(s.BLEPeriodicTelemetry)
	m.RawData = types.BoolValue(s.RawData)
	m.SSLTLSEnable = types.BoolValue(s.SSLTLSEnable)
	lv, _ := intListValue(ctx, s.DeviceClasses)
	m.DeviceClasses = lv
}

func (r *iotServerResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan iotServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sink := &diagSink{&resp.Diagnostics}
	siteID, siteName := r.resolveSite(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.toAPI(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.data.client.CreateIoTServer(ctx, siteID, body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create iot server", err.Error())
		return
	}
	r.refresh(ctx, created, &plan)
	plan.SiteID = types.StringValue(siteID)
	plan.Site = types.StringValue(siteName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *iotServerResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state iotServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, siteName := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetIoTServer(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.refresh(ctx, cur, &state)
	state.SiteID = types.StringValue(siteID)
	state.Site = types.StringValue(siteName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *iotServerResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state iotServerResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.SiteID = state.SiteID
	sink := &diagSink{&resp.Diagnostics}
	siteID, siteName := r.resolveSite(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.toAPI(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	if err := r.data.client.UpdateIoTServer(ctx, siteID, id, body); err != nil {
		resp.Diagnostics.AddError("Unable to update iot server", err.Error())
		return
	}
	cur, err := r.data.client.GetIoTServer(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read iot server after update", err.Error())
		return
	}
	r.refresh(ctx, cur, &plan)
	plan.SiteID = types.StringValue(siteID)
	plan.Site = types.StringValue(siteName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *iotServerResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state iotServerResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, _ := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteIoTServer(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete iot server", err.Error())
		return
	}
	// The DHCP-reservation endpoint taught this lesson: a delete that addresses
	// the wrong key answers 0 and removes nothing, leaving an orphan Terraform
	// believes is gone. Confirm it actually went.
	if _, err := r.data.client.GetIoTServer(ctx, siteID, state.ID.ValueString()); err == nil {
		resp.Diagnostics.AddError("IoT server still present after delete",
			fmt.Sprintf("the controller accepted the delete of %q but the server is still listed",
				state.ID.ValueString()))
	}
}

// ImportState takes the server id, or "<site>/<id>".
func (r *iotServerResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
