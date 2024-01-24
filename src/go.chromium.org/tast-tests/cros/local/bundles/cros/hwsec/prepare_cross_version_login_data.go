// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hwsec

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/hwsec/util"
	hwseclocal "go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/u2fd"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrepareCrossVersionLoginData,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Create snapshot of login-related data, which will be used in hwsec.CrossVersionLogin to mock the login data in older version (see go/cros-cross-version-login-testing)",
		Contacts: []string{
			"cros-hwsec@google.com",
			"chingkang@google.com",
		},
		Data: []string{
			"webauthn.html",
			"bundle.js",
		},
		BugComponent: "b:1188704",
		SoftwareDeps: []string{"chrome", "tpm2_simulator"},
	})
}

func PrepareCrossVersionLoginData(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cmdRunner := hwseclocal.NewCmdRunner()
	helper, err := hwseclocal.NewHelper(cmdRunner)
	if err != nil {
		s.Fatal("Failed to create hwsec local helper: ", err)
	}

	server := u2fd.NewWebAuthnHTTPServer(ctx, s.DataFileSystem())
	defer server.Close(cleanupCtx)

	// This test is only running manually to generate the login data for hwsec.CrossVersionLogin. The data would be uploaded by the scripts (See src/platform2/hwsec-host-utils/cross_version_login/prepare_cross_version_login_data.sh).
	// Therefore, the tmpDir would not be removed at the end of test because the data would be uploaded laterand then fetched when running hwsec.CrossVersionLogin.
	const tmpDir = "/tmp/cross_version_login"
	if err := os.MkdirAll(tmpDir, 0700); err != nil {
		s.Fatalf("Failed to create directory %q: %v", tmpDir, err)
	}

	dataPath := filepath.Join(tmpDir, "data.tar.gz")
	configPath := filepath.Join(tmpDir, "config.json")
	if err := util.PrepareCrossVersionLoginData(ctx, s.Logf, helper.CmdHelper, dataPath, configPath, server.URL+"/webauthn.html"); err != nil {
		s.Fatal("Failed to prepare cross-version login data: ", err)
	}
}
