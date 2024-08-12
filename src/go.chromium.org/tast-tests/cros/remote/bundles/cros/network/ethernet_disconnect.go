// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
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
	"google.golang.org/protobuf/types/known/emptypb"
)

const ethernetDisconnectRebootTimeout = 5 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         EthernetDisconnect,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify DUT behavior for onboard Ethernet connect/disconnect/reboot/suspend and resume",
		Contacts: []string{
			// "cros-connectivity@google.com",
			// "chromeos-connectivity-engprod@google.com",
			"chromeos-connectivity-cienet-external@google.com",
			"cj.tsai@cienet.com",
		},
		BugComponent:   "b:1318544", // ChromeOS > Software > System Services > Connectivity > General
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Attr:           []string{"group:network", "network_e2e_unstable"},
		// The servo_v4p1 is required to use the servo.OnOff() API with servo.DutEthPwrEn.
		// See b/359743894 for more details.
		TestBedDeps:  []string{tbdep.ServoStateWorking, tbdep.ServoComponent("servo_v4p1")},
		VarDeps:      []string{"servo"},
		SoftwareDeps: []string{"reboot"},
		ServiceDeps: []string{
			"tast.cros.wifi.ShillService",
			"tast.cros.network.EthernetService",
		},
		Timeout: 3*time.Minute + 2*ethernetDisconnectRebootTimeout,
	})
}

// EthernetDisconnect verifies DUT behavior for onboard Ethernet connect/disconnect/reboot/suspend and resume.
func EthernetDisconnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	helper := &disconnectEthernetRPCHelper{hint: s.RPCHint(), dut: s.DUT()}
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
	defer func(ctx context.Context) {
		// Ensuring the Ethernet to be enabled back before closing the servo.
		if err := ethernetControl(pxy.Servo(), servo.On)(ctx); err != nil {
			s.Log("Failed to enable Ethernet: ", err)
		}
		pxy.Close(ctx)
	}(cleanupCtx)

	for _, test := range []struct {
		name         string
		setup        action.Action
		numTrials    int // Execute the setup action severals times to perform stress testing.
		verification action.Action
	}{
		{
			name:         "network connectivity test under Ethernet is plugged in",
			setup:        ethernetControl(pxy.Servo(), servo.On),
			numTrials:    1,
			verification: helper.networkAvailable(true /* expectedAvailable */),
		}, {
			name:      "network connectivity test under Ethernet is unplugged",
			setup:     ethernetControl(pxy.Servo(), servo.Off),
			numTrials: 1,
			verification: action.Combine("verify network is not available",
				helper.networkAvailable(false /* expectedAvailable */),
				dutAvailable(pxy.Servo()),
			),
		}, {
			name: "network connectivity test under Ethernet is re-plugged in",
			setup: action.Combine("unplug and plug in",
				ethernetControl(pxy.Servo(), servo.Off),
				ethernetControl(pxy.Servo(), servo.On),
			),
			numTrials:    1,
			verification: helper.networkAvailable(true /* expectedAvailable */),
		}, {
			name: "network connectivity test under Ethernet is plugged and reboot",
			setup: func(ctx context.Context) error {
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
			},
			// Reboot the device several times to verify the Ethernet still can browse the internet.
			numTrials:    2,
			verification: helper.networkAvailable(true /* expectedAvailable */),
		}, {
			name: "network connectivity test under Ethernet is plugged and suspend/resume",
			setup: func(ctx context.Context) error {
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
			},
			// Suspend and resume the device several times to verify the Ethernet still can browse the internet.
			numTrials:    2,
			verification: helper.networkAvailable(true /* expectedAvailable */),
		},
	} {
		s.Run(ctx, test.name, func(ctx context.Context, s *testing.State) {
			// To test DUT behavior when disconnecting Ethernet, the Ethernet should be plugged-in in the beginning.
			if err := ethernetControl(pxy.Servo(), servo.On)(ctx); err != nil {
				s.Fatal("Failed to plug in the Ethernet cable: ", err)
			}

			for i := 0; i < test.numTrials; i++ {
				if err := test.setup(ctx); err != nil {
					s.Fatal("Failed to setup for testing: ", err)
				}
			}

			if err := test.verification(ctx); err != nil {
				s.Fatal("Failed to verify: ", err)
			}
		})
	}
}

type disconnectEthernetRPCHelper struct {
	hint   *testing.RPCHint
	dut    *dut.DUT
	client *rpc.Client
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
	shillSvc := wifi.NewShillServiceClient(r.client.Conn)
	if _, err := shillSvc.SetWifiEnabled(ctx, &wifi.SetWifiEnabledRequest{
		Enabled: enabled,
	}); err != nil {
		return errors.Wrapf(err, "failed to set WiFi enable: %v", enabled)
	}
	return nil
}

func ethernetControl(srv *servo.Servo, enabled servo.OnOffValue) action.Action {
	return func(ctx context.Context) error {
		if err := srv.SetOnOff(ctx, servo.DutEthPwrEn, enabled); err != nil {
			return errors.Wrap(err, "failed to simulate the Ethernet cable to be unplugged/plugged-in through servo")
		}
		return nil
	}
}

// networkAvailable checks if the network is available as expected.
func (r *disconnectEthernetRPCHelper) networkAvailable(expectedAvailable bool) action.Action {
	return func(ctx context.Context) error {
		ethernetSvc := networkSvc.NewEthernetServiceClient(r.client.Conn)
		// The Ethernet might not be connected immediately causing the session creation to fail.
		return testing.Poll(ctx, func(ctx context.Context) error {
			// Verify whether the network connection is available.
			// This RPC times out after 10 seconds, making the entire polling to have a 10-second interval.
			if _, err := ethernetSvc.WaitForEthernet(ctx, &emptypb.Empty{}); err != nil {
				if expectedAvailable {
					return err
				}
				// No error if the network expected to be not available.
				return nil
			} else if !expectedAvailable {
				return errors.New("expect the Ethernet interface is disabled; however it is still available")
			}
			return nil
		}, &testing.PollOptions{Timeout: 1 * time.Minute})
	}
}

// dutAvailable checks the DUT is still available via servo.
func dutAvailable(srv *servo.Servo) action.Action {
	return func(ctx context.Context) error {
		if controllerType, err := srv.GetString(ctx, servo.ActiveDUTController); err != nil {
			return errors.Wrap(err, "failed to get active DUT controller")
		} else if controllerType == "neither" {
			// The active servo device should be "servo_micro" or "ccd_cr50".
			// "neither" means there is no active servo device.
			return errors.New("the DUT is hanging")
		}
		return nil
	}
}
