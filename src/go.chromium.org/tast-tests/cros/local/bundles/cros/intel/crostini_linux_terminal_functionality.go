// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/crostini"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/terminalapp"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrostiniLinuxTerminalFunctionality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies VT-d functionality",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		Attr:         []string{"group:intel-nda"},
		SoftwareDeps: []string{"chrome", "vm_host", "dlc"},
		HardwareDeps: crostini.CrostiniStable,
		Fixture:      "crostiniBullseye",
		Timeout:      5 * time.Minute,
	})
}

func CrostiniLinuxTerminalFunctionality(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(crostini.FixtureData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	terminalApp, err := terminalapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch terminal app after installing Crostini: ", err)
	}
	defer terminalApp.Close()(cleanupCtx)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	cmdOutput := func(cmd string) string {
		out, err := testexec.CommandContext(ctx, "bash", "-c", cmd).Output()
		if err != nil {
			s.Fatalf("Failed to execute %q command: %v", cmd, err)
		}
		return string(out)
	}

	lscpuCommand := "lscpu | grep VT"
	lscpuOut := cmdOutput(lscpuCommand)
	lscpuRe := regexp.MustCompile(`Virtualization.*VT-x`)
	if !lscpuRe.MatchString(lscpuOut) {
		s.Fatalf("Failed to get virtualization VT info: got %q , want match %q", lscpuOut, lscpuRe)
	}

	cmdLineCommand := "cat /proc/cmdline"
	cmdLineOut := cmdOutput(cmdLineCommand)
	cmdLineMatchString := "intel_iommu=on"
	if !strings.Contains(cmdLineOut, cmdLineMatchString) {
		s.Fatalf("Failed to get cmdline info: got %q, want %q", cmdLineOut, cmdLineMatchString)
	}

	dmesgCommand := "dmesg | grep DMAR"
	dmesgOut := cmdOutput(dmesgCommand)
	dmesgMatchString := "DMAR: IOMMU enabled"
	if !strings.Contains(dmesgOut, dmesgMatchString) {
		s.Fatalf("Failed to get dmesg DMAR info: got %q, want %q", dmesgOut, dmesgMatchString)
	}

	terminalWindowName := "Terminal - testuser@penguin: ~"
	if _, err = ash.BringWindowToForeground(ctx, tconn, terminalWindowName); err != nil {
		s.Fatal("Failed to bring the Terminal app to the front: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard event writer: ", err)
	}

	const (
		sudoKvmCommand = "sudo kvm-ok"
	)

	var (
		kvmExistsElement = nodewith.Name("INFO: /dev/kvm exists").Role(role.StaticText)
		kvmUsedElement   = nodewith.Name("KVM acceleration can be used").Role(role.StaticText)
	)

	cui := uiauto.New(tconn)
	if err := uiauto.Combine("check for VT-d functionality",
		terminalApp.RunCommand(kb, sudoKvmCommand),
		cui.WaitUntilExists(kvmExistsElement),
		cui.WaitUntilExists(kvmUsedElement),
	)(ctx); err != nil {
		s.Fatal("Failed to check for VT-d functionality: ", err)
	}
}
