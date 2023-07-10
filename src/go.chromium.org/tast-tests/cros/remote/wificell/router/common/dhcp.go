// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/ip"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/wificell/dhcp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// StartDHCP starts the DHCP server and configures the server IP. If DNS functionality is
// not required, set dnsOpt to nil.
func StartDHCP(ctx context.Context, name, iface, workDir string, host *ssh.Conn, ipr *ip.Runner, ipStart, ipEnd, serverIP, broadcastIP net.IP, mask net.IPMask, dnsOpt *dhcp.DNSOption) (_ *dhcp.Server, retErr error) {
	ctx, st := timing.Start(ctx, "router.StartDHCP")
	defer st.End()

	if err := ipr.FlushIP(ctx, iface); err != nil {
		return nil, err
	}
	maskLen, _ := mask.Size()
	if err := ipr.AddIP(ctx, iface, serverIP, maskLen, ip.AddIPBroadcast(broadcastIP)); err != nil {
		return nil, err
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			if err := ipr.FlushIP(ctx, iface); err != nil {
				testing.ContextLogf(ctx, "Failed to flush the interface %s while StartDHCP has failed: %v", iface, err)
			}
		}
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()
	ds, err := dhcp.StartServer(ctx, host, name, iface, workDir, ipStart, ipEnd, dnsOpt)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start DHCP server")
	}
	return ds, nil
}

// StopDHCP stops the DHCP server and flushes the interface.
func StopDHCP(ctx context.Context, ds *dhcp.Server, ipr *ip.Runner) error {
	var firstErr error
	iface := ds.Interface()
	if err := ds.Close(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop dhcpd"))
	}
	utils.CollectFirstErr(ctx, &firstErr, ipr.FlushIP(ctx, iface))
	return firstErr
}
