// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package autoupdate

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/bundles/cros/autoupdate/util"
	"chromiumos/tast/remote/dutfs"
	"chromiumos/tast/remote/u2fd"
	"chromiumos/tast/remote/updateutil"
	"chromiumos/tast/rpc"
	webauthnpb "chromiumos/tast/services/cros/hwsec"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: NToMWebauthnLogin,
		// We already have lacros variant for normal WebAuthn tests, and cross-version functionality
		// is unrelated specific browser implementation.
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify cross version vault's compatibility",
		Contacts: []string{
			"cros-hwsec@google.com",
			"hcyang@google.com", // Test author
		},
		BugComponent: "b:1188704",
		Attr:         []string{"group:autoupdate"},
		SoftwareDeps: []string{"reboot", "chrome", "auto_update_stable"},
		ServiceDeps: []string{
			"tast.cros.browser.ChromeService",
			"tast.cros.hwsec.WebauthnService",
			"tast.cros.autoupdate.NebraskaService",
			"tast.cros.autoupdate.UpdateService",
			"tast.cros.baserpc.FileSystem",
		},
		Data: []string{
			"webauthn.html",
			"bundle.js",
		},
		Params: []testing.Param{{
			Name:              "tpm1",
			ExtraSoftwareDeps: []string{"tpm1"},
		}, {
			Name:              "gsc",
			ExtraSoftwareDeps: []string{"gsc"},
		}},
		Timeout: util.TotalTestTime,
		Fixture: fixture.Autoupdate,
	})
}

func copyFilesToRemote(ctx context.Context, s *testing.State, cl *dutfs.Client) (string, error) {
	return u2fd.CopyFilesToRemote(ctx, s.DUT(), cl, map[string]string{
		s.DataPath("webauthn.html"): "webauthn.html",
		s.DataPath("bundle.js"):     "bundle.js",
	})
}

func NToMWebauthnLogin(ctx context.Context, s *testing.State) {
	paygen := s.FixtValue().(updateutil.WithPaygen).Paygen()
	filtered := paygen.FilterChannel("stable").FilterDeltaTypes([]string{"OMAHA", "MILESTONE"})

	env, err := util.NewHwsecEnv(s.DUT())
	if err != nil {
		s.Fatal("Failed to create hwsec env: ", err)
	}

	var cred *webauthnpb.WebAuthnCredential
	ops := &util.Operations{
		PreUpdate: func(ctx context.Context) error {
			return util.ClearTpm(ctx, env)
		},
		PostUpdate: func(ctx context.Context) error {
			cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
			if err != nil {
				s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
			}
			defer cl.Close(ctx)
			dutfsClient := dutfs.NewClient(cl.Conn)
			dataPath, err := copyFilesToRemote(ctx, s, dutfsClient)
			if err != nil {
				s.Fatal("Failed to put files to remote")
			}
			cred, err = createUserAndMakeCredential(ctx, env, cl.Conn, dataPath)
			return err
		},
		PostRollback: func(ctx context.Context) error {
			cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
			if err != nil {
				s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
			}
			defer cl.Close(ctx)
			dutfsClient := dutfs.NewClient(cl.Conn)
			dataPath, err := copyFilesToRemote(ctx, s, dutfsClient)
			if err != nil {
				s.Fatal("Failed to put files to remote")
			}
			return loginUserAndGetAssertion(ctx, env, cl.Conn, dataPath, cred)
		},
	}

	if err := util.NToMTest(ctx, s.DUT(), s.OutDir(), s.RPCHint(), ops, filtered, 3 /*deltaM*/); err != nil {
		s.Fatal("Failed to run cross version test: ", err)
	}
}

func passwordAuth(ctx context.Context, client webauthnpb.WebauthnServiceClient) error {
	// Type password into ChromeOS WebAuthn dialog.
	if _, err := client.EnterPassword(ctx, &webauthnpb.EnterPasswordRequest{Password: util.ChromeDefaultPassword}); err != nil {
		return errors.Wrap(err, "failed to type password into ChromeOS auth dialog")
	}
	return nil
}

func createUserAndMakeCredential(ctx context.Context, env *util.HwsecEnv, conn *grpc.ClientConn, dataPath string) (*webauthnpb.WebAuthnCredential, error) {
	client := webauthnpb.NewWebauthnServiceClient(conn)

	// Login Chrome and create a WebAuthn credential.
	if _, err := client.New(ctx, &webauthnpb.NewRequest{
		BrowserType: webauthnpb.BrowserType_ASH,
		DataPath:    dataPath,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to start Chrome")
	}
	defer client.Close(ctx, &empty.Empty{})

	if _, err := client.StartWebauthn(ctx, &webauthnpb.StartWebauthnRequest{
		UserVerification:  webauthnpb.UserVerification_DISCOURAGED,
		AuthenticatorType: webauthnpb.AuthenticatorType_UNSPECIFIED,
		HasDialog:         true,
	}); err != nil {
		return nil, errors.Wrap(err, "failed to start WebAuthn flow")
	}

	authCallback := func(ctx context.Context) error {
		return passwordAuth(ctx, client)
	}
	cred, err := u2fd.RemoteMakeCredentialInLocalSite(ctx, client, authCallback)
	if err != nil {
		return nil, errors.Wrap(err, "failed to perform MakeCredential flow")
	}
	return cred, nil
}

func loginUserAndGetAssertion(ctx context.Context, env *util.HwsecEnv, conn *grpc.ClientConn, dataPath string, cred *webauthnpb.WebAuthnCredential) error {
	client := webauthnpb.NewWebauthnServiceClient(conn)

	// Login Chrome to see if we can still authenticate the relying party using the WebAuthn credential.
	if _, err := client.New(ctx, &webauthnpb.NewRequest{
		KeepState: true,
		DataPath:  dataPath,
	}); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer client.Close(ctx, &empty.Empty{})

	if _, err := client.StartWebauthn(ctx, &webauthnpb.StartWebauthnRequest{
		UserVerification:  webauthnpb.UserVerification_DISCOURAGED,
		AuthenticatorType: webauthnpb.AuthenticatorType_UNSPECIFIED,
		HasDialog:         true,
	}); err != nil {
		return errors.Wrap(err, "failed to start WebAuthn flow")
	}

	authCallback := func(ctx context.Context) error {
		return passwordAuth(ctx, client)
	}
	if err := u2fd.RemoteGetAssertionInLocalSite(ctx, client, cred, authCallback); err != nil {
		return errors.Wrap(err, "failed to perform GetAssertion flow")
	}
	return nil
}
