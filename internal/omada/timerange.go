// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
	"net/http"
)

// Time ranges are reusable schedule profiles (Settings -> Profiles -> Time
// Range). Other objects reference them rather than carrying their own schedule
// — an SSID's `wlanScheduleEnable`, for instance, is useless without one.
//
// The list endpoint returns `{"data": [...]}` with **no** `totalRows`, i.e. it
// does not paginate. listAll handles that correctly: with TotalRows zero it
// stops after the first page, which is where everything already is.

// TimeRangeSlot is one start/end window inside a time range.
type TimeRangeSlot struct {
	DayType    int `json:"dayType"`
	StartTimeH int `json:"startTimeH"`
	StartTimeM int `json:"startTimeM"`
	EndTimeH   int `json:"endTimeH"`
	EndTimeM   int `json:"endTimeM"`
	// RuleID is assigned by the controller; it is read but never sent.
	RuleID int `json:"ruleId,omitempty"`
}

// TimeRange is a schedule profile.
type TimeRange struct {
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name"`
	DayMode int             `json:"dayMode"`
	Mon     bool            `json:"dayMon"`
	Tue     bool            `json:"dayTue"`
	Wed     bool            `json:"dayWed"`
	Thu     bool            `json:"dayThu"`
	Fri     bool            `json:"dayFri"`
	Sat     bool            `json:"daySat"`
	Sun     bool            `json:"daySun"`
	Slots   []TimeRangeSlot `json:"timeList"`
}

func timeRangePath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/profiles/timeranges", siteID)
}

// ListTimeRanges returns every time-range profile on the site.
func (c *Client) ListTimeRanges(ctx context.Context, siteID string) ([]TimeRange, error) {
	return listAll[TimeRange](ctx, c, "time ranges", timeRangePath(siteID))
}

// GetTimeRange returns one time-range profile by id.
func (c *Client) GetTimeRange(ctx context.Context, siteID, id string) (*TimeRange, error) {
	items, err := c.ListTimeRanges(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range items {
		if items[i].ID == id {
			return &items[i], nil
		}
	}
	return nil, fmt.Errorf("time range %q not found", id)
}

// createTimeRangeResult is the create response: the new profile's id.
type createTimeRangeResult struct {
	ProfileID string `json:"profileId"`
}

// CreateTimeRange creates a time-range profile and returns its id.
func (c *Client) CreateTimeRange(ctx context.Context, siteID string, tr TimeRange) (string, error) {
	tr.ID = ""
	for i := range tr.Slots {
		tr.Slots[i].RuleID = 0 // controller-assigned
	}
	var out createTimeRangeResult
	if err := c.Do(ctx, http.MethodPost, timeRangePath(siteID), tr, &out); err != nil {
		return "", fmt.Errorf("creating time range: %w", err)
	}
	if out.ProfileID != "" {
		return out.ProfileID, nil
	}
	// Fall back to resolving by name if the controller returned no id.
	items, err := c.ListTimeRanges(ctx, siteID)
	if err != nil {
		return "", err
	}
	for _, it := range items {
		if it.Name == tr.Name {
			return it.ID, nil
		}
	}
	return "", fmt.Errorf("created time range %q but could not resolve its id", tr.Name)
}

// UpdateTimeRange updates a time-range profile in place.
func (c *Client) UpdateTimeRange(ctx context.Context, siteID, id string, tr TimeRange) error {
	tr.ID = id
	for i := range tr.Slots {
		tr.Slots[i].RuleID = 0
	}
	path := fmt.Sprintf("%s/%s", timeRangePath(siteID), id)
	if err := c.Do(ctx, http.MethodPatch, path, tr, nil); err != nil {
		return fmt.Errorf("updating time range: %w", err)
	}
	return nil
}

// DeleteTimeRange removes a time-range profile.
func (c *Client) DeleteTimeRange(ctx context.Context, siteID, id string) error {
	path := fmt.Sprintf("%s/%s", timeRangePath(siteID), id)
	if err := c.Do(ctx, http.MethodDelete, path, nil, nil); err != nil {
		return fmt.Errorf("deleting time range: %w", err)
	}
	return nil
}
