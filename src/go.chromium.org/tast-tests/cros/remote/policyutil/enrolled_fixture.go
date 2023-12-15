// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/remote/sysutil"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const (
	enrollmentSetupTimeout = 15 * time.Minute
	enrollmentRunTimeout   = 8 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.Enrolled,
		Desc: "Fixture providing enrollment",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
		},
		Impl:            &enrolledFixt{},
		SetUpTimeout:    enrollmentSetupTimeout,
		TearDownTimeout: 5 * time.Minute,
		ResetTimeout:    15 * time.Second,
		PostTestTimeout: 15 * time.Second,
		ServiceDeps: []string{
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.graphics.ScreenshotService",
			"tast.cros.baserpc.FileSystem",
		},
	})
}

type enrolledFixt struct {
	fdmsDir   string
	rpcClient *rpc.Client
}

func dumpVPDContent(ctx context.Context, d *dut.DUT) ([]byte, error) {
	out, err := d.Conn().CommandContext(ctx, "vpd", "-i", "RW_VPD", "-l").Output(ssh.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run the vpd command")
	}

	return out, nil
}

func checkVPDState(ctx context.Context, d *dut.DUT) error {
	// https://chromeos.google.com/partner/dlm/docs/factory/vpd.html#required-rw-fields
	const requiredField = "gbind_attribute"

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return errors.New("no output directory")
	}

	if out, err := dumpVPDContent(ctx, d); err != nil {
		return err
	} else if !strings.Contains(string(out), requiredField) {
		// VPD is not running well, returning an error. Second run will confirm
		// whether the error is transitory.

		if err := ioutil.WriteFile(filepath.Join(outDir, "vpd-dump.txt"), out, 0644); err != nil {
			return errors.New("failed to dump VPD content")
		}

		out, err := dumpVPDContent(ctx, d)
		if err != nil {
			return errors.Wrap(err, "failed the second dump of the VPD")
		}
		if err := ioutil.WriteFile(filepath.Join(outDir, "vpd-dump-2.txt"), out, 0644); err != nil {
			return errors.New("failed to dump VPD content")
		}
		if strings.Contains(string(out), requiredField) {
			return errors.Errorf("VPD error, first run did not find %q, second run did", requiredField)
		}

		return errors.Errorf("VPD error, did not find the required field %q", requiredField)
	}

	return nil
}

func (e *enrolledFixt) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Make sure we have enough time left from the global timeout.
	// Deadline returns with the time remaining until the global timeout
	// if it is closer than the SetUp timeout.
	if deadline, ok := ctx.Deadline(); !ok {
		s.Fatal("Missing deadline for context: ", ctx)
	} else if diff := deadline.Sub(time.Now()); diff < (enrollmentSetupTimeout - time.Minute) {
		s.Fatalf("Not enough time until global timeout: have %s; need %s", diff, enrollmentSetupTimeout)
	}

	// Make sure the DUT is connected at the beginning.
	// TODO(b/239013478): Clean up the connection checks when the issue is resolved.
	if err := s.DUT().Health(ctx); err != nil {
		s.Log("Failed DUT connection check at the beginning: ", err)

		// Try to reconnect to the DUT.
		waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if err := s.DUT().WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to the DUT at the beginning: ", err)
		}
	}

	if err := checkVPDState(ctx, s.DUT()); err != nil {
		if isVM := sysutil.IsRunningOnVM(ctx, s.DUT()); isVM {
			// TODO(b/264435654): This fixture is best-effort and also requires
			// some manual setup on the VM. Add formal support for using the
			// fixture on VMs.
			s.Log("VPD is broken but running on a VM so trying to enroll anyway: ", err)
		} else {
			s.Fatal("VPD is broken: ", err)
		}
	}

	ok := false
	defer func() {
		if !ok {
			s.Log("Removing enrollment after failing SetUp")
			if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
				s.Fatal("Failed to reset TPM: ", err)
			}
		}
	}()

	// Collect errors of enrollment attempts and raise them even in case of a Fatal error.
	var errs []error
	defer func() {
		for _, err := range errs {
			if !ok {
				s.Error("Failed to enroll: ", err)
			} else {
				s.Log("Failed enrolling attempt: ", err)
			}
		}
	}()

	// TODO(b/243629567): Remove the retries when the fixture is stable enough.
	for tries := 1; tries < 4; tries++ {
		// Make sure we have enough time to perform enrollment.
		// This helps differentiate real issues from timeout hitting different components.
		if deadline, ok := ctx.Deadline(); !ok {
			s.Log("Missing deadline for context: ", ctx)
		} else if diff := deadline.Sub(time.Now()); diff < enrollmentRunTimeout {
			s.Logf("Not enough time to perform setup and enrollment: have %s; need %s", diff, enrollmentRunTimeout)
		}

		s.Logf("Attempting enrollment, try %d", tries)
		attemptDir := path.Join(s.OutDir(), fmt.Sprintf("Attempt_%d", tries))

		enrollCtx, cancel := context.WithTimeout(ctx, enrollmentRunTimeout)
		defer cancel()

		if err := EnsureTPMAndSystemStateAreReset(enrollCtx, s.DUT(), s.RPCHint()); err != nil {
			s.Fatal("Failed to reset TPM: ", err)
		}

		// Check connection state after reboot.
		if err := s.DUT().Health(ctx); err != nil {
			s.Fatal("Failed DUT connection check after reboot: ", err)
		}

		cl, err := rpc.Dial(enrollCtx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
		}
		defer cl.Close(enrollCtx)

		if fdmsDir, err := Enroll(enrollCtx, attemptDir, s.DUT(), cl, true); err != nil {
			s.Logf("Attempt %d failed: %v", tries, err)
			errs = append(errs, err)
		} else {
			// When the enrollment is successful, there is no need to retry again.
			s.Logf("Attempt %d succeded", tries)
			e.fdmsDir = fdmsDir
			ok = true
			break
		}
	}

	if ok {
		var err error
		e.rpcClient, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
		if err != nil {
			s.Error("Failed to connect to DUT: ", err)
			errs = append(errs, err)
		}
	}

	// Converting errors to strings as an error array cannot be Unmarshalled.
	var errorStrings []string
	for _, err := range errs {
		errorStrings = append(errorStrings, err.Error())
	}

	return &policy.EnrolledFixtureData{
		FakeDMSDirectory: e.fdmsDir,
		Errors:           errorStrings,
	}
}

func (e *enrolledFixt) TearDown(ctx context.Context, s *testing.FixtState) {
	defer e.rpcClient.Close(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if err := EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	pc := pspb.NewPolicyServiceClient(e.rpcClient.Conn)

	if _, err := pc.RemoveFakeDMSDir(ctx, &pspb.RemoveFakeDMSDirRequest{
		Path: e.fdmsDir,
	}); err != nil {
		s.Fatal("Failed to remove temporary directory for FakeDMS: ", err)
	}
}

// Check if device state has been lost.
func (e *enrolledFixt) Reset(ctx context.Context) error {
	return checkEnrollment(ctx, e.rpcClient)
}

func (*enrolledFixt) PreTest(ctx context.Context, s *testing.FixtTestState) {}

// Tests need to make sure not to clear enrollment. Report a test error if it happens.
func (e *enrolledFixt) PostTest(ctx context.Context, s *testing.FixtTestState) {
	if err := checkEnrollment(ctx, e.rpcClient); err != nil {
		s.Error("Enrollement lost: ", err)
	}
}

// Enroll enrolls the DUT under the managedchrome.com domain.
func Enroll(ctx context.Context, attemptDir string, dut *dut.DUT, rpc *rpc.Client, stopFdms bool) (fdmsDir string, retErr error) {
	// Reserve time for cleaning up and copying the logs from the DUT.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	ok := false
	defer func(ctx context.Context) {
		if !ok {
			screenshotService := graphics.NewScreenshotServiceClient(rpc.Conn)
			attemptNumber := path.Base(attemptDir)
			if _, err := screenshotService.CaptureScreenshot(ctx,
				&graphics.CaptureScreenshotRequest{FilePrefix: "enrollment" + attemptNumber},
			); err != nil {
				testing.ContextLog(ctx, "Failed to capture screenshot: ", err)
			}
		}
	}(cleanupCtx)

	policyClient := pspb.NewPolicyServiceClient(rpc.Conn)
	res, err := policyClient.CreateTempFakeDMSDir(ctx, &pspb.CreateTempFakeDMSDirRequest{
		Path: fakedms.EnrollmentFakeDMSDirRoot,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to create FakeDMS directory")
	}

	fdmsDir = res.Path

	defer func(ctx context.Context) {
		if !ok {
			if _, err := policyClient.RemoveFakeDMSDir(ctx, &pspb.RemoveFakeDMSDirRequest{
				Path: fdmsDir,
			}); err != nil {
				if retErr == nil {
					retErr = errors.Wrap(err, "failed to remove FakeDMS directory")
				} else {
					testing.ContextLog(ctx, "Failed to remove FakeDMS directory: ", err)
				}
			}
		}
	}(cleanupCtx)

	// Always dump the logs.
	defer func(ctx context.Context) {
		if err := os.Mkdir(attemptDir, 0777); err != nil {
			testing.ContextLog(ctx, "Failed to create attempt dir: ", err)
		}

		chromeDir := path.Join(attemptDir, "Chrome")
		if err := os.Mkdir(chromeDir, 0777); err != nil {
			testing.ContextLog(ctx, "Failed to create Chrome dir: ", err)
		}

		if err := linuxssh.GetFile(ctx, dut.Conn(), "/var/log/chrome/chrome", filepath.Join(chromeDir, "chrome.log"), linuxssh.DereferenceSymlinks); err != nil {
			testing.ContextLog(ctx, "Failed to dump Chrome log: ", err)
		}

		fdmsDirHost := path.Join(attemptDir, "FakeDMS")
		if err := os.Mkdir(fdmsDirHost, 0777); err != nil {
			testing.ContextLog(ctx, "Failed to create FakeDMS dir: ", err)
		}

		if err := linuxssh.GetFile(ctx, dut.Conn(), fdmsDir, fdmsDirHost, linuxssh.DereferenceSymlinks); err != nil {
			testing.ContextLog(ctx, "Failed to dump FakeDMS dir: ", err)
		}

		uiDir := "/var/log/ui"
		uiDirHost := path.Join(attemptDir, "ui")
		if err := os.Mkdir(uiDirHost, 0777); err != nil {
			testing.ContextLog(ctx, "Failed to create ui dir: ", err)
		}

		if err := linuxssh.GetFile(ctx, dut.Conn(), uiDir, uiDirHost, linuxssh.DereferenceSymlinks); err != nil {
			testing.ContextLog(ctx, "Failed to dump ui dir: ", err)
		}
	}(cleanupCtx)

	pJSON, err := json.Marshal(policy.NewBlob())
	if err != nil {
		return "", errors.Wrap(err, "failed to marshal policy blob")
	}

	if _, err := policyClient.EnrollUsingChrome(ctx, &pspb.EnrollUsingChromeRequest{
		PolicyJson: pJSON,
		FakedmsDir: fdmsDir,
		SkipLogin:  true,
	}); err != nil {
		return "", errors.Wrap(err, "failed to enroll using Chrome")
	}

	if stopFdms {
		if _, err := policyClient.StopChromeAndFakeDMS(ctx, &empty.Empty{}); err != nil {
			return "", errors.Wrap(err, "failed to stop Chrome and FakeDMS")
		}
	} else {
		if _, err := policyClient.StopChrome(ctx, &empty.Empty{}); err != nil {
			return "", errors.Wrap(err, "failed to stop Chrome")
		}
	}

	ok = true

	return fdmsDir, nil
}
