// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package wwcb contains remote Tast tests that work with Chromebook.
package wwcb

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SelftestIppower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check if IP Power is online and write the IP to the config file",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "wwcbIPPowerIp"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService"},
		Data:         []string{"Capabilities.json"},
	})
}

func SelftestIppower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	// Extend the timeout to 160 seconds to make sure the test can be completed.
	ctx, cancel := ctxutil.Shorten(ctx, 160*time.Second)
	defer cancel()

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	ipPowerIP, ok := s.Var("wwcbIPPowerIp")
	if ok {
		if err := utils.CheckIppowerStatus(ipPowerIP); err != nil {
			s.Fatal("Failed to get IP Power status: ", err)
		}
	} else {
		// Clear the capabilities json file.
		if err := utils.WriteConfigFile(s, map[string]interface{}{}); err != nil {
			s.Fatal("Failed to write config file: ", err)
		}

		// Read Capabilities.json.
		capabilitiesMap := map[string]interface{}{"utils.wwcbIPPowerIp": ""}

		// Find IP Power IP.
		IP, err := utils.IppowerIP(strings.Split(dut.HostName(), ":")[0])
		if err != nil {
			s.Fatal("Failed to obtain IP Power IP: ", err)
		}
		capabilitiesMap["utils.wwcbIPPowerIp"] = IP

		if err := utils.WriteConfigFile(s, capabilitiesMap); err != nil {
			s.Fatal("Failed to write config file: ", err)
		}
	}
}
