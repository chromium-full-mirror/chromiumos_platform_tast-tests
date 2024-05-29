// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"os"
	"path/filepath"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/network/iface"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/wifi/intelfwextractor"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CheckIntelFWDump,
		Desc: "Verifies that device coredumps are not empty",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
			"chromeos-data-eng@google.com",
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:mainline", "group:wificell", "wificell_func", "group:labqual"},
		// TODO(b:169152720), Remove "no_kernel_upstream" to enable the test to run on
		// boards with upstream kernel when upstream iwlwifi is able to produce valid
		// fw dumps.
		SoftwareDeps: []string{"wifi", "no_kernel_upstream", "no_fbpreprocessord"},
		// NB: The WifiIntel dependency tracks a manually maintained list of devices.
		// If the test is skipping when it should run or vice versa, check the hwdep
		// to see if your board is incorrectly included/excluded.
		// On reven board, the test fails due to missing iwlwifi folder(b:264258845).
		// Flex does not update the driver and does not need to recheck the functionality
		// of the firmware dump, so skip this test on reven.
		HardwareDeps:    hwdep.D(hwdep.WifiIntel(), hwdep.SkipOnModel("reven")),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func CheckIntelFWDump(ctx context.Context, s *testing.State) {
	const (
		iwlwifiDir       = "/sys/kernel/debug/iwlwifi"
		devCoreDumpName  = `devcoredump_iwlwifi\.\d{8}\.\d{6}\.\d+\.\d+\.devcore.gz`
		metaDumpName     = `devcoredump_iwlwifi\.\d{8}\.\d{6}\.\d+\.\d+\.meta`
		logDumpName      = `devcoredump_iwlwifi\.\d{8}\.\d{6}\.\d+\.\d+\.log`
		fwDbgCollectPath = "/iwlmvm/fw_dbg_collect"
	)

	// Verify that DUT has Intel WiFi.
	if _, err := os.Stat(iwlwifiDir); os.IsNotExist(err) {
		s.Fatal("iwlwifi directory does not exist on DUT, skipping test")
	}

	if _, err := os.ReadDir(crash.KernelDevCDDir); err != nil {
		// the /sys/class/devcoredump is unregistered due to unexpected kernel errors
		// Intel FW dumping requires the devcoredump kernel module.
		s.Fatal("devcoredump directory does not exist on DUT, indicating kernel errors. Failing the test")
	}

	// Check the existence of file filter created by crash_reporter.
	// Clean it up if so.
	crash.CleanupDevcoredump(ctx)

	// This test uses crash.DevImage because it is designed to test device
	// coredump handling on developer images.  Without it, no .devcore.gz
	// files would be created.
	ctxForTearingDownCrashTest := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()
	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent(), crash.DevImage()); err != nil {
		s.Fatal("SetUpCrashTest failed: ", err)
	}
	defer crash.TearDownCrashTest(ctxForTearingDownCrashTest)

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	ifaceName, err := shill.WifiInterface(ctx, m, 5*time.Second)
	if err != nil {
		s.Fatal("Failed to get the WiFi interface: ", err)
	}

	netIface := iface.NewInterface(ifaceName)

	devName, err := netIface.ParentDeviceName(ctx)
	if err != nil {
		s.Fatal("Failed to get the network parent device name: ", err)
	}

	fwDbgCollect := filepath.Join(filepath.Join(iwlwifiDir, devName), fwDbgCollectPath)
	if _, err := os.Stat(fwDbgCollect); err != nil {
		s.Fatalf("Failed to get the file information for %s: %v", fwDbgCollect, err)
	}

	s.Log("Triggering a devcoredump")
	if err := os.WriteFile(fwDbgCollect, []byte("1"), 0); err != nil {
		s.Fatal("Failed to trigger a devcoredump: ", err)
	}

	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	// We might not be logged in, so also allow system crash dir.
	crashDirs = append(crashDirs, crash.SystemCrashDir)

	// Check that expected device coredump is copied to crash directory.
	ctxForRemovingAllFiles := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 2*time.Second)
	defer cancel()
	s.Log("Waiting for {.devcore.gz, .meta, .log} files to be added to crash directory")
	devCoreFiles, err := crash.WaitForCrashFiles(ctx, crashDirs,
		[]string{devCoreDumpName, metaDumpName, logDumpName})
	if err != nil {
		s.Fatal("Failed while polling crash directory: ", err)
	}
	defer func(ctx context.Context) {
		if err := crash.RemoveAllFiles(ctx, devCoreFiles); err != nil {
			s.Log("Couldn't clean up files: ", err)
		}
	}(ctxForRemovingAllFiles)

	file := devCoreFiles[devCoreDumpName][0]

	var currentFileSize int64
	// Wait for the fw dump to be fully written.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		fileInfo, err := os.Stat(file)
		if err != nil {
			return errors.New("failed to get the file information of the fw core dump")
		}
		if fileInfo.Size() > 0 && fileInfo.Size() == currentFileSize {
			return nil
		}
		currentFileSize = fileInfo.Size()
		return errors.New("failed the fw dump is still being written")
	}, &testing.PollOptions{
		// Ran the test a 100 times on the boards octopus and zork. The file usually
		// takes less than 2ms to be fully written and 10s of milliseconds at worst.
		// Also, Intel observed ~300ms memory read time for iwlwifi memory dumps and
		// few seconds in extreme cases such as using virtualizations. see b/170974763
		// for more context.
		Timeout:  5 * time.Second,
		Interval: 100 * time.Millisecond,
	}); err != nil {
		s.Fatal("Failed to wait for fw dump to be fully written, err: ", err)
	}

	if err := intelfwextractor.ValidateFWDump(ctx, file); err != nil {
		s.Fatal("Failed to validate the fw dump: ", err)
	}

}
