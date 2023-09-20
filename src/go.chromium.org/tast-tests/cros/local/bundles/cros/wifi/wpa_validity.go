// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"fmt"
	"io/ioutil"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/wpacli"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/network/cmd"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WPAValidity,
		Desc: "Verifies wpa_supplicant is up and running",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:mainline", "group:wificell", "wificell_func", "group:cq-medium"},
		SoftwareDeps:    []string{"wifi", "shill-wifi"},
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func WPAValidity(ctx context.Context, s *testing.State) {
	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}

	iface, err := shill.WifiInterface(ctx, manager, 5*time.Second)
	if err != nil {
		s.Fatal("Could not get a WiFi interface: ", err)
	}
	s.Log("WiFi interface: ", iface)

	// Even if we got a WiFi device, iface, from shill, that does not imply
	// that wpa_supplicant of the WiFi device is up and running. Poll for a
	// successful wpa_cli ping for 5 seconds to avoid false negative.
	wRunner := wpacli.NewRunner(&cmd.LocalCmdRunner{NoLogOnError: true})
	var cmdOut []byte
	dumpCmdOut := func(cmdOut []byte) string {
		filename := "wpa_cli.stdout"
		outPath := path.Join(s.OutDir(), filename)
		err := ioutil.WriteFile(outPath, cmdOut, 0644)
		if err != nil {
			return fmt.Sprintf("failed to write output to %s: %s", filename, err)
		}
		return filename
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err2 error
		cmdOut, err2 = wRunner.Ping(ctx, iface)
		return err2
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatalf("Failed to ping wpa_supplicant (stdout dump: %s): %s", dumpCmdOut(cmdOut), err)
	}

}
