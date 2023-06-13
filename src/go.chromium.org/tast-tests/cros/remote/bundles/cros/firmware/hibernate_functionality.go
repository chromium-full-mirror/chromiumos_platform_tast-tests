// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/xmlrpc"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/security"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: HibernateFunctionality,
		Desc: "Verifies that system Hibernate functionality through onboard keyboard",
		Contacts: []string{
			"chromeos-faft@google.com",
			"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome", "reboot"},
		Vars:         []string{"servo"},
		// TODO(b/199674322): Add back to firmware_unstable when this test passes.
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:intel-nda"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func HibernateFunctionality(ctx context.Context, s *testing.State) {
	const (
		kbPressALT = "kbpress 10 0 1"
		kbPressH   = "kbpress 6 1 1"
		kbPressVOL = "kbpress 4 0 1"
	)
	dut := s.DUT()
	h := s.FixtValue().(*fixture.Value).Helper
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	getChargerPollOptions := testing.PollOptions{
		Timeout:  1 * time.Minute,
		Interval: 1 * time.Second,
	}
	s.Log("Stopping power supply")
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.SetDUTPower(ctx, false); err != nil {
		s.Fatal("Failed to remove charger: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if attached, err := h.Servo.GetChargerAttached(ctx); err != nil {
			return err
		} else if attached {
			return errors.New("charger is still attached - use Servo V4 Type-C or supply RPM vars")
		}
		return nil
	}, &getChargerPollOptions); err != nil {
		s.Fatal("Check for charger failed: ", err)
	}
	defer func(ctx context.Context) {
		if err := h.RequireServo(ctx); err != nil {
			s.Fatal("Failed to connect to servo: ", err)
		}
		if err := h.SetDUTPower(ctx, true); err != nil {
			s.Fatal("Failed to attach charger: ", err)
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if attached, err := h.Servo.GetChargerAttached(ctx); err != nil {
				return err
			} else if !attached {
				return errors.New("charger is not attached")
			}
			return nil
		}, &getChargerPollOptions); err != nil {
			s.Fatal("Check for charger failed: ", err)
		}
		if !dut.Connected(ctx) {
			s.Log("Power Normal Pressing")
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
				s.Fatal("Failed to power button press: ", err)
			}
			if err := dut.WaitConnect(waitCtx); err != nil {
				s.Log("Failed to wake up DUT. Retrying")
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
					s.Fatal("Failed to power button press: ", err)
				}
				if err := dut.WaitConnect(waitCtx); err != nil {
					s.Fatal("Failed to wait connect DUT: ", err)
				}
			}
		}
	}(ctx)
	// GoBigSleepLint: Wait for a short delay between cutting power supply.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	cl, err := rpc.Dial(ctx, dut, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	//defer cl.Close(ctx)
	client := security.NewBootLockboxServiceClient(cl.Conn)
	if _, err := client.NewChromeLogin(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove CCD watchdog: ", err)
	}
	cl.Close(ctx)
	s.Log("Pressing ALT+H+VolUp to send the DUT to Hibernate")
	if err := h.Servo.RunECCommand(ctx, kbPressALT); err != nil {
		s.Fatal("Failed to press ALT key: ", err)
	}
	if err := h.Servo.RunECCommand(ctx, kbPressH); err != nil {
		s.Fatal("Failed to press H key: ", err)
	}
	if err := h.Servo.RunECCommand(ctx, kbPressVOL); err != nil {
		s.Fatal("Failed to press volume up key: ", err)
	}
	// GoBigSleepLint: Wait for a short delay after putting DUT in hibernation.
	if err := testing.Sleep(ctx, 4*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	s.Log("Checking DUT for Hibernate")
	if err := dut.WaitUnreachable(ctx); err != nil {
		s.Fatal("Failed to wait DUT to become unreachable: ", err)
	}

	// Verify that EC is non-responsive by querying an EC command.
	s.Log("Verify EC is non-responsive")
	waitECCtx, cancelEC := context.WithTimeout(ctx, 10*time.Second)
	defer cancelEC()

	// Expect no return for the query, and receive error of type FaultError.
	_, errEC := h.Servo.RunECCommandGetOutput(waitECCtx, "version", []string{`.`})
	if errEC == nil {
		s.Fatal(" Failed EC was still responsive after putting DUT in hibernation: ", err)
	}
	var errSend xmlrpc.FaultError
	if !errors.As(errEC, &errSend) {
		s.Fatal("Failed EC was still responsive after putting DUT in hibernation: ", errEC)
	}
	s.Log("EC was non-responsive")
	// Wait for DUT to reboot and reconnect.
	if err = h.SetDUTPower(ctx, true); err != nil {
		s.Fatal("Failed to connect charger: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if attached, err := h.Servo.GetChargerAttached(ctx); err != nil {
			return err
		} else if !attached {
			return errors.New("charger is still not attached - use Servo V4 Type-C or supply RPM vars")
		}
		return nil
	}, &getChargerPollOptions); err != nil {
		s.Fatal("Check for charger failed: ", err)
	}
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
		s.Fatal("Failed to power button press: ", err)
	}
	if err := dut.WaitConnect(waitCtx); err != nil {
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			s.Fatal("Failed to power button press: ", err)
		}
		if err := dut.WaitConnect(waitCtx); err != nil {
			s.Fatal("Failed to wait connect DUT: ", err)
		}
	}
}
