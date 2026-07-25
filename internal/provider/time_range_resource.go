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
	_ resource.Resource                = &timeRangeResource{}
	_ resource.ResourceWithConfigure   = &timeRangeResource{}
	_ resource.ResourceWithImportState = &timeRangeResource{}
)

func NewTimeRangeResource() resource.Resource { return &timeRangeResource{} }

type timeRangeResource struct{ data *providerData }

type timeRangeSlotModel struct {
	DayType     types.Int64 `tfsdk:"day_type"`
	StartHour   types.Int64 `tfsdk:"start_hour"`
	StartMinute types.Int64 `tfsdk:"start_minute"`
	EndHour     types.Int64 `tfsdk:"end_hour"`
	EndMinute   types.Int64 `tfsdk:"end_minute"`
}

type timeRangeResourceModel struct {
	ID      types.String `tfsdk:"id"`
	Site    types.String `tfsdk:"site"`
	SiteID  types.String `tfsdk:"site_id"`
	Name    types.String `tfsdk:"name"`
	DayMode types.Int64  `tfsdk:"day_mode"`
	Mon     types.Bool   `tfsdk:"monday"`
	Tue     types.Bool   `tfsdk:"tuesday"`
	Wed     types.Bool   `tfsdk:"wednesday"`
	Thu     types.Bool   `tfsdk:"thursday"`
	Fri     types.Bool   `tfsdk:"friday"`
	Sat     types.Bool   `tfsdk:"saturday"`
	Sun     types.Bool   `tfsdk:"sunday"`

	Slots []timeRangeSlotModel `tfsdk:"time_slots"`
}

func (r *timeRangeResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_time_range"
}

func (r *timeRangeResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages a reusable time-range (schedule) profile.\n\n" +
			"Other objects reference a time range rather than carrying their own schedule, so " +
			"this is what makes scheduling usable at all — an SSID's schedule flag, for example, " +
			"needs a profile to point at.",
		Attributes: map[string]schema.Attribute{
			"id":      schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":    schema.StringAttribute{Optional: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()}},
			"site_id": schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"name":    schema.StringAttribute{Required: true, MarkdownDescription: "Profile name."},
			"day_mode": schema.Int64Attribute{
				Optional: true, Computed: true, Default: int64default.StaticInt64(0),
				MarkdownDescription: "Controller day-selection mode. `0` uses the per-weekday flags below.",
			},
			"monday":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Active on Monday."},
			"tuesday":   schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Active on Tuesday."},
			"wednesday": schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Active on Wednesday."},
			"thursday":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Active on Thursday."},
			"friday":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Active on Friday."},
			"saturday":  schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Active on Saturday."},
			"sunday":    schema.BoolAttribute{Optional: true, Computed: true, Default: booldefault.StaticBool(false), MarkdownDescription: "Active on Sunday."},
		},
		Blocks: map[string]schema.Block{
			"time_slots": schema.ListNestedBlock{
				MarkdownDescription: "One or more start/end windows, in 24-hour local time.",
				NestedObject: schema.NestedBlockObject{
					Attributes: map[string]schema.Attribute{
						"day_type": schema.Int64Attribute{
							Optional: true, Computed: true, Default: int64default.StaticInt64(0),
							MarkdownDescription: "Controller day-type discriminator; `0` for an ordinary window.",
						},
						"start_hour":   schema.Int64Attribute{Required: true, MarkdownDescription: "Start hour, 0-23."},
						"start_minute": schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), MarkdownDescription: "Start minute, 0-59."},
						"end_hour":     schema.Int64Attribute{Required: true, MarkdownDescription: "End hour, 0-24."},
						"end_minute":   schema.Int64Attribute{Optional: true, Computed: true, Default: int64default.StaticInt64(0), MarkdownDescription: "End minute, 0-59."},
					},
				},
			},
		},
	}
}

func (r *timeRangeResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *timeRangeResource) siteName(m timeRangeResourceModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

// toAPI converts the plan into the client struct.
func (m timeRangeResourceModel) toAPI() omada.TimeRange {
	tr := omada.TimeRange{
		Name:    m.Name.ValueString(),
		DayMode: int(m.DayMode.ValueInt64()),
		Mon:     m.Mon.ValueBool(),
		Tue:     m.Tue.ValueBool(),
		Wed:     m.Wed.ValueBool(),
		Thu:     m.Thu.ValueBool(),
		Fri:     m.Fri.ValueBool(),
		Sat:     m.Sat.ValueBool(),
		Sun:     m.Sun.ValueBool(),
	}
	for _, s := range m.Slots {
		tr.Slots = append(tr.Slots, omada.TimeRangeSlot{
			DayType:    int(s.DayType.ValueInt64()),
			StartTimeH: int(s.StartHour.ValueInt64()),
			StartTimeM: int(s.StartMinute.ValueInt64()),
			EndTimeH:   int(s.EndHour.ValueInt64()),
			EndTimeM:   int(s.EndMinute.ValueInt64()),
		})
	}
	return tr
}

// refresh fills the model from a live time range.
func (m *timeRangeResourceModel) refresh(tr *omada.TimeRange) {
	m.ID = types.StringValue(tr.ID)
	m.Name = types.StringValue(tr.Name)
	m.DayMode = types.Int64Value(int64(tr.DayMode))
	m.Mon = types.BoolValue(tr.Mon)
	m.Tue = types.BoolValue(tr.Tue)
	m.Wed = types.BoolValue(tr.Wed)
	m.Thu = types.BoolValue(tr.Thu)
	m.Fri = types.BoolValue(tr.Fri)
	m.Sat = types.BoolValue(tr.Sat)
	m.Sun = types.BoolValue(tr.Sun)

	m.Slots = nil
	for _, s := range tr.Slots {
		m.Slots = append(m.Slots, timeRangeSlotModel{
			DayType:     types.Int64Value(int64(s.DayType)),
			StartHour:   types.Int64Value(int64(s.StartTimeH)),
			StartMinute: types.Int64Value(int64(s.StartTimeM)),
			EndHour:     types.Int64Value(int64(s.EndTimeH)),
			EndMinute:   types.Int64Value(int64(s.EndTimeM)),
		})
	}
}

func (r *timeRangeResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan timeRangeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID, err := r.data.client.ResolveSiteID(ctx, r.siteName(plan))
	if err != nil {
		resp.Diagnostics.AddError("Unable to resolve site", err.Error())
		return
	}
	id, err := r.data.client.CreateTimeRange(ctx, siteID, plan.toAPI())
	if err != nil {
		resp.Diagnostics.AddError("Unable to create time range", err.Error())
		return
	}
	tr, err := r.data.client.GetTimeRange(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read time range after create", err.Error())
		return
	}
	plan.refresh(tr)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *timeRangeResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state timeRangeResourceModel
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
	tr, err := r.data.client.GetTimeRange(ctx, siteID, state.ID.ValueString())
	if err != nil {
		// Gone upstream: drop it from state so Terraform plans a recreate.
		resp.State.RemoveResource(ctx)
		return
	}
	state.refresh(tr)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *timeRangeResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state timeRangeResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
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
	if err := r.data.client.UpdateTimeRange(ctx, siteID, id, plan.toAPI()); err != nil {
		resp.Diagnostics.AddError("Unable to update time range", err.Error())
		return
	}
	tr, err := r.data.client.GetTimeRange(ctx, siteID, id)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read time range after update", err.Error())
		return
	}
	plan.refresh(tr)
	plan.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *timeRangeResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state timeRangeResourceModel
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
	if err := r.data.client.DeleteTimeRange(ctx, siteID, state.ID.ValueString()); err != nil {
		resp.Diagnostics.AddError("Unable to delete time range", err.Error())
	}
}

// ImportState takes the profile id, or "<site>/<id>".
func (r *timeRangeResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	id := req.ID
	if site, rest, found := strings.Cut(id, "/"); found {
		resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), site)...)
		id = rest
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
}
