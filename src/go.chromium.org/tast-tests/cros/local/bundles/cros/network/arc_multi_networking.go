// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"os"
	"os/exec"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCMultiNetworking,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies guest network setup upon physical interface change",
		Contacts:     []string{"cros-networking@google.com", "taoyl@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func ARCMultiNetworking(ctx context.Context, s *testing.State) {
	const (
		testNetnsName                    = "test"
		ifName                           = "eth99"
		peerIFName                       = "peer99"
		brIFName                         = "arc_eth99"
		vethIFName                       = "veth_eth99"
		networkInitializationPollTimeout = 10 * time.Second // The time to wait for patchpaneld to set up virtual network after physical network changes.
		configurationPollTimeout         = 1 * time.Second  // The time to wait for remaining configurations after detecting virtual network
	)

	// Reserve some time for cleanup code.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	startARC := func() {
		cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
		if err != nil {
			s.Fatal("Failed to connect to Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)
		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		defer a.Close(cleanupCtx)
	}

	startARC()

	shillManager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill client: ", err)
	}
	restoreEthernet, err := arc.HideUnusedEthernet(ctx, shillManager)
	if err != nil {
		s.Fatal("Failed to hide unused ethernet: ", err)
	}
	defer restoreEthernet(cleanupCtx)

	s.Log("Testing multinet behavior on device addition")

	// Create a virtual interface and verify that corresponding data path is set up.
	// Use a ethernet name template so patchpaneld treats it as an ethernet interface.
	if err := testexec.CommandContext(ctx, "/bin/ip", "netns", "add", testNetnsName).Run(testexec.DumpLogOnError); err != nil {
		// Ignore failure here for potential netns already exists case. If it's a legitimate failure it will fail at next step.
		s.Logf("Failed to create test netns %s: %s", testNetnsName, err)
	}
	defer testexec.CommandContext(cleanupCtx, "/bin/ip", "netns", "delete", testNetnsName).Run()

	if err := testexec.CommandContext(ctx, "/bin/ip", "link", "add", ifName, "type", "veth", "peer", "name", peerIFName, "netns", testNetnsName).Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to create test interface %s: %s", ifName, err)
	}
	defer testexec.CommandContext(cleanupCtx, "/bin/ip", "link", "delete", ifName).Run()

	verifyDeviceAdded := func() {
		// Verify bridge and veth created correctly and veth moved to ARC netns.
		testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := os.Stat("/sys/class/net/" + brIFName); os.IsNotExist(err) {
				return errors.Wrapf(err, "bridge %s was not created", brIFName)
			}
			return nil
		}, &testing.PollOptions{Timeout: networkInitializationPollTimeout})

		testing.Poll(ctx, func(ctx context.Context) error {
			if _, err := os.Stat("/sys/class/net/" + vethIFName); os.IsNotExist(err) {
				return errors.Wrapf(err, "veth interface %s was not created", vethIFName)
			}
			return nil
		}, &testing.PollOptions{Timeout: configurationPollTimeout})

		testing.Poll(ctx, func(ctx context.Context) error {
			if err := arc.BootstrapCommand(ctx, "/system/bin/ip", "link", "show", ifName).Run(); err != nil {
				return errors.Wrapf(err, "failed verifying interface %s in ARC", ifName)
			}
			return nil
		}, &testing.PollOptions{Timeout: configurationPollTimeout})

		// Verify forwarding rule set up correctly.
		if err := testexec.CommandContext(ctx, "/sbin/iptables", "-C", "FORWARD", "-o", brIFName, "-j", "ACCEPT", "-w").
			Run(testexec.DumpLogOnError); err != nil {
			s.Fatalf("Cannot verify iptables -A FORWARD -o %s -j ACCEPT -w rule: %s", brIFName, err)
		}
		if err := testexec.CommandContext(ctx, "/sbin/ip6tables", "-C", "FORWARD", "-o", brIFName, "-j", "ACCEPT", "-w").
			Run(testexec.DumpLogOnError); err != nil {
			s.Fatalf("Cannot verify ip6tables -A FORWARD -o %s -j ACCEPT -w rule: %s", brIFName, err)
		}
		if err := testexec.CommandContext(ctx, "/sbin/ip6tables", "-C", "FORWARD", "-i", brIFName, "-j", "ACCEPT", "-w").
			Run(testexec.DumpLogOnError); err != nil {
			s.Fatalf("Cannot verify ip6tables -A FORWARD -i %s -j ACCEPT -w rule: %s", brIFName, err)
		}
	}

	verifyDeviceAdded()

	s.Log("Testing multinet behavior on ARC restart")

	// Log out to ensure the container is down.
	upstart.RestartJob(ctx, "ui")
	if err := upstart.WaitForJobStatus(ctx, "patchpanel", upstartcommon.StartGoal, upstartcommon.RunningState, upstart.RejectWrongGoal, 30*time.Second); err != nil {
		s.Fatal("patchpanel job failed to start: ", err)
	}
	// Restart ARC.
	startARC()

	verifyDeviceAdded()

	s.Log("Testing multinet behavior on device deletion")

	// Remove test device
	if err := testexec.CommandContext(ctx, "/bin/ip", "link", "delete", ifName).Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to delete test interface %s: %s", ifName, err)
	}

	// Verify bridge and veth removed correctly.
	testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat("/sys/class/net/" + brIFName); err == nil {
			return errors.Errorf("bridge %s was not removed", brIFName)
		}
		return nil
	}, &testing.PollOptions{Timeout: networkInitializationPollTimeout})

	testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat("/sys/class/net/" + vethIFName); err == nil {
			return errors.Errorf("veth interface %s was not removed", vethIFName)
		}
		return nil
	}, &testing.PollOptions{Timeout: configurationPollTimeout})

	// Verify iptables forwarding rule removed correctly.
	if err := testexec.CommandContext(ctx, "/sbin/iptables", "-C", "FORWARD", "-i", ifName, "-o", brIFName, "-j", "ACCEPT", "-w").
		Run(testexec.DumpLogOnError); err == nil {
		s.Errorf("iptables -A FORWARD -i %s -o %s -j ACCEPT -w rule is not removed", ifName, brIFName)
	} else if _, ok := err.(*exec.ExitError); !ok {
		s.Error("iptables -C returned unexpected error: ", err)
	}
	if err := testexec.CommandContext(ctx, "/sbin/ip6tables", "-C", "FORWARD", "-i", ifName, "-o", brIFName, "-j", "ACCEPT", "-w").
		Run(testexec.DumpLogOnError); err == nil {
		s.Errorf("ip6tables -A FORWARD -i %s -o %s -j ACCEPT -w rule is not removed", ifName, brIFName)
	} else if _, ok := err.(*exec.ExitError); !ok {
		s.Error("ip6tables -C returned unexpected error: ", err)
	}
	if err := testexec.CommandContext(ctx, "/sbin/ip6tables", "-C", "FORWARD", "-i", brIFName, "-o", ifName, "-j", "ACCEPT", "-w").
		Run(testexec.DumpLogOnError); err == nil {
		s.Errorf("ip6tables -A FORWARD -i %s -o %s -j ACCEPT -w rule is not removed", brIFName, ifName)
	} else if _, ok := err.(*exec.ExitError); !ok {
		s.Error("ip6tables -C returned unexpected error: ", err)
	}
}
