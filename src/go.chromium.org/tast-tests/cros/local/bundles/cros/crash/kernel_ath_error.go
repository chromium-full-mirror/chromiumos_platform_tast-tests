// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/network/iface"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"go.chromium.org/tast/core/testing/wlan"
)

const (
	ath10kCrashType           = "kernel_ath10k_error"
	ieeeDebugPath             = "/sys/kernel/debug/ieee80211"
	simulateAth10kFWCrashPath = "/ath10k/simulate_fw_crash"
	crashAth10kBaseName       = `kernel_ath10k_error_firmware_crashed\.\d{8}\.\d{6}\.\d+\.0`
	crashAth10kKCrashFileName = crashAth10kBaseName + `\.kcrash`
	crashAth10kLogFileName    = crashAth10kBaseName + `\.log\.gz`
	crashAth10kMetaName       = crashAth10kBaseName + `\.meta`
	sigAth10k                 = "sig=ath10k_.*firmware crashed"
	ath11kCrashType           = "kernel_ath11k_error"
	ath11DebugPath            = "/sys/kernel/debug/ath11k/"
	simulateAth11kFWCrashPath = "/simulate_fw_crash"
	crashAth11kBaseName       = `kernel_ath11k_error_firmware_crashed\.\d{8}\.\d{6}\.\d+\.0`
	crashAth11kKCrashFileName = crashAth11kBaseName + `\.kcrash`
	crashAth11kLogFileName    = crashAth11kBaseName + `\.log\.gz`
	crashAth11kMetaName       = crashAth11kBaseName + `\.meta`
	sigAth11k                 = "sig=ath11k_.*firmware crashed"
)

// kernelAthErrorTestCase holds parameters of a KernelAthError test variant.
type kernelAthErrorTestCase struct {
	crashType                 string
	crashSig                  string
	trigger                   string
	debugPath                 string
	simulateFWCrashPath       string
	crashAthMetaFileName      string
	expectedCrashFilesRegexes []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelAthError,
		Desc:         "Verify kernel Ath crashes are logged as expected",
		Contacts:     []string{"chromeos-data-eng@google.com", "arowa@google.com"},
		BugComponent: "b:1032705",
		Attr:         []string{"group:wificell", "wificell_func"},
		SoftwareDeps: []string{"wifi"},
		HardwareDeps: hwdep.D(hwdep.WifiQualcomm()),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		Params: []testing.Param{
			{
				Name: "ath10k",
				Val: kernelAthErrorTestCase{
					crashType:            ath10kCrashType,
					crashSig:             sigAth10k,
					trigger:              "soft",
					debugPath:            ieeeDebugPath,
					simulateFWCrashPath:  simulateAth10kFWCrashPath,
					crashAthMetaFileName: crashAth10kMetaName,
					expectedCrashFilesRegexes: []string{crashAth10kKCrashFileName,
						crashAth10kLogFileName,
						crashAth10kMetaName},
				},
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnWifiDevice(wlan.QualcommWCN6855)),
			},
			{
				Name: "ath11k",
				Val: kernelAthErrorTestCase{
					crashType:            ath11kCrashType,
					crashSig:             sigAth11k,
					trigger:              "assert",
					debugPath:            ath11DebugPath,
					simulateFWCrashPath:  simulateAth11kFWCrashPath,
					crashAthMetaFileName: crashAth11kMetaName,
					expectedCrashFilesRegexes: []string{crashAth11kKCrashFileName,
						crashAth11kLogFileName,
						crashAth11kMetaName},
				},
				// Only run on WiFi chips that use ath11k WiFi.
				ExtraHardwareDeps: hwdep.D(hwdep.WifiDevice(wlan.QualcommWCN6855)),
			},
		},
	})
}

func KernelAthError(ctx context.Context, s *testing.State) {
	opt := crash.WithMockConsent()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 3*time.Second)
	defer cancel()
	tc := s.Param().(kernelAthErrorTestCase)
	if err := crash.SetUpCrashTest(ctx, crash.FilterCrashes(tc.crashType), opt); err != nil {
		s.Fatal("SetUpCrashTest failed: ", err)
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	if err := crash.RestartAnomalyDetectorWithSendAll(ctx, true); err != nil {
		s.Fatal("Failed to restart anomaly detector: ", err)
	}
	// Restart anomaly detector to clear its --testonly-send-all flag at the end of execution.
	defer crash.RestartAnomalyDetector(ctx)

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	ifaceName, err := shill.WifiInterface(ctx, m, 5*time.Second)
	if err != nil {
		s.Fatal("Failed to get the WiFi interface: ", err)
	}

	netIface := iface.NewInterface(ifaceName)

	var athPath string
	if tc.crashType == ath10kCrashType {
		phyName, err := netIface.PhyName(ctx)
		if err != nil {
			s.Fatal("Failed to get the network parent device name: ", err)
		}
		athPath = filepath.Join(tc.debugPath, phyName)
	} else {
		devName, err := netIface.ParentDeviceName(ctx)
		if err != nil {
			s.Fatal("Failed to get the network parent device name: ", err)
		}
		devBusName, err := netIface.DeviceBusName(ctx)
		if err != nil {
			s.Fatal("Failed to get the network parent device name: ", err)
		}
		// The ath11k path for simulat_fw_crash has the format:
		// "/sys/kernel/debug/ath11k/devBusName-devName/simulate_fw_crash"
		athPath = filepath.Join(tc.debugPath, devBusName+"-"+devName)
	}

	athPath = filepath.Join(athPath, tc.simulateFWCrashPath)
	if _, err := os.Stat(athPath); os.IsNotExist(err) {
		s.Fatalf("Failed to find the file %s: %v", athPath, err)
	}

	s.Log("Inducing artificial ath crash")
	if err := ioutil.WriteFile(athPath, []byte(tc.trigger), 0); err != nil {
		s.Fatal("Failed to induce ath crash: ", err)
	}

	s.Log("Waiting for files")
	removeFilesCtx := ctx
	ctx, cancel = ctxutil.Shorten(removeFilesCtx, time.Second)
	defer cancel()

	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	// We might not be logged in, so also allow system crash dir.
	crashDirs = append(crashDirs, crash.SystemCrashDir)
	files, err := crash.WaitForCrashFiles(ctx, crashDirs, tc.expectedCrashFilesRegexes, crash.Timeout(60*time.Second))
	if err != nil {
		s.Fatal("Couldn't find expected files: ", err)
	}
	defer func(ctx context.Context) {
		if err := crash.RemoveAllFiles(ctx, files); err != nil {
			s.Error("Couldn't clean up files: ", err)
		}
	}(removeFilesCtx)

	if len(files[tc.crashAthMetaFileName]) == 1 {
		metaFile := files[tc.crashAthMetaFileName][0]
		contents, err := ioutil.ReadFile(metaFile)
		if err != nil {
			s.Errorf("Couldn't read meta file %s contents: %v", metaFile, err)
		} else {
			if res, err := regexp.Match(tc.crashSig, contents); err != nil {
				s.Error("Failed to frun regex: ", err)
				if err := crash.MoveFilesToOut(ctx, s.OutDir(), metaFile); err != nil {
					s.Error("Failed to save the meta file: ", err)
				}
			} else if !res {
				s.Error("Failed to find the expected Ath signature")
				if err := crash.MoveFilesToOut(ctx, s.OutDir(), metaFile); err != nil {
					s.Error("Failed to save the meta file: ", err)
				}
			}

			if !strings.Contains(string(contents), "upload_var_weight=50") {
				s.Error(".meta file did not contain expected weight")
				if err := crash.MoveFilesToOut(ctx, s.OutDir(), metaFile); err != nil {
					s.Error("Failed to save the meta file: ", err)
				}
			}
		}
	} else {
		s.Errorf("Unexpectedly found multiple meta files: %q", files[tc.crashAthMetaFileName])
		if err := crash.MoveFilesToOut(ctx, s.OutDir(), files[tc.crashAthMetaFileName]...); err != nil {
			s.Error("Failed to save unexpected crashes: ", err)
		}
	}
}
