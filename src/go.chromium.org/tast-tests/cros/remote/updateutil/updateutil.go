// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package updateutil

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes"
	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/chromiumos/config/go/api/test/tls"
	"go.chromium.org/chromiumos/config/go/api/test/tls/dependencies/longrunning"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/tast-tests/cros/common/testexec"
	aupb "go.chromium.org/tast-tests/cros/services/cros/autoupdate"
	"go.chromium.org/tast-tests/cros/services/cros/baserpc"
	"go.chromium.org/tast-tests/cros/services/cros/nebraska"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// tlwAddress is used to connect to the Test Lab Wiring,
// which is used for the communication with the image caching service.
var tlwAddress = testing.RegisterVarString(
	"updateutil.tlwAddress",
	"10.254.254.254:7151",
	"The address {host:port} of the TLW service",
)

// ReadBootID reads back the current boot_id from the DUT.
func ReadBootID(ctx context.Context, dut *dut.DUT) (string, error) {
	out, err := dut.Conn().CommandContext(ctx, "cat", "/proc/sys/kernel/random/boot_id").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// ApplyDeferredUpdate applies the deferred update, reboot, and wait for the DUT becomes reachable again.
func ApplyDeferredUpdate(ctx context.Context, dut *dut.DUT) error {
	bootID, err := ReadBootID(ctx, dut)
	if err != nil {
		return errors.Wrap(err, "failed to read the boot_id before applying the update")
	}

	// Call update_engine DBus method to apply the update and reboot.
	// The command is non-blocking, so need to wait for the operations to complete afterward.
	if err := dut.Conn().CommandContext(ctx, "update_engine_client", "--apply_deferred_update").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to apply deferred update")
	}

	// Wait for reboot and boot_id change.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		if err := dut.WaitConnect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to DUT")
		}
		id, err := ReadBootID(ctx, dut)
		if err != nil {
			return errors.Wrap(err, "failed to read boot_id")
		}
		if id == bootID {
			return errors.New("boot_id did not change")
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute * 2, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to reboot")
	}
	return nil
}

// EnsureUpdateStatusIdle ensures update engine is running and its status is
// idle.
func EnsureUpdateStatusIdle(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) error {
	cl, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	updateClient := aupb.NewUpdateServiceClient(cl.Conn)
	if _, err := updateClient.EnsureUpdateEngineReady(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "update engine is not idle")
	}

	return nil
}

// ResetUpdateStatus resets update engine to ensure it is left in a clean state.
// Must be called after requesting an update.
func ResetUpdateStatus(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) error {
	cl, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	updateClient := aupb.NewUpdateServiceClient(cl.Conn)
	if _, err := updateClient.ResetUpdateEngine(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to reset update engine")
	}

	return nil
}

// UpdateFromGS updates the DUT to an image found in the Google Storage under the builder path folder.
// It saves the logs (udpdate engine logs and Nebraska logs) to the given outdir.
func UpdateFromGS(ctx context.Context, dut *dut.DUT, outdir string, rpcHint *testing.RPCHint, builderPath string) (retErr error) {
	up := func(ctx context.Context, conn *grpc.ClientConn, req *aupb.UpdateRequest) error {
		updateClient := aupb.NewUpdateServiceClient(conn)
		if _, err := updateClient.CheckForUpdate(ctx, req); err != nil {
			return errors.Wrap(err, "failed to update")
		}
		return nil
	}
	return updateFromGSInternal(ctx, dut, outdir, rpcHint, builderPath, up)
}

// PeriodicUpdateFromGS triggers periodic update check to update the DUT to an image found in the Google Storage under the builder path folder.
// If `targetStatus` passed in a non-empty value, verify the update ended with this status.
func PeriodicUpdateFromGS(ctx context.Context, dut *dut.DUT, outdir string, rpcHint *testing.RPCHint, builderPath, targetStatus string) (retErr error) {
	up := func(ctx context.Context, conn *grpc.ClientConn, req *aupb.UpdateRequest) error {
		updateClient := aupb.NewUpdateServiceClient(conn)

		lsbContent, err := json.Marshal(map[string]string{"CHROMEOS_AUSERVER": req.OmahaUrl})
		if err != nil {
			return errors.Wrap(err, "failed to create custom lsb_release")
		}
		updateClient.OverwriteStatefulLSBRelease(ctx, &aupb.LSBRelease{ContentJson: lsbContent})
		// Clear stateful lsb_release after finish.
		defer updateClient.OverwriteStatefulLSBRelease(ctx, &aupb.LSBRelease{ContentJson: []byte("{}")})

		status, err := updateClient.PeriodicCheckForUpdate(ctx, &empty.Empty{})
		if err != nil {
			return errors.Wrap(err, "failed to do periodic update")
		}

		if targetStatus != "" && status.CurrentOperation != targetStatus {
			return errors.Errorf("update ended with unexpected status: %s", status.CurrentOperation)
		}
		return nil
	}
	return updateFromGSInternal(ctx, dut, outdir, rpcHint, builderPath, up)
}

// ConfigureNebraskaFromGS creates a Nebraska client and configures it to serve
// an image indicated by the `builderPath`.
// Returns a client and a port at which Nebraska runs.
func ConfigureNebraskaFromGS(ctx context.Context, conn *grpc.ClientConn, dut *dut.DUT, outdir, builderPath string) (nebraskaCl nebraska.ServiceClient, port int32, retErr error) {
	cleanupCtx := ctx
	setupCtx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	// Limit the timeout for caching the update files.
	cachingCtx, cancel := context.WithTimeout(setupCtx, 3*time.Minute)
	defer cancel()

	gsPathPrefix := fmt.Sprintf("gs://chromeos-image-archive/%s", builderPath)

	// Cache the selected update image in a server accessible by the DUT.
	// The update images are stored in a GS bucket which requires corp access.
	url, err := cacheForDUT(cachingCtx, dut, tlwAddress.Value(), gsPathPrefix)
	if err != nil {
		return nil, 0, errors.Wrap(err, "unexpected error when caching file")
	}

	// Limit the timeout for preparation steps before the update.
	preparationCtx, cancel := context.WithTimeout(setupCtx, time.Minute)
	defer cancel()

	// Find the metadata file in the GS bucket, as we need the full filename to download it from the caching server.
	out, err := testexec.CommandContext(preparationCtx, "gsutil", "ls", gsPathPrefix+"/chromeos_*_full_dev*bin.json").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to list files in the GS bucket")
	}

	paths := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(paths) > 1 {
		// A longer context is used here, otherwise it won't work in case of a timeout.
		if err := bucketContentToFile(cachingCtx, gsPathPrefix, filepath.Join(outdir, "gs_bucket_content.txt")); err != nil {
			testing.ContextLog(cachingCtx, "Could not save GS bucket content: ", err)
		}

		return nil, 0, errors.Errorf("unexpected number of files; got %d, want 1", len(paths))
	}
	metadataFilename := filepath.Base(paths[0])

	nebraskaClient := nebraska.NewServiceClient(conn)
	startResponse, err := nebraskaClient.Start(preparationCtx, &nebraska.StartRequest{})
	if err != nil {
		return nil, 0, errors.Wrap(err, "failed to start Nebraska")
	}
	defer func(ctx context.Context) {
		if retErr != nil {
			_, err := nebraskaClient.Stop(ctx, &empty.Empty{})
			retErr = errors.Join(retErr, err)
		}
	}(cleanupCtx)

	args := []string{
		"--tries=1",
		"--connect-timeout=20",
		"-P", startResponse.RuntimeRoot, // download folder
		url + "/" + metadataFilename, // payload metadata address
	}

	// Download the payload metadata from the caching server.
	testing.ContextLogf(preparationCtx, "Downloading payload metadata %q", metadataFilename)
	if _, err := dut.Conn().CommandContext(preparationCtx, "wget", args...).CombinedOutput(testexec.DumpLogOnError); err != nil {
		// List files to see if it contains the file we wanted to download.
		// We are saving the whole list in case the file name pattern is changed.
		// A longer context is used here, otherwise it won't work in case of a timeout.
		if err := bucketContentToFile(cachingCtx, gsPathPrefix, filepath.Join(outdir, "gs_bucket_content.txt")); err != nil {
			testing.ContextLog(cachingCtx, "Could not save GS bucket content: ", err)
		}

		return nil, 0, errors.Wrap(err, "failed to download payload metadata")
	}

	if _, err = nebraskaClient.UpdatePayload(ctx, &nebraska.UpdatePayloadRequest{Update: &nebraska.Payload{
		Address:        url,
		MetadataFolder: startResponse.RuntimeRoot}}); err != nil {
		return nil, 0, errors.Wrap(err, "failed to stage payload in Nebraska")
	}

	return nebraskaClient, startResponse.Port, nil
}

type updateFunc func(ctx context.Context, conn *grpc.ClientConn, rec *aupb.UpdateRequest) error

// updateFromGSInternal sets up Nebraska and starts the update.
func updateFromGSInternal(ctx context.Context, dut *dut.DUT, outdir string, rpcHint *testing.RPCHint, builderPath string, doUpdate updateFunc) (retErr error) {
	// Limit the timeout for the update.
	updateCtx, cancel := context.WithTimeout(ctx, UpdateTimeout)
	defer cancel()

	// Reserve cleanup time for copying the logs from the DUT.
	cleanupCtx := updateCtx
	updateCtx, cancel = ctxutil.Shorten(updateCtx, 2*time.Minute)
	defer cancel()

	// Limit the timeout for preparation step before the update.
	preparationCtx, cancel := context.WithTimeout(updateCtx, time.Minute)
	defer cancel()

	// Connect to DUT.
	cl, err := rpc.Dial(preparationCtx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(cleanupCtx)

	nebraskaClient, nebraskaPort, err := ConfigureNebraskaFromGS(updateCtx, cl.Conn, dut, outdir, builderPath)
	if err != nil {
		return errors.Wrap(err, "failed to start Nebraska")
	}
	defer func(ctx context.Context) {
		_, err := nebraskaClient.Stop(ctx, &empty.Empty{})
		retErr = errors.Join(retErr, err)
	}(cleanupCtx)

	// Get the update log files even if the update fails.
	defer func(ctx context.Context) {
		if err := linuxssh.GetFile(ctx, dut.Conn(), "/var/log/update_engine.log", filepath.Join(outdir, "update_engine.log"), linuxssh.DereferenceSymlinks); err != nil {
			testing.ContextLog(ctx, "Failed to save update engine log: ", err)
		}
	}(cleanupCtx)

	// Trigger the update and wait for the results.
	return doUpdate(updateCtx, cl.Conn, &aupb.UpdateRequest{
		OmahaUrl: fmt.Sprintf("http://127.0.0.1:%d/update?critical_update=True", nebraskaPort),
	})
}

// cacheForDUT caches the required update files in a caching server which is available from the DUT.
// The required files include the update payload and the payload metadata.
func cacheForDUT(ctx context.Context, dut *dut.DUT, TLWAddress, gsPathPrefix string) (string, error) {
	conn, err := grpc.Dial(TLWAddress, grpc.WithInsecure())
	if err != nil {
		return "", err
	}
	defer conn.Close()

	client := tls.NewWiringClient(conn)

	host, _, err := net.SplitHostPort(dut.HostName())
	if err != nil {
		return "", errors.Wrapf(err, "failed to extract DUT hostname from %q", dut.HostName())
	}

	operation, err := client.CacheForDut(ctx, &tls.CacheForDutRequest{
		Url:     gsPathPrefix,
		DutName: host,
	})
	if err != nil {
		return "", errors.Wrap(err, "failed to start CacheForDut request; did you forget to start the TLW service?")
	}

	// Wait until the long running operation of caching is completed.
	operation, err = lroWait(ctx, longrunning.NewOperationsClient(conn), operation.Name)
	if err != nil {
		return "", errors.Wrap(err, "failed to wait for CacheForDut")
	}

	if status := operation.GetError(); status != nil {
		return "", errors.Errorf("failed to get CacheForDut, %s", status)
	}

	respAny := operation.GetResponse()
	if respAny == nil {
		return "", errors.Errorf("failed to get CacheForDut response for URL=%s and Name=%s", gsPathPrefix, dut.HostName())
	}

	resp := &tls.CacheForDutResponse{}
	if err := ptypes.UnmarshalAny(respAny, resp); err != nil {
		return "", errors.Errorf("unexpected response from CacheForDut, %v", respAny)
	}

	testing.ContextLogf(ctx, "The cache URL for %q is %q", gsPathPrefix, resp.GetUrl())
	return resp.GetUrl(), nil
}

// lroWait waits until the long-running operation specified by the provided operation name is done.
// If the operation is already done, it returns immediately.
// Unlike OperationsClient's WaitOperation(), it only returns on context
// timeout or completion of the operation.
// lroWait is duplicate of the function from an infra package, which is not available in Tast.
// It uses different types than the one at src/platform/dev/src/chromiumos/lro/wait.go.
func lroWait(ctx context.Context, client longrunning.OperationsClient, name string, opts ...grpc.CallOption) (*longrunning.Operation, error) {
	// Exponential backoff is used for retryable gRPC errors. In future, we
	// may want to make these parameters configurable.
	const initialBackoffMillis = 1000
	const maxAttempts = 4
	attempt := 0

	// WaitOperation() can return before the provided timeout even though the
	// underlying operation is in progress. It may also fail for retryable
	// reasons. Thus, we must loop until timeout ourselves.
	for {
		// WaitOperation respects timeout in the RPC Context as well as through
		// an explicit field in WaitOperationRequest. We depend on Context
		// cancellation for timeouts (like everywhere else in this codebase).
		// On timeout, WaitOperation() will return an appropriate error
		// response.
		op, err := client.WaitOperation(ctx, &longrunning.WaitOperationRequest{
			Name: name,
		}, opts...)
		switch status.Code(err) {
		case codes.OK:
			attempt = 0
		case codes.Unavailable, codes.ResourceExhausted:
			// Retryable error; retry with exponential backoff.
			if attempt >= maxAttempts {
				return op, err
			}
			delay := rand.Int63n(initialBackoffMillis * (1 << attempt))
			testing.Sleep(ctx, time.Duration(delay)*time.Millisecond) // GoBigSleepLint waiting for resource that is expensive to poll.
			attempt++
		default:
			// Non-retryable error
			return op, err
		}
		if op.Done {
			return op, nil
		}
	}
}

// bucketContentToFile copies the content list of a given GS bucket folder into a logfile.
func bucketContentToFile(ctx context.Context, gsFolder, logPath string) error {
	out, err := testexec.CommandContext(ctx, "gsutil", "ls", gsFolder).Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to list files in the GS bucket")
	}

	if err := ioutil.WriteFile(logPath, out, 0644); err != nil {
		return errors.Wrapf(err, "failed to write GS bucket content content to %s", logPath)
	}

	testing.ContextLogf(ctx, "The content of the %q folder was copied to %s", gsFolder, logPath)

	return nil
}

// VerifyInvalidatedUpdate verifies that an update is invalidated by checking
// that the root and the firmware partitions are reset to pre-update states.
// Also verifies that the appropriate firmware flags are correctly reset.
func VerifyInvalidatedUpdate(ctx context.Context, dut *dut.DUT, preUpdateFwAct, preUpdateRootSlot string) error {
	if err := VerifyInvalidatedFirmwareUpdate(ctx, dut, preUpdateFwAct); err != nil {
		return err
	}

	// Verify that the root partition is properly reset.
	rootdev, err := dut.Conn().CommandContext(ctx, "rootdev", "-s").Output()
	if err != nil {
		return err
	}
	if string(rootdev) != preUpdateRootSlot {
		return errors.New("Root partition is not invalidated")
	}

	return nil
}

// VerifyUpdateInvalidationPostReboot checks that a device still in a valid state
// after the invalidation of an update and after reboot.
// Checks that the update engine is in the IDLE state, and verifies
// that powerwash does not happen after reboot by checking the existence of
// the install attributes file.
// Clients are expected to reboot a device before calling this method.
func VerifyUpdateInvalidationPostReboot(ctx context.Context, dut *dut.DUT, rpcHint *testing.RPCHint) error {
	// Check the status of the update engine.
	if err := EnsureUpdateStatusIdle(ctx, dut, rpcHint); err != nil {
		return errors.Wrap(err, "failed to check the update engine state")
	}

	// Check if the install attributes file still exists.
	cl, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	const installAttributesPath = "/home/.shadow/install_attributes.pb"
	fs := baserpc.NewFileSystemClient(cl.Conn)
	if _, err := fs.Stat(ctx, &baserpc.StatRequest{Name: installAttributesPath}); err != nil {
		return errors.Wrap(err, "failed to read install attribute file")
	}

	return nil
}
