// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package networkui

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	dututil "go.chromium.org/tast-tests/cros/remote/dut"
	networkSvc "go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const ethernetDisconnectRebootTimeout = 5 * time.Minute

type ethernetDisconnectTestAction func(context.Context, *servo.Servo, *disconnectEthernetRPCHelper) error
type ethernetDisconnectTestParam struct {
	setups        []ethernetDisconnectTestAction
	numTrials     int // Execute the setup action severals times to perform stress testing.
	verifications []ethernetDisconnectTestAction
}

func init() {
	testing.AddTest(&testing.Test{
		Func: EthernetDisconnect,
		Desc: "Verify DUT behavior for onboard Ethernet connect/disconnect/reboot/suspend and resume",
		Contacts: []string{
			// "cros-device-enablement@google.com",
			// "chromeos-connectivity-engprod@google.com",
			"chromeos-connectivity-cienet-external@google.com",
			"cj.tsai@cienet.com",
		},
		BugComponent:   "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		LifeCycleStage: testing.LifeCycleInDevelopment,
		// TODO(b/542029943): Re-enable once the test is stable.
		// Attr:           []string{"group:network", "network_e2e_unstable"},
		// The servo_v4p1 is required to use the servo.OnOff() API with servo.DutEthPwrEn.
		// See b/359743894 for more details.
		TestBedDeps:  append([]string{tbdep.ServoComponent("servo_v4p1")}, tbdep.ServoPresentAndWorking...),
		VarDeps:      []string{"servo"},
		SoftwareDeps: []string{"reboot"},
		ServiceDeps: []string{
			"tast.cros.wifi.ShillService",
			"tast.cros.network.EthernetService",
		},
		Params: []testing.Param{
			{
				Name: "plugged_in",
				Val: ethernetDisconnectTestParam{
					numTrials:     1,
					verifications: []ethernetDisconnectTestAction{networkAvailable(true /* expectedAvailable */)},
				},
				Timeout: 3 * time.Minute,
			}, {
				Name: "unplugged",
				Val: ethernetDisconnectTestParam{
					setups:    []ethernetDisconnectTestAction{ethernetControl(servo.Off)},
					numTrials: 1,
					verifications: []ethernetDisconnectTestAction{
						networkAvailable(false /* expectedAvailable */),
						dutAvailable,
					},
				},
				Timeout: 3 * time.Minute,
			}, {
				Name: "replugged_in",
				Val: ethernetDisconnectTestParam{
					setups: []ethernetDisconnectTestAction{
						ethernetControl(servo.Off),
						ethernetControl(servo.On),
					},
					numTrials:     1,
					verifications: []ethernetDisconnectTestAction{networkAvailable(true /* expectedAvailable */)},
				},
				Timeout: 3 * time.Minute,
			}, {
				Name: "reboot",
				Val: ethernetDisconnectTestParam{
					setups:        []ethernetDisconnectTestAction{ethernetDisconnectReboot},
					numTrials:     2,
					verifications: []ethernetDisconnectTestAction{networkAvailable(true /* expectedAvailable */)},
				},
				Timeout: 3*time.Minute + 2*ethernetDisconnectRebootTimeout,
			}, {
				Name: "suspend",
				Val: ethernetDisconnectTestParam{
					setups:        []ethernetDisconnectTestAction{ethernetDisconnectSuspend},
					numTrials:     2,
					verifications: []ethernetDisconnectTestAction{networkAvailable(true /* expectedAvailable */)},
				},
				Timeout: 3*time.Minute + 2*ethernetDisconnectRebootTimeout,
			},
		},
	})
}

// EthernetDisconnect verifies DUT behavior for onboard Ethernet connect/disconnect/reboot/suspend and resume.
func EthernetDisconnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	wifiChips := s.Features("").GetHardware().GetHardwareFeatures().GetWifi().GetWifiChips()
	helper := &disconnectEthernetRPCHelper{hint: s.RPCHint(), dut: s.DUT(), hasWifi: len(wifiChips) > 0}
	if err := helper.establish(ctx); err != nil {
		s.Fatal("Failed to establish RPC helper: ", err)
	}
	defer helper.cleanup(cleanupCtx)

	if err := helper.setWifiEnabled(ctx, false); err != nil {
		s.Fatal("Failed to disable WiFi: ", err)
	}
	// Ensuring the Wi-Fi technology to be enabled back after the tests are completed.
	defer func(ctx context.Context) {
		if err := helper.setWifiEnabled(ctx, true); err != nil {
			s.Log("Failed to enable Wi-Fi: ", err)
		}
	}(cleanupCtx)

	pxy, err := servo.NewProxy(ctx, s.RequiredVar("servo"), helper.dut.KeyFile(), helper.dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	// Ensure the Ethernet is enabled before the test is started.
	if err := ethernetControl(servo.On)(ctx, pxy.Servo(), helper); err != nil {
		s.Fatal("Failed to enable Ethernet: ", err)
	}

	defer func(ctx context.Context) {
		// Ensuring the Ethernet to be enabled back before closing the servo.
		if err := ethernetControl(servo.On)(ctx, pxy.Servo(), helper); err != nil {
			s.Log("Failed to enable Ethernet: ", err)
		}
		pxy.Close(ctx)
	}(cleanupCtx)

	param := s.Param().(ethernetDisconnectTestParam)
	for i := 0; i < param.numTrials; i++ {
		for _, setup := range param.setups {
			if err := setup(ctx, pxy.Servo(), helper); err != nil {
				s.Fatal("Failed to setup for testing: ", err)
			}
		}
	}

	for _, verification := range param.verifications {
		if err := verification(ctx, pxy.Servo(), helper); err != nil {
			s.Fatal("Failed to verify: ", err)
		}
	}
}

type disconnectEthernetRPCHelper struct {
	hint    *testing.RPCHint
	dut     *dut.DUT
	client  *rpc.Client
	hasWifi bool
}

// cleanup cleans up the RPC client.
func (r *disconnectEthernetRPCHelper) cleanup(ctx context.Context) error {
	if r.client != nil {
		if err := r.client.Close(ctx); err != nil {
			return errors.Wrap(err, "failed to close the RPC client")
		}
		r.client = nil
	}
	return nil
}

// establish establishes the RPC client.
func (r *disconnectEthernetRPCHelper) establish(ctx context.Context) error {
	rpcClient, err := rpc.Dial(ctx, r.dut, r.hint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	r.client = rpcClient
	return nil
}

// setWifiEnabled sets Wi-Fi technology enabled/disabled.
func (r *disconnectEthernetRPCHelper) setWifiEnabled(ctx context.Context, enabled bool) error {
	if !r.hasWifi {
		// Skip this step if the DUT doesn't have WiFi chips.
		return nil
	}

	shillSvc := wifi.NewShillServiceClient(r.client.Conn)
	if _, err := shillSvc.SetWifiEnabled(ctx, &wifi.SetWifiEnabledRequest{
		Enabled: enabled,
	}); err != nil {
		return errors.Wrapf(err, "failed to set WiFi enable: %v", enabled)
	}
	return nil
}

func ethernetControl(enabled servo.OnOffValue) ethernetDisconnectTestAction {
	return func(ctx context.Context, srv *servo.Servo, _ *disconnectEthernetRPCHelper) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			return srv.SetStringAndCheck(ctx, servo.StringControl(servo.DutEthPwrEn), string(enabled))
		}, &testing.PollOptions{Timeout: time.Minute, Interval: 5 * time.Second})
	}
}

// networkAvailable checks if the network is available as expected.
func networkAvailable(expectedAvailable bool) ethernetDisconnectTestAction {
	return func(ctx context.Context, _ *servo.Servo, helper *disconnectEthernetRPCHelper) error {
		ethernetSvc := networkSvc.NewEthernetServiceClient(helper.client.Conn)
		// The Ethernet might not be connected immediately causing the session creation to fail.
		return testing.Poll(ctx, func(ctx context.Context) error {
			// RPC calls could get stuck when no internet is available. (see b/370629636)
			rpcCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
			defer cancel()
			// Verify whether the network connection is available.
			// This RPC times out after 10 seconds, making the entire polling to have a
			// 10-second interval.
			if _, err := ethernetSvc.WaitForEthernet(rpcCtx, &emptypb.Empty{}); err != nil {
				if expectedAvailable {
					return errors.Wrap(err, "failed to wait for Ethernet")
				}
				// No error if the network expected to be not available.
				return nil
			} else if !expectedAvailable {
				return errors.New("expect the Ethernet interface is disabled; however it is still available")
			}
			return nil
		}, &testing.PollOptions{Timeout: 2 * time.Minute})
	}
}

// dutAvailable checks the DUT is still available via servo.
func dutAvailable(ctx context.Context, srv *servo.Servo, _ *disconnectEthernetRPCHelper) error {
	// The communication with servo usually completes within 10 seconds, but sometimes
	// it may take longer.
	return testing.Poll(ctx, func(ctx context.Context) error {
		// This function times out after 10 seconds, making the entire polling to have a
		// 10-second interval.
		if controllerType, err := srv.GetString(ctx, servo.ActiveDUTController); err != nil {
			return errors.Wrap(err, "failed to get active DUT controller")
		} else if controllerType == "neither" {
			// The active servo device should be "servo_micro" or "ccd_cr50".
			// "neither" means there is no active servo device.
			return errors.New("the DUT is hanging")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute})
}

func ethernetDisconnectReboot(ctx context.Context, _ *servo.Servo, helper *disconnectEthernetRPCHelper) error {
	if err := helper.cleanup(ctx); err != nil {
		// Simply logs the error since this error does not affects the following procedures
		// and the connection will be reestablished afterward.
		testing.ContextLog(ctx, "Failed to cleanup RPC: ", err)
	}
	if err := helper.dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot")
	}
	// After rebooting, the RPC resource will be invalidated and will need to be reestablished.
	return helper.establish(ctx)
}

func ethernetDisconnectSuspend(ctx context.Context, _ *servo.Servo, helper *disconnectEthernetRPCHelper) error {
	if err := helper.cleanup(ctx); err != nil {
		// Simply logs the error since this error does not affects the following procedures
		// and the connection will be reestablished afterward.
		testing.ContextLog(ctx, "Failed to cleanup RPC: ", err)
	}
	if err := dututil.SuspendDUT(ctx, helper.dut, 10 /* seconds */); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}
	// After suspending, the RPC resource will be invalidated and will need to be reestablished.
	return helper.establish(ctx)
}
