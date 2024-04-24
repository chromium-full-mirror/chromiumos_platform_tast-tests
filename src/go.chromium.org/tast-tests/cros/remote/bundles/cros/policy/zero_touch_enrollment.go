// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/gaiaenrollment"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	"go.chromium.org/tast-tests/cros/services/cros/hwsec"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

const zeroTouchEnrollmentTimeout = 25 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         ZeroTouchEnrollment,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "ZTE Enroll a device without checking policies",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"vsavu@google.com",
		},
		BugComponent: "b:1111632",
		Attr:         []string{"group:dmserver-zteenrollment-daily"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.tape.Service", "tast.cros.hwsec.OwnershipService", "tast.cros.graphics.ScreenshotService"},
		Fixture:      fixture.CleanOwnership,
		Timeout:      30 * time.Minute,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Zero Touch Enrollment.
			Value: "screenplay-6f0905f0-9ecd-4974-b4a1-7e4b828b5dc2",
		}, {
			Key: "feature_id",
			// Enroll an unmanaged device that was pre-provisioned for ZTE to
			// ensure it enrolls without user interaction.
			// COM_FOUND_CUJ25_TASK4_WF1
			Value: "screenplay-cd82fc31-3640-4ccb-ba06-33ddffa54733",
		}},
		Params: []testing.Param{
			{
				Name: "autopush",
				Val: gaiaenrollment.TestParams{
					DMServer:             policy.DMServerAlphaURL,
					PoolID:               tape.ZTETestAutomation,
					SerialNumber:         "policy.ZeroTouchEnrollment.serial_number",
					HardwareModel:        "policy.ZeroTouchEnrollment.hardware_model",
					DeviceProvisionToken: "policy.ZeroTouchEnrollment.device_provision_token",
					CustomerID:           "policy.ZeroTouchEnrollment.customer_id",
					BatchKey:             "policy.ZeroTouchEnrollment.batch_key",
				},
			},
		},
		Vars: []string{
			"ui.signinProfileTestExtensionManifestKey",
			tape.ServiceAccountVar,
			"policy.ZeroTouchEnrollment.serial_number",
			"policy.ZeroTouchEnrollment.hardware_model",
			"policy.ZeroTouchEnrollment.device_provision_token",
			"policy.ZeroTouchEnrollment.customer_id",
			"policy.ZeroTouchEnrollment.batch_key",
		},
	})
}

func ZeroTouchEnrollment(ctx context.Context, s *testing.State) {
	param := s.Param().(gaiaenrollment.TestParams)
	dmServerURL := param.DMServer
	poolID := param.PoolID
	serialNumber := s.RequiredVar(param.SerialNumber)
	hardwareModel := s.RequiredVar(param.HardwareModel)
	deviceProvisionToken := s.RequiredVar(param.DeviceProvisionToken)
	customerID := s.RequiredVar(param.CustomerID)
	batchKey := s.RequiredVar(param.BatchKey)

	if err := preProvisionDevice(ctx, serialNumber, hardwareModel, deviceProvisionToken, customerID, batchKey); err != nil {
		s.Fatal("Failed to pre-provision device: ", err)
	}

	if err := setVpdValuesForInitialEnrollment(ctx, s.DUT().Conn()); err != nil {
		s.Fatal("Failed to get VPD ready for ZTE: ", err)
	}

	// Shorten deadline to leave time separately for logging and cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(cleanupCtx)

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	captureScreenshotOnError := func(ctx context.Context, hasError func() bool) {
		if !hasError() {
			return
		}

		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "enrollmentError"})
	}
	defer captureScreenshotOnError(cleanupCtx, s.HasError)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}
	timeout := int32(zeroTouchEnrollmentTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, false /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	// Deprovision the DUT at the end of the test. As devices might get
	// provisioned even when the enrollment fails we need to defer the
	// deprovisioning before enrolling.
	defer func(ctx context.Context) {
		if err := tapeClient.DeprovisionHelper(ctx, cl, acc.OrgUnitPath); err != nil {
			s.Fatal("Failed to deprovision device: ", err)
		}
	}(cleanupCtx)

	// It may take a while for our preprovisioning command to succeed, wait for a bit and then retry ZTE a few times.
	// GoBigSleepLint: Waiting a bit speeds up the test because provisioning takes time.
	testing.Sleep(ctx, time.Minute)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Give ZTE attempt 5 minutes to succeed, then retry.
		oobeCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		// Reconnect to the device because cleaning the TPM restarts Chrome.
		cl, err = rpc.Dial(oobeCtx, s.DUT(), s.RPCHint())
		if err != nil {
			return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
		}
		defer cl.Close(ctx)

		policyClient := ps.NewPolicyServiceClient(cl.Conn)

		if _, err := policyClient.ZeroTouchEnrollUsingChrome(oobeCtx, &ps.ZeroTouchEnrollUsingChromeRequest{
			DmserverURL: dmServerURL,
			ManifestKey: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		}); err != nil {
			// Clear TPM to reset any state left by the failed ZTE attempt.
			ownershipClient := hwsec.NewOwnershipServiceClient(cl.Conn)

			if _, err := ownershipClient.EnsureTPMAndSystemStateAreReset(ctx, &empty.Empty{}); err != nil {
				return errors.Wrap(err, "failed to reset the TPM locally")
			}

			return err
		}

		return nil
	}, &testing.PollOptions{Interval: 2 * time.Minute}); err != nil {
		s.Fatal("Failed to ZTE enroll using chrome: ", err)
	}
}

func setVpdValuesForInitialEnrollment(ctx context.Context, dutConn *ssh.Conn) error {
	// Delete check_enrollment from vpd if it exists.
	if err := dutConn.CommandContext(ctx, "vpd", "-i", "RW_VPD", "-g", "check_enrollment").Run(); err == nil {
		if err := dutConn.CommandContext(ctx, "vpd", "-i", "RW_VPD", "-d", "check_enrollment").Run(exec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to delete check_enrollment")
		}
	}

	// Setting the RLZ ping embargo end date to one month ago.
	currentTime := time.Now()
	last1Month := currentTime.AddDate(0, -1, 0)
	timeLayout := "2006-01-02"
	oneMonthAgoDate := last1Month.Format(timeLayout)
	oneMonthAgo := fmt.Sprintf("rlz_embargo_end_date=\"%s\"", oneMonthAgoDate)
	if err := dutConn.CommandContext(ctx, "vpd", "-i", "RW_VPD", "-s", oneMonthAgo).Run(exec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to set rlz date")
	}

	// Refresh dump_vpd_log because we don't reboot.
	if err := dutConn.CommandContext(ctx, "dump_vpd_log", "--force").Run(exec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to refresh dump_vpd_log")
	}

	return nil
}

func preProvisionDevice(ctx context.Context, serialNumber, hardwareModel, deviceProvisionToken, customerID, batchKey string) error {
	// Prepare and issue a request.
	bodyCommand := fmt.Sprintf("{\"requests\": [ {\"preProvisionedDevice\":{serialNumber:\"%s\",hardwareModel:\"%s\",devicePreProvisioningToken:\"%s\",attestedDeviceId:\"%s\",customer_id:\"%s\"}}]}", serialNumber, hardwareModel, deviceProvisionToken, serialNumber, customerID)
	urlWithBatchKey := fmt.Sprintf("https://chromecommercial.googleapis.com/v1/preProvisionedDevices:batchCreate?key=%s", batchKey)
	body := strings.NewReader(bodyCommand)
	req, err := http.NewRequest("POST", urlWithBatchKey, body)
	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return errors.Wrap(err, "failed to issue request")
	}
	defer resp.Body.Close()

	// Validate response.
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return errors.Wrap(err, "failed to read response")
	}
	type preProvisionResponse struct {
		PreProvisionedDevice interface{}
		Status               interface{}
	}
	type preProvisionResponseList struct {
		Responses []preProvisionResponse
	}
	var parsedResponse preProvisionResponseList
	if err := json.Unmarshal(respBytes, &parsedResponse); err != nil {
		return errors.Wrap(err, "failed to parse response")
	}
	if len(parsedResponse.Responses) != 1 ||
		parsedResponse.Responses[0].Status != nil ||
		parsedResponse.Responses[0].PreProvisionedDevice == nil {
		return errors.Errorf("unsuccessful response: %s", string(respBytes))
	}
	testing.ContextLog(ctx, "Succesfully pre-provisioned device")
	return nil
}
