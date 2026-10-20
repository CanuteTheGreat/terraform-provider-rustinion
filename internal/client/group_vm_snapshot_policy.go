package client

import (
	"context"
	"fmt"
	"net/http"
)

// GroupVMSnapshotPolicy mirrors `vm_snapshots::VmSnapshotPolicy`: whether
// a group's devices that are VMs on a hypervisor Rustinion also manages
// (Proxmox VE, libvirt/KVM) are snapshotted on that host before a firmware
// upgrade or OS-update install, and how many Rustinion snapshots are kept
// per VM. A per-group singleton that always exists (on, keep 2).
type GroupVMSnapshotPolicy struct {
	Enabled bool  `json:"enabled"`
	Keep    int64 `json:"keep"`
}

// Defaults the server applies to a group that never set these.
var DefaultGroupVMSnapshotPolicy = GroupVMSnapshotPolicy{
	Enabled: true,
	Keep:    2,
}

func groupVMSnapshotPath(groupID string) string {
	return fmt.Sprintf("/api/v1/customer/groups/%s/vm-snapshot-policy", groupID)
}

func (c *Client) GetGroupVMSnapshotPolicy(ctx context.Context, groupID string) (*GroupVMSnapshotPolicy, error) {
	var out GroupVMSnapshotPolicy
	if err := c.do(ctx, http.MethodGet, groupVMSnapshotPath(groupID), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) PutGroupVMSnapshotPolicy(ctx context.Context, groupID string, p GroupVMSnapshotPolicy) (*GroupVMSnapshotPolicy, error) {
	var out GroupVMSnapshotPolicy
	if err := c.do(ctx, http.MethodPut, groupVMSnapshotPath(groupID), p, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
