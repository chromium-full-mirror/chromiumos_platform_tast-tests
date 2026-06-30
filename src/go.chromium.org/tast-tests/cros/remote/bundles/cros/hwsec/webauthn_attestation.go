// Copyright 2022 The ChromiumOS Authors
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

type webauthnAttestationParam struct {
	isSimulator bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: WebauthnAttestation,
		Desc: "Checks that WebAuthn under each mode provides correct attestation",
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
			Val: webauthnAttestationParam{
				isSimulator: false,
			},
			ExtraTestBedDeps: tbdep.ServoPresentAndWorking,
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:mainline", "informational", "group:u2fd"},
			ExtraSoftwareDeps: []string{"tpm2_simulator"},
			Val: webauthnAttestationParam{
				isSimulator: true,
			},
			ExtraTestBedDeps: tbdep.ServoPresentAndWorking,
		}},
		Vars: []string{"servo"},
	})
}

func WebauthnAttestation(ctx context.Context, s *testing.State) {
	// Create hwsec helper.
	cmdRunner := hwsecremote.NewCmdRunner(s.DUT())
	helper, err := hwsecremote.NewFullHelper(cmdRunner, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to create hwsec remote helper: ", err)
	}

	var pbHelper hwsec.PowerButtonHelper
	if !s.Param().(webauthnAttestationParam).isSimulator {
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
	cr := webauthnpb.NewWebauthnServiceClient(cl.Conn)
	if _, err := cr.New(ctx, &webauthnpb.NewRequest{
		DataPath:                   dataPath,
		AllowEnterpriseAttestation: true,
	}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx, &empty.Empty{})

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

	powerButtonAuthCallback := func(ctx context.Context) error {
		if err := pbHelper.PressAndRelease(ctx); err != nil {
			return errors.Wrap(err, "failed to press the power key")
		}
		return nil
	}

	// Clean up the flags in u2fd after the tests finished.
	defer util.SetU2fdFlags(ctx, helper, false, false, false)
	for _, tc := range []struct {
		name            string
		g2f             bool
		attestationType webauthnpb.AttestationType
	}{
		{
			name:            "u2f_none",
			g2f:             false,
			attestationType: webauthnpb.AttestationType_NONE,
		},
		{
			name:            "u2f_direct",
			g2f:             false,
			attestationType: webauthnpb.AttestationType_DIRECT,
		},
		{
			name:            "u2f_enterprise",
			g2f:             false,
			attestationType: webauthnpb.AttestationType_ENTERPRISE,
		},
		{
			name:            "g2f_none",
			g2f:             true,
			attestationType: webauthnpb.AttestationType_NONE,
		},
		{
			name:            "g2f_direct",
			g2f:             true,
			attestationType: webauthnpb.AttestationType_DIRECT,
		},
		{
			name:            "g2f_enterprise",
			g2f:             true,
			attestationType: webauthnpb.AttestationType_ENTERPRISE,
		},
	} {
		result := s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			// Set u2f/g2f mode to enable power button press authentication and enterprise
			// attestation.
			if tc.g2f {
				util.SetU2fdFlags(ctx, helper, false, true, false)
			} else {
				util.SetU2fdFlags(ctx, helper, true, false, false)
			}
			// Wait for u2fd to create the HID device.
			_, err = util.U2fDevicePath(ctx, cmdRunner)
			if err != nil {
				s.Fatal("Failed to get u2f device path: ", err)
			}
			if _, err := cr.StartWebauthn(ctx, &webauthnpb.StartWebauthnRequest{
				UserVerification:  webauthnpb.UserVerification_DISCOURAGED,
				AuthenticatorType: webauthnpb.AuthenticatorType_CROSS_PLATFORM,
				AttestationType:   tc.attestationType,
				HasDialog:         false,
			}); err != nil {
				s.Fatal("Failed to start WebAuthn flow: ", err)
			}
			cred, err := u2fd.RemoteMakeCredentialInLocalSite(ctx, cr, powerButtonAuthCallback)
			if err != nil {
				s.Fatal("Failed to perform MakeCredential flow: ", err)
			}
			serialNumberB64 := cred.SerialNumberB64
			if tc.attestationType == webauthnpb.AttestationType_NONE {
				if serialNumberB64 != "" {
					s.Fatal("Expected empty SN, got ", serialNumberB64)
				}
			} else {
				if serialNumberB64 == "" {
					s.Fatal("Expected non-empty SN, got empty")
				}
				// Software attestation serial number is 0.
				const softwareAttestationSnB64 = "AA"
				attestationIsReal := serialNumberB64 != softwareAttestationSnB64
				attestationShouldBeReal := tc.attestationType == webauthnpb.AttestationType_ENTERPRISE && tc.g2f
				if attestationShouldBeReal != attestationIsReal {
					s.Fatalf("Unexpected attestation realness: expected %v, got %v", attestationShouldBeReal, attestationIsReal)
				}
			}
			if err := u2fd.RemoteGetAssertionInLocalSite(ctx, cr, cred, powerButtonAuthCallback); err != nil {
				s.Fatal("Failed to perform GetAssertion flow: ", err)
			}
		})
		if !result {
			break
		}
	}
}
