// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
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
	// This timeout will be adjusted when pseudonymization is enabled.
	firmwareDumpProcessingTimeout = 5 * time.Second
	// File expiration configuration for fbpreprocessord.
	firmwareDumpExpirationTimeConfig = 20 * time.Second
	// Ensures fbpreprocessord deletes generated dump files after the configured
	// expiration time for processed dumps. This is an extra 10s after file
	// expiration to make sure the operation finishes.
	firmwareDumpExpirationOpsTimeout = 10 * time.Second
	firmwareDumpExpirationTimeout    = 2 * (firmwareDumpExpirationTimeConfig + firmwareDumpExpirationOpsTimeout)
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
			Timeout: chrome.EnrollmentAndLoginTimeout + firmwareDumpExpirationTimeout,
		}, {
			Name: "policy_all",
			Val: validateFirmwareDumpGenerationTestCase{
				policy:         "all",
				isMultiSession: false,
			},
			Timeout: chrome.EnrollmentAndLoginTimeout + firmwareDumpExpirationTimeout,
		}, {
			Name: "policy_none",
			Val: validateFirmwareDumpGenerationTestCase{
				policy:         "none",
				isMultiSession: false,
			},
			Timeout: chrome.EnrollmentAndLoginTimeout,
		}, {
			Name: "multi_session_policy_all",
			Val: validateFirmwareDumpGenerationTestCase{
				policy:         "all",
				isMultiSession: true,
			},
			// Includes two user login events
			Timeout: 2 * chrome.EnrollmentAndLoginTimeout,
		}},
	})
}

func ValidateFirmwareDumpGeneration(ctx context.Context, s *testing.State) {
	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
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

// cleanUp function removes any generated firmware dump if found in
// given path and also logsout the user.
func cleanUp(ctx context.Context, s *testing.State, dumpPath string) {
	// As part of test wrapup, ensure the generated firmware dump is cleared.
	s.Log("Remove generated firmwaredump files for test cleanup")
	e := removeContents(dumpPath)
	if e != nil {
		s.Fatal("Failed to clean the raw_dumps directory after the test")
	}

	// This should always be the executed after removing the content of
	// processed dumps directory.
	s.Log("Initiating user logout")
	// Emulate logout. chrome.Close() does not log out. So, here,
	// manually restart "ui" job for the emulation.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to log out: ", err)
	}
}

// firmwareDumpValidator contains the core logic of this test i.e. the test
// environment preparation before triggering firmware dump, followed by
// firmware dump generation.
func firmwareDumpValidator(ctx context.Context, rl *retry.Loop, s *testing.State) error {
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
	// Ensures firmware dumps are deleted if generated and user is logged out on
	// test completion.
	defer cleanUp(cleanupCtx, s, dumpPath)

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
