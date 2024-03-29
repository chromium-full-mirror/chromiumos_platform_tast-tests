// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifiutil

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// P2POnOffRobustnessTestcase defines P2P robustness testcase.
type P2POnOffRobustnessTestcase struct {
	PrintableName string
	Opts          []p2p.GroupOption
	Rounds        int
}

// P2POnOffRound sets up p2p, makes sure teardown is run then runs actions from P2PConnect round.
func P2POnOffRound(ctx context.Context, tf *wificell.TestFixture, ops ...p2p.GroupOption) (retErr error) {
	// Configure GO according to the testcase specs.
	err := tf.P2PConfigureGO(ctx, wificell.P2PDeviceDUT, ops...)
	if err != nil {
		return errors.Wrap(err, "failed to start P2P session on DUT")
	}

	defer func(ctx context.Context) {
		err = tf.P2PDeconfigureGO(ctx)
		retErr = errors.Join(retErr, err) // We can't overwrite ret value.
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testing.ContextLog(ctx, "P2P Group started")

	// Rest of the round is functionally identical to P2PConnectRound.
	err = P2PConnectRound(ctx, tf)
	if err != nil {
		return errors.Wrap(err, "failed to associate to DUT")
	}
	return nil
}

// P2PConnectRound connects peer DUT to the P2P GO on the main DUT, then confirms connection by running a short ping burst.
func P2PConnectRound(ctx context.Context, tf *wificell.TestFixture) (retErr error) {
	err := tf.P2PConnect(ctx, wificell.P2PDeviceCompanionDUT)
	if err != nil {
		return errors.Wrap(err, "failed to connect to P2P Group")
	}
	// Defer disconnect just in case something breaks.
	defer func(ctx context.Context) {
		err = tf.P2PDisconnect(ctx)
		retErr = errors.Join(retErr, err) // We can't overwrite ret value.
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := tf.P2PAssertPingFromGO(ctx); err != nil {
		return errors.Wrap(err, "failed to ping the p2p client from the p2p group owner (GO)")
	}

	return nil
}

// P2POnOffRobustnessTest runs P2P On/Off Robustness Test.
func P2POnOffRobustnessTest(ctx context.Context, s *testing.State, tf *wificell.TestFixture, tc P2POnOffRobustnessTestcase,
	rounds int, thresholds ResourceThreshold, processes string, pv *perf.Values) error {
	resInfoGO, err := GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
	if err != nil {
		return errors.Wrap(err, "failed to get resource info")
	}
	resInfoClient, err := GetResourceInfo(ctx, tf.DUT(wificell.PeerDUT1).Conn(), processes)
	if err != nil {
		return errors.Wrap(err, "failed to get resource info")
	}
	resInfosGO := []ResourceInfo{resInfoGO}
	resInfosClient := []ResourceInfo{resInfoClient}

	// Make sure output is recorded even in case of error, this might be the reason of the issue.
	defer func(ctx context.Context) {
		if err := ReportResources(ctx, resInfosGO, s.OutDir(), fmt.Sprintf("%s.tsv", "p2p_on_off_go_"+tc.PrintableName)); err != nil {
			s.Error("Failed to write P2P GO resources report: ", err)
		}
		if err := ReportResources(ctx, resInfosClient, s.OutDir(), fmt.Sprintf("%s.tsv", "p2p_on_off_client_"+tc.PrintableName)); err != nil {
			s.Error("Failed to write P2P Client resources report: ", err)
		}
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// We're running in a simple loop instead of s.Run() on purpose, we want to bail out on the first error.
	for i := 0; i < rounds; i++ {
		testing.ContextLogf(ctx, "P2P round #%v", i+1)

		err := P2POnOffRound(ctx, tf, tc.Opts...)
		if err != nil {
			return errors.Wrapf(err, "failure during round %v", i)
		}
		// Convert to a standard understandable by perf.
		resInfoGO, err := GetResourceInfo(ctx, tf.DUT(wificell.DefaultDUT).Conn(), processes)
		if err != nil {
			return errors.Wrap(err, "failed to get resource info")
		}
		resInfoClient, err := GetResourceInfo(ctx, tf.DUT(wificell.PeerDUT1).Conn(), processes)
		if err != nil {
			return errors.Wrap(err, "failed to get resource info")
		}
		// Check if pid of processes changed.
		if err := ValidatePids(resInfosGO[0], resInfoGO); err != nil {
			return errors.Wrap(err, "error while validating GO PIDs")
		}
		if err := ValidatePids(resInfosClient[0], resInfoClient); err != nil {
			return errors.Wrap(err, "error while validating P2P Client PIDs")
		}
		resInfosGO = append(resInfosGO, resInfoGO)
	}
	testing.ContextLog(ctx, "GO Start: ", resInfosGO[0].String())
	testing.ContextLog(ctx, "GO End:   ", resInfosGO[len(resInfosGO)-1].String())
	testing.ContextLog(ctx, "Client Start: ", resInfosClient[0].String())
	testing.ContextLog(ctx, "CLient End:   ", resInfosClient[len(resInfosClient)-1].String())
	if err := ValidateResourceInfo(ctx, resInfosGO[0], resInfosGO[len(resInfosGO)-1], thresholds); err != nil {
		return errors.Wrap(err, "GO resource validation failed")
	}
	if err := ValidateResourceInfo(ctx, resInfosClient[0], resInfosClient[len(resInfosClient)-1], thresholds); err != nil {
		return errors.Wrap(err, "Client resource validation failed")
	}

	return nil
}
