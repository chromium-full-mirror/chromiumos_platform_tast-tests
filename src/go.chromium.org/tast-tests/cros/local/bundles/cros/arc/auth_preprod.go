// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/testenv"
	"go.chromium.org/tast-tests/cros/local/testenv/proxy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AuthPreprod,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test ARC authentication through OOBE, provision and app install against preprod envs of gaia and android auth server",
		Contacts:     []string{"arc-core@google.com", "jinrongwu@google.com"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		SoftwareDeps: []string{"chrome", "play_store", "gaia", "android_vm", "no_qemu"},
		Attr:         []string{"group:external-dependency"},
		Data:         []string{"gaia_sandbox_config.json"},
		SearchFlags: []*testing.StringPair{
			testenv.SearchFlag(testenv.AndroidAuthPreprod),
			testenv.SearchFlag(testenv.AndroidCheckinPreprod),
			testenv.SearchFlag(testenv.OAuthPreprod),
			testenv.SearchFlag(testenv.GAIASandbox),
		},
		Timeout: chrome.GAIALoginTimeout + arc.BootTimeout + 10*time.Minute,
		VarDeps: []string{ui.GaiaPoolDefaultVarName},
	})
}

func AuthPreprod(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, time.Second*10)
	defer cancel()

	bootParams := []string{
		"--params=androidboot.pause_provisioning=1", // to pause/resume provisioning
		// Pre-append the verifiedbootstate parameter while starting ARCVM.
		"^--params=androidboot.verifiedbootstate=orange", // to enable adb root on user image
	}

	if err := arc.WriteArcvmDevConf(ctx, strings.Join(bootParams, "\n")); err != nil {
		s.Fatal("Failed to set arcvm_dev.conf: ", err)
	}
	defer arc.RestoreArcvmDevConf(cleanupCtx)

	urlMap := map[string]string{
		`https://android.googleapis.com/auth`:                          `https://jmt17.google.com/canary/auth`,
		`https://android.googleapis.com/checkin`:                       `https://jmt17.google.com/canary/checkin`,
		`https://oauthtokenbootstrap.googleapis.com/v1/tokenbootstrap`: `https://staging-oauthtokenbootstrap.sandbox.googleapis.com/v1/tokenbootstrap`,
	}
	opts := []proxy.Option{
		proxy.URLRedirect(urlMap),
		proxy.CustomCA(true),
		proxy.DumpHTTPFlow(true),
		proxy.DumpFull(true),
	}

	mp, err := proxy.NewMitmProxy(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to create new proxy: ", err)
	}
	defer mp.Close(cleanupCtx)

	options := []chrome.Option{
		chrome.ARCSupported(),
		chrome.GAIALoginPool(dma.CredsFromPool(ui.GaiaPoolDefaultVarName)),
		chrome.ExtraArgs("--disable-features=OobeChoobe,OobeDisplaySize,OobeTouchpadScrollDirection"),
		chrome.ExtraArgs(fmt.Sprintf("--proxy-server=%s", mp.ProxyAddress())),
	}

	options = append(options, chrome.UseGaiaConfig(s.DataPath("gaia_sandbox_config.json")))

	cr, err := chrome.New(ctx, options...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	s.Log("Opting in")
	if err := optin.PerformNoWait(ctx, cr, tconn); err != nil {
		s.Fatal("Failed to optin: ", err)
	}

	s.Log("Waiting for ARC to boot")
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	s.Log("Inserting test certificate")
	path, _, err := mp.RootCertificate(ctx)
	if err != nil {
		s.Fatal("Failed to find root certificate: ", err)
	}
	if err := a.AddCaCert(ctx, path, proxy.CaHashcodeVar.Value()); err != nil {
		s.Fatal("Failed to insert test certificate: ", err)
	}

	s.Log("Resuming provisioning")
	if err := a.ResumeProvisioning(ctx); err != nil {
		s.Fatal("Failed to resume provisioning: ", err)
	}

	s.Log("Waiting for Play Store to show")
	if err := optin.WaitForPlayStoreShown(ctx, tconn, time.Minute); err != nil {
		s.Fatal("Failed to wait for Play Store: ", err)
	}

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed to create UIAutomator: ", err)
	}
	defer d.Close(cleanupCtx)

	s.Log("Installing app")
	if err := playstore.InstallApp(ctx, a, d, "org.telegram.messenger", &playstore.Options{TryLimit: -1, InstallationTimeout: 10 * time.Minute}); err != nil {
		s.Fatal("install the app: ", err)
	}
}
