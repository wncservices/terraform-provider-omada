// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
	"fmt"
)

// Network is a LAN network (VLAN interface) on a site.
type Network struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Purpose       string `json:"purpose"`       // "interface" | "vlan"
	VLANID        int    `json:"vlan"`          // controller field is "vlan"
	GatewaySubnet string `json:"gatewaySubnet"` // CIDR, e.g. 192.168.30.1/24
	DHCPEnabled   bool   `json:"dhcpGuardEnable"`
}

// ListNetworks returns every LAN network for the given site, following pagination.
func (c *Client) ListNetworks(ctx context.Context, siteID string) ([]Network, error) {
	var all []Network
	page := 1
	const size = 100

	for {
		var pr PaginatedResult
		path := fmt.Sprintf("/sites/%s/setting/lan/networks?currentPage=%d&currentPageSize=%d", siteID, page, size)
		if err := c.Do(ctx, "GET", path, nil, &pr); err != nil {
			return nil, fmt.Errorf("listing networks: %w", err)
		}

		var chunk []Network
		if len(pr.Data) > 0 {
			if err := json.Unmarshal(pr.Data, &chunk); err != nil {
				return nil, fmt.Errorf("decoding networks page %d: %w", page, err)
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
