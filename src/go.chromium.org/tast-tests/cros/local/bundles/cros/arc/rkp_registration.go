// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	testClassName     = "com.android.rkpdapp.RkpRegistrationCheck"
	hostnamePropName  = "remote_provisioning.hostname"
	hostnamePropValue = "remoteprovisioning.googleapis.com"
	successLogRegex   = "SUCCESS: Device key for 'default' is registered"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     RkpRegistration,
		Desc:     "Runs RkpRegistrationCheck.jar to ensure ARCVM can reach the Android Attestation Server",
		Contacts: []string{"arc-commercial@google.com", "batoon@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 60*time.Second,
		SoftwareDeps: []string{"android_vm_t", "chrome", "no_qemu"},
		VarDeps:      []string{},
		Params: []testing.Param{{
			ExtraData: []string{
				"RkpRegistrationCheck.jar",
			},
			Val: "RkpRegistrationCheck.jar",
		}},
	})
}

func RkpRegistration(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	testJarName := s.Param().(string)
	testJarPath := s.DataPath(testJarName)

	// Allow adb root on user builds. This needs to be done before starting ARCVM.
	if err := arc.WriteArcvmDevConf(ctx, "--params=androidboot.arc.allow_adb_root=1"); err != nil {
		s.Fatal("Failed to set arcvm_dev.conf: ", err)
	}
	defer arc.RestoreArcvmDevConf(cleanupCtx)

	cr, err := chrome.New(ctx,
		chrome.ARCEnabled(),
		chrome.UnRestrictARCCPU(),
		chrome.EnableFeatures("ArcAttestation"),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	s.Log("Restarting adbd as root")
	if err := a.Root(ctx); err != nil {
		s.Fatal("Failed to start adb root: ", err)
	}

	if err := a.Command(ctx, "setprop", hostnamePropName, hostnamePropValue).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to set remote_provisioning.hostname: ", err)
	}

	s.Log("Pushing RkpRegistrationCheck to Android")
	path, err := a.PushFileToTmpDir(ctx, testJarPath)
	if err != nil {
		s.Fatal("Failed to push test jar file to ARC: ", err)
	}
	defer a.Command(cleanupCtx, "rm", path).Run(testexec.DumpLogOnError)

	if err = a.Command(ctx, "chmod", "0755", path).Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to change the permission of %s: %v", path, err)
	}

	s.Log("Executing RkpRegistrationCheck")
	result, err := a.ShellCommand(ctx, "CLASSPATH="+path,
		"exec", "app_process", "/system/bin", testClassName).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to execute RkpRegistrationCheck: ", err)
	}

	s.Log("RkpRegistrationCheck output:\n" + string(result))

	// This test will only pass on lab devices.
	// It is expected to fail for DUTs used during development.
	re := regexp.MustCompile(successLogRegex)
	regexMatch := re.FindStringSubmatch(string(result))
	if regexMatch == nil {
		s.Fatal("Failed to find success log in output of RkpRegistrationCheck")
	}
}
