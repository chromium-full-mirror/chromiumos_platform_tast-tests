// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/mmconst"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// callboxHotspotTestProfileName is the profile we create and use for cellular tethering tests.
const callboxHotspotTestProfileName = "test"

type shillCallboxHotspotTestParam struct {
	ModbOverrideProto string
	// Expected APN name for the bearer of type DEFAULT when tethering is enabled, if any.
	EnabledDefaultApn string
	// Expected APN name for the bearer of type DUN when tethering is enabled, if any.
	EnabledTetheringApn string
	// Expected APN name for the bearer of type DEFAULT when tethering is disabled.
	DisabledDefaultApn string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillCallboxHotspot,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verifies the different APN connection setups for tethering work",
		Contacts:       []string{"chromeos-cellular-team@google.com", "aleksandermj@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_amari_callbox"},
		Params: []testing.Param{
			// DUN is DEFAULT
			{
				Name:      "dun_is_default_ipv4",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_is_default_ipv4.pbf", "callbox-ipv4", "", "callbox-ipv4"},
				ExtraData: []string{"callbox_tethering_dun_is_default_ipv4.pbf"},
			}, {
				Name:      "dun_is_default_ipv4v6",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_is_default_ipv4v6.pbf", "callbox-ipv4v6", "", "callbox-ipv4v6"},
				ExtraData: []string{"callbox_tethering_dun_is_default_ipv4v6.pbf"},
			}, {
				Name:      "dun_is_default_ipv6",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_is_default_ipv6.pbf", "callbox-ipv6", "", "callbox-ipv6"},
				ExtraData: []string{"callbox_tethering_dun_is_default_ipv6.pbf"},
			},
			// DUN and DEFAULT separate APNs and "DUN as DEFAULT" required by operator
			{
				Name:      "dun_as_default_ipv4",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_as_default_ipv4.pbf", "", "callbox-dun-ipv4", "callbox-ipv4"},
				ExtraData: []string{"callbox_tethering_dun_as_default_ipv4.pbf"},
			}, {
				Name:      "dun_as_default_ipv4v6",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_as_default_ipv4v6.pbf", "", "callbox-dun-ipv4v6", "callbox-ipv4v6"},
				ExtraData: []string{"callbox_tethering_dun_as_default_ipv4v6.pbf"},
			}, {
				Name:      "dun_as_default_ipv6",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_as_default_ipv6.pbf", "", "callbox-dun-ipv6", "callbox-ipv6"},
				ExtraData: []string{"callbox_tethering_dun_as_default_ipv6.pbf"},
			}, {
				// Additional test with current DEFAULT as IPv4 and targeted DUN as IPv6
				Name:      "dun_as_default_ipv4_default_ipv6_dun",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_as_default_ipv4_and_ipv6.pbf", "", "callbox-dun-ipv6", "callbox-ipv4"},
				ExtraData: []string{"callbox_tethering_dun_as_default_ipv4_and_ipv6.pbf"},
			}, {
				// Additional test with current DEFAULT as IPv6 and targeted DUN as IPv4
				Name:      "dun_as_default_ipv6_default_ipv4_dun",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_as_default_ipv6_and_ipv4.pbf", "", "callbox-dun-ipv4", "callbox-ipv6"},
				ExtraData: []string{"callbox_tethering_dun_as_default_ipv6_and_ipv4.pbf"},
			},
			// DUN and DEFAULT separate APNs: will use a multiplexed DUN if supported, otherwise "DUN as DEFAULT"
			{
				Name:      "dun_and_default_ipv4",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_and_default_ipv4.pbf", "callbox-ipv4", "callbox-dun-ipv4", "callbox-ipv4"},
				ExtraData: []string{"callbox_tethering_dun_and_default_ipv4.pbf"},
			}, {
				Name:      "dun_and_default_ipv4v6",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_and_default_ipv4v6.pbf", "callbox-ipv4v6", "callbox-dun-ipv4v6", "callbox-ipv4v6"},
				ExtraData: []string{"callbox_tethering_dun_and_default_ipv4v6.pbf"},
			}, {
				Name:      "dun_and_default_ipv6",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_and_default_ipv6.pbf", "callbox-ipv6", "callbox-dun-ipv6", "callbox-ipv6"},
				ExtraData: []string{"callbox_tethering_dun_and_default_ipv6.pbf"},
			}, {
				// Additional test with current DEFAULT as IPv4 and targeted DUN as IPv6
				Name:      "dun_and_default_ipv4_default_ipv6_dun",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_and_default_ipv4_and_ipv6.pbf", "callbox-ipv4", "callbox-dun-ipv6", "callbox-ipv4"},
				ExtraData: []string{"callbox_tethering_dun_and_default_ipv4_and_ipv6.pbf"},
			}, {
				// Additional test with current DEFAULT as IPv6 and targeted DUN as IPv4
				Name:      "dun_and_default_ipv6_default_ipv4_dun",
				Val:       shillCallboxHotspotTestParam{"callbox_tethering_dun_and_default_ipv6_and_ipv4.pbf", "callbox-ipv6", "callbox-dun-ipv4", "callbox-ipv6"},
				ExtraData: []string{"callbox_tethering_dun_and_default_ipv6_and_ipv4.pbf"},
			}},
		Fixture:      "cellularResetShillProfileOnPostTest",
		HardwareDeps: hwdep.D(hwdep.WifiSAP()),
		Timeout:      2 * time.Minute,
	})
}

func validateExpectedBearers(ctx context.Context, s *testing.State, modem *modemmanager.Modem, expectedDefaultApn, expectedTetheringApn string) {
	// The bearer for APN type tethering may not exist, (e.g. in "DUN is DEFAULT" scenarios).
	tetheringBearer, err := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeTethering)
	if expectedTetheringApn != "" {
		if err != nil {
			s.Fatal("Failed to get tethering bearer: ", err)
		}
		if tetheringBearer.Connected() != true {
			s.Fatal("Tethering bearer is not connected")
		}
		tetheringBearerApn, err := tetheringBearer.GetAPN(ctx)
		if err != nil {
			s.Fatal("Tethering bearer doesn't have an APN: ", err)
		}
		if tetheringBearerApn != expectedTetheringApn {
			s.Fatalf("Expected tethering APN mismatch: got %s, want %s", tetheringBearerApn, expectedTetheringApn)
		}
	} else if err == nil {
		s.Fatal("Unexpected tethering bearer found")
	}

	// The bearer for APN type default may not exist (e.g. in "DUN as DEFAULT" scenarios).
	defaultBearer, err := modem.GetFirstConnectedDataBearer(ctx, mmconst.BearerAPNTypeDefault)
	if expectedDefaultApn != "" {
		if err != nil {
			// shill doesn't support multiplexing yet so it did a fallback to "DUN as DEFAULT".
			if expectedTetheringApn != "" {
				err = cellular.TagKnownBug(ctx, err, "b/249388098")
			}
			s.Fatal("Failed to get default bearer: ", err)
		}
		if defaultBearer.Connected() != true {
			s.Fatal("Default bearer is not connected")
		}
		defaultBearerApn, err := defaultBearer.GetAPN(ctx)
		if err != nil {
			s.Fatal("Default bearer doesn't have an APN: ", err)
		}
		if defaultBearerApn != expectedDefaultApn {
			s.Fatalf("Expected default APN mismatch: got %s, want %s", defaultBearerApn, expectedDefaultApn)
		}
	} else if err == nil {
		s.Fatal("Unexpected default bearer found")
	}
}

func ShillCallboxHotspot(ctx context.Context, s *testing.State) {
	params := s.Param().(shillCallboxHotspotTestParam)
	modbOverrideProto := params.ModbOverrideProto

	serviceProps := map[string]interface{}{
		shillconst.TetheringConfUpstreamTech: shillconst.TypeCellular,
		shillconst.TetheringConfAutoDisable:  false,
	}

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

	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	//TODO(b/267804414): SetTetheringAllowed is only needed during fishfooding and can be removed later.
	helper.Manager.SetTetheringAllowed(ctx, true)

	if err := helper.Manager.SetExperimentalTetheringFunctionality(ctx, true); err != nil {
		s.Fatal("Unable to set ExperimentalTetheringFunctionality: ", err)
	}

	if _, err := helper.Manager.CreateFakeUserProfile(ctx, callboxHotspotTestProfileName); err != nil {
		s.Fatalf("Failed to create fake profile %q", callboxHotspotTestProfileName)
	}
	defer func(ctx context.Context) {
		if err := helper.Manager.RemoveFakeUserProfile(ctx, callboxHotspotTestProfileName); err != nil {
			s.Fatalf("Failed to remove profile %q", callboxHotspotTestProfileName)
		}
	}(ctx)

	testing.ContextLog(ctx, "Configure tethering")
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

	modem, err := modemmanager.NewModem(ctx)
	if err != nil {
		s.Fatal("Failed to get modem info after enabling tethering: ", err)
	}

	// Setup expectations
	enabledDefaultApn := params.EnabledDefaultApn
	enabledTetheringApn := params.EnabledTetheringApn

	// If the modem doesn't support multiplexing and the test expects a separate
	// multiplexed tethering APN connected, rework the expectations accordingly.
	maxActiveBearers, err := modem.GetMaxActiveMultiplexedBearers(ctx, modem)
	if err != nil {
		s.Fatal("Failed to get MaxActiveMultiplexedBearers: ", err)
	}
	if maxActiveBearers <= 1 && enabledDefaultApn != "" && enabledTetheringApn != "" {
		testing.ContextLog(ctx, "Fixing test expectations in modem without multiplex support")
		enabledDefaultApn = ""
	}

	testing.ContextLog(ctx, "Validating expected data bearers after enabling tethering")
	validateExpectedBearers(ctx, s, modem, enabledDefaultApn, enabledTetheringApn)

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
		s.Fatal("Failed to check if service is connected after disabling tethering: ", err)
	}

	testing.ContextLog(ctx, "Validating expected data bearers after disabling tethering")
	validateExpectedBearers(ctx, s, modem, params.DisabledDefaultApn, "")
}
