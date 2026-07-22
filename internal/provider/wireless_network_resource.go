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
	_ resource.Resource                = &wirelessResource{}
	_ resource.ResourceWithConfigure   = &wirelessResource{}
	_ resource.ResourceWithImportState = &wirelessResource{}
)

func NewWirelessNetworkResource() resource.Resource { return &wirelessResource{} }

type wirelessResource struct{ data *providerData }

type wirelessResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Site        types.String `tfsdk:"site"`
	SiteID      types.String `tfsdk:"site_id"`
	WLANGroupID types.String `tfsdk:"wlan_group_id"`
	Name        types.String `tfsdk:"name"`
	Band        types.Int64  `tfsdk:"band"`
	Security    types.Int64  `tfsdk:"security"`
	PSK         types.String `tfsdk:"psk"`
	Broadcast   types.Bool   `tfsdk:"broadcast"`
	VLANEnable  types.Bool   `tfsdk:"vlan_enable"`
	VLANID      types.Int64  `tfsdk:"vlan_id"`
	GuestNet    types.Bool   `tfsdk:"guest_net"`

	PortalEnable       types.Bool  `tfsdk:"portal_enable"`
	AccessEnable       types.Bool  `tfsdk:"access_enable"`
	WLANScheduleEnable types.Bool  `tfsdk:"wlan_schedule_enable"`
	MacFilterEnable    types.Bool  `tfsdk:"mac_filter_enable"`
	Enable11r          types.Bool  `tfsdk:"enable_11r"`
	PMFMode            types.Int64 `tfsdk:"pmf_mode"`
	MLOEnable          types.Bool  `tfsdk:"mlo_enable"`
	HidePwd            types.Bool  `tfsdk:"hide_pwd"`
	WanAccess          types.Bool  `tfsdk:"wan_access"`
	ProhibitWifiShare  types.Bool  `tfsdk:"prohibit_wifi_share"`

	PSKVersion    types.Int64 `tfsdk:"psk_version"`
	PSKEncryption types.Int64 `tfsdk:"psk_encryption"`
	PSKGikRekey   types.Bool  `tfsdk:"psk_gik_rekey"`

	RateLimitDown     types.Bool `tfsdk:"rate_limit_down_enable"`
	RateLimitUp       types.Bool `tfsdk:"rate_limit_up_enable"`
	SSIDRateLimitDown types.Bool `tfsdk:"ssid_rate_limit_down_enable"`
	SSIDRateLimitUp   types.Bool `tfsdk:"ssid_rate_limit_up_enable"`

	RateCtrl2g   types.Bool `tfsdk:"rate_ctrl_2g"`
	RateCtrl5g   types.Bool `tfsdk:"rate_ctrl_5g"`
	RateCtrl6g   types.Bool `tfsdk:"rate_ctrl_6g"`
	ManageRate2g types.Bool `tfsdk:"manage_rate_2g"`
	ManageRate5g types.Bool `tfsdk:"manage_rate_5g"`

	MulticastEnable      types.Bool  `tfsdk:"multicast_enable"`
	MulticastChannelUtil types.Int64 `tfsdk:"multicast_channel_util"`
	MulticastArpCast     types.Bool  `tfsdk:"multicast_arp_cast"`
	MulticastIPv6Cast    types.Bool  `tfsdk:"multicast_ipv6_cast"`
	MulticastFilter      types.Bool  `tfsdk:"multicast_filter"`

	DHCPOption82Enable types.Bool `tfsdk:"dhcp_option82_enable"`
}

func (r *wirelessResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_wireless_network"
}

func (r *wirelessResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	b := func(d string) schema.BoolAttribute {
		return schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: d}
	}
	i := func(d string) schema.Int64Attribute {
		return schema.Int64Attribute{Optional: true, Computed: true, MarkdownDescription: d}
	}
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a wireless SSID within a WLAN group: bands, security, VLAN tagging, PMF, roaming, rate limiting, multicast and MAC filtering.\n\n" +
			"`psk` is **write-only** — it is never read back into state, so the WiFi password does not land in your repo or state file. Updates deep-merge the PSK object, so an update that omits `psk` leaves the existing key untouched.",
		Attributes: map[string]schema.Attribute{
			"id":            schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":          schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id":       schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"wlan_group_id": schema.StringAttribute{Required: true, MarkdownDescription: "WLAN group this SSID belongs to. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"name":          schema.StringAttribute{Required: true, MarkdownDescription: "The SSID (network name)."},
			"band":          schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(7), MarkdownDescription: "Radio band bitmask: 1=2.4GHz, 2=5GHz, 4=6GHz (7=all)."},
			"security":      schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(3), MarkdownDescription: "Security mode: 0=open, 3=WPA2/WPA3-PSK."},
			"psk":           schema.StringAttribute{Optional: true, Sensitive: true, MarkdownDescription: "Pre-shared key (WiFi password). **Write-only** — never refreshed from the controller."},
			"broadcast":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(true), MarkdownDescription: "Whether the SSID is broadcast (visible)."},
			"vlan_enable":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Whether the SSID is tagged to a VLAN."},
			"vlan_id":       schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(1), MarkdownDescription: "VLAN ID when vlan_enable is true."},
			"guest_net":     schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Whether this is a guest network."},

			"portal_enable":        b("Captive portal on this SSID."),
			"access_enable":        b("Access control."),
			"wlan_schedule_enable": b("SSID schedule."),
			"mac_filter_enable":    b("MAC filtering."),
			"enable_11r":           b("802.11r fast roaming."),
			"pmf_mode":             i("Protected Management Frames mode."),
			"mlo_enable":           b("Multi-Link Operation (WiFi 7)."),
			"hide_pwd":             b("Hide the password in the UI."),
			"wan_access":           b("Allow WAN access from this SSID."),
			"prohibit_wifi_share":  b("Prohibit WiFi sharing."),

			"psk_version":    i("WPA version code for the PSK."),
			"psk_encryption": i("PSK encryption code."),
			"psk_gik_rekey":  b("GIK rekeying."),

			"rate_limit_down_enable":      b("Per-client download rate limit."),
			"rate_limit_up_enable":        b("Per-client upload rate limit."),
			"ssid_rate_limit_down_enable": b("SSID-wide download rate limit."),
			"ssid_rate_limit_up_enable":   b("SSID-wide upload rate limit."),

			"rate_ctrl_2g":   b("Rate control on 2.4GHz."),
			"rate_ctrl_5g":   b("Rate control on 5GHz."),
			"rate_ctrl_6g":   b("Rate control on 6GHz."),
			"manage_rate_2g": b("Management rate control on 2.4GHz."),
			"manage_rate_5g": b("Management rate control on 5GHz."),

			"multicast_enable":       b("Multicast/broadcast forwarding."),
			"multicast_channel_util": i("Channel utilisation threshold."),
			"multicast_arp_cast":     b("ARP broadcast conversion."),
			"multicast_ipv6_cast":    b("IPv6 multicast conversion."),
			"multicast_filter":       b("Multicast filtering."),

			"dhcp_option82_enable": b("DHCP option 82 insertion."),
		},
	}
}

func (r *wirelessResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func (r *wirelessResource) siteName(m wirelessResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *wirelessResource) fields(m wirelessResourceModel) map[string]any {
	f := map[string]any{
		"name":           m.Name.ValueString(),
		"band":           m.Band.ValueInt64(),
		"security":       m.Security.ValueInt64(),
		"broadcast":      m.Broadcast.ValueBool(),
		"vlanEnable":     m.VLANEnable.ValueBool(),
		"vlanId":         m.VLANID.ValueInt64(),
		"guestNetEnable": m.GuestNet.ValueBool(),
	}
	pb := func(k string, v types.Bool) {
		if !v.IsNull() && !v.IsUnknown() {
			f[k] = v.ValueBool()
		}
	}
	pi := func(k string, v types.Int64) {
		if !v.IsNull() && !v.IsUnknown() {
			f[k] = v.ValueInt64()
		}
	}
	pb("portalEnable", m.PortalEnable)
	pb("accessEnable", m.AccessEnable)
	pb("wlanScheduleEnable", m.WLANScheduleEnable)
	pb("macFilterEnable", m.MacFilterEnable)
	pb("enable11r", m.Enable11r)
	pi("pmfMode", m.PMFMode)
	pb("mloEnable", m.MLOEnable)
	pb("hidePwd", m.HidePwd)
	pb("wanAccess", m.WanAccess)
	pb("prohibitWifiShare", m.ProhibitWifiShare)

	sub := func(key string, build func(map[string]any)) {
		m2 := map[string]any{}
		build(m2)
		if len(m2) > 0 {
			f[key] = m2
		}
	}
	sub("pskSetting", func(o map[string]any) {
		if !m.PSKVersion.IsNull() && !m.PSKVersion.IsUnknown() {
			o["versionPsk"] = m.PSKVersion.ValueInt64()
		}
		if !m.PSKEncryption.IsNull() && !m.PSKEncryption.IsUnknown() {
			o["encryptionPsk"] = m.PSKEncryption.ValueInt64()
		}
		if !m.PSKGikRekey.IsNull() && !m.PSKGikRekey.IsUnknown() {
			o["gikRekeyPskEnable"] = m.PSKGikRekey.ValueBool()
		}
	})
	sub("rateLimit", func(o map[string]any) {
		if !m.RateLimitDown.IsNull() && !m.RateLimitDown.IsUnknown() {
			o["downLimitEnable"] = m.RateLimitDown.ValueBool()
		}
		if !m.RateLimitUp.IsNull() && !m.RateLimitUp.IsUnknown() {
			o["upLimitEnable"] = m.RateLimitUp.ValueBool()
		}
	})
	sub("ssidRateLimit", func(o map[string]any) {
		if !m.SSIDRateLimitDown.IsNull() && !m.SSIDRateLimitDown.IsUnknown() {
			o["downLimitEnable"] = m.SSIDRateLimitDown.ValueBool()
		}
		if !m.SSIDRateLimitUp.IsNull() && !m.SSIDRateLimitUp.IsUnknown() {
			o["upLimitEnable"] = m.SSIDRateLimitUp.ValueBool()
		}
	})
	sub("rateAndBeaconCtrl", func(o map[string]any) {
		if !m.RateCtrl2g.IsNull() && !m.RateCtrl2g.IsUnknown() {
			o["rate2gCtrlEnable"] = m.RateCtrl2g.ValueBool()
		}
		if !m.RateCtrl5g.IsNull() && !m.RateCtrl5g.IsUnknown() {
			o["rate5gCtrlEnable"] = m.RateCtrl5g.ValueBool()
		}
		if !m.RateCtrl6g.IsNull() && !m.RateCtrl6g.IsUnknown() {
			o["rate6gCtrlEnable"] = m.RateCtrl6g.ValueBool()
		}
		if !m.ManageRate2g.IsNull() && !m.ManageRate2g.IsUnknown() {
			o["manageRateControl2gEnable"] = m.ManageRate2g.ValueBool()
		}
		if !m.ManageRate5g.IsNull() && !m.ManageRate5g.IsUnknown() {
			o["manageRateControl5gEnable"] = m.ManageRate5g.ValueBool()
		}
	})
	sub("multiCastSetting", func(o map[string]any) {
		if !m.MulticastEnable.IsNull() && !m.MulticastEnable.IsUnknown() {
			o["multiCastEnable"] = m.MulticastEnable.ValueBool()
		}
		if !m.MulticastChannelUtil.IsNull() && !m.MulticastChannelUtil.IsUnknown() {
			o["channelUtil"] = m.MulticastChannelUtil.ValueInt64()
		}
		if !m.MulticastArpCast.IsNull() && !m.MulticastArpCast.IsUnknown() {
			o["arpCastEnable"] = m.MulticastArpCast.ValueBool()
		}
		if !m.MulticastIPv6Cast.IsNull() && !m.MulticastIPv6Cast.IsUnknown() {
			o["ipv6CastEnable"] = m.MulticastIPv6Cast.ValueBool()
		}
		if !m.MulticastFilter.IsNull() && !m.MulticastFilter.IsUnknown() {
			o["filterEnable"] = m.MulticastFilter.ValueBool()
		}
	})
	sub("dhcpOption82", func(o map[string]any) {
		if !m.DHCPOption82Enable.IsNull() && !m.DHCPOption82Enable.IsUnknown() {
			o["dhcpEnable"] = m.DHCPOption82Enable.ValueBool()
		}
	})
	return f
}

// apply refreshes state from the controller. psk is write-only and is
// deliberately never populated from the API.
func (r *wirelessResource) apply(w *omada.WirelessNetwork, m *wirelessResourceModel) {
	m.ID = types.StringValue(w.ID)
	m.Name = types.StringValue(w.Name)
	m.Band = types.Int64Value(int64(w.Band))
	m.Security = types.Int64Value(int64(w.Security))
	m.Broadcast = types.BoolValue(w.Broadcast)
	m.VLANEnable = types.BoolValue(w.VLANEnable)
	m.VLANID = types.Int64Value(int64(w.VLANID))
	m.GuestNet = types.BoolValue(w.GuestNet)

	m.PortalEnable = types.BoolValue(w.PortalEnable)
	m.AccessEnable = types.BoolValue(w.AccessEnable)
	m.WLANScheduleEnable = types.BoolValue(w.WLANScheduleEnable)
	m.MacFilterEnable = types.BoolValue(w.MacFilterEnable)
	m.Enable11r = types.BoolValue(w.Enable11r)
	m.PMFMode = types.Int64Value(int64(w.PMFMode))
	m.MLOEnable = types.BoolValue(w.MLOEnable)
	m.HidePwd = types.BoolValue(w.HidePwd)
	m.WanAccess = types.BoolValue(w.WanAccess)
	m.ProhibitWifiShare = types.BoolValue(w.ProhibitWifiShare)

	m.PSKVersion = types.Int64Value(int64(w.PSKSetting.VersionPsk))
	m.PSKEncryption = types.Int64Value(int64(w.PSKSetting.EncryptionPsk))
	m.PSKGikRekey = types.BoolValue(w.PSKSetting.GikRekey)

	m.RateLimitDown = types.BoolValue(w.RateLimit.DownLimitEnable)
	m.RateLimitUp = types.BoolValue(w.RateLimit.UpLimitEnable)
	m.SSIDRateLimitDown = types.BoolValue(w.SSIDRateLimit.DownLimitEnable)
	m.SSIDRateLimitUp = types.BoolValue(w.SSIDRateLimit.UpLimitEnable)

	m.RateCtrl2g = types.BoolValue(w.RateBeaconCtrl.Rate2g)
	m.RateCtrl5g = types.BoolValue(w.RateBeaconCtrl.Rate5g)
	m.RateCtrl6g = types.BoolValue(w.RateBeaconCtrl.Rate6g)
	m.ManageRate2g = types.BoolValue(w.RateBeaconCtrl.ManageRate2g)
	m.ManageRate5g = types.BoolValue(w.RateBeaconCtrl.ManageRate5g)

	m.MulticastEnable = types.BoolValue(w.MultiCastSetting.MultiCastEnable)
	m.MulticastChannelUtil = types.Int64Value(int64(w.MultiCastSetting.ChannelUtil))
	m.MulticastArpCast = types.BoolValue(w.MultiCastSetting.ArpCastEnable)
	m.MulticastIPv6Cast = types.BoolValue(w.MultiCastSetting.IPv6CastEnable)
	m.MulticastFilter = types.BoolValue(w.MultiCastSetting.FilterEnable)

	m.DHCPOption82Enable = types.BoolValue(w.DHCPOption82.DhcpEnable)
}

func (r *wirelessResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan wirelessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	created, err := r.data.client.CreateSSID(ctx, siteID, plan.WLANGroupID.ValueString(), r.fields(plan), plan.PSK.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create SSID", err.Error())
		return
	}
	r.apply(created, &plan)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wirelessResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state wirelessResourceModel
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
	w, err := r.data.client.GetSSID(ctx, siteID, state.WLANGroupID.ValueString(), state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	r.apply(w, &state)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *wirelessResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan wirelessResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	updated, err := r.data.client.UpdateSSID(ctx, siteID, plan.WLANGroupID.ValueString(), plan.ID.ValueString(), r.fields(plan), plan.PSK.ValueString())
	if err != nil {
		resp.Diagnostics.AddError("Unable to update SSID", err.Error())
		return
	}
	r.apply(updated, &plan)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *wirelessResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state wirelessResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteSSID(ctx, state.SiteID.ValueString(), state.WLANGroupID.ValueString(), state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete SSID", err.Error())
	}
}

// ImportState accepts "<wlan_group_id>/<ssid_id>" or "<site>/<wlan_group_id>/<ssid_id>".
func (r *wirelessResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	parts := strings.Split(req.ID, "/")
	switch len(parts) {
	case 3:
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wlan_group_id"), parts[1])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[2])...)
	case 2:
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("wlan_group_id"), parts[0])...)
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), parts[1])...)
	default:
		resp.Diagnostics.AddError("Invalid import ID", "expected '<wlan_group_id>/<ssid_id>' or '<site>/<wlan_group_id>/<ssid_id>'")
	}
}
