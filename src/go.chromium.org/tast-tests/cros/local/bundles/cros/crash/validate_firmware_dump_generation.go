// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/debugd"
	"go.chromium.org/tast-tests/cros/local/network/iface"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// Firmware dump file pattern.
	firmwareDumpFilePattern = "devcoredump_iwlwifi.*.devcore.gz"
	// Example file name for the compressed firmware dump tarball.
	firmwareDumpFileTarball = "wifi_firmware_dumps.tar.zst"
	// This timeout will be adjusted when pseudonymization is enabled.
	firmwareDumpProcessingTimeout = 5 * time.Second
	// File expiration configuration for fbpreprocessord.
	firmwareDumpExpirationTimeConfig = 20 * time.Second
	// Ensures fbpreprocessord deletes generated dump files after the configured
	// expiration time for processed dumps. This is an extra 10s after file
	// expiration to make sure the operation finishes.
	firmwareDumpExpirationOpsTimeout = 10 * time.Second
	firmwareDumpExpirationTimeout    = 2 * (firmwareDumpExpirationTimeConfig + firmwareDumpExpirationOpsTimeout)
	// Retry number for Chrome Gaia login attempts.
	firmwareDumpValidatorChromeLoginMaxRetry = 2
	// Timeout for a user session, including Chrome enrollment/login and
	// miscellaneous configuration/operations. Firmware dump enablement is
	// controlled by user policy and CUJs are related to user sessions.
	firmwareDumpValidatorPerSessionTimeout = chrome.EnrollmentAndLoginTimeout*firmwareDumpValidatorChromeLoginMaxRetry + time.Minute
)

type validateFirmwareDumpGenerationTestCase struct {
	policy         string
	isMultiSession bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ValidateFirmwareDumpGeneration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Trigger firmware dump and ensure it exists if allowed by policy",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		// ChromeOS > Platform > Connectivity > WiFi
		BugComponent:    "b:893827",
		Attr:            []string{"group:mainline", "informational", "group:criticalstaging", "group:wificell", "wificell_func"},
		SoftwareDeps:    []string{"chrome", "fbpreprocessord"},
		HardwareDeps:    hwdep.D(hwdep.WifiIntel()),
		Requirements:    []string{tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc},
		VarDeps:         []string{"connectivityfwdumps.gaiaLoginAccount"},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.UserFeedbackWithLowLevelDebugDataAllowed{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{{
			Name: "policy_wifi",
			Val: validateFirmwareDumpGenerationTestCase{
				policy:         "wifi",
				isMultiSession: false,
			},
			Timeout: firmwareDumpValidatorPerSessionTimeout + firmwareDumpExpirationTimeout,
		}, {
			Name: "policy_all",
			Val: validateFirmwareDumpGenerationTestCase{
				policy:         "all",
				isMultiSession: false,
			},
			Timeout: firmwareDumpValidatorPerSessionTimeout + firmwareDumpExpirationTimeout,
		}, {
			Name: "policy_none",
			Val: validateFirmwareDumpGenerationTestCase{
				policy:         "none",
				isMultiSession: false,
			},
			Timeout: firmwareDumpValidatorPerSessionTimeout,
		}, {
			Name: "multi_session_policy_all",
			Val: validateFirmwareDumpGenerationTestCase{
				policy:         "all",
				isMultiSession: true,
			},
			// Includes two user login events
			Timeout: 2 * firmwareDumpValidatorPerSessionTimeout,
		}},
	})
}

func ValidateFirmwareDumpGeneration(ctx context.Context, s *testing.State) {
	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: firmwareDumpValidatorChromeLoginMaxRetry,
		DoRetries:   true,
		Errorf:      s.Errorf,
		Logf:        s.Logf}

	// Adding in a poll, because the firmwareDumpValidator will be retried
	// if chrome instance fails to create with fakeDMS policy.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		return firmwareDumpValidator(ctx, rl, s)
	}, nil); err != nil {
		s.Fatal("Firmware dump validation failed: ", err)
	}
}

// getUserHash gets the user hash of the current user session
func getUserHash(ctx context.Context, sm *session.SessionManager, creds credconfig.Creds) (string, error) {
	var userHash string
	// Ensure that the daemon-store directory is created for logged in user.
	err := testing.Poll(ctx, func(ctx context.Context) error {
		// Make session manager calls to get user hash to build up user's
		// daemon store directory.
		username, hash, e := sm.RetrievePrimarySession(ctx)
		if e != nil {
			return testing.PollBreak(errors.Wrap(e, "failed to get user hash"))
		}
		if hash == "" {
			return errors.New("hash found to be empty")
		}
		if username != creds.User {
			return errors.New("username doesn't match login credential")
		}
		userHash = hash
		return nil
	}, &testing.PollOptions{Timeout: cryptohome.WaitForUserTimeout, Interval: 400 * time.Millisecond})
	return userHash, err
}

// checkIfDumpFileExists verifies that firmware dump file is created after firmware dump is triggered.
func checkIfDumpFileExists(ctx context.Context, filePath string, s *testing.State, policy string) (bool, error) {
	dumpFilePath := filepath.Join(filePath, firmwareDumpFilePattern)
	s.Log("Waiting until firmware dump is created")
	s.Log(dumpFilePath)

	var e error
	var match []string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		match, e = filepath.Glob(dumpFilePath)
		if e != nil {
			return errors.Wrap(e, "Cound not find file")
		}

		if len(match) > 0 {
			return nil
		}
		return errors.New("Waiting for firmware dump creation")
	}, &testing.PollOptions{Timeout: firmwareDumpProcessingTimeout, Interval: 400 * time.Millisecond}); err != nil {
		// Some unknown error other than file discovery.
		if e != nil {
			return false, e
		}
		// Timeout looking for file.
		return false, nil
	}
	return true, nil
}

// dumpsInDir filters for targeted type of dump files under the DirEntry.
func dumpsInDir(entries []os.DirEntry, filePattern string) []os.DirEntry {
	var ret []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if match, _ := regexp.MatchString(filePattern, entry.Name()); match {
			ret = append(ret, entry)
		}
	}
	return ret
}

// removeContents removes all content of directory provided in the argument.
func removeContents(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	names, err := d.Readdirnames(-1)
	if err != nil {
		return err
	}
	for _, name := range names {
		err = os.RemoveAll(filepath.Join(dir, name))
		if err != nil {
			return err
		}
	}
	return nil
}

// firmwareDumpExpectedByPolicy returns true if policy allows firmware dumps
func firmwareDumpExpectedByPolicy(policy string) bool {
	if policy == "wifi" || policy == "all" {
		return true
	}
	return false
}

// dirExists returns whether the given file or directory exists
func dirExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// firmwareDumpValidator contains the core logic of this test i.e. the test
// environment preparation before triggering firmware dump, followed by
// firmware dump generation.
func firmwareDumpValidator(ctx context.Context, rl *retry.Loop, s *testing.State) error {
	/*
		This test validates the end-to-end behavior of firmware dump generation,
		with the following steps:
		1- Obtain debugfs APIs that can trigger firmware dumps.
		2- Configure chrome and use test account to perform Gaia login.
			Configuration includes policy and platform feature flag.
		3- Ensure fbpreprocessord daemonstore directories are created.
		4- Trigger firmware dump.
		5- Validate the existence of firmware dumps.
		6- Ensure debugd compresses and passes the dump files to chrome.
		7-1- For test variants with single user session, verify the dump files
			are deleted when they expire.
		7-2- For test variants with multiple user sessions, re-enter the same
			user session (so that the same cryptohome is mounted) and make sure
			the dump files are deleted on session change.
	*/
	const (
		iwlwifiDir          = "/sys/kernel/debug/iwlwifi"
		fwDbgCollectPath    = "/iwlmvm/fw_dbg_collect"
		fbpreprocessordPath = "/run/daemon-store/fbpreprocessord/"
		processedDumpPath   = "/processed_dumps"
		rawDumpDirComponent = "/raw_dumps"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	tc := s.Param().(validateFirmwareDumpGenerationTestCase)

	// Test cases that include multiple user sessions don't verify dump file
	// expiration. Don't overwrite with a shorter time, and use the default
	// expiration configuration (30 min) that is long enough to survive multiple
	// user sessions.
	// For other test cases, overwrite with a shorter expiration configuration.
	if !tc.isMultiSession {
		s.Logf("Configuring the file expiration time to %v for fbpreprocessord", firmwareDumpExpirationTimeConfig)
		// Restarts fbpreprocessord so that we can configure the file expiration
		// time.
		fileExpirationString := fmt.Sprintf("%v", firmwareDumpExpirationTimeConfig.Seconds())
		if err := upstart.RestartJob(ctx, "fbpreprocessord", upstart.WithArg("FILE_EXPIRATION_SECS", fileExpirationString)); err != nil {
			s.Fatal("Failed to restart and configure fbpreprocessord: ", err)
		}
		defer func(ctx context.Context, s *testing.State) {
			s.Log("Restore fbpreprocessord file expiration time")
			if err := upstart.RestartJob(ctx, "fbpreprocessord"); err != nil {
				s.Fatal("Failed to restore fbpreprocessord: ", err)
			}
		}(cleanupCtx, s)
	}

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	s.Log("Ensuring WiFi device is enabled")
	if enabled, err := m.IsEnabled(ctx, shill.TechnologyWifi); err != nil {
		s.Fatal("Failed to check the enablement status of WiFi device: ", err)
	} else if !enabled {
		s.Log("WiFi device was not enabled, start enabling")
		e := m.EnableTechnology(ctx, shill.TechnologyWifi)
		if e != nil {
			s.Fatal("Failed to enable WiFi: ", e)
		}
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

	fwDbgCollect := filepath.Join(iwlwifiDir, devName, fwDbgCollectPath)
	if _, err := os.Stat(fwDbgCollect); err != nil {
		s.Fatalf("Failed to get the file information for %s: %v", fwDbgCollect, err)
	}

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to session manager: ", err)
	}

	creds, err := credconfig.PickRandomCreds(s.RequiredVar("connectivityfwdumps.gaiaLoginAccount"))
	if err != nil {
		s.Fatal("Failed to parse managed user creds: ", err)
	}
	login := chrome.GAIALogin(creds)

	policytypes := []string{tc.policy}
	policies := []policy.Policy{&policy.UserFeedbackWithLowLevelDebugDataAllowed{Val: policytypes}}
	fdms, err := policyutil.SetUpFakePolicyServer(ctx, s.OutDir(), creds.User, policies)
	if err != nil {
		return rl.Exit("setup fake policy server", err)
	}
	defer fdms.Stop(cleanupCtx)

	cr, err := chrome.New(
		ctx,
		login,
		chrome.DMSPolicy(fdms.URL),
		chrome.EnableFeatures("CrOSLateBootAllowFirmwareDumps"),
	)
	if err != nil {
		return rl.Retry("connect to Chrome", err)
	}
	defer cr.Close(cleanupCtx)

	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent()); err != nil {
		return errors.Wrap(err, "SetUpCrashTest failed")
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return rl.Exit("create test API Connection", err)
	}

	if err := policyutil.Verify(ctx, tconn, []policy.Policy{&policy.UserFeedbackWithLowLevelDebugDataAllowed{Val: policytypes}}); err != nil {
		return rl.Exit("verify UserFeedbackWithLowlevelDebugDataAllowed policy", err)
	}

	// Ensure that the daemon-store directory is created for logged in user.
	hash, err := getUserHash(ctx, sm, creds)
	if err != nil {
		s.Fatal("Failed to get user hash, exiting the test: ", err)
	}
	dumpPath := filepath.Join(fbpreprocessordPath, hash, processedDumpPath)

	// Wait for raw_dump path to be created. We do not want to trigger firmware dump
	// until user login is registered and raw_dumps directory is created.
	rawDumpPath := filepath.Join(fbpreprocessordPath, hash, rawDumpDirComponent)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		exist, e := dirExists(rawDumpPath)
		if e != nil {
			return testing.PollBreak(errors.Wrap(e, "failed to find directory"))
		}

		if !exist {
			return errors.New("raw dump directory not found yet, retrying")
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Second, Interval: 400 * time.Millisecond}); err != nil {
		s.Fatal("Failed to find raw dump path existence")
	}
	s.Log("Raw dumps directory created")

	s.Log("Sleeping for 3 secs for user policy to be pulled by fbpreprocessord")
	// GoBigSleepLint: User policies are queried with a delay of 2 seconds due to
	// b/317161262, so lets give few seconds for policy to be queried.
	testing.Sleep(ctx, 3*time.Second)

	dumpFilePath := filepath.Join(dumpPath, firmwareDumpFilePattern)
	// Before triggering new firmware dump ensure there are no existing firmware dumps.
	if matches, _ := filepath.Glob(dumpFilePath); len(matches) > 0 {
		e := removeContents(dumpPath)
		if e != nil {
			s.Fatal("Failed to remove existing firmware dump: ", e)
		}
	}

	s.Log("Triggering a devcoredump")
	if err := os.WriteFile(fwDbgCollect, []byte("1"), 0); err != nil {
		s.Fatal("Failed to trigger a devcoredump: ", err)
	}

	var exist bool
	// Verify if firmware dump is generated after firmware dump trigger
	exist, err = checkIfDumpFileExists(ctx, dumpPath, s, tc.policy)
	if err != nil {
		s.Fatal("Firmware dump file not generated error: ", err)
	}

	if !exist {
		// Fail if no firmware dump is generated even when allowed by policy.
		if firmwareDumpExpectedByPolicy(tc.policy) {
			s.Fatal("Firmware dump file not generated")
		}
		s.Log("Firmware dump file not generated, as expected since policy disables the feature")
		return nil
	}

	// Fatal error if firmware dump is created when not allowed by policy.
	if !firmwareDumpExpectedByPolicy(tc.policy) {
		s.Fatal("Firmware dump generated when not allowed by policy")
	}
	s.Log("Firmware dump file successfully generated")

	// Keep records of the generated dump files. They will be used to verify the
	// integrity of files after compression, passing through pipe, and
	// decompression.
	var dumpMap = make(map[string]os.DirEntry)
	entries, err := os.ReadDir(dumpPath)
	if err != nil {
		s.Fatalf("Failed to read directory %s: %s", dumpPath, err)
	}
	for _, entry := range dumpsInDir(entries, firmwareDumpFilePattern) {
		dumpMap[entry.Name()] = entry
	}

	// Simulate the process that debugd passes the compressed firmware dump
	// files to chrome through pipe. The writer fd will be passed to debugd
	// via D-Bus method call; the buffer from the reader fd will be decompressed
	// and verified.
	s.Log("Verifying debugd sends the compressed firmware dump tarball through pipe")
	fdr, fdw, err := os.Pipe()
	if err != nil {
		s.Fatal("Failed to create pipe: ", err)
	}
	defer func() {
		fdw.Close()
		fdr.Close()
	}()

	// Fetch compressed firmware dump files from debugd through pipe.
	dbgd, err := debugd.New(ctx)
	if err != nil {
		s.Fatal("Failed to create debugd proxy: ", err)
	}
	outfds := map[int]dbus.UnixFD{
		int(debugd.WifiFirmwareDump): dbus.UnixFD(fdw.Fd()),
	}
	go func() {
		s.Log("Fetching compressed firmware dump tarball from debugd")
		if err := dbgd.GetFeedbackBinaryLogs(ctx, creds.User, outfds); err != nil {
			s.Fatal("Failed to obtain the compressed firmware dump files from debugd: ", err)
		}
		s.Log("Successfully passed compressed firmware dump files from debugd through pipe. Closing writer side pipe")
		fdw.Close()
	}()

	// Receive compressed firmware dump files from the reader side of pipe and
	// decompress to verify the firmware dump file names.
	fwdmpDir, err := os.MkdirTemp("", "")
	if err != nil {
		s.Fatalf("Failed to created temp dir %s: %s", fwdmpDir, err)
	}
	outputDir, err := os.MkdirTemp(fwdmpDir, "")
	if err != nil {
		s.Fatalf("Failed to created temp dir %s for decompressed dump files: %s", outputDir, err)
	}
	defer os.RemoveAll(fwdmpDir)
	fwdumpFile := filepath.Join(fwdmpDir, firmwareDumpFileTarball)
	fwdmp, err := os.Create(fwdumpFile)
	if err != nil {
		s.Fatal("Failed to create the firmware dump file: ", err)
	}
	defer fwdmp.Close()
	if _, err := io.Copy(fwdmp, fdr); err != nil {
		s.Fatal("Failed to read error: ", err)
	}
	if err := testexec.CommandContext(ctx, "tar", "-xf", fwdmp.Name(), "-C", outputDir).Run(testexec.DumpLogOnError); err != nil {
		s.Logf("Failed to extract %s: %s", outputDir, err)
	}
	var outputEntries []os.DirEntry
	if rawOutputEntries, err := os.ReadDir(outputDir); err != nil {
		s.Fatalf("Failed to read directory %s: %s", outputDir, err)
	} else {
		outputEntries = dumpsInDir(rawOutputEntries, firmwareDumpFilePattern)
	}
	if len(outputEntries) != len(dumpMap) {
		s.Logf("Dump file list before compression: %#v", entries)
		s.Logf("Dump file list after decompression: %#v", outputEntries)
		s.Fatal("The number of dump files from the decompressed tarball doesn't match the number of generated dumps")
	}
	for _, entry := range outputEntries {
		val, ok := dumpMap[entry.Name()]
		if !ok {
			s.Fatalf("Decompressed file %s not found in the record before compression", entry.Name())
		}
		inputInfo, err := val.Info()
		if err != nil {
			s.Fatalf("Failed to obtain file information for the dump file before compression %s: %s", entry.Name(), err)
		}
		outputInfo, err := entry.Info()
		if err != nil {
			s.Fatalf("Failed to obtain file information for the dump file after decompression %s: %s", entry.Name(), err)
		}
		if inputInfo.Size() != outputInfo.Size() {
			s.Logf("Dump file size before compression: %d", inputInfo.Size())
			s.Logf("Dump file size after decompression: %d", outputInfo.Size())
			s.Fatalf("The decompressed file %s may be corrupted: size doesn't match", entry.Name())
		}
	}
	s.Log("Successfully found firmware dump files in the tarball passed by debugd through pipe")

	// For single-session test cases, verifies that the dump files are deleted
	// by fbpreprocessord after pre-configured file expiration time.
	if !tc.isMultiSession {
		s.Logf("Sleeping for %v until processed dump files expire", firmwareDumpExpirationTimeConfig)
		// GoBigSleepLint: Wait till processed dump files expire, as part of the
		// testing event flow.
		testing.Sleep(ctx, firmwareDumpExpirationTimeConfig)

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			// Verify if firmware dump is deleted after expiration
			exist, e := checkIfDumpFileExists(ctx, dumpPath, s, tc.policy)
			if e != nil {
				return e
			}
			if exist {
				return errors.New("firmware dump file not deleted after expiration")
			}
			return nil
		}, &testing.PollOptions{Timeout: firmwareDumpExpirationOpsTimeout, Interval: 400 * time.Millisecond}); err != nil {
			s.Fatal("Firmware dump file expiration test fails: ", err)
		}
		s.Log("Firmware dump successfully deleted after expiration")
		return nil
	}

	// The following section verifies that fbpreprocessord deletes firmware dump
	// files when user session changes for multi-session test cases.
	s.Log("Verifying fbpreprocessord deletes firmware dump files on session change")

	// Logout and confirm with signals from session manager.
	const state = "stopped"
	sw, err := sm.WatchSessionStateChanged(ctx, state)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close(ctx)
	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		s.Fatal("Failed to logout: ", err)
	}
	s.Logf("Waiting for SessionStateChanged %q D-Bus signal from session_manager", state)
	select {
	case <-sw.Signals:
		s.Log("Got SessionStateChanged signal")
	case <-ctx.Done():
		s.Fatal("Didn't get SessionStateChanged signal: ", ctx.Err())
	}

	// Login again with the same user credential as the previous verification.
	cr, err = chrome.New(
		ctx,
		login,
		chrome.DMSPolicy(fdms.URL),
		chrome.EnableFeatures("CrOSLateBootAllowFirmwareDumps"),
	)
	if err != nil {
		return rl.Retry("connect to Chrome", err)
	}
	defer cr.Close(cleanupCtx)
	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		return rl.Exit("create test API Connection", err)
	}
	if err := policyutil.Verify(ctx, tconn, []policy.Policy{&policy.UserFeedbackWithLowLevelDebugDataAllowed{Val: policytypes}}); err != nil {
		return rl.Exit("verify UserFeedbackWithLowlevelDebugDataAllowed policy", err)
	}

	// Ensure that the daemon-store directory is created with the same hash.
	if h, e := getUserHash(ctx, sm, creds); e != nil {
		s.Fatal("Failed to get user hash, exiting the test: ", e)
	} else if h != hash {
		s.Fatal("Failed to login with the same credential")
	}
	s.Log("User login successful")

	// Ensure there's no dump file.
	exist, err = checkIfDumpFileExists(ctx, dumpPath, s, tc.policy)
	if err != nil {
		s.Fatal("Firmware dump file not generated error: ", err)
	}
	if exist {
		s.Fatal("Firmware Dump file not deleted after user login")
	}
	s.Log("Firmware dump successfully deleted after user login")
	return nil
}
