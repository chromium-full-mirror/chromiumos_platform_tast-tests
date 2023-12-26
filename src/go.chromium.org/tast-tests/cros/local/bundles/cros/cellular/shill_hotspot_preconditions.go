// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast-tests/cros/local/shill"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// hotspotTestProfileName is the profile we create and use for cellular tethering tests.
const hotspotTestProfileName = "test"

type hotspotPreconditionsTestParams struct {
	disableCellularWithoutStop bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillHotspotPreconditions,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies that cellular is ready for the hotspot feature. This test is not a complete test, and it will be continuously updated as progress is made in the hotspot project",
		Contacts:       []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com", "aleksandermj@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Params: []testing.Param{{
			Name: "start_and_stop",
			Val: hotspotPreconditionsTestParams{
				disableCellularWithoutStop: false,
			},
		}, {
			Name: "start_and_disable",
			Val: hotspotPreconditionsTestParams{
				disableCellularWithoutStop: true,
			},
		}},
		Fixture: "cellular",
		Timeout: 2 * time.Minute,
	})
}

func verifyConnectivityInBearer(ctx context.Context, s *testing.State, helper *cellular.Helper, bearer *modemmanager.Bearer) error {
	// We cannot rely on the IP settings exposed by the shill service, because for the case of
	// multiplexed DUN, there is none. The IP method exposed by the corresponding bearer already
	// tells us the type of IP setup shill is doing, so use that as an indication of which
	// connectivity test to run.
	ipv4config := bearer.IP4Config()
	ipv4 := ipv4config.Method == uint32(mmconst.BearerIPMethodStatic) || ipv4config.Method == uint32(mmconst.BearerIPMethodDHCP)
	ipv6config := bearer.IP6Config()
	ipv6 := ipv6config.Method == uint32(mmconst.BearerIPMethodStatic) || ipv6config.Method == uint32(mmconst.BearerIPMethodDHCP)
	iface := bearer.Interface()
	s.Logf("Bearer connected (%s) ipv4: %v, ipv6: %v", iface, ipv4, ipv6)

	verifyHostIPConnectivity := func(ctx context.Context) error {
		if err := cellular.VerifyIPConnectivityByInterface(ctx, testexec.CommandContext, ipv4, ipv6, iface, "/bin"); err != nil {
			return errors.Wrap(err, "failed connectivity test")
		}
		return nil
	}
	if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPConnectivity); err != nil {
		return errors.Wrap(err, "failed to run test on cellular interface")
	}
	return nil
}

func getDefaultBearerApn(ctx context.Context, modem *modemmanager.Modem) (string, error) {
	// Store the original connected APN, so that we can validate it is the same one once we disable tethering.
	bearer, err := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	if err != nil {
		return "", errors.Wrap(err, "failed to get default bearer")
	}
	apn, err := bearer.GetAPN()
	if err != nil {
		return "", errors.Wrap(err, "failed to get default bearer APN")
	}
	return apn, nil
}

func ShillHotspotPreconditions(ctx context.Context, s *testing.State) {
	params := s.Param().(hotspotPreconditionsTestParams)
	disableCellularWithoutStop := params.disableCellularWithoutStop

	helper := s.FixtValue().(*cellular.FixtData).Helper

	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		s.Fatal("Failed to get modem info: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Second)
	defer cancel()
	defer func(ctx context.Context) {
		cellular.CheckIfL850VerizonAndFixDefaultAPN(ctx)
	}(cleanupCtx)

	// TODO(b/267804414): Set tethering Allowed is only needed during fishfooding and can be removed later.
	testing.ContextLog(ctx, "Set tethering allowed")
	if err := helper.Manager.SetTetheringAllowed(ctx, true); err != nil {
		s.Fatal("Unable to set tethering allowed: ", err)
	}

	if err := helper.Manager.SetExperimentalTetheringFunctionality(ctx, true); err != nil {
		s.Fatal("Unable to set ExperimentalTetheringFunctionality: ", err)
	}

	testing.ContextLog(ctx, "Create fake user profile")
	if _, err := helper.Manager.CreateFakeUserProfile(ctx, hotspotTestProfileName); err != nil {
		s.Fatalf("Failed to create fake profile %q: %s", hotspotTestProfileName, err)
	}
	defer func(ctx context.Context) {
		if err := helper.Manager.RemoveFakeUserProfile(ctx, hotspotTestProfileName); err != nil {
			s.Fatalf("Failed to remove profile %q: %s", hotspotTestProfileName, err)
		}
	}(ctx)

	testing.ContextLog(ctx, "Connect before enabling tethering")
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to Cellular for hotspot: ", err)
	}

	apnBeforeTethering, err := getDefaultBearerApn(ctx, modem)
	if err != nil {
		s.Fatal("Failed to get APN before enabling tethering: ", err)
	}
	testing.ContextLog(ctx, "APN before enabling tethering retrieved: ", apnBeforeTethering)

	status, err := helper.Manager.CheckTetheringReadiness(ctx)
	if err != nil {
		s.Fatalf("Failed to check tethering readiness: %s. Status: %q", err, status)
	}
	if status == shillconst.TetheringReadinessNotAllowedByCarrier {
		// There is nothing else to test. Return early
		s.Log("Carrier does not allow tethering")
		return
	}
	if status != shillconst.TetheringReadinessReady {
		s.Fatalf("Got TetheringReadiness %q, want %q", status, shillconst.TetheringReadinessReady)
	}

	testing.ContextLog(ctx, "Configure tethering")
	serviceProps := map[string]interface{}{
		shillconst.TetheringConfUpstreamTech: shillconst.TypeCellular,
		shillconst.TetheringConfAutoDisable:  false,
	}
	if err := helper.Manager.ConfigureTethering(ctx, serviceProps); err != nil {
		s.Fatal("Failed to configure tethering: ", err)
	}

	testing.ContextLog(ctx, "Enable tethering")
	if err := helper.Manager.EnableTethering(ctx); err != nil {
		s.Fatal("Failed to enable tethering: ", err)
	}

	testing.ContextLog(ctx, "Wait for tethering enabled")
	if _, err := helper.Manager.WaitForTetheringState(ctx, shillconst.TetheringStateActive); err != nil {
		s.Fatal("Failed to reach active tethering state: ", err)
	}

	testing.ContextLog(ctx, "Check if still connected after enabling tethering")
	if err := helper.IsConnected(ctx); err != nil {
		s.Fatal("Failed to check if service is connected after enabling tethering: ", err)
	}

	// Once tethering is enabled, we may have:
	//  * One single DEFAULT bearer: "DUN is DEFAULT" scenario, where we re-use the already
	//    existing DEFAULT connection.
	//  * One single DUN bearer: "DUN as DEFAULT" scenario, where we disconnect the existing
	//    DEFAULT and connect the DUN instead.
	//  * One DEFAULT bearer and one DUN bearer: "DUN and DEFAULT" scenario, where we have two
	//    separate multiplexed connections at the same time.
	defaultBearer, _ := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	tetheringBearer, _ := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeTethering)
	if defaultBearer == nil && tetheringBearer == nil {
		s.Fatal("Failed to setup tethering: no connected bearers found")
	}

	// Verify connectivity in the existing bearers
	if defaultBearer != nil {
		s.Log("Validating connectivity in default bearer")
		if err := verifyConnectivityInBearer(ctx, s, helper, defaultBearer); err != nil {
			s.Fatal("Failed to validate connectivity in default bearer: ", err)
		}
	}
	if tetheringBearer != nil {
		s.Log("Validating connectivity in tethering bearer")
		if err := verifyConnectivityInBearer(ctx, s, helper, tetheringBearer); err != nil {
			s.Fatal("Failed to validate connectivity in tethering bearer: ", err)
		}
	}

	if disableCellularWithoutStop {
		// If we disable cellular without stopping tethering first, we expect tethering to be reported
		// as idle.

		testing.ContextLog(ctx, "Disable cellular technology")
		if err := helper.Manager.DisableTechnology(ctx, shill.TechnologyCellular); err != nil {
			s.Fatal("Cellular technology disabling failed: ", err)
		}

		testing.ContextLog(ctx, "Wait for tethering disabled")
		if _, err := helper.Manager.WaitForTetheringState(ctx, shillconst.TetheringStateIdle); err != nil {
			s.Fatal("Failed to reach idle tethering state: ", err)
		}
	} else {
		// If we only stop tethering, we expect tethering to be reported as idle, and then modem
		// connected to the original default PDN

		testing.ContextLog(ctx, "Disable tethering")
		if err := helper.Manager.DisableTethering(ctx); err != nil {
			s.Fatal("Failed to disable tethering: ", err)
		}

		testing.ContextLog(ctx, "Wait for tethering disabled")
		if _, err := helper.Manager.WaitForTetheringState(ctx, shillconst.TetheringStateIdle); err != nil {
			s.Fatal("Failed to reach idle tethering state: ", err)
		}

		testing.ContextLog(ctx, "Check if still connected after disabling tethering")
		if err := helper.IsConnected(ctx); err != nil {
			s.Fatal("No longer connected after disabling tethering: ", err)
		}

		// Compare the APN with the original one we had before enabling tethering.
		apnAfterTethering, err := getDefaultBearerApn(ctx, modem)
		if err != nil {
			s.Fatal("Failed to get APN after enabling tethering: ", err)
		}
		testing.ContextLog(ctx, "APN after enabling tethering retrieved: ", apnAfterTethering)
		if apnBeforeTethering != apnAfterTethering {
			s.Fatalf("APN before tethering (%s) is different to the one after tethering (%s)", apnBeforeTethering, apnAfterTethering)
		}
	}
}
