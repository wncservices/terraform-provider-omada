// Copyright (c) wncservices
// SPDX-License-Identifier: MPL-2.0

package omada

import (
	"context"
	"fmt"
)

// ACL type values.
const (
	ACLTypeGateway = 0
	ACLTypeSwitch  = 1
	ACLTypeEAP     = 2
)

// ACLDirection is the traffic direction for a gateway ACL.
type ACLDirection struct {
	LanToWan bool     `json:"lanToWan"`
	LanToLan bool     `json:"lanToLan"`
	WanInIDs []string `json:"wanInIds"`
	VpnInIDs []string `json:"vpnInIds"`
}

// ACL is a firewall ACL rule. Verified against a live v6.2 controller
// (/setting/firewall/acls?type=N). Create returns null; update uses PUT.
type ACL struct {
	ID              string       `json:"id"`
	Type            int          `json:"type"`
	Name            string       `json:"name"`
	Status          bool         `json:"status"`
	Policy          int          `json:"policy"` // 0=deny, 1=permit
	Protocols       []int        `json:"protocols"`
	SourceType      int          `json:"sourceType"`
	SourceIDs       []string     `json:"sourceIds"`
	DestinationType int          `json:"destinationType"`
	DestinationIDs  []string     `json:"destinationIds"`
	Direction       ACLDirection `json:"direction"`
}

// ACLInput is the create/update payload. customAclPorts/customAclDevices are
// sent empty (not yet modelled); ID is set for PUT updates.
type ACLInput struct {
	ID               string       `json:"id,omitempty"`
	Type             int          `json:"type"`
	Name             string       `json:"name"`
	Status           bool         `json:"status"`
	Policy           int          `json:"policy"`
	Protocols        []int        `json:"protocols"`
	SourceType       int          `json:"sourceType"`
	SourceIDs        []string     `json:"sourceIds"`
	DestinationType  int          `json:"destinationType"`
	DestinationIDs   []string     `json:"destinationIds"`
	Direction        ACLDirection `json:"direction"`
	CustomACLPorts   []any        `json:"customAclPorts"`
	CustomACLDevices []any        `json:"customAclDevices"`
	StateMode        int          `json:"stateMode"`
	Syslog           bool         `json:"syslog"`
}

func aclsPath(siteID string) string {
	return fmt.Sprintf("/sites/%s/setting/firewall/acls", siteID)
}

// ListACLs returns the ACL rules of a given type for a site.
func (c *Client) ListACLs(ctx context.Context, siteID string, aclType int) ([]ACL, error) {
	base := fmt.Sprintf("%s?type=%d", aclsPath(siteID), aclType)
	return listAll[ACL](ctx, c, "acls", base)
}

// GetACL returns a single ACL by id (within a type).
func (c *Client) GetACL(ctx context.Context, siteID string, aclType int, id string) (*ACL, error) {
	acls, err := c.ListACLs(ctx, siteID, aclType)
	if err != nil {
		return nil, err
	}
	for i := range acls {
		if acls[i].ID == id {
			return &acls[i], nil
		}
	}
	return nil, fmt.Errorf("acl %q not found on site %q", id, siteID)
}

// CreateACL creates a rule (null result → resolved by name within its type).
func (c *Client) CreateACL(ctx context.Context, siteID string, in *ACLInput) (*ACL, error) {
	if in.CustomACLPorts == nil {
		in.CustomACLPorts = []any{}
	}
	if in.CustomACLDevices == nil {
		in.CustomACLDevices = []any{}
	}
	if err := c.Do(ctx, "POST", aclsPath(siteID), in, nil); err != nil {
		return nil, fmt.Errorf("creating acl: %w", err)
	}
	acls, err := c.ListACLs(ctx, siteID, in.Type)
	if err != nil {
		return nil, err
	}
	
	// Collect all ACLs matching the requested name to detect duplicates and
	// prevent binding state to the wrong firewall rule.
	var matches []*ACL
	for i := range acls {
		if acls[i].Name == in.Name {
			matches = append(matches, &acls[i])
		}
	}
	
	if len(matches) == 0 {
		return nil, fmt.Errorf("acl %q not found after create", in.Name)
	}
	
	// If multiple ACLs share the same name, attempt to disambiguate by comparing
	// additional attributes (policy, source/destination types, status) that were
	// set in the create request. This mitigates the risk of binding Terraform
	// state to a different ACL when duplicate names exist or a concurrent actor
	// creates a same-named ACL between the POST and list operations.
	if len(matches) > 1 {
		var candidates []*ACL
		for _, acl := range matches {
			if acl.Policy == in.Policy &&
				acl.SourceType == in.SourceType &&
				acl.DestinationType == in.DestinationType &&
				acl.Status == in.Status {
				candidates = append(candidates, acl)
			}
		}
		
		// If disambiguation succeeded, use the unique candidate.
		if len(candidates) == 1 {
			return candidates[0], nil
		}
		
		// If disambiguation failed or multiple candidates remain, fail with a
		// clear error to prevent operating on the wrong firewall rule.
		if len(candidates) == 0 {
			return nil, fmt.Errorf("acl %q: found %d ACLs with the same name after create, but none match the requested attributes (policy=%d, sourceType=%d, destinationType=%d, status=%t); cannot determine which ACL was created — this may indicate a race condition or duplicate ACL names on site %q",
				in.Name, len(matches), in.Policy, in.SourceType, in.DestinationType, in.Status, siteID)
		}
		return nil, fmt.Errorf("acl %q: found %d ACLs with the same name and matching attributes after create; cannot uniquely identify which ACL was created — duplicate ACL names are not supported on site %q",
			in.Name, len(candidates), siteID)
	}
	
	return matches[0], nil
}

// UpdateACL replaces a rule (PUT — PATCH is unsupported here).
func (c *Client) UpdateACL(ctx context.Context, siteID, id string, in *ACLInput) (*ACL, error) {
	in.ID = id
	if in.CustomACLPorts == nil {
		in.CustomACLPorts = []any{}
	}
	if in.CustomACLDevices == nil {
		in.CustomACLDevices = []any{}
	}
	if err := c.Do(ctx, "PUT", aclsPath(siteID)+"/"+id, in, nil); err != nil {
		return nil, fmt.Errorf("updating acl %q: %w", id, err)
	}
	return c.GetACL(ctx, siteID, in.Type, id)
}

// DeleteACL removes a rule.
func (c *Client) DeleteACL(ctx context.Context, siteID, id string) error {
	if err := c.Do(ctx, "DELETE", aclsPath(siteID)+"/"+id, nil, nil); err != nil {
		return fmt.Errorf("deleting acl %q: %w", id, err)
	}
	return nil
}
