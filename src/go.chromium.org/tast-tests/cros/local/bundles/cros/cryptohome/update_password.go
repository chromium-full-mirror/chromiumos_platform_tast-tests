// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"bytes"
	"context"
	"io/ioutil"
	"path/filepath"
	"time"

	uda "go.chromium.org/chromiumos/system_api/user_data_auth_proto"

	cryptohomecommon "go.chromium.org/tast-tests/cros/common/cryptohome"
	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdatePassword,
		Desc: "Update password auth factor and authenticate with the new password",
		Contacts: []string{
			"cryptohome-core@google.com",
			"iscsi@google.com",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome"},
		Fixture:      "ussAuthSessionFixture",
	})
}

func UpdatePassword(ctx context.Context, s *testing.State) {
	const (
		oldUserPassword = "old secret"
		newUserPassword = "new secret"
		passwordLabel   = "online-password"
		wrongLabel      = "wrong label"
		testFile        = "file"
		testFileContent = "content"
	)

	fixture := s.FixtValue().(*cryptohome.AuthSessionFixture)
	userName := fixture.TestUserName

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Create and mount the persistent user.
	_, authSessionID, err := client.StartAuthSession(ctx, userName /*ephemeral=*/, false, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session: ", err)
	}
	if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
		s.Fatal("Failed to create persistent user: ", err)
	}
	if _, err := client.PreparePersistentVault(ctx, authSessionID /*ecryptfs=*/, false); err != nil {
		s.Fatal("Failed to prepare persistent vault: ", err)
	}

	// Write a test file to verify persistence.
	userPath, err := cryptohome.UserPath(ctx, userName)
	if err != nil {
		s.Fatal("Failed to get user vault path: ", err)
	}
	filePath := filepath.Join(userPath, testFile)
	if err := ioutil.WriteFile(filePath, []byte(testFileContent), 0644); err != nil {
		s.Fatal("Failed to write a file to the vault: ", err)
	}

	authenticateWithPassword := func(password string) (string, error) {
		// Authenticate a new auth session via the auth factor and mount the user.
		_, authSessionID, err = client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
		if err != nil {
			return "", errors.Wrap(err, "failed to start auth session for re-mounting")
		}
		authReply, err := client.AuthenticateAuthFactor(ctx, authSessionID, passwordLabel, password)
		if err != nil {
			return authSessionID, errors.Wrap(err, "failed to authenticate with auth session")
		}
		if err := cryptohomecommon.ExpectAuthIntents(authReply.AuthProperties.AuthorizedFor, []uda.AuthIntent{
			uda.AuthIntent_AUTH_INTENT_DECRYPT,
			uda.AuthIntent_AUTH_INTENT_VERIFY_ONLY,
		}); err != nil {
			return authSessionID, errors.Wrap(err, "unexpected AuthSession authorized intents")
		}
		if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ephemeral*/); err != nil {
			return authSessionID, errors.Wrap(err, "failed to prepare persistent vault")
		}

		// Verify that the test file is still there.
		if content, err := ioutil.ReadFile(filePath); err != nil {
			return authSessionID, errors.Wrap(err, "failed to read back test file")
		} else if bytes.Compare(content, []byte(testFileContent)) != 0 {
			return authSessionID, errors.Errorf("incorrect tests file content. got: %q, want: %q", content, testFileContent)
		}
		return authSessionID, nil
	}

	// Add a password auth factor to the user.
	if err := client.AddAuthFactor(ctx, authSessionID, passwordLabel, oldUserPassword); err != nil {
		s.Fatal("Failed to create persistent user: ", err)
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Successfully authenticate with password.
	authSessionID, err = authenticateWithPassword(oldUserPassword)
	if err != nil {
		s.Fatal("Failed to authenticate with password: ", err)
	}
	defer client.InvalidateAuthSession(ctxForCleanUp, authSessionID)

	// Try to update password auth factor with wrong label.
	err = client.UpdatePasswordAuthFactor(ctx, authSessionID, wrongLabel /*label*/, newUserPassword)
	if err := cryptohomecommon.ExpectCryptohomeErrorCode(err, uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_KEY_NOT_FOUND); err != nil {
		s.Fatal("Failed to get the correct error code for auth factor update: ", err)
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Successfully authenticate with old password.
	authSessionID, err = authenticateWithPassword(oldUserPassword)
	if err != nil {
		s.Fatal("Failed to authenticate with password after update attempt: ", err)
	}
	defer client.InvalidateAuthSession(ctxForCleanUp, authSessionID)

	// Update password auth factor.
	if err := client.UpdatePasswordAuthFactor(ctx, authSessionID, passwordLabel /*label*/, newUserPassword); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for re-mounting: ", err)
	}

	// Authentication with old password fails.
	authSessionID, err = authenticateWithPassword(oldUserPassword)
	var authExitErr *hwsec.CmdExitError
	if !errors.As(err, &authExitErr) {
		s.Fatalf("Unexpected error for authentication with old password: got %q; want *hwsec.CmdExitError", err)
	}
	if authExitErr.ExitCode != (int)(uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_AUTHORIZATION_KEY_FAILED) {
		s.Fatalf("Unexpected exit code for authentication with old password: got %d; want %d",
			authExitErr.ExitCode, uda.CryptohomeErrorCode_CRYPTOHOME_ERROR_AUTHORIZATION_KEY_FAILED)
	}

	// Successfully authenticate with new password.
	authSessionID, err = authenticateWithPassword(newUserPassword)
	if err != nil {
		s.Fatal("Failed to authenticate with password: ", err)
	}
	defer client.InvalidateAuthSession(ctxForCleanUp, authSessionID)
}
