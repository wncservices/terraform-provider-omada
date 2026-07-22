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

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &siteSettingsResource{}
	_ resource.ResourceWithConfigure   = &siteSettingsResource{}
	_ resource.ResourceWithImportState = &siteSettingsResource{}
)

func NewSiteSettingsResource() resource.Resource { return &siteSettingsResource{} }

type siteSettingsResource struct{ data *providerData }

type siteSettingsResourceModel struct {
	ID   types.String `tfsdk:"id"`
	Site types.String `tfsdk:"site"`

	AdvancedFeatureEnable types.Bool `tfsdk:"advanced_feature_enable"`
	AirtimeFairness2g     types.Bool `tfsdk:"airtime_fairness_2g"`
	AirtimeFairness5g     types.Bool `tfsdk:"airtime_fairness_5g"`
	AirtimeFairness6g     types.Bool `tfsdk:"airtime_fairness_6g"`
	AlertEnable           types.Bool `tfsdk:"alert_enable"`
	AlertDelayEnable      types.Bool `tfsdk:"alert_delay_enable"`
	AutoUpgradeEnable     types.Bool `tfsdk:"auto_upgrade_enable"`
	BandSteeringEnable    types.Bool `tfsdk:"band_steering_enable"`
	ChannelLimitEnable    types.Bool `tfsdk:"channel_limit_enable"`
	LEDEnable             types.Bool `tfsdk:"led_enable"`
	LLDPEnable            types.Bool `tfsdk:"lldp_enable"`
	MeshEnable            types.Bool `tfsdk:"mesh_enable"`
	MeshAutoFailover      types.Bool `tfsdk:"mesh_auto_failover"`
	MeshDefaultGateway    types.Bool `tfsdk:"mesh_default_gateway"`
	MeshFullSector        types.Bool `tfsdk:"mesh_full_sector"`
	RememberDeviceEnable  types.Bool `tfsdk:"remember_device_enable"`
	RemoteLogEnable       types.Bool `tfsdk:"remote_log_enable"`
	RemoteLogMoreClient   types.Bool `tfsdk:"remote_log_more_client_log"`
	RoamFastEnable        types.Bool `tfsdk:"roaming_fast_roaming_enable"`
	RoamAIEnable          types.Bool `tfsdk:"roaming_ai_roaming_enable"`
	Roam11kReport         types.Bool `tfsdk:"roaming_dual_band_11k_report_enable"`
	RoamForceDisassoc     types.Bool `tfsdk:"roaming_force_disassociation_enable"`
	RoamNonStick          types.Bool `tfsdk:"roaming_non_stick_enable"`
	RoamNonPingPong       types.Bool `tfsdk:"roaming_non_ping_pong_enable"`
	SpeedTestEnable       types.Bool `tfsdk:"speed_test_enable"`

	AlertDelay          types.Int64 `tfsdk:"alert_delay"`
	BSConnThreshold     types.Int64 `tfsdk:"band_steering_connection_threshold"`
	BSDiffThreshold     types.Int64 `tfsdk:"band_steering_difference_threshold"`
	BSMaxFailures       types.Int64 `tfsdk:"band_steering_max_failures"`
	BSMultiBandMode     types.Int64 `tfsdk:"band_steering_multi_band_mode"`
	BeaconIntvMode2g    types.Int64 `tfsdk:"beacon_intv_mode_2g"`
	BeaconDtim2g        types.Int64 `tfsdk:"beacon_dtim_period_2g"`
	BeaconRts2g         types.Int64 `tfsdk:"beacon_rts_threshold_2g"`
	BeaconFrag2g        types.Int64 `tfsdk:"beacon_fragmentation_threshold_2g"`
	BeaconIntvMode5g    types.Int64 `tfsdk:"beacon_intv_mode_5g"`
	BeaconDtim5g        types.Int64 `tfsdk:"beacon_dtim_period_5g"`
	BeaconRts5g         types.Int64 `tfsdk:"beacon_rts_threshold_5g"`
	BeaconFrag5g        types.Int64 `tfsdk:"beacon_fragmentation_threshold_5g"`
	BeaconInterval6g    types.Int64 `tfsdk:"beacon_interval_6g"`
	BeaconIntvMode6g    types.Int64 `tfsdk:"beacon_intv_mode_6g"`
	BeaconDtim6g        types.Int64 `tfsdk:"beacon_dtim_period_6g"`
	BeaconRts6g         types.Int64 `tfsdk:"beacon_rts_threshold_6g"`
	BeaconFrag6g        types.Int64 `tfsdk:"beacon_fragmentation_threshold_6g"`
	RemoteLogPort       types.Int64 `tfsdk:"remote_log_port"`
	SpeedTestIntervalMs types.Int64 `tfsdk:"speed_test_interval"`
}

// Table-driven field mapping: TF attribute <-> settings group/key.
type ssBool struct {
	attr, group, key, desc string
	ref                    func(*siteSettingsResourceModel) *types.Bool
}
type ssInt struct {
	attr, group, key, desc string
	ref                    func(*siteSettingsResourceModel) *types.Int64
}

var ssBools = []ssBool{
	{"advanced_feature_enable", "advancedFeature", "enable", "Advanced features.", func(m *siteSettingsResourceModel) *types.Bool { return &m.AdvancedFeatureEnable }},
	{"airtime_fairness_2g", "airtimeFairness", "enable2g", "Airtime fairness on 2.4GHz.", func(m *siteSettingsResourceModel) *types.Bool { return &m.AirtimeFairness2g }},
	{"airtime_fairness_5g", "airtimeFairness", "enable5g", "Airtime fairness on 5GHz.", func(m *siteSettingsResourceModel) *types.Bool { return &m.AirtimeFairness5g }},
	{"airtime_fairness_6g", "airtimeFairness", "enable6g", "Airtime fairness on 6GHz.", func(m *siteSettingsResourceModel) *types.Bool { return &m.AirtimeFairness6g }},
	{"alert_enable", "alert", "enable", "Alerts.", func(m *siteSettingsResourceModel) *types.Bool { return &m.AlertEnable }},
	{"alert_delay_enable", "alert", "delayEnable", "Delay before alerting.", func(m *siteSettingsResourceModel) *types.Bool { return &m.AlertDelayEnable }},
	{"auto_upgrade_enable", "autoUpgrade", "enable", "Automatic firmware upgrades.", func(m *siteSettingsResourceModel) *types.Bool { return &m.AutoUpgradeEnable }},
	{"band_steering_enable", "bandSteering", "enable", "Band steering.", func(m *siteSettingsResourceModel) *types.Bool { return &m.BandSteeringEnable }},
	{"channel_limit_enable", "channelLimit", "enable", "Channel limit.", func(m *siteSettingsResourceModel) *types.Bool { return &m.ChannelLimitEnable }},
	{"led_enable", "led", "enable", "Device status LEDs.", func(m *siteSettingsResourceModel) *types.Bool { return &m.LEDEnable }},
	{"lldp_enable", "lldp", "enable", "LLDP.", func(m *siteSettingsResourceModel) *types.Bool { return &m.LLDPEnable }},
	{"mesh_enable", "mesh", "meshEnable", "Mesh networking.", func(m *siteSettingsResourceModel) *types.Bool { return &m.MeshEnable }},
	{"mesh_auto_failover", "mesh", "autoFailoverEnable", "Mesh auto-failover.", func(m *siteSettingsResourceModel) *types.Bool { return &m.MeshAutoFailover }},
	{"mesh_default_gateway", "mesh", "defGatewayEnable", "Mesh default gateway.", func(m *siteSettingsResourceModel) *types.Bool { return &m.MeshDefaultGateway }},
	{"mesh_full_sector", "mesh", "fullSector", "Mesh full-sector DFS.", func(m *siteSettingsResourceModel) *types.Bool { return &m.MeshFullSector }},
	{"remember_device_enable", "rememberDevice", "enable", "Remember clients.", func(m *siteSettingsResourceModel) *types.Bool { return &m.RememberDeviceEnable }},
	{"remote_log_enable", "remoteLog", "enable", "Remote syslog.", func(m *siteSettingsResourceModel) *types.Bool { return &m.RemoteLogEnable }},
	{"remote_log_more_client_log", "remoteLog", "moreClientLog", "Verbose client logging.", func(m *siteSettingsResourceModel) *types.Bool { return &m.RemoteLogMoreClient }},
	{"roaming_fast_roaming_enable", "roaming", "fastRoamingEnable", "802.11r fast roaming.", func(m *siteSettingsResourceModel) *types.Bool { return &m.RoamFastEnable }},
	{"roaming_ai_roaming_enable", "roaming", "aiRoamingEnable", "AI roaming.", func(m *siteSettingsResourceModel) *types.Bool { return &m.RoamAIEnable }},
	{"roaming_dual_band_11k_report_enable", "roaming", "dualBand11kReportEnable", "Dual-band 802.11k reports.", func(m *siteSettingsResourceModel) *types.Bool { return &m.Roam11kReport }},
	{"roaming_force_disassociation_enable", "roaming", "forceDisassociationEnable", "Force disassociation.", func(m *siteSettingsResourceModel) *types.Bool { return &m.RoamForceDisassoc }},
	{"roaming_non_stick_enable", "roaming", "nonStickRoamingEnable", "Non-sticky roaming.", func(m *siteSettingsResourceModel) *types.Bool { return &m.RoamNonStick }},
	{"roaming_non_ping_pong_enable", "roaming", "nonPingPongRoamingEnable", "Anti ping-pong roaming.", func(m *siteSettingsResourceModel) *types.Bool { return &m.RoamNonPingPong }},
	{"speed_test_enable", "speedTest", "enable", "Periodic WAN speed test.", func(m *siteSettingsResourceModel) *types.Bool { return &m.SpeedTestEnable }},
}

var ssInts = []ssInt{
	{"alert_delay", "alert", "delay", "Alert delay (seconds).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.AlertDelay }},
	{"band_steering_connection_threshold", "bandSteering", "connectionThreshold", "Band-steering connection threshold.", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BSConnThreshold }},
	{"band_steering_difference_threshold", "bandSteering", "differenceThreshold", "Band-steering difference threshold.", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BSDiffThreshold }},
	{"band_steering_max_failures", "bandSteering", "maxFailures", "Band-steering max failures.", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BSMaxFailures }},
	{"band_steering_multi_band_mode", "bandSteeringForMultiBand", "mode", "Multi-band band-steering mode.", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BSMultiBandMode }},
	{"beacon_intv_mode_2g", "beaconControl", "beaconIntvMode2g", "Beacon interval mode (2.4GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconIntvMode2g }},
	{"beacon_dtim_period_2g", "beaconControl", "dtimPeriod2g", "DTIM period (2.4GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconDtim2g }},
	{"beacon_rts_threshold_2g", "beaconControl", "rtsThreshold2g", "RTS threshold (2.4GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconRts2g }},
	{"beacon_fragmentation_threshold_2g", "beaconControl", "fragmentationThreshold2g", "Fragmentation threshold (2.4GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconFrag2g }},
	{"beacon_intv_mode_5g", "beaconControl", "beaconIntvMode5g", "Beacon interval mode (5GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconIntvMode5g }},
	{"beacon_dtim_period_5g", "beaconControl", "dtimPeriod5g", "DTIM period (5GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconDtim5g }},
	{"beacon_rts_threshold_5g", "beaconControl", "rtsThreshold5g", "RTS threshold (5GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconRts5g }},
	{"beacon_fragmentation_threshold_5g", "beaconControl", "fragmentationThreshold5g", "Fragmentation threshold (5GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconFrag5g }},
	{"beacon_interval_6g", "beaconControl", "beaconInterval6g", "Beacon interval (6GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconInterval6g }},
	{"beacon_intv_mode_6g", "beaconControl", "beaconIntvMode6g", "Beacon interval mode (6GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconIntvMode6g }},
	{"beacon_dtim_period_6g", "beaconControl", "dtimPeriod6g", "DTIM period (6GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconDtim6g }},
	{"beacon_rts_threshold_6g", "beaconControl", "rtsThreshold6g", "RTS threshold (6GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconRts6g }},
	{"beacon_fragmentation_threshold_6g", "beaconControl", "fragmentationThreshold6g", "Fragmentation threshold (6GHz).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.BeaconFrag6g }},
	{"remote_log_port", "remoteLog", "port", "Remote syslog port.", func(m *siteSettingsResourceModel) *types.Int64 { return &m.RemoteLogPort }},
	{"speed_test_interval", "speedTest", "interval", "Speed-test interval (minutes).", func(m *siteSettingsResourceModel) *types.Int64 { return &m.SpeedTestIntervalMs }},
}

func (r *siteSettingsResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_site_settings"
}

func (r *siteSettingsResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	attrs := map[string]schema.Attribute{
		"id":   schema.StringAttribute{Computed: true, MarkdownDescription: "The site ID.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
		"site": schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
	}
	for _, f := range ssBools {
		attrs[f.attr] = schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: f.desc}
	}
	for _, f := range ssInts {
		attrs[f.attr] = schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: f.desc}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages site-wide settings (a singleton): LED, mesh, roaming, band steering, airtime fairness, LLDP, auto-upgrade, alerts, remote logging, speed test and RF beacon control.\n\n" +
			"Unset attributes keep their current controller value. The device account (a credential) and read-only metadata are never touched. Destroying this resource does not reset anything.",
		Attributes: attrs,
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

// refresh fills the model from the live settings object.
func (r *siteSettingsResource) refresh(st map[string]any, m *siteSettingsResourceModel) {
	for _, f := range ssBools {
		if v, ok := omada.SettingBool(st, f.group, f.key); ok {
			*f.ref(m) = types.BoolValue(v)
		} else {
			*f.ref(m) = types.BoolNull()
		}
	}
	for _, f := range ssInts {
		if v, ok := omada.SettingInt(st, f.group, f.key); ok {
			*f.ref(m) = types.Int64Value(v)
		} else {
			*f.ref(m) = types.Int64Null()
		}
	}
}

// groupsFrom builds the grouped patch payload from known model values.
func (r *siteSettingsResource) groupsFrom(m siteSettingsResourceModel) map[string]map[string]any {
	out := map[string]map[string]any{}
	set := func(group, key string, v any) {
		if out[group] == nil {
			out[group] = map[string]any{}
		}
		out[group][key] = v
	}
	for _, f := range ssBools {
		if v := f.ref(&m); !v.IsNull() && !v.IsUnknown() {
			set(f.group, f.key, v.ValueBool())
		}
	}
	for _, f := range ssInts {
		if v := f.ref(&m); !v.IsNull() && !v.IsUnknown() {
			set(f.group, f.key, v.ValueInt64())
		}
	}
	return out
}

func (r *siteSettingsResource) write(ctx context.Context, m *siteSettingsResourceModel, diags *diagSink) {
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(*m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return
	}
	if err := r.data.client.PatchSiteSettings(ctx, siteID, r.groupsFrom(*m)); err != nil {
		diags.AddError("Unable to update site settings", err.Error())
		return
	}
	st, err := r.data.client.GetSiteSettings(ctx, siteID)
	if err != nil {
		diags.AddError("Unable to read site settings", err.Error())
		return
	}
	r.refresh(st, m)
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
	st, err := r.data.client.GetSiteSettings(ctx, siteID)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read site settings", err.Error())
		return
	}
	r.refresh(st, &state)
	state.ID = types.StringValue(siteID)
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
func (r *siteSettingsResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid import ID", "expected the site name, e.g. `terraform import omada_site_settings.this Home`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), req.ID)...)
}
