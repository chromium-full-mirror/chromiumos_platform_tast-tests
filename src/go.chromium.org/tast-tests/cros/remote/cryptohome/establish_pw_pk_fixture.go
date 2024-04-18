// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cryptohome

import (
	"context"
	"strings"
	"time"

	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast/core/testing"
)

const (
	// Static point on the ecdh p256 curve for TPM initialization.
	pointX = "3037d0359236fefb563c30e5c388db97f80c806282b4201d37f75e70f4e9a893"
	pointY = "e865c0b9db57b41835006bb3d57e09117c86df1b208cefae5f73f44fc19d363f"

	establishPwPairingKeyFixtureName = "establishPwPairingKeyFixture"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: establishPwPairingKeyFixtureName,
		Desc: "Reboots the DUT if PinWeaver pairing key establishment operation is blocked due to user login, and establishes the pairing key",
		Contacts: []string{
			"hcyang@google.com",
			"cryptohome-core@google.com",
		},
		BugComponent: "b:1188704",
		SetUpTimeout: 3 * time.Minute,
		Impl:         &establishPwPairingKeyFixture{},
	})
}

type establishPwPairingKeyFixture struct{}

type generateStatus int

const (
	successful generateStatus = iota
	alreadyExists
	blocked
	otherError
)

func generatePairingKey(ctx context.Context, cmdRunner *hwsecremote.CmdRunnerRemote) generateStatus {
	// Initialize TPM with a biometrics daemon key pair exchange.
	// The key pair used here is irrelevant of actual tests with fake biometrics daemon.
	reply, err := cmdRunner.Run(ctx, "pinweaver_client", "generate_ba_pk", "0", pointX, pointY)
	if err != nil {
		testing.ContextLog(ctx, "Failed to establish pinweaver pairing key: ", err)
		return otherError
	}
	replyString := string(reply)
	if strings.Contains(replyString, "EC_SUCCESS") {
		return successful
	} else if strings.Contains(replyString, "PW_ERR_BIO_AUTH_PK_ALREADY_ESTABLISHED") {
		return alreadyExists
	} else if strings.Contains(replyString, "PW_ERR_BIO_AUTH_ACCESS_DENIED") {
		return blocked
	} else {
		testing.ContextLogf(ctx, "Failed to initialize pinweaver pairing key with unknown error: %s", reply)
		return otherError
	}
}

func (f *establishPwPairingKeyFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewHelper(cmdRunner, s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec remote helper: ", err)
	}

	// Try establishing the Pk. The Pk might be already established (fine for us) or blocked
	// from establishment if it fails. For the latter case, we can reboot the device and retry.
	status := generatePairingKey(ctx, cmdRunner)
	if status == successful || status == alreadyExists {
		return nil
	} else if status == blocked {
		// Use hwsecremote's helper to reboot the DUT so that it can wait until the D-Bus hwsec
		// interested in are available.
		if err := helper.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
	} else {
		s.Fatal("pinweaver_client failed with unknown error")
	}

	status = generatePairingKey(ctx, cmdRunner)
	if status != successful {
		s.Fatal("Failed to establish pinweaver pairing key after rebooting")
	}
	return nil
}

func (f *establishPwPairingKeyFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

func (f *establishPwPairingKeyFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *establishPwPairingKeyFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *establishPwPairingKeyFixture) Reset(ctx context.Context) error {
	return nil
}
