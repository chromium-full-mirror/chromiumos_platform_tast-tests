// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"io/ioutil"
	"strings"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	recoverytool "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CryptohomeRecoveryFailure,
		Desc: "Verify cryptohome recovery failures are logged as expected",
		Contacts: []string{
			"chromeos-data-eng@google.com",
			"iscsi@google.com",
		},
		BugComponent: "b:1032705",
		Fixture:      "ussAuthSessionFixture",
		Attr:         []string{"group:mainline", "informational"},
	})
}

func CryptohomeRecoveryFailure(ctx context.Context, s *testing.State) {
	const recoveryFailureName = "cryptohome"

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up the crash test, ignoring non-cryptohome-recovery-failure crashes.
	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent(), crash.FilterCrashes(recoveryFailureName)); err != nil {
		s.Fatal("SetUpCrashTest failed: ", err)
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	// Restart anomaly detector to clear its cache of recently seen recovery
	// failures and ensure this one is logged.
	if err := crash.RestartAnomalyDetectorWithSendAll(ctx, true); err != nil {
		s.Fatal("Failed to restart anomaly detector: ", err)
	}

	// Restart anomaly detector to clear its --testonly-send-all flag at the end of execution.
	defer crash.RestartAnomalyDetector(cleanupCtx)

	s.Log("Inducing artificial recovery request failure")
	if err := induceRecoveryRequestFailure(ctx); err != nil {
		s.Fatal("Failed to induce recovery request failure: ", err)
	}

	const (
		logFileRegex  = recoveryFailureName + `\.\d{8}\.\d{6}\.\d+\.0\.log`
		metaFileRegex = recoveryFailureName + `\.\d{8}\.\d{6}\.\d+\.0\.meta`
	)
	expectedRegexes := []string{logFileRegex, metaFileRegex}

	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	// We might not be logged in, so also allow system crash dir.
	crashDirs = append(crashDirs, crash.SystemCrashDir)

	files, err := crash.WaitForCrashFiles(ctx, crashDirs, expectedRegexes)
	if err != nil {
		s.Fatal("Couldn't find expected files: ", err)
	}
	defer func() {
		if err := crash.RemoveAllFiles(cleanupCtx, files); err != nil {
			s.Log("Couldn't clean up files: ", err)
		}
	}()

	metaFiles := files[metaFileRegex]
	if len(metaFiles) != 1 {
		s.Errorf("Unexpected number of crash files: expected %v, got %v", 1, len(metaFiles))
	}
	meta := metaFiles[0]
	contents, err := ioutil.ReadFile(meta)
	if err != nil {
		s.Errorf("Couldn't read log file %s: %v", meta, err)
	}
	if !strings.Contains(string(contents), "upload_var_weight=10\n") {
		s.Error("Meta file didn't contain weight=10. Saving file")
		if err := crash.MoveFilesToOut(ctx, s.OutDir(), meta); err != nil {
			s.Error("Could not move meta file to out dir: ", err)
		}
	}
	if !strings.Contains(string(contents), "sig=GetRecoveryRequest-3-recovery-failure\n") {
		s.Error("Meta file didn't contain sig=GetRecoveryRequest-3-recovery-failure. Saving file")
		if err := crash.MoveFilesToOut(ctx, s.OutDir(), meta); err != nil {
			s.Error("Could not move meta file to out dir: ", err)
		}
	}
}

func induceRecoveryRequestFailure(ctx context.Context) error {
	const (
		userName      = "foo@bar.baz"
		userPassword  = "secret"
		passwordLabel = "online-password"
		recoveryLabel = "test-recovery"
		userGaiaID    = "123456789"
		deviceUserID  = "123-456-AA-BB"
	)
	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Create and mount the persistent user.
	_, authSessionID, err := client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return errors.Wrap(err, "failed to start auth session")
	}
	defer client.InvalidateAuthSession(ctx, authSessionID)

	if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
		return errors.Wrap(err, "failed to create persistent user")
	}
	defer cryptohome.RemoveVault(ctx, userName)

	if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
		return errors.Wrap(err, "failed to prepare new persistent vault")
	}
	defer client.UnmountAll(ctx)

	// Add a password auth factor to the user.
	if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, userPassword); err != nil {
		return errors.Wrap(err, "failed to add a password authfactor")
	}

	testTool, err := recoverytool.NewRecoveryTestTool(cmdRunner)
	if err != nil {
		return errors.Wrap(err, "failed to initialize RecoveryTestTool")
	}
	defer func(testTool *recoverytool.RecoveryTestTool) error {
		if err := testTool.RemoveDir(); err != nil {
			return errors.Wrap(err, "failed to remove dir")
		}
		return nil
	}(testTool)

	mediatorPubKey, err := testTool.FetchFakeMediatorPubKeyHex(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get mediator pub key")
	}

	// Add a recovery auth factor to the user.
	if err := client.AddRecoveryAuthFactor(ctx, authSessionID, recoveryLabel, mediatorPubKey, userGaiaID, deviceUserID); err != nil {
		return errors.Wrap(err, "failed to add a recovery auth factor")
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		return errors.Wrap(err, "failed to unmount vaults for re-mounting")
	}

	_, authSessionID, err = client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		return errors.Wrap(err, "failed to start auth session")
	}
	defer client.InvalidateAuthSession(ctx, authSessionID)

	// Invalid epoch value causes the "Failed to parse epoch response"
	// (kLocFailedParseEpochResponseInGenerateRecoveryRequest) error.
	if _, err := client.FetchRecoveryRequest(ctx, authSessionID, recoveryLabel, "invalid_epoch" /*epoch*/); err == nil {
		return errors.New("FetchRecoveryRequest succeeded with invalid epoch")
	}

	return nil
}
