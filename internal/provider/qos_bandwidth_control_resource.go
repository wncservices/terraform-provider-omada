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
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                   = &qosBandwidthControlResource{}
	_ resource.ResourceWithConfigure      = &qosBandwidthControlResource{}
	_ resource.ResourceWithImportState    = &qosBandwidthControlResource{}
	_ resource.ResourceWithValidateConfig = &qosBandwidthControlResource{}
)

func NewQoSBandwidthControlResource() resource.Resource { return &qosBandwidthControlResource{} }

type qosBandwidthControlResource struct{ data *providerData }

type qosBandwidthControlModel struct {
	ID                types.String `tfsdk:"id"`
	Site              types.String `tfsdk:"site"`
	SiteID            types.String `tfsdk:"site_id"`
	WAN               types.String `tfsdk:"wan"`
	Enable            types.Bool   `tfsdk:"enable"`
	Direction         types.Int64  `tfsdk:"direction"`
	InBandwidth       types.Int64  `tfsdk:"in_bandwidth"`
	OutBandwidth      types.Int64  `tfsdk:"out_bandwidth"`
	ClassRatio        types.List   `tfsdk:"class_ratio"`
	OutPrioritization types.Bool   `tfsdk:"out_prioritization"`
	UDPBandwidthCtrl  types.Bool   `tfsdk:"udp_bandwidth_control"`
}

func (r *qosBandwidthControlResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_qos_bandwidth_control"
}

func (r *qosBandwidthControlResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages gateway bandwidth control (QoS) for one WAN port.\n\n" +
			"The controller permits **one rule per WAN port** and rejects a second with `-43310`. " +
			"Since a rule is identified by its WAN, changing `wan` replaces the rule.\n\n" +
			"~> **`in_bandwidth` and `out_bandwidth` must match the real link rate.** The shaper " +
			"divides these figures between priority classes, so a value above the true line rate makes " +
			"the queueing ineffective, and one below it throttles the connection permanently.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"wan": schema.StringAttribute{
				Required: true,
				MarkdownDescription: "WAN interface id, in the controller's `1_<hex>` form — the same identifier " +
					"`omada_disable_nat.interface` takes. Changing it forces replacement.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"enable": schema.BoolAttribute{
				Optional: true, Computed: true, Default: booldefault.StaticBool(false),
				MarkdownDescription: "Whether shaping is active. Defaults to `false` so a rule can be staged " +
					"and reviewed before it touches traffic.",
			},
			"direction": schema.Int64Attribute{
				Required:            true,
				MarkdownDescription: "Controller enum for the direction shaped. The UI writes `2`.",
			},
			"in_bandwidth":  schema.Int64Attribute{Required: true, MarkdownDescription: "Downstream link rate, in kbps (e.g. `1000000` for 1 Gbps)."},
			"out_bandwidth": schema.Int64Attribute{Required: true, MarkdownDescription: "Upstream link rate, in kbps."},
			"class_ratio": schema.ListAttribute{
				ElementType:         types.Int64Type,
				Required:            true,
				MarkdownDescription: "Share of bandwidth per priority class, as four percentages summing to 100.",
			},
			"out_prioritization":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Prioritise egress traffic."},
			"udp_bandwidth_control": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Apply shaping to UDP as well as TCP."},
		},
	}
}

func (r *qosBandwidthControlResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *qosBandwidthControlResource) siteName(m qosBandwidthControlModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (r *qosBandwidthControlResource) resolveSite(ctx context.Context, m qosBandwidthControlModel, diags *diagSink) string {
	if id := m.SiteID.ValueString(); id != "" {
		return id
	}
	id, err := r.data.client.ResolveSiteID(ctx, r.siteName(m))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return ""
	}
	return id
}

func (r *qosBandwidthControlResource) toAPI(ctx context.Context, m qosBandwidthControlModel, diags *diagSink) omada.QoSBandwidthControl {
	ratio, d := intSlice(ctx, m.ClassRatio)
	if d.HasError() {
		diags.AddError("Invalid class_ratio", "could not read the class ratio list")
	}
	return omada.QoSBandwidthControl{
		WAN:               m.WAN.ValueString(),
		Status:            m.Enable.ValueBool(),
		Direction:         int(m.Direction.ValueInt64()),
		InBandwidth:       int(m.InBandwidth.ValueInt64()),
		OutBandwidth:      int(m.OutBandwidth.ValueInt64()),
		ClassRatio:        ratio,
		OutPrioritization: m.OutPrioritization.ValueBool(),
		UDPBandwidthCtrl:  m.UDPBandwidthCtrl.ValueBool(),
	}
}

func (m *qosBandwidthControlModel) refresh(ctx context.Context, b *omada.QoSBandwidthControl) {
	m.ID = types.StringValue(b.ID)
	m.WAN = types.StringValue(b.WAN)
	m.Enable = types.BoolValue(b.Status)
	m.Direction = types.Int64Value(int64(b.Direction))
	m.InBandwidth = types.Int64Value(int64(b.InBandwidth))
	m.OutBandwidth = types.Int64Value(int64(b.OutBandwidth))
	m.OutPrioritization = types.BoolValue(b.OutPrioritization)
	m.UDPBandwidthCtrl = types.BoolValue(b.UDPBandwidthCtrl)
	lv, _ := intListValue(ctx, b.ClassRatio)
	m.ClassRatio = lv
}

func (r *qosBandwidthControlResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan qosBandwidthControlModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sink := &diagSink{&resp.Diagnostics}
	siteID := r.resolveSite(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.toAPI(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	id, err := r.data.client.CreateQoSBandwidthControl(ctx, siteID, body)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create bandwidth control", err.Error())
		return
	}
	cur, err := r.data.client.GetQoSBandwidthControl(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read bandwidth control after create", err.Error())
		return
	}
	plan.refresh(ctx, cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *qosBandwidthControlResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state qosBandwidthControlModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	cur, err := r.data.client.GetQoSBandwidthControl(ctx, siteID, state.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	state.refresh(ctx, cur)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *qosBandwidthControlResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state qosBandwidthControlModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	sink := &diagSink{&resp.Diagnostics}
	siteID := r.resolveSite(ctx, state, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	body := r.toAPI(ctx, plan, sink)
	if resp.Diagnostics.HasError() {
		return
	}
	id := state.ID.ValueString()
	if err := r.data.client.UpdateQoSBandwidthControl(ctx, siteID, id, body); err != nil {
		resp.Diagnostics.AddError("Unable to update bandwidth control", err.Error())
		return
	}
	cur, err := r.data.client.GetQoSBandwidthControl(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read bandwidth control after update", err.Error())
		return
	}
	plan.refresh(ctx, cur)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *qosBandwidthControlResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state qosBandwidthControlModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := r.resolveSite(ctx, state, &diagSink{&resp.Diagnostics})
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteQoSBandwidthControl(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete bandwidth control", err.Error())
	}
}

// ImportState takes the rule id, or "<site>/<id>".
func (r *qosBandwidthControlResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}

// ValidateConfig checks class_ratio at plan time rather than letting the
// controller reject it during apply. Both rules are confirmed against
// hardware: four entries, and they must total 100.
func (r *qosBandwidthControlResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var cfg qosBandwidthControlModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() || cfg.ClassRatio.IsNull() || cfg.ClassRatio.IsUnknown() {
		return
	}
	ratio, d := intSlice(ctx, cfg.ClassRatio)
	resp.Diagnostics.Append(d...)
	if resp.Diagnostics.HasError() {
		return
	}
	if len(ratio) != 4 {
		resp.Diagnostics.AddAttributeError(path.Root("class_ratio"),
			"class_ratio must have exactly four entries",
			fmt.Sprintf("the gateway shapes traffic into four priority classes; got %d entries", len(ratio)))
		return
	}
	sum := 0
	for _, v := range ratio {
		sum += v
	}
	if sum != 100 {
		resp.Diagnostics.AddAttributeError(path.Root("class_ratio"),
			"class_ratio must total 100",
			fmt.Sprintf("the four class shares are percentages of the link rate; got %d", sum))
	}
}
