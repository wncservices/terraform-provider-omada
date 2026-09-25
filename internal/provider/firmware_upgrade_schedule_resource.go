// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/int64validator"
	"github.com/hashicorp/terraform-plugin-framework-validators/setvalidator"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &firmwareUpgradeScheduleResource{}
	_ resource.ResourceWithConfigure   = &firmwareUpgradeScheduleResource{}
	_ resource.ResourceWithImportState = &firmwareUpgradeScheduleResource{}
)

func NewFirmwareUpgradeScheduleResource() resource.Resource {
	return &firmwareUpgradeScheduleResource{}
}

// firmwareUpgradeScheduleResource is controller-scoped (DESIGN §2.7b): it has
// no `site`. The sites it covers are a list of names like any other attribute.
type firmwareUpgradeScheduleResource struct{ data *providerData }

type firmwareUpgradeScheduleResourceModel struct {
	ID          types.String `tfsdk:"id"`
	Sites       types.Set    `tfsdk:"sites"`
	Models      types.Set    `tfsdk:"models"`
	TimingType  types.Int64  `tfsdk:"timing_type"`
	Hour        types.Int64  `tfsdk:"hour"`
	Minute      types.Int64  `tfsdk:"minute"`
	DayOfWeek   types.Int64  `tfsdk:"day_of_week"`
	DayOfMonth  types.Int64  `tfsdk:"day_of_month"`
	MonthOfYear types.Int64  `tfsdk:"month_of_year"`
	Channel     types.Int64  `tfsdk:"channel"`
}

func (r *firmwareUpgradeScheduleResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_firmware_upgrade_schedule"
}

func (r *firmwareUpgradeScheduleResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a periodic firmware upgrade schedule (Global View → Firmware → Periodic Update).\n\n" +
			"At the scheduled time the controller upgrades every device of the listed `models` on the listed " +
			"`sites` to the newest firmware in the channel. Devices reboot as they upgrade, so pick a " +
			"maintenance window.\n\n" +
			"**Controller-scoped:** there is no `site` attribute, because a schedule belongs to the " +
			"controller and names its sites itself. This is not the same setting as " +
			"`omada_site_settings.auto_upgrade_enable`, which upgrades a site's devices whenever the " +
			"controller notices a release, with no schedule.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"sites": schema.SetAttribute{
				ElementType: types.StringType, Required: true,
				Validators:          []validator.Set{setvalidator.SizeAtLeast(1)},
				MarkdownDescription: "Names of the sites the schedule covers.",
			},
			"models": schema.SetAttribute{
				ElementType: types.StringType, Required: true,
				Validators: []validator.Set{setvalidator.SizeAtLeast(1)},
				MarkdownDescription: "Device models to upgrade, as the controller names them (its `compoundModel`), " +
					"e.g. `EAP670(US) v2.0` or `ES205G v1.20`. Each must be present on at least one of the " +
					"`sites`: the controller only offers models it has adopted there.",
			},
			"timing_type": schema.Int64Attribute{
				Required:            true,
				Validators:          []validator.Int64{int64validator.Between(omada.FirmwareTimingDaily, omada.FirmwareTimingYearly)},
				MarkdownDescription: "How often it runs: `1` daily, `2` weekly, `3` monthly, `4` yearly.",
			},
			"hour": schema.Int64Attribute{
				Required:            true,
				Validators:          []validator.Int64{int64validator.Between(0, 23)},
				MarkdownDescription: "Hour of the run, 0-23, in the site's local time.",
			},
			"minute": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				Validators:          []validator.Int64{int64validator.Between(0, 59)},
				MarkdownDescription: "Minute of the run, 0-59.",
			},
			"day_of_week": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				Validators:          []validator.Int64{int64validator.Between(0, 6)},
				MarkdownDescription: "Weekly schedules: `0` Sunday through `6` Saturday. Ignored otherwise.",
			},
			"day_of_month": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(1),
				Validators:          []validator.Int64{int64validator.Between(1, 31)},
				MarkdownDescription: "Monthly and yearly schedules: day of the month, 1-31. Ignored otherwise.",
			},
			"month_of_year": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(1),
				Validators:          []validator.Int64{int64validator.Between(1, 12)},
				MarkdownDescription: "Yearly schedules: `1` January through `12` December. Ignored otherwise.",
			},
			"channel": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(omada.FirmwareChannelStable),
				MarkdownDescription: "Firmware channel. `0` is Stable, the only channel the controller offers here.",
			},
		},
	}
}

func (r *firmwareUpgradeScheduleResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	if data, ok := req.ProviderData.(*providerData); ok {
		r.data = data
	} else {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("expected *providerData, got %T", req.ProviderData))
	}
}

func firmwareStringSet(ctx context.Context, s types.Set) ([]string, diag.Diagnostics) {
	if s.IsNull() || s.IsUnknown() {
		return nil, nil
	}
	var out []string
	d := s.ElementsAs(ctx, &out, false)
	return out, d
}

// inputFrom resolves the plan into a request: site names to ids, and model
// names to the controller's model records for those sites.
func (r *firmwareUpgradeScheduleResource) inputFrom(ctx context.Context, m firmwareUpgradeScheduleResourceModel) (*omada.FirmwareUpgradeSchedule, diag.Diagnostics) {
	var diags diag.Diagnostics
	siteNames, d := firmwareStringSet(ctx, m.Sites)
	diags.Append(d...)
	modelNames, d := firmwareStringSet(ctx, m.Models)
	diags.Append(d...)
	if diags.HasError() {
		return nil, diags
	}

	siteIDs := make([]string, 0, len(siteNames))
	for _, name := range siteNames {
		id, err := r.data.client.ResolveSiteID(ctx, name)
		if err != nil {
			diags.AddAttributeError(path.Root("sites"), "Unable to resolve site", err.Error())
			return nil, diags
		}
		siteIDs = append(siteIDs, id)
	}

	available, err := r.data.client.FirmwareModelsForSites(ctx, siteIDs)
	if err != nil {
		diags.AddError("Unable to list device models", err.Error())
		return nil, diags
	}
	byName := make(map[string]omada.FirmwareModelInfo, len(available))
	for _, a := range available {
		byName[a.CompoundModel] = omada.FirmwareModelInfo{CompoundModel: a.CompoundModel, ShowModel: a.ShowModel}
	}
	models := make([]omada.FirmwareModelInfo, 0, len(modelNames))
	for _, name := range modelNames {
		info, ok := byName[name]
		if !ok {
			names := make([]string, 0, len(byName))
			for n := range byName {
				names = append(names, n)
			}
			sort.Strings(names)
			diags.AddAttributeError(path.Root("models"), "Unknown device model",
				fmt.Sprintf("%q is not a model on the selected sites. Models there: %s.", name, strings.Join(names, ", ")))
			return nil, diags
		}
		models = append(models, info)
	}

	return &omada.FirmwareUpgradeSchedule{
		SiteIDs: siteIDs,
		Models:  models,
		Occurrence: omada.FirmwareUpgradeOccurrence{
			TimingType:  int(m.TimingType.ValueInt64()),
			Hour:        int(m.Hour.ValueInt64()),
			Minute:      int(m.Minute.ValueInt64()),
			DayOfWeek:   int(m.DayOfWeek.ValueInt64()),
			DayOfMonth:  int(m.DayOfMonth.ValueInt64()),
			MonthOfYear: int(m.MonthOfYear.ValueInt64()),
		},
		Channel: int(m.Channel.ValueInt64()),
	}, diags
}

func (r *firmwareUpgradeScheduleResource) apply(ctx context.Context, s *omada.FirmwareUpgradeSchedule, m *firmwareUpgradeScheduleResourceModel) diag.Diagnostics {
	var diags diag.Diagnostics
	m.ID = types.StringValue(s.ID)

	sites, d := types.SetValueFrom(ctx, types.StringType, nilToEmpty(s.SiteNames))
	diags.Append(d...)
	m.Sites = sites

	modelNames := make([]string, 0, len(s.Models))
	for _, mi := range s.Models {
		modelNames = append(modelNames, mi.CompoundModel)
	}
	models, d := types.SetValueFrom(ctx, types.StringType, modelNames)
	diags.Append(d...)
	m.Models = models

	m.TimingType = types.Int64Value(int64(s.Occurrence.TimingType))
	m.Hour = types.Int64Value(int64(s.Occurrence.Hour))
	m.Minute = types.Int64Value(int64(s.Occurrence.Minute))
	m.DayOfWeek = types.Int64Value(int64(s.Occurrence.DayOfWeek))
	m.DayOfMonth = types.Int64Value(int64(s.Occurrence.DayOfMonth))
	m.MonthOfYear = types.Int64Value(int64(s.Occurrence.MonthOfYear))
	m.Channel = types.Int64Value(int64(s.Channel))
	return diags
}

func (r *firmwareUpgradeScheduleResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan firmwareUpgradeScheduleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := r.inputFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	created, err := r.data.client.CreateFirmwareUpgradeSchedule(ctx, in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create firmware upgrade schedule", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, created, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firmwareUpgradeScheduleResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state firmwareUpgradeScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	s, err := r.data.client.GetFirmwareUpgradeSchedule(ctx, state.ID.ValueString())
	if errors.Is(err, omada.ErrFirmwareUpgradeScheduleNotFound) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Unable to read firmware upgrade schedule", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, s, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *firmwareUpgradeScheduleResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan firmwareUpgradeScheduleResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	in, diags := r.inputFrom(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	updated, err := r.data.client.UpdateFirmwareUpgradeSchedule(ctx, plan.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update firmware upgrade schedule", err.Error())
		return
	}
	resp.Diagnostics.Append(r.apply(ctx, updated, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *firmwareUpgradeScheduleResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state firmwareUpgradeScheduleResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteFirmwareUpgradeSchedule(ctx, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete firmware upgrade schedule", err.Error())
	}
}

// ImportState takes the schedule id. The web UI does not show it; read it from
// GET /{omadacId}/api/v2/upgrade/autoCheck.
func (r *firmwareUpgradeScheduleResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}
