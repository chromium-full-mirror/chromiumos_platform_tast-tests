// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	uda "chromiumos/system_api/user_data_auth_proto"
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ConcurrentSessions,
		Desc: "Test performing mutating operations using concurrent auth sessions",
		Contacts: []string{
			"cryptohome-core@google.com",
			"jadmanski@chromium.org",
		},
		BugComponent: "b:1088399", // ChromeOS > Security > Cryptohome
		Attr:         []string{"group:mainline", "group:cryptohome", "informational"},
	})
}

func ConcurrentSessions(ctx context.Context, s *testing.State) {
	const (
		userName       = "foo@bar.baz"
		userPassword1  = "secret"
		password1Label = "online-password"
		userPassword2  = "doublesecret"
		password2Label = "offline-password"
		userPin        = "12345"
		pinLabel       = "luggage-pin"
	)

	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	client := hwsec.NewCryptohomeClient(cmdRunner)

	// Clean up obsolete state, in case there's any.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults for preparation: ", err)
	}
	if err := cryptohome.RemoveVault(ctx, userName); err != nil {
		s.Fatal("Failed to remove old vault for preparation: ", err)
	}

	// Create and mount the persistent user.
	_, authSession1ID, err := client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session: ", err)
	}
	if err := client.CreatePersistentUser(ctx, authSession1ID); err != nil {
		s.Fatal("Failed to create persistent user: ", err)
	}
	if _, err := client.PreparePersistentVault(ctx, authSession1ID, false /*ecryptfs*/); err != nil {
		s.Fatal("Failed to prepare new persistent vault: ", err)
	}
	defer client.UnmountAll(ctxForCleanUp)

	// Add an initial password auth factor to the user. This is needed to allow
	// additional auth sessions to be created.
	if err := client.AddAuthFactor(ctx, authSession1ID, password1Label, userPassword1); err != nil {
		s.Fatal("Failed to add password auth factor: ", err)
	}

	// Create a second auth session.
	_, authSession2ID, err := client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session: ", err)
	}

	// Add password #2 to the user via session #1.
	if err := client.AddAuthFactor(ctx, authSession1ID, password2Label, userPassword2); err != nil {
		s.Fatal("Failed to add password auth factor: ", err)
	}

	// Now authenticate session #2 using password #2.
	if _, err := client.AuthenticateAuthFactor(ctx, authSession2ID, password2Label, userPassword2); err != nil {
		s.Fatal("Failed to authenticate session #2: ", err)
	}

	// Add a PIN auth factor to the user via session #2.
	if err := client.AddPinAuthFactor(ctx, authSession2ID, pinLabel, userPin); err != nil {
		s.Fatal("Failed to add password auth factor: ", err)
	}

	// Now re-authenticate session #1 using the PIN added via session #2.
	if _, err := client.AuthenticatePinAuthFactor(ctx, authSession1ID, pinLabel, userPin); err != nil {
		s.Fatal("Failed to re-authenticate session #1: ", err)
	}
}
