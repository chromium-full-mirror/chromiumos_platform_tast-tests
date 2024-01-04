// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/arcent"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/retry"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	arcApkCacheTestTimeout = 13 * time.Minute
	apkCacheDir            = "/mnt/stateful_partition/unencrypted/apkcache"
	apkCacheFilesDir       = "/mnt/stateful_partition/unencrypted/apkcache/files"
	createdSessionPrefix   = "CacheManager: Created session: "
	closedSessionPrefix    = "CacheManager: Closed session: "
	testPackage            = "com.google.android.calculator"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ManagedAppApkCache,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that apk is cached after forced app installation in ARC",
		Contacts:     []string{"arc-commercial@google.com", "batoon@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "play_store"},
		Timeout:      arcApkCacheTestTimeout,
		VarDeps:      []string{tape.ServiceAccountVar, arcent.LoginPoolVar},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ArcEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "betty",
				ExtraSoftwareDeps: []string{"android_container", "qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm", "no_android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "x",
				ExtraSoftwareDeps: []string{"android_vm_t", "no_qemu"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "betty_vm",
				ExtraSoftwareDeps: []string{"android_vm", "qemu"},
				ExtraAttr:         []string{"informational", "group:hw_agnostic"},
			}},
		Fixture: fixture.CleanOwnership,
	})
}

func ManagedAppApkCache(ctx context.Context, s *testing.State) {
	cachedApkLoadedRegEx := "cachedApk loaded for " + testPackage

	rl := &retry.Loop{Attempts: 1,
		MaxAttempts: 2,
		DoRetries:   true,
		Fatalf:      s.Fatalf,
		Logf:        s.Logf}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := testing.Poll(ctx, func(ctx context.Context) (retErr error) {
		s.Log("Deleting apk cache directory")
		if err := os.RemoveAll(apkCacheDir); err != nil {
			rl.Retry("delete apk cache directory: ", err)
		}

		creds, err := credconfig.PickNRandomCreds(s.RequiredVar(arcent.LoginPoolVar), 2 /*n*/)
		if err != nil {
			rl.Exit("get login creds", err)
		}

		packages := []string{testPackage}
		a, cr, err := loginAndWaitForARC(ctx, cleanupCtx, s, chrome.FakeEnterpriseEnroll(creds[0]), creds[0], packages, rl)
		if err != nil {
			return err
		}

		if err := verifyFirstUserSession(ctx, s, a, rl, packages); err != nil {
			return err
		}

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return rl.Retry("get test API connection", err)
		}

		s.Log("Signing out first user")
		if err := quicksettings.SignOut(ctx, tconn); err != nil {
			return rl.Retry("logout", err)
		}
		cr.Close(cleanupCtx)
		a.Close(cleanupCtx)

		// Log in different user, check that test app is served from cache.
		a, cr, err = loginAndWaitForARC(ctx, cleanupCtx, s, chrome.KeepEnrollment(), creds[1], packages, rl)
		if err != nil {
			return err
		}
		defer cr.Close(cleanupCtx)
		defer a.Close(cleanupCtx)

		exp := regexp.MustCompile(cachedApkLoadedRegEx)
		if err := a.WaitForLogcat(ctx, arc.RegexpPred(exp)); err != nil {
			return rl.Exit("find log that package was retrieved from cache", err)
		}

		// Confirm that test app is force-installed by ARC policy.
		if err := a.WaitForPackages(ctx, packages); err != nil {
			return rl.Retry("force install packages", err)
		}

		return nil
	}, nil); err != nil {
		s.Fatal("Managed app APK cache test failed: ", err)
	}
}

func loginAndWaitForARC(ctx, cleanupCtx context.Context, s *testing.State, enroll chrome.Option,
	creds credconfig.Creds, packages []string, rl *retry.Loop) (*arc.ARC, *chrome.Chrome, error) {
	const provisioningTimeout = 4 * time.Minute

	login := chrome.GAIALogin(creds)
	fdms, err := arcent.SetupPolicyServerWithArcAppsAffiliated(ctx, s.OutDir(), creds.User, packages, arcent.InstallTypeForceInstalled, arcent.PlayStoreModeAllowList, true /*affiliated*/)
	if err != nil {
		return nil, nil, rl.Exit("setup fake policy server", err)
	}
	defer fdms.Stop(cleanupCtx)

	cr, err := chrome.New(
		ctx,
		login,
		enroll,
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.DMSPolicy(fdms.URL),
		chrome.ExtraArgs(append(arc.DisableSyncFlags())...))
	if err != nil {
		return nil, nil, rl.Retry("connect to Chrome", err)
	}

	// Ensure that ARC is launched.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		return nil, nil, rl.Retry("start ARC by policy", err)
	}

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		return nil, nil, rl.Retry("wait for ARC provisioning", err)
	}

	return a, cr, nil
}

func verifyFirstUserSession(ctx context.Context, s *testing.State, a *arc.ARC, rl *retry.Loop, packages []string) error {
	notCachedRegEx       := fmt.Sprintf(
		"(no cachedApk found for %s)|(Package %s version [0-9]+ does not exist in cache)|(Missed *in cache.*%s)",
		testPackage, testPackage, testPackage)
	pushingInCacheRegEx  := "Pushing in cache " + testPackage

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Dump logcat from the first user session since it gets overwritten
	// by the second user session.
	defer dumpLogcatToOutDir(cleanupCtx, s, a, "logcat_first_session.txt")

	exp := regexp.MustCompile(notCachedRegEx)
	if err := a.WaitForLogcat(ctx, arc.RegexpPred(exp)); err != nil {
		return rl.Exit("find log that package is not already cached", err)
	}

	exp = regexp.MustCompile(pushingInCacheRegEx)
	if err := a.WaitForLogcat(ctx, arc.RegexpPred(exp)); err != nil {
		return rl.Exit("find log that package was cached", err)
	}

	// Confirm that test app is force-installed by ARC policy.
	if err := a.WaitForPackages(ctx, packages); err != nil {
		return rl.Retry("force install packages", err)
	}

	if err := waitForCacheSizeToIncrease(ctx, rl); err != nil {
		return rl.Exit("count new files in cache", err)
	}

	if err := waitForCacheSessionsToClose(ctx, s, a); err != nil {
		return rl.Exit("wait for open cache sessions to close", err)
	}
	return nil
}

func waitForCacheSizeToIncrease(ctx context.Context, rl *retry.Loop) error {
	err := testing.Poll(ctx, func(ctx context.Context) error {
		files, err := os.ReadDir(apkCacheFilesDir)
		if err != nil {
			// Cache file directory might not be created yet. Wait until it's available.
			if strings.Contains(err.Error(), "no such file or directory") {
				return err
			}
			return testing.PollBreak(err)
		}
		if len(files) <= 0 {
			// Wait for new file to be cached.
			return errors.New("Number of files in cache did not increase")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second})

	if err != nil {
		return rl.Exit("read cache file directory", err)
	}

	return nil
}

func waitForCacheSessionsToClose(ctx context.Context, s *testing.State, a *arc.ARC) error {
	createdSessionRegEx := createdSessionPrefix + "[id=[0-9]+"
	closedSessionRegEx := closedSessionPrefix + "[0-9]+"

	grepArg := fmt.Sprintf("-E \"(%s)|(%s)\"", createdSessionRegEx, closedSessionRegEx)
	logcat, err := a.OutputLogcatGrep(ctx, grepArg)
	if err != nil {
		return err
	}
	openSessionIds := getOpenSessionIds(s, logcat)
	for _, id := range openSessionIds {
		exp := regexp.MustCompile(closedSessionPrefix + id)
		s.Log("Waiting for cache session to close: " + id)
		if err := a.WaitForLogcat(ctx, arc.RegexpPred(exp)); err != nil {
			return err
		}
	}
	return nil
}

func getOpenSessionIds(s *testing.State, logcat []byte) []string {
	createdSessionRegEx := createdSessionPrefix + "\\[id=([0-9]+)"
	closedSessionRegEx := closedSessionPrefix + "([0-9]+)"

	createdSessionIds := getSessionIds(s, createdSessionRegEx, string(logcat))
	closedSessionIds := getSessionIds(s, closedSessionRegEx, string(logcat))
	return getDifference(createdSessionIds, closedSessionIds)
}

func getSessionIds(s *testing.State, regex, logContent string) map[string]bool {
	sessionIds := make(map[string]bool)
	r := regexp.MustCompile(regex)
	matches := r.FindAllStringSubmatch(logContent, -1)
	if matches == nil {
		s.Log("No matches found for pattern: " + regex)
		return sessionIds
	}
	for _, m := range matches {
		sessionIds[m[1]] = true
	}
	return sessionIds
}

func getDifference(a, b map[string]bool) []string {
	var diff []string
	for key := range a {
		if _, ok := b[key]; !ok {
			// Add any keys in a that are not in b.
			diff = append(diff, key)
		}
	}
	return diff
}

func dumpLogcatToOutDir(ctx context.Context, s *testing.State, a *arc.ARC, fileName string) {
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Log("Failed to get out dir")
	}

	logPath := filepath.Join(dir, fileName)
	if err := a.DumpLogcat(ctx, logPath); err != nil {
		s.Log("Failed to dump logcat: ", err)
	}
}
