// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// Site is a controller site.
type Site struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Primary bool   `json:"primary"`
}

// ListSites returns every site on the controller, following pagination.
func (c *Client) ListSites(ctx context.Context) ([]Site, error) {
	return listAll[Site](ctx, c, "sites", "/sites")
}

// ResolveSiteID returns the controller id for the given site name. If name is
// empty it returns the primary site (or the only site, if there is just one).
func (c *Client) ResolveSiteID(ctx context.Context, name string) (string, error) {
	sites, err := c.ListSites(ctx)
	if err != nil {
		return "", err
	}
	if name == "" {
		for _, s := range sites {
			if s.Primary {
				return s.ID, nil
			}
		}
		if len(sites) == 1 {
			return sites[0].ID, nil
		}
		return "", fmt.Errorf("no site specified and no primary site found; set the provider or resource `site`")
	}
	for _, s := range sites {
		if s.Name == name {
			return s.ID, nil
		}
	}
	return "", fmt.Errorf("no site named %q found on the controller", name)
}
