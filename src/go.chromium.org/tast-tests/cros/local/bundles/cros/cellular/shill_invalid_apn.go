// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"io"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast/core/testing"
)

const numTries = 5
const invalidApnCooldown = 15 * time.Minute
const invalidApnAttempts = 2

var apnsToConnect = []map[string]string{{
	shillconst.DevicePropertyCellularAPNInfoApnName:   "callbox-invalid-apn",
	shillconst.DevicePropertyCellularAPNInfoApnSource: "ui",
	shillconst.DevicePropertyCellularAPNInfoApnIPType: shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv6,
	shillconst.DevicePropertyCellularAPNInfoApnTypes:  shillconst.DevicePropertyCellularAPNInfoApnTypeDefault,
}, {
	shillconst.DevicePropertyCellularAPNInfoApnName:   "callbox-ipv4",
	shillconst.DevicePropertyCellularAPNInfoApnSource: "ui",
	shillconst.DevicePropertyCellularAPNInfoApnIPType: shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv6,
	shillconst.DevicePropertyCellularAPNInfoApnTypes:  shillconst.DevicePropertyCellularAPNInfoApnTypeDefault,
}, {
	shillconst.DevicePropertyCellularAPNInfoApnName:   "callbox-ipv6",
	shillconst.DevicePropertyCellularAPNInfoApnSource: "ui",
	shillconst.DevicePropertyCellularAPNInfoApnIPType: shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4,
	shillconst.DevicePropertyCellularAPNInfoApnTypes:  shillconst.DevicePropertyCellularAPNInfoApnTypeDefault,
}, {
	shillconst.DevicePropertyCellularAPNInfoApnName:     "callbox-ipv4-chap",
	shillconst.DevicePropertyCellularAPNInfoApnUsername: "invalid-username",
	shillconst.DevicePropertyCellularAPNInfoApnSource:   "ui",
	shillconst.DevicePropertyCellularAPNInfoApnIPType:   shillconst.DevicePropertyCellularAPNInfoApnIPTypeIPv4,
	shillconst.DevicePropertyCellularAPNInfoApnTypes:    shillconst.DevicePropertyCellularAPNInfoApnTypeDefault,
}}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillInvalidApn,
		Desc:         "Verifies handling of the invalid APNs",
		Contacts:     []string{"chromeos-cellular-team@google.com", "michamazur@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_amari_callbox", "cellular_unstable"},
		Fixture:      "cellular",
		Timeout:      20 * time.Minute,
	})
}

func ShillInvalidApn(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Fail immediately if there is a known bug that will cause the test to run until it times out.
	err := cellular.TagKnownBugOnModem(ctx, nil, "b/263815534", cellular.ModemFwFilterL850MR7AndLower)
	err = cellular.TagKnownBugOnModem(ctx, err, "b/290110554", cellular.ModemFwFilterFM350MR3AndLower)
	err = cellular.TagKnownBugOnModem(ctx, err, "b/289519883", cellular.ModemFwFilterFM101MR1)
	if err != nil {
		s.Fatalf("Fail early to avoid wasting DUT time: %s", err)
	}

	// Prepare shill configuration.
	if err := helper.ResetShill(ctx); err != nil {
		s.Fatal("Failed to reset Shill: ", err)
	}

	autoconnectChanged, err := helper.SetServiceAutoConnect(ctx, false)
	if err != nil {
		s.Fatal("Failed to disable autoconnect: ", err)
	}
	if autoconnectChanged {
		defer helper.SetServiceAutoConnect(ctx, true)
	}

	if err = helper.SetCustomAPNList(ctx, apnsToConnect); err != nil {
		s.Fatal("Unable to set the custom APN: ", err)
	}
	defer helper.ClearCustomAPNList(ctx)

	// Prepare modem.
	if err := helper.WaitForEnabledState(ctx, true); err != nil {
		s.Fatal("Cellular service did not reach Enabled state: ", err)
	}

	if err := helper.WaitForModemRegisteredAfterReset(ctx, 60*time.Second); err != nil {
		s.Fatal("Modem not registered: ", err)
	}

	// Try invalid APN few times to observe throttling in Shill.
	for i := 1; i <= numTries; i++ {
		s.Log("Trigger connection attempt and check errors: try ", i)
		// Invalid APN error should be reported on first attempt but
		// must be throttled after few tries.
		// invalidApnAttempts should equal Cellular::kInvalidApnAttempts
		if !checkConnectionErrors(ctx, s, helper) {
			if i == 1 {
				s.Fatal("No Invalid APN error was reported")
			}
		} else if i > invalidApnAttempts {
			s.Fatal("Invalid APN was not flagged by Shill")
		}
	}

	s.Log("Delay test execution for kInvalidApnCooldown period")
	// GoBigSleepLint: sleep to check expiration of invalid apn flag
	// Delay should be equal to Cellular::kInvalidApnCooldown
	testing.Sleep(ctx, invalidApnCooldown)

	// Check errors again after the invalid list expires
	if !checkConnectionErrors(ctx, s, helper) {
		s.Fatal("Invalid APN list did not expire after cooldown time")
	}
}

func checkConnectionErrors(ctx context.Context, s *testing.State, helper *cellular.Helper) bool {
	// Create reader for /var/log/net.log.
	reader, err := syslog.NewLineReader(ctx, syslog.NetLogFile, false, nil)
	if err != nil {
		s.Fatal("Failed to initialize syslog reader: ", err)
	}
	defer reader.Close()

	if _, err := helper.ConnectWithTimeout(ctx, 30*time.Second); err == nil {
		s.Fatal("Error: Connection succeded with invalid APN")
	} else if strings.Contains(err.Error(), "invalid-apn") {
		s.Log("Connection failed as expected with invalid-apn error")
	} else if strings.Contains(err.Error(), "No valid APNs") {
		s.Log("Connection failed as expected due to empty try list")
	} else {
		s.Fatal("Connection failed not due to invalid APN: ", err)
	}

	// Search logs for connection errors.
	for {
		line, err := reader.ReadLine()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.Fatal("Failed to read logs: ", err)
		}

		if strings.Contains(line, "MultipleAccessToPdnConnectionNotAllowed") ||
			strings.Contains(line, "Core.Throttled") ||
			strings.Contains(line, "Status.OperationNotAllowed") ||
			strings.Contains(line, "MobileEquipment.PhoneFailure") ||
			strings.Contains(line, "MobileEquipment.Unknown") ||
			strings.Contains(line, "Ipv4OnlyAllowed") ||
			strings.Contains(line, "Ipv6OnlyAllowed") ||
			strings.Contains(line, "Ipv4v6OnlyAllowed") ||
			strings.Contains(line, "MissingOrUnknownApn") ||
			strings.Contains(line, "ServiceOptionNotSubscribed") ||
			strings.Contains(line, "UserAuthenticationFailed") {
			s.Log("Found retriable error in logs: ", line)
			return true
		}
	}

	return false
}
