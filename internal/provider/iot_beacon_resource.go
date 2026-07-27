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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &iotBeaconResource{}
	_ resource.ResourceWithConfigure   = &iotBeaconResource{}
	_ resource.ResourceWithImportState = &iotBeaconResource{}
)

func NewIoTBeaconResource() resource.Resource { return &iotBeaconResource{} }

type iotBeaconResource struct{ data *providerData }

type iotBeaconResourceModel struct {
	ID     types.String `tfsdk:"id"`
	Site   types.String `tfsdk:"site"`
	SiteID types.String `tfsdk:"site_id"`

	Name           types.String `tfsdk:"name"`
	Enable         types.Bool   `tfsdk:"enable"`
	DeviceMACs     types.List   `tfsdk:"device_macs"`
	UUID           types.String `tfsdk:"uuid"`
	Major          types.String `tfsdk:"major"`
	Minor          types.String `tfsdk:"minor"`
	TransmitPower  types.Int64  `tfsdk:"transmit_power"`
	MeasurePower   types.Int64  `tfsdk:"measure_power"`
	AdvInterval    types.Int64  `tfsdk:"adv_interval"`
	BoundDeviceNum types.Int64  `tfsdk:"bound_device_num"`
	BuiltIn        types.Int64  `tfsdk:"built_in"`
}

func (r *iotBeaconResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_iot_beacon"
}

func (r *iotBeaconResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages an iBeacon profile: a BLE advertisement broadcast by IoT-capable " +
			"access points (Settings -> IoT -> iBeacon).\n\n" +
			"~> **Requires access points with a BLE radio.** A profile must name at least one AP in " +
			"`device_macs`, and the controller checks that list against its own IoT-capable device " +
			"inventory. On hardware without a BLE radio (the EAP610, for example) that inventory is " +
			"empty and every create is refused with `-33284 The devices in the device list are not in " +
			"the current site`. **Create and delete are therefore implemented but unverified** — they " +
			"could not be exercised on the development site. Read, import and update are confirmed live.\n\n" +
			"~> **A beacon is a public broadcast.** Any phone in range can read the UUID, major and " +
			"minor and use them to identify the location — that is the entire point, and it does not " +
			"stop at people you invited. Treat the triple as a published identifier, not a secret, and " +
			"do not encode anything sensitive in it.",
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
				MarkdownDescription: "Profile name, as shown in the controller.",
			},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Whether the access points actually broadcast this profile. " +
					"Defaults to `false` so a profile can be staged before it goes on air.",
			},
			"device_macs": schema.ListAttribute{
				// macType, not StringType: the controller stores MACs dashed and
				// upper-case, and Terraform refuses to let a provider rewrite a
				// Required attribute — a config using colons would fail with
				// "inconsistent result after apply". Semantic equality makes the
				// two spellings the same value instead. Same reasoning as
				// omada_dhcp_reservation.
				ElementType: macType{},
				Required:    true,
				MarkdownDescription: "MAC addresses of the access points that broadcast this profile, in " +
					"any common spelling. May not be empty.",
			},
			"uuid": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "iBeacon UUID, as 32 hex characters without dashes " +
					"(for example `0123456789abcdef0123456789abcdef`).",
			},
			"major": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("0000"),
				MarkdownDescription: "iBeacon major value, as 4 hex characters.",
			},
			"minor": schema.StringAttribute{
				Optional: true, Computed: true, Default: stringdefault.StaticString("0000"),
				MarkdownDescription: "iBeacon minor value, as 4 hex characters.",
			},
			"transmit_power": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				MarkdownDescription: "Controller enum for broadcast power. `0` is the value observed live. " +
					"Lower power means a smaller broadcast footprint.",
			},
			"measure_power": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(-65),
				MarkdownDescription: "Calibrated signal strength at one metre, in dBm — receivers use it to " +
					"estimate distance. Negative; `-65` is the controller's default.",
			},
			"adv_interval": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(500),
				MarkdownDescription: "Advertisement interval in milliseconds. `500` is the controller's " +
					"default; a shorter interval is found faster but costs airtime.",
			},
			"bound_device_num": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "How many devices the controller has bound to this profile. Read-only.",
			},
			"built_in": schema.Int64Attribute{
				Computed:            true,
				MarkdownDescription: "Controller flag marking a built-in profile. Read-only.",
			},
		},
	}
}

func (r *iotBeaconResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *iotBeaconResource) siteName(m iotBeaconResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *iotBeaconResource) resolveSite(ctx context.Context, m iotBeaconResourceModel, diags *diagSink) (string, string) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return "", ""
	}
	return site.ID, site.Name
}

func (r *iotBeaconResource) toAPI(ctx context.Context, m iotBeaconResourceModel, diags *diagSink) omada.IoTBeacon {
	macs, d := stringSlice(ctx, m.DeviceMACs)
	if d.HasError() {
		diags.AddError("Invalid device_macs", "could not read the device MAC list")
	}
	// Normalise to the controller's spelling so a config using colons matches
	// what the controller stores, the same way omada_dhcp_reservation does.
	for i := range macs {
		macs[i] = omada.NormalizeMAC(macs[i])
	}
	return omada.IoTBeacon{
		Name:          m.Name.ValueString(),
		Enable:        m.Enable.ValueBool(),
		MACList:       macs,
		UUID:          m.UUID.ValueString(),
		Major:         m.Major.ValueString(),
		Minor:         m.Minor.ValueString(),
		TransmitPower: int(m.TransmitPower.ValueInt64()),
		MeasurePower:  int(m.MeasurePower.ValueInt64()),
		AdvIntervalMS: int(m.AdvInterval.ValueInt64()),
	}
}

func (r *iotBeaconResource) refresh(ctx context.Context, b *omada.IoTBeacon, m *iotBeaconResourceModel) {
	m.ID = types.StringValue(b.ID)
	m.Name = types.StringValue(b.Name)
	m.Enable = types.BoolValue(b.Enable)
	m.UUID = types.StringValue(b.UUID)
	m.Major = types.StringValue(b.Major)
	m.Minor = types.StringValue(b.Minor)
	m.TransmitPower = types.Int64Value(int64(b.TransmitPower))
	m.MeasurePower = types.Int64Value(int64(b.MeasurePower))
	m.AdvInterval = types.Int64Value(int64(b.AdvIntervalMS))
	m.BoundDeviceNum = types.Int64Value(int64(b.BoundDeviceNum))
	m.BuiltIn = types.Int64Value(int64(b.BuiltIn))
	lv, _ := stringListValue(ctx, b.MACList)
	m.DeviceMACs = lv
}

func (r *iotBeaconResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan iotBeaconResourceModel
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
	created, err := r.data.client.CreateIoTBeacon(ctx, siteID, body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create iot beacon profile", err.Error())
		return
	}
	r.refresh(ctx, created, &plan)
	plan.SiteID = types.StringValue(siteID)
	plan.Site = types.StringValue(siteName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *iotBeaconResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state iotBeaconResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, siteName := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetIoTBeacon(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.refresh(ctx, cur, &state)
	state.SiteID = types.StringValue(siteID)
	state.Site = types.StringValue(siteName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *iotBeaconResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state iotBeaconResourceModel
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
	if err := r.data.client.UpdateIoTBeacon(ctx, siteID, id, body); err != nil {
		resp.Diagnostics.AddError("Unable to update iot beacon profile", err.Error())
		return
	}
	cur, err := r.data.client.GetIoTBeacon(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read iot beacon profile after update", err.Error())
		return
	}
	r.refresh(ctx, cur, &plan)
	plan.SiteID = types.StringValue(siteID)
	plan.Site = types.StringValue(siteName)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *iotBeaconResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state iotBeaconResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, _ := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	if err := r.data.client.DeleteIoTBeacon(ctx, siteID, id); err != nil {
		resp.Diagnostics.AddError("Unable to delete iot beacon profile", err.Error())
		return
	}
	// Same confirmation as omada_iot_server and omada_dhcp_reservation: an
	// accepted delete is not proof the object went.
	if _, err := r.data.client.GetIoTBeacon(ctx, siteID, id); err == nil {
		resp.Diagnostics.AddError("IoT beacon profile still present after delete",
			fmt.Sprintf("the controller accepted the delete of %q but the profile is still listed", id))
	}
}

// ImportState takes the profile id, or "<site>/<id>".
func (r *iotBeaconResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
