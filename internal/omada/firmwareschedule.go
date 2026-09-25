// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// Firmware auto-upgrade schedules (Global View -> Firmware -> Periodic Update).
//
// Controller-scoped like ControllerSettings (DESIGN §2.7b): there is no site in
// the path. Each schedule instead names the sites and device models it covers,
// and at the scheduled time the controller upgrades those models on those sites
// to the newest firmware in the chosen channel.
//
//	GET    /upgrade/autoCheck               list (paginated); there is no GET by id
//	POST   /upgrade/autoCheck               create -> {"autoCheckId": "..."}
//	PATCH  /upgrade/autoCheck/{id}          update; takes the whole body
//	DELETE /upgrade/autoCheck/{id}          delete
//	GET    /upgrade/autoCheck/sites/{id}    the schedule's sites as {id, name}
//	POST   /upgrade/models                  {"siteIds": [...]} -> models on those sites
//
// Verified against a live v6.3.0.45 software controller with disposable
// schedules (create, update, delete), mapped from the web app's firmware page.
// Behaviour the client depends on:
//
//   - A PATCH carrying only the changed fields is refused (-1001 "must not be
//     null"), so Update always sends the complete schedule.
//   - DELETE of an id that does not exist still returns success, so absence is
//     detected from the list, never from a delete.
//   - The list reports sites by name only; the ids come from /sites/{id}.
//   - Several schedules may cover the same model; the controller does not
//     reject the overlap.
//   - modelTypeInfos entries need compoundModel and showModel; the version the
//     model list also returns is not required on write.

// Timing types for FirmwareUpgradeOccurrence.TimingType, in the order the UI
// offers them. Live: a weekly schedule is 2 and a yearly one 4.
const (
	FirmwareTimingDaily   = 1
	FirmwareTimingWeekly  = 2
	FirmwareTimingMonthly = 3
	FirmwareTimingYearly  = 4
)

// FirmwareChannelStable is the only channel the controller offers for
// periodic updates (the UI lists nothing else).
const FirmwareChannelStable = 0

const firmwareSchedulePath = "/upgrade/autoCheck"

// ErrFirmwareUpgradeScheduleNotFound is returned by GetFirmwareUpgradeSchedule
// when no schedule has the id, so callers can tell a deleted schedule apart
// from a failed request.
var ErrFirmwareUpgradeScheduleNotFound = errors.New("firmware upgrade schedule not found")

// FirmwareUpgradeOccurrence is when a schedule runs, in the site's local time.
// Every field is always sent; the controller reads only the ones its timing
// type uses (DayOfWeek for weekly, DayOfMonth for monthly, DayOfMonth and
// MonthOfYear for yearly). DayOfWeek is 0 = Sunday .. 6 = Saturday and
// MonthOfYear is 1 = January .. 12 = December (both verified live).
type FirmwareUpgradeOccurrence struct {
	TimingType  int `json:"timingType"`
	Hour        int `json:"hour"`
	Minute      int `json:"minute"`
	DayOfWeek   int `json:"dayOfWeek"`
	DayOfMonth  int `json:"dayOfMonth"`
	MonthOfYear int `json:"monthOfYear"`
}

// FirmwareModelInfo identifies a device model, e.g. "EAP670(US) v2.0".
type FirmwareModelInfo struct {
	CompoundModel string `json:"compoundModel"`
	ShowModel     string `json:"showModel"`
	Version       string `json:"version,omitempty"`
}

// FirmwareUpgradeSchedule is one periodic-update rule.
type FirmwareUpgradeSchedule struct {
	ID         string
	SiteIDs    []string
	SiteNames  []string
	Models     []FirmwareModelInfo
	Occurrence FirmwareUpgradeOccurrence
	Channel    int
}

// firmwareScheduleBody is the create/update payload.
type firmwareScheduleBody struct {
	SiteIDs        []string                  `json:"siteIds"`
	ModelTypeInfos []FirmwareModelInfo       `json:"modelTypeInfos"`
	Occurrence     FirmwareUpgradeOccurrence `json:"occurrence"`
	Channel        int                       `json:"channel"`
}

// firmwareScheduleEntry is one row of the list endpoint.
type firmwareScheduleEntry struct {
	ID             string                    `json:"id"`
	ModelTypeInfos []FirmwareModelInfo       `json:"modelTypeInfos"`
	Occurrence     FirmwareUpgradeOccurrence `json:"occurrence"`
	Channel        int                       `json:"channel"`
}

type firmwareScheduleSite struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func firmwareBody(in *FirmwareUpgradeSchedule) firmwareScheduleBody {
	sites := in.SiteIDs
	if sites == nil {
		sites = []string{}
	}
	models := in.Models
	if models == nil {
		models = []FirmwareModelInfo{}
	}
	return firmwareScheduleBody{SiteIDs: sites, ModelTypeInfos: models, Occurrence: in.Occurrence, Channel: in.Channel}
}

// FirmwareModelsForSites returns the device models present on the given sites,
// as the controller names them. The web UI offers exactly this list when a
// schedule is created, and a schedule's models must come from it.
func (c *Client) FirmwareModelsForSites(ctx context.Context, siteIDs []string) ([]FirmwareModelInfo, error) {
	var out struct {
		ModelTypeInfos []FirmwareModelInfo `json:"modelTypeInfos"`
	}
	if err := c.Do(ctx, http.MethodPost, "/upgrade/models", map[string]any{"siteIds": siteIDs}, &out); err != nil {
		return nil, fmt.Errorf("listing firmware models: %w", err)
	}
	return out.ModelTypeInfos, nil
}

// GetFirmwareUpgradeSchedule returns one schedule, with its sites resolved to
// ids and names. It returns ErrFirmwareUpgradeScheduleNotFound when the id is
// not in the list.
func (c *Client) GetFirmwareUpgradeSchedule(ctx context.Context, id string) (*FirmwareUpgradeSchedule, error) {
	entries, err := listAll[firmwareScheduleEntry](ctx, c, "firmware upgrade schedules", firmwareSchedulePath)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.ID != id {
			continue
		}
		var sites struct {
			Sites []firmwareScheduleSite `json:"sites"`
		}
		if err := c.Do(ctx, http.MethodGet, firmwareSchedulePath+"/sites/"+id, nil, &sites); err != nil {
			return nil, fmt.Errorf("reading sites of firmware upgrade schedule %q: %w", id, err)
		}
		out := &FirmwareUpgradeSchedule{ID: e.ID, Models: e.ModelTypeInfos, Occurrence: e.Occurrence, Channel: e.Channel}
		for _, s := range sites.Sites {
			out.SiteIDs = append(out.SiteIDs, s.ID)
			out.SiteNames = append(out.SiteNames, s.Name)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%w: %q", ErrFirmwareUpgradeScheduleNotFound, id)
}

// CreateFirmwareUpgradeSchedule creates a schedule and returns it as read back.
func (c *Client) CreateFirmwareUpgradeSchedule(ctx context.Context, in *FirmwareUpgradeSchedule) (*FirmwareUpgradeSchedule, error) {
	var out struct {
		AutoCheckID string `json:"autoCheckId"`
	}
	if err := c.Do(ctx, http.MethodPost, firmwareSchedulePath, firmwareBody(in), &out); err != nil {
		return nil, fmt.Errorf("creating firmware upgrade schedule: %w", err)
	}
	if out.AutoCheckID == "" {
		return nil, errors.New("creating firmware upgrade schedule: controller returned no autoCheckId")
	}
	return c.GetFirmwareUpgradeSchedule(ctx, out.AutoCheckID)
}

// UpdateFirmwareUpgradeSchedule replaces a schedule. The controller refuses a
// partial PATCH, so the whole schedule is sent every time.
func (c *Client) UpdateFirmwareUpgradeSchedule(ctx context.Context, id string, in *FirmwareUpgradeSchedule) (*FirmwareUpgradeSchedule, error) {
	if err := c.Do(ctx, http.MethodPatch, firmwareSchedulePath+"/"+id, firmwareBody(in), nil); err != nil {
		return nil, fmt.Errorf("updating firmware upgrade schedule %q: %w", id, err)
	}
	return c.GetFirmwareUpgradeSchedule(ctx, id)
}

// DeleteFirmwareUpgradeSchedule removes a schedule. The controller answers
// success for an unknown id as well.
func (c *Client) DeleteFirmwareUpgradeSchedule(ctx context.Context, id string) error {
	if err := c.Do(ctx, http.MethodDelete, firmwareSchedulePath+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting firmware upgrade schedule %q: %w", id, err)
	}
	return nil
}
