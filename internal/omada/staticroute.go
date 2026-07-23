// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// Static route types.
const (
	RouteTypeNextHop = 0
)

// StaticRoute is a gateway static route. Verified against a live v6.2
// controller (/setting/transmission/staticRoutings): create is POST, update is
// PUT (PATCH is rejected), delete is DELETE.
type StaticRoute struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Status       bool     `json:"status"`
	Destinations []string `json:"destinations"`
	RouteType    int      `json:"routeType"`
	NextHopIP    string   `json:"nextHopIp"`
	Metric       int      `json:"metric"`
}

func staticRoutesPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/transmission/staticRoutings", siteID)
}

// ListStaticRoutes returns all static routes for a site.
func (c *Client) ListStaticRoutes(ctx context.Context, siteID string) ([]StaticRoute, error) {
	return listAll[StaticRoute](ctx, c, "static routes", staticRoutesPath(siteID))
}

// GetStaticRoute returns one route by id.
func (c *Client) GetStaticRoute(ctx context.Context, siteID, id string) (*StaticRoute, error) {
	routes, err := c.ListStaticRoutes(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range routes {
		if routes[i].ID == id {
			return &routes[i], nil
		}
	}
	return nil, fmt.Errorf("static route %q not found on site %q", id, siteID)
}

// CreateStaticRoute creates a route (null result → resolved by name).
func (c *Client) CreateStaticRoute(ctx context.Context, siteID string, in *StaticRoute) (*StaticRoute, error) {
	if in.Destinations == nil {
		in.Destinations = []string{}
	}
	if err := c.Do(ctx, "POST", staticRoutesPath(siteID), in, nil); err != nil {
		return nil, fmt.Errorf("creating static route: %w", err)
	}
	routes, err := c.ListStaticRoutes(ctx, siteID)
	if err != nil {
		return nil, err
	}
	for i := range routes {
		if routes[i].Name == in.Name {
			return &routes[i], nil
		}
	}
	return nil, fmt.Errorf("static route %q not found after create", in.Name)
}

// UpdateStaticRoute replaces a route (PUT — PATCH is unsupported here).
func (c *Client) UpdateStaticRoute(ctx context.Context, siteID, id string, in *StaticRoute) (*StaticRoute, error) {
	in.ID = id
	if in.Destinations == nil {
		in.Destinations = []string{}
	}
	if err := c.Do(ctx, "PUT", staticRoutesPath(siteID)+"/"+id, in, nil); err != nil {
		return nil, fmt.Errorf("updating static route %q: %w", id, err)
	}
	return c.GetStaticRoute(ctx, siteID, id)
}

// DeleteStaticRoute removes a route.
func (c *Client) DeleteStaticRoute(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", staticRoutesPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting static route %q: %w", id, err)
	}
	return nil
}
