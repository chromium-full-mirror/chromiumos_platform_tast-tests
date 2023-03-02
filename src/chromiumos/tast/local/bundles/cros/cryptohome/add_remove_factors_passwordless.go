// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"time"

	uda "chromiumos/system_api/user_data_auth_proto"
	cryptohomecommon "chromiumos/tast/common/cryptohome"
	"chromiumos/tast/common/hwsec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/cryptohome"
	hwseclocal "chromiumos/tast/local/hwsec"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AddRemoveFactorsPasswordless,
		Desc: "Test adding, removing, and listing auth factors without a password",
		Contacts: []string{
			"cryptohome-core@google.com",
			"jadmanski@chromium.org",
		},
		BugComponent: "b:1088399",
		SoftwareDeps: []string{"pinweaver"},
		Fixture:      "ussAuthSessionFixture",
	})
}

// containsType check if am array of auth factor types contains a specific type.
func containsType(typeArray []uda.AuthFactorType, typeValue uda.AuthFactorType) bool {
	for _, value := range typeArray {
		if value == typeValue {
			return true
		}
	}
	return false
}

func AddRemoveFactorsPasswordless(ctx context.Context, s *testing.State) {
	const (
		ownerName     = "owner@bar.baz"
		userName      = "foo@bar.baz"
		userPassword  = "secret"
		passwordLabel = "online-password"
		userPin       = "12345"
		pinLabel      = "luggage-pin"
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
	// Create and mount the persistent user.
	_, authSessionID, err := client.StartAuthSession(ctx, userName, false /*ephemeral*/, uda.AuthIntent_AUTH_INTENT_DECRYPT)
	if err != nil {
		s.Fatal("Failed to start auth session: ", err)
	}
	if err := client.CreatePersistentUser(ctx, authSessionID); err != nil {
		s.Fatal("Failed to create persistent user: ", err)
	}
	defer cryptohome.RemoveVault(ctxForCleanUp, userName)
	if _, err := client.PreparePersistentVault(ctx, authSessionID, false /*ecryptfs*/); err != nil {
		s.Fatal("Failed to prepare new persistent vault: ", err)
	}
	defer client.UnmountAll(ctxForCleanUp)

	// List the auth factors before we've added any factors.
	listFactorsAtStartReply, err := client.ListAuthFactors(ctx, userName)
	if err != nil {
		s.Fatal("Failed to list auth factors before adding any factors: ", err)
	}
	if err := cryptohomecommon.ExpectAuthFactorsWithTypeAndLabel(
		listFactorsAtStartReply.ConfiguredAuthFactorsWithStatus,
		nil); err != nil {
		s.Fatal("Mismatch in configured auth factors before adding factors (-got, +want): ", err)
	}
	if !containsType(listFactorsAtStartReply.SupportedAuthFactors, uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD) {
		s.Fatal("Password not reported as a supported auth factor before adding any factors")
	}
	if !containsType(listFactorsAtStartReply.SupportedAuthFactors, uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN) {
		s.Fatal("PIN not reported as a supported auth factor before adding any factors")
	}

	// Add a password auth factor to the user.
	if err := client.AddPinAuthFactor(ctx, authSessionID, pinLabel, userPin); err != nil {
		s.Fatal("Failed to add PIN auth factor: ", err)
	}

	// List the auth factors for the user now that we've added a PIN factor.
	listFactorsAfterAddPinReply, err := client.ListAuthFactors(ctx, userName)
	if err != nil {
		s.Fatal("Failed to list auth factors after adding PIN: ", err)
	}
	if err := cryptohomecommon.ExpectAuthFactorsWithTypeAndLabel(
		listFactorsAfterAddPinReply.ConfiguredAuthFactorsWithStatus,
		[]*uda.AuthFactorWithStatus{{
			AuthFactor: &uda.AuthFactor{
				Type:  uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN,
				Label: pinLabel,
			},
		}}); err != nil {
		s.Fatal("Mismatch in configured auth factors after adding PIN (-got, +want): ", err)
	}
	if !containsType(listFactorsAtStartReply.SupportedAuthFactors, uda.AuthFactorType_AUTH_FACTOR_TYPE_PASSWORD) {
		s.Fatal("Password not reported as a supported auth factor after adding PIN")
	}
	if !containsType(listFactorsAtStartReply.SupportedAuthFactors, uda.AuthFactorType_AUTH_FACTOR_TYPE_PIN) {
		s.Fatal("PIN not reported as a supported auth factor after adding PIN")
	}

	// Unmount the user.
	if err := client.UnmountAll(ctx); err != nil {
		s.Fatal("Failed to unmount vaults: ", err)
	}
}
