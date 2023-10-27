// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	localTcpdump "go.chromium.org/tast-tests/cros/local/network/tcpdump"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           StressDisconnectConnect,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that host has network connectivity via cellular interface",
		Contacts:       []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_run_isolated"},
		Fixture:        "cellular",
		Timeout:        10 * time.Minute,
		VarDeps:        []string{"cellular.gaiaAccountPool"},
		SoftwareDeps:   []string{"chrome"},
	})
}

const (
	tcpdumpStderrFile = "pcap-cellular.stderr"
	tcpdumpStdoutFile = "pcap-cellular.stdout"
	tcpdumpFile       = "pcap-cellular.pcap"
	testIPv6URL       = "https://test-ipv6.com"
)

func StressDisconnectConnect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	helper := s.FixtValue().(*cellular.FixtData).Helper

	gaiaCreds, err := credconfig.PickRandomCreds(s.RequiredVar("cellular.gaiaAccountPool"))
	if err != nil {
		s.Fatal("Failed to parse cellular user creds: ", err)
	}
	uiHelper, err := cellular.NewUIHelper(ctx, gaiaCreds.User, gaiaCreds.Pass)
	if err != nil {
		faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, uiHelper.Tconn)
		s.Fatal("Failed to create cellular.NewUiHelper: ", err)
	}
	defer upstart.RestartJob(cleanupCtx, "ui")

	dumpSocketStats := func(ctx context.Context) {
		out, _ := testexec.CommandContext(ctx, "/bin/bash", "-c", "/sbin/ss -utp | /bin/grep chrome").Output(testexec.DumpLogOnError)
		s.Log("ss: ", string(out))
	}

	prepareDirFile := func(ctx context.Context, filename string) (*os.File, error) {
		if err := os.MkdirAll(path.Dir(filename), 0755); err != nil {
			return nil, errors.Wrapf(err, "failed to create basedir for %q", filename)
		}
		f, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE, 0644)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot open file %q", filename)
		}
		return f, nil
	}

	startCapture := func(ctx context.Context) error {
		runner := localTcpdump.NewLocalRunner()
		outDir, ok := testing.ContextOutDir(ctx)
		if !ok {
			return errors.New("outdir not found")
		}
		stdoutFile, err := prepareDirFile(ctx, path.Join(outDir, tcpdumpStdoutFile))
		if err != nil {
			return errors.Wrap(err, "failed to open stdout log of tcpdump")
		}
		stderrFile, err := prepareDirFile(ctx, path.Join(outDir, tcpdumpStderrFile))
		if err != nil {
			return errors.Wrap(err, "failed to open stderr log of tcpdump")
		}

		if err := runner.StartTcpdump(ctx, "any", path.Join(outDir, tcpdumpFile), stdoutFile, stderrFile); err != nil {
			return errors.Wrap(err, "failed to start tcpdump")
		}

		cleanupCtx := ctx
		ctx, cancel := runner.ReserveForClose(ctx)
		defer cancel()
		defer func(cleanupCtx context.Context) {
			runner.Close(cleanupCtx)
		}(cleanupCtx)
		return nil
	}

	openWebPage := func(ctx context.Context, url string) (*chrome.Conn, error) {
		conn, err := uiHelper.UIHandler.NewChromeTab(ctx, uiHelper.Cr.Browser(), url, false)
		if err != nil {
			return nil, errors.Wrap(err, "failed to open web page")
		}
		defer func() {
			if err != nil {
				conn.CloseTarget(ctx)
				conn.Close()
			}
		}()
		if err := webutil.WaitForRender(ctx, conn, 1*time.Minute); err != nil {
			return nil, errors.Wrap(err, "failed to wait for render to finish")
		}

		if err := webutil.WaitForQuiescence(ctx, conn, 1*time.Minute); err != nil {
			return nil, errors.Wrap(err, "failed to wait for web page to finish loading")
		}
		return conn, nil
	}

	stressTestDisconnectConnect := func(ctx context.Context) error {
		if err := startCapture(ctx); err != nil {
			return err
		}
		defer dumpSocketStats(ctx)
		for i := 1; i < 6; i++ {
			s.Logf("Test loop: %d", i)
			conn, err := openWebPage(ctx, testIPv6URL)
			if err != nil {
				return errors.Wrapf(err, "failed to open: %s", testIPv6URL)
			}
			defer conn.Close()
			defer conn.CloseTarget(ctx)
			ipv4, ipv6, err := helper.GetNetworkProvisionedCellularIPTypes(ctx)
			if err != nil {
				s.Fatal("Failed to read APN info: ", err)
			}
			s.Log("ipv4: ", ipv4, " ipv6: ", ipv6)
			if err := cellular.VerifyIPConnectivityUsingCurl(ctx, testexec.CommandContext, ipv4, ipv6, "/usr/bin"); err != nil {
				return errors.Wrap(err, "failed connectivity test")
			}
			s.Log("Disconnect")
			if _, err := helper.Disconnect(ctx); err != nil {
				return errors.Wrap(err, "failed to disable modem")
			}
			dumpSocketStats(ctx)
			s.Log("Connect")
			// Enable and get service to set autoconnect based on test parameters.
			if _, err := helper.Connect(ctx); err != nil {
				return errors.Wrap(err, "failed to enable modem")
			}
		}
		return nil
	}

	for i := 0; i < 2; i++ {
		if err := helper.RunTestOnCellularInterface(ctx, stressTestDisconnectConnect); err != nil {
			s.Fatal("Failed to run test on cellular interface: ", err)
		}
		// GoBigSleepLint: A short sleep before we disable ethernet and wifi again.
		if err := testing.Sleep(ctx, 10*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
	}
}
