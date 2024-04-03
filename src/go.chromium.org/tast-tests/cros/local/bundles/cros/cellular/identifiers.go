// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Identifiers,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that a modem returns valid identifiers",
		Contacts:     []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_run_isolated", "cellular_cq", "cellular_dut_check"},
		Timeout:      4 * time.Minute,
		Fixture:      "cellular",
	})
}

func Identifiers(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}

	shillImei, err := helper.GetIMEIFromShill(ctx)
	if err != nil {
		s.Fatal("Could not get current IMEI from shill: ", err)
	}
	modemImei, err := modem.GetEquipmentIdentifier(ctx)
	if err != nil {
		s.Fatal("Failed to read EquipmentIdentifier: ", err)
	}
	if err := validateIdentifiers("IMEI", shillImei, modemImei, 14, 16); err != nil {
		// Some NL668 engineering samples used in DVT/PVT devices may lose their IMEI number after an update or recovery.
		// Ref b/277647418, b/241292924, b/201554938 for details.
		err = cellular.TagKnownBugOnModem(ctx, err, "b/277647418", cellular.ModemFwFilterNL668A01)
		s.Fatal("IMEI validation failed: ", err)

	}

	shillImsi, err := helper.GetIMSIFromShill(ctx)
	if err != nil {
		s.Fatal("Could not get current IMSI from shill: ", err)
	}
	modemImsi, err := modem.GetIMSI(ctx)
	if err != nil {
		s.Fatal("Failed to read SIM IMSI from modemmanager: ", err)
	}
	if err := validateIdentifiers("IMSI", shillImsi, modemImsi, 0, 15); err != nil {
		s.Fatal("IMSI validation failed: ", err)
	}

	_, homeProviderCode, err := helper.GetHomeProviderFromShill(ctx)
	if err != nil {
		s.Fatal("Could not get current Home Provider code from shill: ", err)
	}
	operatorIdentifier, err := modem.GetOperatorIdentifier(ctx)
	if err != nil {
		s.Fatal("Failed to read Operator Identifier from modemmanager: ", err)
	}
	// If modemmanager fails to expose this property, the
	// HomeProvider information is obtained offline from
	// mobile_provider_database. We don't check that case here.
	if operatorIdentifier != "" {
		if err := validateIdentifiers("HomeProvide.Code", homeProviderCode, operatorIdentifier, 5, 6); err != nil {
			s.Fatal("HomeProvide.Code validation failed: ", err)
		}
	}

	iccid, err := helper.GetCurrentICCID(ctx)
	if err != nil {
		s.Fatal("Could not get current ICCID from shill: ", err)
	}
	simIdentifier, err := modem.GetSimIdentifier(ctx)
	if err != nil {
		s.Fatal("Failed to read SIM Identifier from modemmanager: ", err)
	}
	if err := validateIdentifiers("ICCID", iccid, simIdentifier, 0, 20); err != nil {
		s.Fatal("ICCID validation failed: ", err)
	}
	//ensure Shill is in a stable registered state before reading serving operator info
	if err := helper.WaitForModemRegisteredAfterReset(ctx, 10*time.Second); err == nil {
		_, servingOperatorCode, err := helper.GetServingOperatorFromShill(ctx)
		if err != nil {
			s.Fatal("Could not get current serving operator from shill: ", err)
		}
		operatorCode, err := modem.GetOperatorCode(ctx)
		if err != nil {
			s.Fatal("Failed to read serving operator from ModemManager: ", err)
		}
		if err := validateIdentifiers("ServingOperator.Code", servingOperatorCode, operatorCode, 5, 6); err != nil {
			s.Fatal("ServingOperator.Code validation failed: ", err)
		}
	}
}

func validateIdentifiers(label, shillValue, modemValue string, minLen, maxLen int) error {
	if shillValue != modemValue {
		return errors.Errorf("shill value %s for %s does not match MM value %s", shillValue, label, modemValue)
	}
	if len(shillValue) < minLen || len(shillValue) > maxLen {
		return errors.Errorf("invalid %s value %s", label, shillValue)
	}

	return nil

}
