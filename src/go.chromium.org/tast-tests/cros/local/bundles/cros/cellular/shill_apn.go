// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type apnTestParam struct {
	ModbOverrideProto     string
	ExpectedLastAttachAPN string
	ExpectedLastGoodAPN   string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillApn,
		Desc:         "Verifies that the cellular device can connect with different APN configurations",
		Contacts:     []string{"chromeos-cellular-team@google.com", "andrewlassalle@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_amari_callbox"},
		Params: []testing.Param{{
			Name:      "round_robin_attach_apn",
			Val:       apnTestParam{"callbox_round_robin_attach.pbf", "callbox-ipv4", "callbox-ipv4"},
			ExtraData: []string{"callbox_round_robin_attach.pbf"},
		}, {
			Name:      "round_robin_connect_ipv4_default_attach",
			Val:       apnTestParam{"callbox_round_robin_connect_ipv4_default_attach.pbf", "callbox-default-attach", "callbox-ipv4"},
			ExtraData: []string{"callbox_round_robin_connect_ipv4_default_attach.pbf"},
			ExtraAttr: []string{"cellular_unstable"},
		}, {
			Name:      "round_robin_connect_all_invalid_apn_errors",
			Val:       apnTestParam{"callbox_round_robin_connect_all_invalid_apn_errors.pbf", "", "callbox-default-attach"},
			ExtraData: []string{"callbox_round_robin_connect_all_invalid_apn_errors.pbf"},
			ExtraAttr: []string{"cellular_unstable"},
		}, {
			Name:      "null_attach_ipv4v6",
			Val:       apnTestParam{"callbox_null_attach_ipv4v6.pbf", "", "callbox-ipv4v6"},
			ExtraData: []string{"callbox_null_attach_ipv4v6.pbf"},
		}, {
			Name:      "null_attach_ipv6",
			Val:       apnTestParam{"callbox_null_attach_ipv6.pbf", "", "callbox-ipv6"},
			ExtraData: []string{"callbox_null_attach_ipv6.pbf"},
		}, {
			Name:      "null_attach_ipv4",
			Val:       apnTestParam{"callbox_null_attach_ipv4.pbf", "", "callbox-ipv4"},
			ExtraData: []string{"callbox_null_attach_ipv4.pbf"},
		}, {
			Name:      "attach_ipv6",
			Val:       apnTestParam{"callbox_attach_ipv6.pbf", "callbox-ipv6", "callbox-ipv6"},
			ExtraData: []string{"callbox_attach_ipv6.pbf"},
		}, {
			Name: "attach_ip_default",
			// Unknown ip_type should fallback to IPv4
			Val:       apnTestParam{"callbox_attach_ip_default.pbf", "callbox-ipv4", "callbox-ipv4"},
			ExtraData: []string{"callbox_attach_ip_default.pbf"},
		}, {
			Name: "attach_authentication_unknown",
			// Unknown authentication should fallback to CHAP
			Val:       apnTestParam{"callbox_attach_auth_unknown.pbf", "callbox-ipv4-chap", "callbox-ipv4-chap"},
			ExtraData: []string{"callbox_attach_auth_unknown.pbf"},
		}, {
			Name:      "attach_authentication_pap",
			Val:       apnTestParam{"callbox_attach_auth_pap.pbf", "callbox-ipv4-pap", "callbox-ipv4-pap"},
			ExtraData: []string{"callbox_attach_auth_pap.pbf"},
		}, {
			Name:      "attach_authentication_chap",
			Val:       apnTestParam{"callbox_attach_auth_chap.pbf", "callbox-ipv4-chap", "callbox-ipv4-chap"},
			ExtraData: []string{"callbox_attach_auth_chap.pbf"},
		}, {
			Name:      "default_attach_different_connect_apn_ipv4",
			Val:       apnTestParam{"callbox_default_attach_different_connect_apn_ipv4.pbf", "callbox-default-attach", "callbox-ipv4"},
			ExtraData: []string{"callbox_default_attach_different_connect_apn_ipv4.pbf"},
		}, {
			Name:      "default_attach_different_connect_apn_ipv4v6",
			Val:       apnTestParam{"callbox_default_attach_different_connect_apn_ipv4v6.pbf", "callbox-default-attach", "callbox-ipv4v6"},
			ExtraData: []string{"callbox_default_attach_different_connect_apn_ipv4v6.pbf"},
		}},
		Fixture: "cellularResetShillProfileOnPostTest",
		Timeout: 2 * time.Minute,
	})
}

func ShillApn(ctx context.Context, s *testing.State) {
	params := s.Param().(apnTestParam)
	modbOverrideProto := params.ModbOverrideProto
	expectedLastGoodAPN := params.ExpectedLastGoodAPN
	expectedLastAttachAPN := params.ExpectedLastAttachAPN

	// Fail immediately if there is a known bug that will cause the test to run until it times out.
	if strings.HasSuffix(s.TestName(), "round_robin_connect_all_invalid_apn_errors") {
		err := cellular.TagKnownBugOnModem(ctx, nil, "b/263815534", cellular.ModemFwFilterL850MR7AndLower)
		err = cellular.TagKnownBugOnModem(ctx, err, "b/290110554", cellular.ModemFwFilterFM350MR3AndLower)
		err = cellular.TagKnownBugOnModem(ctx, err, "b/289519883", cellular.ModemFwFilterFM101MR1)
		if err != nil {
			s.Fatalf("Fail early to avoid wasting DUT time: %s", err)
		}
	}

	helper := s.FixtValue().(*cellular.FixtData).Helper

	if _, err := helper.Disable(ctx); err != nil {
		s.Fatal("Failed to disable cellular: ", err)
	}

	deferCleanUp, err := cellular.SetServiceProvidersExclusiveOverride(ctx, s.DataPath(modbOverrideProto))
	if err != nil {
		s.Fatal("Failed to set service providers override: ", err)
	}
	defer deferCleanUp()
	errs := helper.ResetShill(ctx)
	if errs != nil {
		s.Fatal("Failed to reset shill: ", errs)
	}

	if err := helper.WaitForEnabledState(ctx, true); err != nil {
		s.Fatal("Cellular service did not reach Enabled state: ", err)
	}

	if err := helper.WaitForModemRegisteredAfterReset(ctx, 60*time.Second); err != nil {
		s.Fatal("Modem not registered: ", err)
	}

	if _, err := helper.ConnectWithTimeout(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	serviceLastAttachAPN, err := helper.GetCellularLastAttachAPN(ctx)
	if err != nil {
		s.Fatal("Error getting Service properties: ", err)
	}
	serviceLastGoodAPN, err := helper.GetCellularLastGoodAPN(ctx)
	if err != nil {
		s.Fatal("Error getting Service properties: ", err)
	}
	testing.ContextLog(ctx, "serviceLastAttachAPN:", serviceLastAttachAPN)
	testing.ContextLog(ctx, "serviceAPN", serviceLastGoodAPN)

	apnName := serviceLastAttachAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	if apnName != expectedLastAttachAPN {
		err := cellular.TagKnownBugOnModem(ctx, nil, "b/287791293", cellular.ModemFwFilterFM101MR1)
		s.Fatalf("Last Attach APN doesn't match: got %q, want %q. %s", apnName, expectedLastAttachAPN, cellular.ErrorToCleanString(err))
	}

	apnName = serviceLastGoodAPN[shillconst.DevicePropertyCellularAPNInfoApnName]
	if apnName != expectedLastGoodAPN {
		s.Fatalf("Last good APN doesn't match: got %q, want %q", apnName, expectedLastGoodAPN)
	}
	ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
	if err != nil {
		s.Fatal("Failed to read network provisioned IP types: ", err)
	}
	s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)

	verifyHostIPConnectivity := func(ctx context.Context) error {
		if err := cellular.VerifyIPConnectivityUsingCurl(ctx, testexec.CommandContext, ipv4, ipv6, "/usr/bin"); err != nil {
			return errors.Wrap(err, "failed connectivity test")
		}
		return nil
	}

	if err := helper.RunTestOnCellularInterface(ctx, verifyHostIPConnectivity); err != nil {
		s.Fatal("Failed to run test on cellular interface: ", err)
	}
}
