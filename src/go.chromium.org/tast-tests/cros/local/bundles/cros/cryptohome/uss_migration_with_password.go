// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UssMigrationWithPassword,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test migration of a vault keyset user with only password to USS",
		Contacts: []string{
			"cryptohome-core@google.com",
			"hardikgoyal@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      4 * time.Minute,
	})
}

func checkMetricPresence(ctx context.Context, cr *chrome.Chrome, backingStoreConfigUMA string, metricToCheck int64) error {
	testAPIConnection, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect to test API")
	}

	buckets, err := metrics.WaitForHistogram(ctx, testAPIConnection, backingStoreConfigUMA, 30*time.Second)
	if err != nil {
		return errors.Wrap(err, "could not get requested histogram")
	}

	if len(buckets.Buckets) != 1 {
		return errors.Wrap(err, "unexpected number of buckets")
	}

	bucket := buckets.Buckets[0]
	// Expected histogram is [metricToCheck, metricToCheck+1, 1].
	if bucket.Min != metricToCheck || bucket.Max != metricToCheck+1 || bucket.Count != 1 {
		return errors.Wrap(err, "unexpected histogram update ")
	}
	return nil
}

func UssMigrationWithPassword(ctx context.Context, s *testing.State) {
	const (
		userName              = "foo@bar.baz"
		userPassword          = "secret"
		passwordLabel         = "online-password"
		backingStoreConfigUMA = "Cryptohome.AuthFactorBackingStoreConfig"
		// User has no auth factors.
		noKeysetsPresent = 0
		// All factors are stored in vault keysets.
		vaultKeyset = 1
		// All factors are stored in the user secret stash.
		userSecretStash = 2
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}
	daemonController := helper.DaemonController()

	// Wait for cryptohomed becomes available if needed.
	if err := daemonController.Ensure(ctx, hwsec.CryptohomeDaemon); err != nil {
		s.Fatal("Failed to ensure cryptohomed: ", err)
	}

	// Clean up obsolete state, in case there's any.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// See ClearHistogramTransferFile for more about why this is needed.
	if err := metrics.ClearHistogramTransferFile(); err != nil {
		s.Error("Could not truncate existing metrics files: ", err)
	}

	func() {
		// Disable USS explicitly. This flag will be cleaned up after this block runs.
		ussDisableFlagCleanup, ussErr := helper.DisableUserSecretStash(ctx)
		if ussErr != nil {
			s.Fatal("Failed to enable the UserSecretStash experiment: ", err)
		}
		defer ussDisableFlagCleanup(ctxForCleanUp)

		cr, err := chrome.New(ctx,
			chrome.FakeLogin(chrome.Creds{User: userName, Pass: userPassword}),
			chrome.KeepState())
		if err != nil {
			s.Fatal("Failed to start Chrome at login screen: ", err)
		}
		defer cr.Close(ctx)

		if err = checkMetricPresence(ctx, cr, backingStoreConfigUMA, noKeysetsPresent); err != nil {
			s.Fatal("Failed to get empty metric for backingStoreConfig: ", err)
		}

		// Restart UI to logout.
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}()

	// Rest of login attempts run with USS enabled.
	ussFlagCleanup, ussErr := helper.EnableUserSecretStash(ctx)
	if ussErr != nil {
		s.Fatal("Failed to enable the UserSecretStash experiment: ", err)
	}
	defer ussFlagCleanup(ctxForCleanUp)
	func() {
		cr, err := chrome.New(ctx,
			chrome.FakeLogin(chrome.Creds{User: userName, Pass: userPassword}),
			chrome.KeepState())

		if err != nil {
			s.Fatal("Failed to start Chrome at login screen: ", err)
		}
		defer cr.Close(ctx)

		if err = checkMetricPresence(ctx, cr, backingStoreConfigUMA, vaultKeyset); err != nil {
			s.Fatal("Failed to get vaultKeyset metric for backingStoreConfig: ", err)
		}

		// Restart UI to logout.
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}()

	func() {
		cr, err := chrome.New(ctx,
			chrome.FakeLogin(chrome.Creds{User: userName, Pass: userPassword}),
			chrome.KeepState())
		if err != nil {
			s.Fatal("Failed to start Chrome at login screen: ", err)
		}
		defer cr.Close(ctx)

		if err = checkMetricPresence(ctx, cr, backingStoreConfigUMA, userSecretStash); err != nil {
			s.Fatal("Failed to get userSecretStash metric for backingStoreConfig: ", err)
		}

		// Restart UI to logout.
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatal("Failed to restart ui: ", err)
		}
	}()
}
