// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/network/ip"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/common/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/remote/wificell/pcap"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/timing"
)

// StartCapture starts a packet capturer.
// After getting a Capturer instance, c, the caller should call r.StopCapture(ctx, c) at the end,
// and use the shortened ctx (provided by r.ReserveForStopCapture(ctx, c)) before r.StopCapture()
// to reserve time for it to run.
func StartCapture(ctx context.Context, nd *iw.NetDev, host *ssh.Conn, ipr *ip.Runner, im *IfaceManager, iwr *iw.Runner, workDir, name string, ch int, is6GHz bool, freqOps []iw.SetFreqOption, pcapOps ...pcap.Option) (ret *pcap.Capturer, retErr error) {
	ctx, st := timing.Start(ctx, "router.StartCapture")
	defer st.End()

	freq, err := hostapd.ChannelToFrequencyWithBand(ch, is6GHz)
	if err != nil {
		return nil, err
	}

	iface := nd.IfName
	shared := im.IsPhyBusyAny(nd.PhyNum)

	im.SetBusy(iface)
	defer func() {
		if retErr != nil {
			im.SetAvailable(iface)
		}
	}()

	if err := ipr.SetLinkUp(ctx, iface); err != nil {
		return nil, err
	}
	defer func() {
		if retErr != nil {
			if err := ipr.SetLinkDown(ctx, iface); err != nil {
				testing.ContextLogf(ctx, "Failed to set %s down, err=%s", iface, err.Error())
			}
		}
	}()

	if !shared {
		// The interface is not shared, set up frequency and bandwidth.
		if err := iwr.SetFreq(ctx, iface, freq, freqOps...); err != nil {
			return nil, errors.Wrapf(err, "failed to set frequency for interface %s", iface)
		}
	} else {
		testing.ContextLogf(ctx, "Skip configuring of the shared interface %s", iface)
	}

	c, err := pcap.StartCapturer(ctx, host, name, iface, workDir, pcapOps...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to start a packet capturer")
	}
	return c, nil
}

// StopCapture stops the packet capturer and releases related resources.
func StopCapture(ctx context.Context, ipr *ip.Runner, im *IfaceManager, capturer *pcap.Capturer) error {
	if capturer == nil {
		return nil
	}
	ctx, st := timing.Start(ctx, "router.StopCapture")
	defer st.End()

	var firstErr error
	iface := capturer.Interface()
	if err := capturer.Close(ctx); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, errors.Wrap(err, "failed to stop capturer"))
	}
	if err := ipr.SetLinkDown(ctx, iface); err != nil {
		utils.CollectFirstErr(ctx, &firstErr, err)
	}
	im.SetAvailable(iface)
	return firstErr
}
