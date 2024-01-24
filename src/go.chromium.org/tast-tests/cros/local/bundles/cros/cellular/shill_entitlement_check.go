// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/testing"
)

type shillCellularEntitlementCheckTestParam struct {
	ModbOverrideProto string
	// Whether the entitlement check should return OK or NOK.
	ExpectedEntitlementCheckResult string
	// If using the IMSI param in |mhs_entitlement_param|, the test needs to configure the server to return OK/NOK for the specific IMSI.
	// When doing a static check, the result of the entitlement check only depends on the url used for testing, and requires no configuration on the server. This is needed when not using any |mhs_entitlement_param| params
	// When doing a dynamic test, the test will communicate with the entitlement check server, and it will configure the server to return OK/NOK.
	SendImsi bool
}

const (
	entitlementCheckStatusCodeAllowed         = 0
	entitlementCheckStatusCodeUserNotAllowed  = 1000
	entitlementCheckStatusCodeUserSyntaxError = 1001
	entitlementCheckStatusCodeServerError     = 5000
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillEntitlementCheck,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies the entitlement check feature for tethering works. The test depends on an entitlement check server running on the callbox",
		Contacts:       []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Params: []testing.Param{{
			Name:      "static_check_ok_ipv4",
			Val:       shillCellularEntitlementCheckTestParam{"callbox_attach_ip_default.pbf", shillconst.TetheringReadinessReady, false},
			ExtraData: []string{"callbox_attach_ip_default.pbf"},
		}, {
			Name:      "static_check_ok_ipv4v6",
			Val:       shillCellularEntitlementCheckTestParam{"callbox_attach_ipv4v6.pbf", shillconst.TetheringReadinessReady, false},
			ExtraData: []string{"callbox_attach_ipv4v6.pbf"},
		}, {
			Name:      "static_check_ok_ipv6",
			Val:       shillCellularEntitlementCheckTestParam{"callbox_attach_ipv6.pbf", shillconst.TetheringReadinessReady, false},
			ExtraData: []string{"callbox_attach_ipv6.pbf"},
		}, {
			Name:      "static_check_nok",
			Val:       shillCellularEntitlementCheckTestParam{"callbox_default_and_dun_with_entitlement_returns_NOK.pbf", shillconst.TetheringReadinessNotAllowedUserNotEntitled, false},
			ExtraData: []string{"callbox_default_and_dun_with_entitlement_returns_NOK.pbf"},
		}, {
			Name:      "dynamic_check_with_imsi_ok",
			Val:       shillCellularEntitlementCheckTestParam{"callbox_default_and_dun_with_entitlement_imsi.pbf", shillconst.TetheringReadinessReady, true},
			ExtraData: []string{"callbox_default_and_dun_with_entitlement_imsi.pbf"},
		}, {
			Name:      "dynamic_check_with_imsi_nok",
			Val:       shillCellularEntitlementCheckTestParam{"callbox_default_and_dun_with_entitlement_imsi.pbf", shillconst.TetheringReadinessNotAllowedUserNotEntitled, true},
			ExtraData: []string{"callbox_default_and_dun_with_entitlement_imsi.pbf"},
		}, {
			Name:      "url_not_reachable",
			Val:       shillCellularEntitlementCheckTestParam{"callbox_default_unreachable_entitlement_server.pbf", shillconst.TetheringReadinessNotAllowedUserNotEntitled, false},
			ExtraData: []string{"callbox_default_unreachable_entitlement_server.pbf"},
		}},
		Fixture: "cellularResetShillProfileOnPostTest",
		Timeout: 2 * time.Minute,
	})
}

func ShillEntitlementCheck(ctx context.Context, s *testing.State) {
	params := s.Param().(shillCellularEntitlementCheckTestParam)
	modbOverrideProto := params.ModbOverrideProto
	expectedEntitlementCheckResult := params.ExpectedEntitlementCheckResult
	sendImsi := params.SendImsi

	helper := s.FixtValue().(*cellular.FixtData).Helper

	deferCleanUp, err := cellular.SetServiceProvidersExclusiveOverride(ctx, s.DataPath(modbOverrideProto))
	if err != nil {
		s.Fatal("Failed to set service providers override: ", err)
	}
	defer deferCleanUp()

	errs := helper.ResetShill(ctx)
	if errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	if err := helper.WaitForModemRegisteredAfterReset(ctx, 20*time.Second); err != nil {
		s.Fatal("Modem not registered: ", err)
	}

	if _, err := helper.ConnectWithTimeout(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	imsi, err := helper.GetIMSIFromShill(ctx)
	if err != nil {
		s.Fatal("Failed to get IMSI: ", err)
	}
	var callbox *cellular.CallboxServer = nil
	callboxTemp, err := cellular.NewCallboxServer(ctx)
	if err != nil {
		s.Fatal("Failed to create CallboxServer object: ", err)
	}
	callbox = callboxTemp
	if sendImsi {
		if err = callbox.ResetEntitlementValueForThisDevice(ctx, imsi); err != nil {
			s.Fatal("Failed reset the entitlement check value on server: ", err)
		}
		var expectedResult int32 = entitlementCheckStatusCodeUserNotAllowed
		if expectedEntitlementCheckResult == shillconst.TetheringReadinessReady {
			expectedResult = entitlementCheckStatusCodeAllowed
		}

		if err = callbox.SetupEntitlementReturnCodeForThisDevice(ctx, imsi, expectedResult); err != nil {
			s.Fatal("Failed set the entitlement check value on server: ", err)
		}
	}

	//TODO(b/267804414): Set tethering Allowed is only needed during fishfooding and can be removed later.
	helper.Manager.SetTetheringAllowed(ctx, true)

	if err := helper.Manager.SetExperimentalTetheringFunctionality(ctx, true); err != nil {
		s.Fatal("Unable to set ExperimentalTetheringFunctionality: ", err)
	}

	status, err := helper.Manager.CheckTetheringReadiness(ctx)
	if err != nil {
		s.Fatalf("Failed to check tethering readiness: %s. Status: %q", err, status)
	}
	if status != expectedEntitlementCheckResult {
		s.Fatalf("Got entitlement check %q, want %q", status, expectedEntitlementCheckResult)
	}

	if expectedEntitlementCheckResult == shillconst.TetheringReadinessReady {
		// The entitlement check should fail, but shill should use the previous cached value and return |TetheringReadinessReady|.
		if err = callbox.SetupEntitlementReturnCodeForThisDevice(ctx, imsi, entitlementCheckStatusCodeServerError); err != nil {
			s.Fatal("Failed set the entitlement check value on server: ", err)
		}
		status, err = helper.Manager.CheckTetheringReadiness(ctx)
		if err != nil {
			s.Fatalf("Failed to check tethering readiness: %s. Status: %q", err, status)
		}
		if status != expectedEntitlementCheckResult {
			s.Fatalf("Entitlement check not cached. Got %q, want %q", status, expectedEntitlementCheckResult)
		}

		// On a http error, the cached value should be reset to an error.
		if err = callbox.ResetEntitlementValueForThisDevice(ctx, imsi); err != nil {
			s.Fatal("Failed reset the entitlement check value on server: ", err)
		}
		if err = callbox.IgnoreNextEntitlementCheckForThisDevice(ctx); err != nil {
			s.Fatal("Failed configure IgnoreNextEntitlementCheckForThisDevice: ", err)
		}
		status, err = helper.Manager.CheckTetheringReadiness(ctx)
		if err != nil {
			s.Fatalf("Failed to check tethering readiness: %s. Status: %q", err, status)
		}
		if status != shillconst.TetheringReadinessNotAllowedUserNotEntitled {
			s.Fatalf("Entitlement check cached value not reset. Got %q, want %q", status, shillconst.TetheringReadinessNotAllowedUserNotEntitled)
		}

	}
}
