// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rollback

import (
	"context"
	"encoding/json"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/hwsec"
	types "go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/rollback"
	rpb "go.chromium.org/tast-tests/cros/services/cros/rollback"
	tape_service "go.chromium.org/tast-tests/cros/services/cros/tape"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

const enterpriseRollbackEnrolledTimeout = 12 * time.Minute
const networkID = "rollback-PSK-Wifi"

func init() {
	testing.AddTest(&testing.Test{
		Func:         EnterpriseRollbackEnrolled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that Enterprise Rollback recovers policy networks after re-enrollment",
		Contacts: []string{
			"mpolzer@google.com",
			"chromeos-commercial-remote-management@google.com",
		},
		BugComponent: "b:1031231",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps: []string{
			"tast.cros.tape.Service",
			"tast.cros.policy.PolicyService",
			"tast.cros.hwsec.OwnershipService",
			"tast.cros.rollback.EnterpriseRollbackService",
		},
		Timeout: enterpriseRollbackEnrolledTimeout,
		Vars: []string{
			tape.ServiceAccountVar,
		},
	})
}

// EnterpriseRollbackEnrolled runs a faked version of enterprise rollback while
// staying on the same image and re-enrolling automatically into GAIA.
// TODO(b:237527904): Move all service calls to rollback file to clean up test.
// TODO(b:237527904): Stop auto-reenrollment and verify networks are set after
// Enterprise Rollback.
func EnterpriseRollbackEnrolled(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()

	if err := rollback.ClearRollbackAndSystemData(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Error("Failed to clean rollback data before test: ", err)
	}

	rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer rpcClient.Close(cleanupCtx)

	if err = deprovision(ctx, s.DUT(), s.RPCHint(), s.RequiredVar(tape.ServiceAccountVar)); err != nil {
		s.Error("Failed to deprovision before test: ", err)
	}
	defer deprovision(cleanupCtx, s.DUT(), s.RPCHint(), s.RequiredVar(tape.ServiceAccountVar))

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(enterpriseRollbackEnrolledTimeout.Seconds())
	poolID := tape.DefaultManaged
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(poolID))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(cleanupCtx)

	networkPolicy := &tape.DetailsNetworksWifi{
		Details: tape.NetworkDetailsWifi{
			Ssid:                 "MyTapeNetwork",
			HiddenSsid:           false,
			AutomaticallyConnect: false,
			Security:             "WPA-PSK",
			Passphrase:           "VerySecure!",
		},
	}
	updateMask := []string{"details.ssid", "details.hiddenSsid", "details.automaticallyConnect", "details.security", "details.passphrase"}
	if err := tapeClient.SetPolicy(ctx, networkPolicy, updateMask, tape.NetworkKey{NetworkID: networkID}, acc.RequestID); err != nil {
		s.Fatal("Failed to set the network policy: ", err)
	}

	// Schedule cleanup. If all went well, the device is re-enrolled at this state.
	// We also need to be able to clean up if the device is not enrolled or in a
	// strange state.
	defer func(ctx context.Context) {
		if err := rollback.ClearRollbackAndSystemData(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to clean rollback data after test: ", err)
		}
	}(cleanupCtx)

	// We enroll using the rollback service instead of policyutil to be able to
	// verify the network API is retrieved and the policy networks are verified
	// successfully before we fake rollback. If network is not correctly verified
	// after faking rollback, there may be an error when re-configuring the
	// network policy. Ensure the account associated with the chrome connection
	// and logged in after rollback is the same one used when running this check.
	rollbackService := rpb.NewEnterpriseRollbackServiceClient(rpcClient.Conn)
	if _, err := rollbackService.Connect(ctx, &rpb.SessionState{
		Ownership: rpb.Ownership_ENROLLED_LOGGED_IN,
		LoginData: &rpb.LoginData{
			Username:    acc.Username,
			Password:    acc.Password,
			DmserverUrl: policy.DMServerProdURL,
		},
	}); err != nil {
		s.Fatal("Failed to enroll and connect to network API on the client: ", err)
	}
	defer rollbackService.CloseConnections(cleanupCtx, &empty.Empty{})

	if err := waitForNetwork(ctx, rollbackService, networkID, types.DevicePolicyOS); err != nil {
		s.Fatal("Network before Rollback check error: ", err)
	}

	tapeService := tape_service.NewServiceClient(rpcClient.Conn)

	ids, err := tapeService.GetDeviceID(ctx, &empty.Empty{})
	if err != nil {
		s.Error("Failed to get device id: ", err)
	}

	stableDeviceSecret, err := getStableDeviceSecret(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get stable device secret: ", err)
	}

	if err := tapeClient.StoreDeprovisioningIDs(ctx, ids.DeviceID, ids.CustomerID, stableDeviceSecret); err != nil {
		s.Error("Failed to store ids in tape: ", err)
	}

	_, err = rollback.SaveRollbackData(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to save rollback data: ", err)
	}

	// Ineffective reset is ok here as the device steps through oobe automatically
	// and takes ownership of the TPM.
	s.Log("Simulating powerwash and rebooting the DUT to fake rollback")
	if err := rollback.SimulatePowerwashAndReboot(ctx, s.DUT()); err != nil && !errors.Is(err, hwsec.ErrIneffectiveReset) {
		s.Fatal("Failed to simulate powerwash and reboot to fake an enterprise rollback: ", err)
	}

	rpcClient, err = rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer rpcClient.Close(cleanupCtx)

	// TODO(b:237527904) Figure out a way to stop OOBE here and ensure that our
	// network is present before re-enrolling.

	s.Log("Re-enrolling automatically using DMServer")
	rollbackService = rpb.NewEnterpriseRollbackServiceClient(rpcClient.Conn)
	if _, err := rollbackService.Connect(ctx, &rpb.SessionState{
		Ownership: rpb.Ownership_AUTO_ENROLLING,
		LoginData: &rpb.LoginData{
			Username:    acc.Username,
			Password:    acc.Password,
			DmserverUrl: policy.DMServerProdURL,
		},
	}); err != nil {
		s.Fatal("Failed to re-enroll and connect to network API on the client: ", err)
	}

	s.Log("Waiting for login screen during re-enrollment to log in")
	if _, err := rollbackService.Login(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Login failed; re-enrollment may have failed: ", err)
	}

	s.Log("Awaiting network check after Rollback")
	if err := waitForNetwork(ctx, rollbackService, networkID, types.DevicePolicyOS); err != nil {
		s.Fatal("Network check error: ", err)
	}
}

func waitForNetwork(ctx context.Context, rollbackService rpb.EnterpriseRollbackServiceClient, guid string, source types.OncSource) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		response, err := rollbackService.GetNetworkProperties(ctx, &rpb.GetNetworkPropertiesRequest{
			Guid: guid,
		})
		if err != nil {
			return errors.Wrap(err, "failed to get properties, maybe the network is not configured yet")
		}
		if response.Properties == nil {
			return errors.New("GetNetworkProperties returned nothing, that means network is not configured yet")
		}

		var properties types.ManagedProperties
		if err := json.Unmarshal(response.GetProperties().SerializedData, &properties); err != nil {
			return testing.PollBreak(errors.Wrap(err, "error while desearializing to make sure network is policy re-configured"))
		}

		if properties.Source != source {
			return errors.Errorf("network is configured, but not (yet) by %v, instead it is %v", source, properties.Source)
		}

		return nil
	}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 60 * time.Second}); err != nil {
		return errors.Wrap(err, "failed while waiting for managed network to be configured")
	}
	return nil
}

func deprovision(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint, serviceAccount string) (retErr error) {
	testing.ContextLog(ctx, "Deprovisioning")
	rpcClient, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect the rpc client")
	}
	defer rpcClient.Close(ctx)

	stableDeviceSecret, err := getStableDeviceSecret(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to get stable device secret")
	}

	tapeClient, err := tape.NewClient(ctx, []byte(serviceAccount))
	if err != nil {
		return errors.Wrap(err, "failed to start tape client")
	}

	provisioned, err := tapeClient.Provisioned(ctx, tape.WithStableDeviceSecret(stableDeviceSecret))
	if err != nil {
		return errors.Wrap(err, "failed to check if the device is provisioned")
	} else if provisioned {
		testing.ContextLog(ctx, "Device was provisioned before the test ran, probably because a previous test did not clean up correctly")
		if err := tapeClient.DeprovisionAndVerify(ctx, tape.WithStableDeviceSecret(stableDeviceSecret)); err != nil {
			return errors.Wrap(err, "failed to deprovision")
		}
	}

	return nil
}

func getStableDeviceSecret(ctx context.Context, dut *dut.DUT) (string, error) {
	const stableDeviceSecretFileName = "/sys/firmware/vpd/ro/stable_device_secret_DO_NOT_SHARE"
	data, err := linuxssh.ReadFile(ctx, dut.Conn(), stableDeviceSecretFileName)
	if err != nil {
		return "", errors.Wrapf(err, "failed to read %s", stableDeviceSecretFileName)
	}
	return string(data), nil
}
