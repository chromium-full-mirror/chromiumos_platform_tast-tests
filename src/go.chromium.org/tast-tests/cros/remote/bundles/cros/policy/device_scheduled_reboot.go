// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	kspb "go.chromium.org/tast-tests/cros/services/cros/kiosk"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeviceScheduledReboot,
		Desc:         "Behavior of DeviceScheduledReboot policy for kiosk",
		BugComponent: "b:1263917", // ChromeOS > Software > Commercial (Enterprise) > Testing
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com", // Test owner
		},
		Attr: []string{
			// Disabled by TORA. See: b/318082620
			// "group:enrollment",
		},
		SoftwareDeps: []string{"chrome", "reboot"},
		ServiceDeps: []string{
			"tast.cros.kiosk.KioskService", "tast.cros.hwsec.OwnershipService",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceScheduledReboot{}, pci.VerifiedFunctionalityJS),
		},
		Timeout: 15 * time.Minute, // Test uses local time to schedule a reboot, waits for the reboot to happen and then waits for the device to reconnect.
	})
}

// currentTimeDUT uses `date` utility on a DUT to get current time in DUT's local timezone.
func currentTimeDUT(ctx context.Context, dut *dut.DUT) (hour, minute int, err error) {
	hourBytes, err := dut.Conn().CommandContext(ctx, "date", "+%-H").Output()
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to get DUT's current time")
	}
	minuteBytes, err := dut.Conn().CommandContext(ctx, "date", "+%-M").Output()
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to get DUT's current time")
	}

	hour, err = strconv.Atoi(strings.TrimSpace(string(hourBytes)))
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to convert current time to int")
	}
	minute, err = strconv.Atoi(strings.TrimSpace(string(minuteBytes)))
	if err != nil {
		return 0, 0, errors.Wrap(err, "failed to convert current time to int")
	}

	return hour, minute, nil
}

// futureTime adds 2 minutes to the given time.
func futureTime(hour, minute int) (int, int) {
	if minute > 58 {
		hour = (hour + 1) % 24
	}
	minute = (minute + 2) % 60

	return hour, minute
}

func DeviceScheduledReboot(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM: ", err)
		}
	}(cleanupCtx)
	if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	kioskClient := kspb.NewKioskServiceClient(cl.Conn)

	if _, err := kioskClient.StartKiosk(ctx, &empty.Empty{}); err != nil {
		s.Fatal(err, " failed to start kiosk")
	}
	defer func(ctx context.Context) {
		cl, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(ctx)
		kioskClient = kspb.NewKioskServiceClient(cl.Conn)
		if _, err := kioskClient.CloseKiosk(ctx, &empty.Empty{}); err != nil {
			s.Fatal(err, " failed to close kiosk")
		}
	}(cleanupCtx)

	// Fetch current time and fast-forward 2 minutes.
	hour, minute, err := currentTimeDUT(ctx, s.DUT())
	if err != nil {
		s.Fatal(err, " failed to get current time")
	}
	hour, minute = futureTime(hour, minute)

	deviceScheduledRebootPb := policy.NewBlob()
	deviceScheduledRebootPb.AddPolicy(&policy.DeviceScheduledReboot{Val: &policy.DeviceScheduledRebootValue{
		DayOfMonth: 11,
		DayOfWeek:  "TUESDAY",
		Frequency:  "DAILY",
		RebootTime: &policy.DeviceScheduledRebootValueRebootTime{
			Hour:   hour,
			Minute: minute,
		},
	}})

	deviceScheduledRebootJSON, err := json.Marshal(deviceScheduledRebootPb)
	if err != nil {
		s.Fatal("Failed to serialize policies: ", err)
	}
	if _, err = kioskClient.UpdatePolicies(ctx, &kspb.UpdatePoliciesRequest{
		PolicyJson: deviceScheduledRebootJSON,
	}); err != nil {
		s.Fatal(err, " failed to start kiosk with policies")
	}

	func() {
		sdCtx, cancel := context.WithTimeout(cleanupCtx, 7*time.Minute)
		defer cancel()
		s.Log("Wait for DUT to become unreachable")
		if err := s.DUT().WaitUnreachable(sdCtx); err != nil {
			s.Fatal("Failed to wait for unreachable: ", err)
		}
	}()

	func() {
		waitConnectCtx, cancel := context.WithTimeout(cleanupCtx, 3*time.Minute)
		defer cancel()
		s.Log("Wait for DUT to power ON")
		if err := s.DUT().WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	}()
}
