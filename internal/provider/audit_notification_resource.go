// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/boolplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/wncservices/terraform-provider-omada/internal/omada"
)

var (
	_ resource.Resource                = &auditNotificationResource{}
	_ resource.ResourceWithConfigure   = &auditNotificationResource{}
	_ resource.ResourceWithImportState = &auditNotificationResource{}
)

func NewAuditNotificationResource() resource.Resource { return &auditNotificationResource{} }

type auditNotificationResource struct{ data *providerData }

type auditNotificationModel struct {
	ID            types.String                `tfsdk:"id"`
	Site          types.String                `tfsdk:"site"`
	SiteID        types.String                `tfsdk:"site_id"`
	WebhookEnable types.Bool                  `tfsdk:"webhook_enable"`
	Logs          map[string]auditToggleModel `tfsdk:"log"`
}

// auditToggleModel is deliberately not notificationToggleModel: an audit-log
// entry carries only a `webhook` flag — no `email`, no `enable` — so reusing
// the richer model would declare attributes the controller has no field for.
type auditToggleModel struct {
	Webhook types.Bool `tfsdk:"webhook"`
}

func (r *auditNotificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_audit_notification"
}

func (r *auditNotificationResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Manages which audit-log categories are sent to the webhook.\n\n" +
			"This is a singleton per site — import it with the site name rather than creating it.\n\n" +
			"`log` is a **sparse map keyed by the controller's category key** (`AUTHENTICATION`, " +
			"`CLIENTS`, …): name only the categories you want to manage and the rest keep whatever the " +
			"controller has. Audit categories carry no `email` or `enable` toggle — only `webhook`.",
		Attributes: map[string]schema.Attribute{
			"id":             schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"site":           schema.StringAttribute{Optional: true, Computed: true, MarkdownDescription: "Site name. Defaults to the primary site. Changing forces replacement.", PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown(), stringplanmodifier.RequiresReplace()}},
			"site_id":        schema.StringAttribute{Computed: true, PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()}},
			"webhook_enable": schema.BoolAttribute{Optional: true, Computed: true, MarkdownDescription: "Deliver audit-log notifications to the webhook.", PlanModifiers: []planmodifier.Bool{boolplanmodifier.UseStateForUnknown()}},
			"log": schema.MapNestedAttribute{
				Optional:            true,
				MarkdownDescription: "Audit-log categories to manage, keyed by the controller's category key.",
				NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
					"webhook": schema.BoolAttribute{
						Optional:            true,
						MarkdownDescription: "Send this audit category to the webhook. Omit to leave the controller's value alone.",
					},
				}},
			},
		},
	}
}

func (r *auditNotificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

func (r *auditNotificationResource) siteName(m auditNotificationModel) string {
	if !m.Site.IsNull() && m.Site.ValueString() != "" {
		return m.Site.ValueString()
	}
	return r.data.defaultSite
}

func (m *auditNotificationModel) refresh(doc map[string]any) {
	if v, ok := omada.SettingBool(doc, "webhookSetting", "webhookEnable"); ok {
		m.WebhookEnable = types.BoolValue(v)
	} else {
		m.WebhookEnable = types.BoolNull()
	}
	live := omada.NotificationEntries(doc, "logNotifications")
	if m.Logs != nil {
		out := make(map[string]auditToggleModel, len(m.Logs))
		for key, want := range m.Logs {
			e, ok := live[key]
			if !ok {
				continue
			}
			t := auditToggleModel{Webhook: types.BoolNull()}
			if !want.Webhook.IsNull() {
				if v, ok := e["webhook"].(bool); ok {
					t.Webhook = types.BoolValue(v)
				}
			}
			out[key] = t
		}
		m.Logs = out
	}
}

func (r *auditNotificationResource) write(ctx context.Context, plan *auditNotificationModel, diags *diagSink) {
	site, err := r.data.client.ResolveSite(ctx, r.siteName(*plan))
	if err != nil {
		diags.AddError("Unable to resolve site", err.Error())
		return
	}
	groups := map[string]map[string]any{}
	if b := boolPtr(plan.WebhookEnable); b != nil {
		groups["webhookSetting"] = map[string]any{"webhookEnable": *b}
	}
	toggles := map[string]map[string]omada.NotificationToggle{}
	if len(plan.Logs) > 0 {
		byKey := make(map[string]omada.NotificationToggle, len(plan.Logs))
		for k, t := range plan.Logs {
			byKey[k] = omada.NotificationToggle{Webhook: boolPtr(t.Webhook)}
		}
		toggles["logNotifications"] = byKey
	}
	if err := r.data.client.UpdateNotificationDoc(ctx, site.ID, omada.AuditNotificationDoc, groups, toggles); err != nil {
		diags.AddError("Unable to update audit notifications", err.Error())
		return
	}
	doc, err := r.data.client.GetNotificationDoc(ctx, site.ID, omada.AuditNotificationDoc)
	if err != nil {
		diags.AddError("Unable to read audit notifications", err.Error())
		return
	}
	plan.refresh(doc)
	plan.ID = types.StringValue(site.ID)
	plan.SiteID = types.StringValue(site.ID)
	plan.Site = types.StringValue(site.Name)
}

func (r *auditNotificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan auditNotificationModel
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

func (r *auditNotificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan auditNotificationModel
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

func (r *auditNotificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state auditNotificationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	siteID := state.SiteID.ValueString()
	if siteID == "" {
		site, err := r.data.client.ResolveSite(ctx, r.siteName(state))
		if err != nil {
			resp.Diagnostics.AddError("Unable to resolve site", err.Error())
			return
		}
		siteID = site.ID
		state.Site = types.StringValue(site.Name)
	}
	doc, err := r.data.client.GetNotificationDoc(ctx, siteID, omada.AuditNotificationDoc)
	if err != nil {
		resp.Diagnostics.AddError("Unable to read audit notifications", err.Error())
		return
	}
	state.refresh(doc)
	state.ID = types.StringValue(siteID)
	state.SiteID = types.StringValue(siteID)
	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

// Delete is a no-op: audit notification settings always exist.
func (r *auditNotificationResource) Delete(_ context.Context, _ resource.DeleteRequest, _ *resource.DeleteResponse) {
}

// ImportState takes the site name.
func (r *auditNotificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if req.ID == "" {
		resp.Diagnostics.AddError("Invalid import ID",
			"expected the site name, e.g. `terraform import omada_audit_notification.this Home`")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("site"), req.ID)...)
}
