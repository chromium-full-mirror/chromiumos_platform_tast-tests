// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	"go.chromium.org/tast-tests/cros/common/pkcs11"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/hwsec/util"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	hwsecremote "go.chromium.org/tast-tests/cros/remote/hwsec"
	"go.chromium.org/tast-tests/cros/remote/u2fd"
	webauthnpb "go.chromium.org/tast-tests/cros/services/cros/hwsec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

type webauthnU2fModeParam struct {
	isSimulator bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: WebauthnU2fMode,
		Desc: "Checks that WebAuthn under u2f mode succeeds in different configurations",
		Contacts: []string{
			"cros-hwsec@google.com",
			"hcyang@google.com",
			"chromeos-faft@google.com",
		},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"chrome", "gsc"},
		ServiceDeps: []string{
			"tast.cros.hwsec.WebauthnService",
			"tast.cros.hwsec.AttestationDBusService",
			"tast.cros.baserpc.FileSystem",
		},
		Data: []string{
			"webauthn.html",
			"bundle.js",
		},
		Params: []testing.Param{{
			ExtraAttr:         []string{"group:firmware", "firmware_cr50"},
			ExtraSoftwareDeps: []string{"no_tpm2_simulator"},
			Val: webauthnU2fModeParam{
				isSimulator: false,
			},
			ExtraTestBedDeps: tbdep.ServoPresentAndWorking,
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:mainline", "informational", "group:u2fd"},
			ExtraSoftwareDeps: []string{"tpm2_simulator"},
			Val: webauthnU2fModeParam{
				isSimulator: true,
			},
			ExtraTestBedDeps: tbdep.ServoPresentAndWorking,
		}},
		Vars: []string{"servo"},
	})
}

func WebauthnU2fMode(ctx context.Context, s *testing.State) {
	const password = "testpass"

	// Create hwsec helper.
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewFullHelper(cmdRunner, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to create hwsec remote helper: ", err)
	}

	var pbHelper hwsec.PowerButtonHelper
	if !s.Param().(webauthnU2fModeParam).isSimulator {
		// Connect to servo.
		servoSpec, _ := s.Var("servo")
		pxy, err := servo.NewProxy(ctx, servoSpec, s.DUT().KeyFile(), s.DUT().KeyDir())
		if err != nil {
			s.Fatal("Failed to connect to servo: ", err)
		}
		defer pxy.Close(ctx)
		svo := pxy.Servo()
		if err := svo.RequireDebugHeader(ctx); err != nil {
			s.Fatal("Require debug servo header: ", err)
		}
		pbHelper = util.NewServoPowerButtonHelper(svo)
	} else {
		simulatorController := hwsec.NewTi50EmulatorController(cmdRunner)
		pbHelper = simulatorController.PowerButtonHelper()
	}

	// Ensure TPM is ready before running the tests.
	if err := helper.EnsureTPMIsReady(ctx, hwsec.DefaultTakingOwnershipTimeout); err != nil {
		s.Fatal("Failed to ensure TPM is ready: ", err)
	}

	// Connect to the chrome service server on the DUT.
	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	dutfsClient := dutfs.NewClient(cl.Conn)
	dataPath, err := util.CopyFilesToRemote(ctx, s, dutfsClient)
	if err != nil {
		s.Fatal("Failed to put files to remote")
	}

	// u2fd reads files from the user's home dir, so we need to log in.
	client := webauthnpb.NewWebauthnServiceClient(cl.Conn)
	if _, err := client.New(ctx, &webauthnpb.NewRequest{
		DataPath: dataPath,
	}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer client.Close(ctx, &empty.Empty{})

	// Ensure TPM is prepared for enrollment.
	if err := helper.EnsureIsPreparedForEnrollment(ctx, hwsec.DefaultPreparationForEnrolmentTimeout); err != nil {
		s.Fatal("Failed to ensure resetting TPM: ", err)
	}

	chaps, err := pkcs11.NewChaps(ctx, cmdRunner, helper.CryptohomeClient())
	if err != nil {
		s.Fatal("Failed to create chaps client: ", err)
	}

	// Ensure chaps finished the initialization.
	// U2F didn't depend on chaps, but chaps would block the TPM operations, and caused U2F timeout.
	if err := util.EnsureChapsSlotsInitialized(ctx, chaps); err != nil {
		s.Fatal("Failed to ensure chaps slots: ", err)
	}

	// Set u2f mode to enable power button press authentication.
	util.SetU2fdFlags(ctx, helper, true, true, false)
	// Clean up the flags in u2fd after the tests finished.
	defer util.SetU2fdFlags(ctx, helper, false, false, false)
	// Wait for u2fd to create the HID device.
	_, err = util.U2fDevicePath(ctx, cmdRunner)
	if err != nil {
		s.Fatal("Failed to get u2f device path: ", err)
	}

	passwordAuthCallback := func(ctx context.Context) error {
		// Type password into ChromeOS WebAuthn dialog.
		if _, err := client.EnterPassword(ctx, &webauthnpb.EnterPasswordRequest{Password: password}); err != nil {
			return errors.Wrap(err, "failed to type password into ChromeOS auth dialog")
		}
		return nil
	}
	powerButtonAuthCallback := func(ctx context.Context) error {
		if err := pbHelper.PressAndRelease(ctx); err != nil {
			return errors.Wrap(err, "failed to press the power key")
		}
		return nil
	}

	for _, tc := range []struct {
		name              string
		userVerification  webauthnpb.UserVerification
		authenticatorType webauthnpb.AuthenticatorType
		hasDialog         bool
		authCallback      func(ctx context.Context) error
	}{
		{
			name:              "discouraged_unspecified",
			userVerification:  webauthnpb.UserVerification_DISCOURAGED,
			authenticatorType: webauthnpb.AuthenticatorType_UNSPECIFIED,
			hasDialog:         false,
			authCallback:      powerButtonAuthCallback,
		},
		{
			name:              "discouraged_cross_plaform",
			userVerification:  webauthnpb.UserVerification_DISCOURAGED,
			authenticatorType: webauthnpb.AuthenticatorType_CROSS_PLATFORM,
			hasDialog:         false,
			authCallback:      powerButtonAuthCallback,
		},
		{
			name:              "discouraged_platform",
			userVerification:  webauthnpb.UserVerification_DISCOURAGED,
			authenticatorType: webauthnpb.AuthenticatorType_PLATFORM,
			hasDialog:         false,
			authCallback:      powerButtonAuthCallback,
		},
		{
			name:              "preferred_unspecified",
			userVerification:  webauthnpb.UserVerification_PREFERRED,
			authenticatorType: webauthnpb.AuthenticatorType_UNSPECIFIED,
			hasDialog:         true,
			authCallback:      passwordAuthCallback,
		},
		{
			name:              "preferred_cross_plaform",
			userVerification:  webauthnpb.UserVerification_PREFERRED,
			authenticatorType: webauthnpb.AuthenticatorType_CROSS_PLATFORM,
			hasDialog:         false,
			authCallback:      powerButtonAuthCallback,
		},
		{
			name:              "preferred_platform",
			userVerification:  webauthnpb.UserVerification_PREFERRED,
			authenticatorType: webauthnpb.AuthenticatorType_PLATFORM,
			hasDialog:         true,
			authCallback:      passwordAuthCallback,
		},
		{
			name:              "required_unspecified",
			userVerification:  webauthnpb.UserVerification_REQUIRED,
			authenticatorType: webauthnpb.AuthenticatorType_UNSPECIFIED,
			hasDialog:         true,
			authCallback:      passwordAuthCallback,
		},
		{
			name:              "required_platform",
			userVerification:  webauthnpb.UserVerification_REQUIRED,
			authenticatorType: webauthnpb.AuthenticatorType_PLATFORM,
			hasDialog:         true,
			authCallback:      passwordAuthCallback,
		},
	} {
		result := s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			if _, err := client.StartWebauthn(ctx, &webauthnpb.StartWebauthnRequest{
				UserVerification:  tc.userVerification,
				AuthenticatorType: tc.authenticatorType,
				HasDialog:         tc.hasDialog,
			}); err != nil {
				s.Fatal("Failed to start WebAuthn flow: ", err)
			}

			cred, err := u2fd.RemoteMakeCredentialInLocalSite(ctx, client, tc.authCallback)
			if err != nil {
				s.Fatal("Failed to perform MakeCredential flow: ", err)
			}
			if err := u2fd.RemoteGetAssertionInLocalSite(ctx, client, cred, tc.authCallback); err != nil {
				s.Fatal("Failed to perform GetAssertion flow: ", err)
			}
		})
		if !result {
			break
		}
	}
}
