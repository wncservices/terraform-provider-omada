// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// listPageSize is the per-request page size for list endpoints. The controller
// caps a single page, so large object sets are fetched across several requests.
const listPageSize = 100

// pageURL appends the controller's pagination query to base, choosing the right
// separator if base already carries a query string (e.g. an ACL `type=` filter).
func pageURL(base string, page, size int) string {
	sep := "?"
	if strings.Contains(base, "?") {
		sep = "&"
	}
	return fmt.Sprintf("%s%scurrentPage=%d&currentPageSize=%d", base, sep, page, size)
}

// listAll fetches every page of a controller list endpoint and decodes the items
// into T. base is the endpoint path, optionally already carrying query params.
// The loop stops once it has collected TotalRows items or a page comes back
// empty (a controller that ignores paging returns everything on page 1, which
// still terminates here). `what` names the resource for error messages.
func listAll[T any](ctx context.Context, c *Client, what, base string) ([]T, error) {
	var all []T
	for page := 1; ; page++ {
		var pr PaginatedResult
		if err := c.Do(ctx, "GET", pageURL(base, page, listPageSize), nil, &pr); err != nil {
			return nil, fmt.Errorf("listing %s: %w", what, err)
		}
		var chunk []T
		if len(pr.Data) > 0 {
			if err := json.Unmarshal(pr.Data, &chunk); err != nil {
				return nil, fmt.Errorf("decoding %s page %d: %w", what, page, err)
			}
		}
		all = append(all, chunk...)
		if len(all) >= pr.TotalRows || len(chunk) == 0 {
			break
		}
	}
	return all, nil
}
