// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/network/ip"
	"go.chromium.org/tast-tests/cros/common/network/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell/framesender"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
)

// NewFrameSender creates a new framesender.Sender object.
func NewFrameSender(ctx context.Context, ipr *ip.Runner, im *IfaceManager, nd *iw.NetDev, host *ssh.Conn, workDir, iface string) (ret *framesender.Sender, retErr error) {
	im.SetBusy(nd.IfName)
	defer func() {
		if retErr != nil {
			im.SetAvailable(nd.IfName)
		}
	}()

	if err := cloneMAC(ctx, ipr, nd.IfName, iface); err != nil {
		return nil, errors.Wrap(err, "failed to clone MAC")
	}
	if err := ipr.SetLinkUp(ctx, nd.IfName); err != nil {
		return nil, err
	}
	return framesender.New(host, nd.IfName, workDir), nil
}

// CloseFrameSender closes frame sender and releases related resources.
func CloseFrameSender(ctx context.Context, ipr *ip.Runner, im *IfaceManager, s *framesender.Sender) error {
	err := ipr.SetLinkDown(ctx, s.Interface())
	im.SetAvailable(s.Interface())
	return err
}

// cloneMAC clones the MAC address of src to dst.
func cloneMAC(ctx context.Context, ipr *ip.Runner, dst, src string) error {
	mac, err := ipr.MAC(ctx, src)
	if err != nil {
		return err
	}
	return ipr.SetMAC(ctx, dst, mac)
}
