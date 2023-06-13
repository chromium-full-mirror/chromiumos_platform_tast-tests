// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EthernetBehaviour,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Reboot DUT and verify Ethernet behaviour",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		ServiceDeps:  []string{"tast.cros.network.EthernetService"},
		Attr:         []string{"group:intel-nda"},
		Vars:         []string{"intel.EthernetBehaviour.iterations"},
		SoftwareDeps: []string{"chrome"},
		BugComponent: "b:157291",
		Fixture:      fixture.NormalMode,
		Timeout:      5 * time.Minute,
	})
}

func EthernetBehaviour(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	iterations := 1
	if iter, ok := s.Var("intel.EthernetBehaviour.iterations"); ok {
		i, err := strconv.Atoi(iter)
		if err != nil {
			s.Fatal("Failed to parse iterations value: ", iter)
		}
		iterations = i
	}

	usbDetectionRe := regexp.MustCompile(`Class=.*(480M|5000M|10G|20G)`)
	out, err := h.DUT.Conn().CommandContext(ctx, "lsusb", "-t").Output()
	if err != nil {
		s.Fatal("Failed to execute lsusb command: ", err)
	}

	if !usbDetectionRe.MatchString(string(out)) {
		s.Fatal("Failed: the DUT failed to discover an USB Ethernet device using Type-A adapter")
	}

	for i := 1; i <= iterations; i++ {
		s.Logf("Iterations: %d/%d", i, iterations)
		if err := h.DUT.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot the DUT: ", err)
		}
		cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(ctx)

		client := network.NewEthernetServiceClient(cl.Conn)

		if _, err := client.New(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}

		if _, err := client.WaitForEthernet(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to wait for an Ethernet: ", err)
		}

		const cmd = `ip route get 8.8.8.8`
		out, err = h.DUT.Conn().CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
		if err != nil {
			s.Error("Failed to get the default route: ", err)
		}
		if !strings.Contains(string(out), "eth0") {
			s.Fatal("Failed to verify the Ethernet is in the default IP route: ", err)
		}
	}
}
