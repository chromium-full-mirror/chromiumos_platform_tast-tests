// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CarrierLabelMatches,
		Desc: "Verifies that the current cellular carrier matches the default network",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
		},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_run_isolated", "cellular_dut_check", "cellular_carrier_dependent"},
		Fixture:      "cellular",
		Timeout:      2 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "att",
				Val:       cellular.CarrierAtt,
				ExtraAttr: []string{"cellular_carrier_att"},
			},
			{
				Name:      "amarisoft",
				Val:       cellular.CarrierAmarisoft,
				ExtraAttr: []string{"cellular_carrier_amarisoft"},
			},
			{
				Name:      "tmobile",
				Val:       cellular.CarrierTmobile,
				ExtraAttr: []string{"cellular_carrier_tmobile"},
			},
			{
				Name:      "verizon",
				Val:       cellular.CarrierVerizon,
				ExtraAttr: []string{"cellular_carrier_verizon"},
			},
			{
				Name:      "softbank",
				Val:       cellular.CarrierSoftbank,
				ExtraAttr: []string{"cellular_carrier_softbank"},
			},
			{
				Name:      "rakuten",
				Val:       cellular.CarrierRakuten,
				ExtraAttr: []string{"cellular_carrier_rakuten"},
			},
			{
				Name:      "ee",
				Val:       cellular.CarrierEEUK,
				ExtraAttr: []string{"cellular_carrier_ee"},
			},
			{
				Name:      "vodafone",
				Val:       cellular.CarrierVodafoneUK,
				ExtraAttr: []string{"cellular_carrier_vodafone"},
			},
			{
				Name:      "kddi",
				Val:       cellular.CarrierKDDI,
				ExtraAttr: []string{"cellular_carrier_kddi"},
			},
			{
				Name:      "docomo",
				Val:       cellular.CarrierDocomo,
				ExtraAttr: []string{"cellular_carrier_docomo"},
			},
			{
				Name: "fi",
				// FI shares operator number with T-MOBILE since it's an MVNO.
				Val:       cellular.CarrierTmobile,
				ExtraAttr: []string{"cellular_carrier_fi"},
			},
			{
				Name:      "roger",
				Val:       cellular.CarrierRoger,
				ExtraAttr: []string{"cellular_carrier_roger"},
			},
			{
				Name:      "telus",
				Val:       cellular.CarrierTelus,
				ExtraAttr: []string{"cellular_carrier_telus"},
			},
		},
	})
}

func CarrierLabelMatches(ctx context.Context, s *testing.State) {
	modem, err := modemmanager.NewModemWithSim(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim (precondition): ", err)
	}

	operatorID, err := modem.GetOperatorIdentifier(ctx)
	if err != nil {
		s.Fatal("Cannot get the OperatorIdentifier: ", err)
	}

	carrier, err := cellular.GetCarrier(operatorID)
	if err != nil {
		s.Fatal("Failed to get carrier from operator ID: ", err)
	}

	expectedCarrier := s.Param().(cellular.Carrier)
	if carrier != expectedCarrier {
		s.Fatalf("Wrong carrier detected, expected: %v, got %v", expectedCarrier, carrier)
	}
}
