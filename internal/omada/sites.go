// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
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
	var all []Site
	page := 1
	const size = 100

	for {
		var pr PaginatedResult
		path := fmt.Sprintf("/sites?currentPage=%d&currentPageSize=%d", page, size)
		if err := c.Do(ctx, "GET", path, nil, &pr); err != nil {
			return nil, fmt.Errorf("listing sites: %w", err)
		}

		var chunk []Site
		if len(pr.Data) > 0 {
			if err := json.Unmarshal(pr.Data, &chunk); err != nil {
				return nil, fmt.Errorf("decoding sites page %d: %w", page, err)
			}
		}
		all = append(all, chunk...)

		if len(all) >= pr.TotalRows || len(chunk) == 0 {
			break
		}
		page++
	}
	return all, nil
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
