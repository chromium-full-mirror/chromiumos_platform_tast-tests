// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakedms implements a library for setting policies via a locally-hosted
// Device Management Server.
package fakedms

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"time"

	empb "chromiumos/policy/chromium/policy/enterprise_management_proto"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/caller"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/grpc"
)

// LogFile is the name of the log file for FakeDMS.
const LogFile = "fakedms.log"

// PolicyFile is the name of the config file for FakeDMS.
const PolicyFile = "policy.json"

// StateFile is the name of the state file for FakeDMS.
const StateFile = "state.json"

// GRPCUnixSocketURI is the path of the unix socket to communicate with the gRPC server in the FakeDMS.
const GRPCUnixSocketURI = "fake_dmserver_grpc.sock"

// EnrollmentFakeDMSDir is the directory where FakeDMS stores state during enrollment.
// Used to share state between the enrolled fixture and the fakeDMSEnrolled fixtures.
// TODO(crbug.com/1187473): Remove
const EnrollmentFakeDMSDir = "/var/enrolling-fdms"

// fakeDMServerPath is the path where the executable binary of the fake_dmserver is located.
var fakeDMServerPath = "/usr/local/libexec/chrome-binary-tests/fake_dmserver"

// Regular expression to match any characters in the policy selector that must
// be sanitized prior to this selector being as as part of the file name.
var selectorSanitizeRE = regexp.MustCompile("[^A-Za-z0-9.@-]")

// A FakeDMS struct contains information about a running policy_testserver instance.
type FakeDMS struct {
	cmd              *testexec.Cmd // fakedms process
	URL              string        // fakedms url; needs to be passed to Chrome; set in start()
	done             chan struct{} // channel that is closed when Wait() completes
	policyPath       string        // where policies are written for server to read
	grpcServerSocket string        // the unix socket to connect the client in tests with the grpc server in fake_dmserver

	remoteCommandsServiceClient     empb.RemoteCommandsServiceClient // the grpc client for remote commands
	clientConn                      *grpc.ClientConn                 // the connection to the grpc server
	internalRemoteCommandID         int64                            // internal incremental id to for remote commands sent to the fake_dmserver.
	persistentPolicies              []policy.Policy                  // policies that are always set
	persistentPublicAccountPolicies map[string][]policy.Policy       // public account policies that are always set
	persistentPolicyUser            *string                          // policyUser that is always set, nil if not used
	// persistentDisableDomainVerification, when set to true, allows the server to
	// return a valid signature for any domain in field
	// policy_response.responses.new_public_key_verification_signature_deprecated;
	// when set to nil or false, only a limited set of test domains are
	// signed such as managedchrome.com, example.com and chromepolicytest.com
	persistentDisableDomainVerification *bool
}

// HasFakeDMS is an interface for fixture values that contain a FakeDMS instance. It allows
// retrieval of the underlying FakeDMS object.
type HasFakeDMS interface {
	FakeDMS() *FakeDMS
}

// FakeDMS retrieves the underlying FakeDMS object.
func (fdms *FakeDMS) FakeDMS() *FakeDMS {
	return fdms
}

// New creates and starts a fake Domain Management Server to serve policies.
// outDir is used to write logs and policies, and should either be in a
// temporary location (and deleted by caller) or in the test's results directory.
func New(ctx context.Context, outDir string) (*FakeDMS, error) {
	policyPath := filepath.Join(outDir, PolicyFile)
	logPath := filepath.Join(outDir, LogFile)
	statePath := filepath.Join(outDir, StateFile)
	grpcUnixSocketURI := "unix://" + filepath.Join(outDir, GRPCUnixSocketURI)

	fr, fw, err := os.Pipe()
	if err != nil {
		return nil, errors.Wrap(err, "cannot create startup-pipe file")
	}
	defer func() {
		if err := fr.Close(); err != nil {
			testing.ContextLog(ctx, "Could not close startup-pipe read file: ", err)
		}
		if err := fw.Close(); err != nil {
			testing.ContextLog(ctx, "Could not close startup-pipe write file: ", err)
		}
	}()

	args := []string{
		fmt.Sprintf("--policy-blob-path=%s", policyPath),
		fmt.Sprintf("--log-path=%s", logPath),
		fmt.Sprintf("--client-state-path=%s", statePath),
		fmt.Sprintf("--grpc-unix-socket-uri=%s", grpcUnixSocketURI),
		// cmd.ExtraFiles (set below) assigns element i to file descriptor 3+i.
		// See exec.Cmd for more info.
		"--startup-pipe=3",
		"--min-log-level=0",
	}

	cmd := testexec.CommandContext(ctx, fakeDMServerPath, args...)

	cmd.ExtraFiles = []*os.File{fw}

	fdms := &FakeDMS{
		cmd:              cmd,
		done:             make(chan struct{}, 1),
		policyPath:       policyPath,
		grpcServerSocket: grpcUnixSocketURI,
	}

	if err = fdms.start(ctx, fr); err != nil {
		return nil, err
	}

	// Create a connection to the gRPC server in the fake_dmserver.
	conn, err := grpc.Dial(fdms.grpcServerSocket, grpc.WithInsecure())
	if err != nil {
		return nil, errors.Wrap(err, "couldn't dial the gRPC server in the fake_dmserver")
	}
	fdms.clientConn = conn
	// Create a gRPC remoteCommandsServiceClient to communicate directly with the fake_dmserver.
	fdms.remoteCommandsServiceClient = empb.NewRemoteCommandsServiceClient(conn)
	fdms.internalRemoteCommandID = 1
	return fdms, nil
}

// start runs the FakeDMS and verifies that it is alive.
// p is a pipe reader created and passed in by New().
func (fdms *FakeDMS) start(ctx context.Context, p *os.File) error {
	if err := fdms.cmd.Start(); err != nil {
		return errors.Wrap(err, "FakeDMS start command failed")
	}

	go func() {
		if err := fdms.cmd.Wait(testexec.DumpLogOnError); err != nil {
			testing.ContextLog(ctx, "FakeDMS server stopped unexpectedly: ", err)
		}
		close(fdms.done)
	}()

	// Read from the startup-pipe to see when the server has completed startup.
	// example contents: $^@^@^@{"host": "127.0.0.1", "port": 34051}
	// Note that the leading 4 characters are skipped when decoding.
	type pResult struct {
		URL string
		Err error
	}
	pDone := make(chan pResult, 1)

	go func() {
		var addr struct {
			Host string
			Port int
		}
		if err := json.NewDecoder(p).Decode(&addr); err != nil {
			pDone <- pResult{Err: errors.Wrap(err, "could not read host/port info")}
			return
		}

		pDone <- pResult{URL: fmt.Sprintf("http://%s:%d", addr.Host, addr.Port)}
	}()

	// Wait for server to write host/port info or to exit prematurely.
	select {
	case <-fdms.done:
		return errors.New("FakeDMS command exited early")
	case <-ctx.Done():
		fdms.kill(ctx)
		return errors.Errorf("test has timed out: %s", ctx.Err())
	case <-time.After(30 * time.Second):
		fdms.kill(ctx)
		return errors.New("FakeDMS took more than 30 seconds to start")
	case p := <-pDone:
		if p.Err != nil {
			fdms.kill(ctx)
			return errors.Wrap(p.Err, "could not get host/port info")
		}
		fdms.URL = p.URL
	}

	testing.ContextLog(ctx, "FakeDMS is up and running on ", fdms.URL)
	return nil
}

// WritePolicyBlob will write the given PolicyBlob to be read by the FakeDMS.
func (fdms *FakeDMS) WritePolicyBlob(pb *policy.Blob) error {
	if fdms.persistentDisableDomainVerification != nil {
		pb.UseUniversalSigningKeys = *fdms.persistentDisableDomainVerification
	}
	// Make sure persistent policies are always set.
	pb.AddPolicies(fdms.persistentPolicies)
	for k, v := range fdms.persistentPublicAccountPolicies {
		pb.AddPublicAccountPolicies(k, v)
	}
	if fdms.persistentPolicyUser != nil {
		pb.PolicyUser = *fdms.persistentPolicyUser
	}

	pJSON, err := json.Marshal(pb)
	if err != nil {
		return errors.Wrap(err, "could not convert policies to JSON")
	}

	if err := ioutil.WriteFile(fdms.policyPath, pJSON, 0644); err != nil {
		return errors.Wrap(err, "could not write JSON to file")
	}

	return nil
}

// WritePolicyBlobRaw writes the given PolicyBlob JSON string to be read by the FakeDMS.
// To apply persistent settings, pJSON is unmarshalled and then marshalled as PolicyBlob.
func (fdms *FakeDMS) WritePolicyBlobRaw(pJSON []byte) error {
	var pb policy.Blob
	if err := json.Unmarshal(pJSON, &pb); err != nil {
		return errors.Wrap(err, "failed to parse raw policy blob")
	}

	if err := fdms.WritePolicyBlob(&pb); err != nil {
		return err
	}

	return nil
}

// allowedPersistentPackages lists packages that are allowed to set persistent settings for FakeDMS.
var allowedPersistentPackages = []string{
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures",
}

// SetPersistentPolicies will ensure that the provided policies are always set.
func (fdms *FakeDMS) SetPersistentPolicies(persistentPolicies []policy.Policy) {
	caller.Check(2, allowedPersistentPackages)

	fdms.persistentPolicies = persistentPolicies
}

// SetPersistentPublicAccountPolicies will ensure that the provided public account policies are always set.
func (fdms *FakeDMS) SetPersistentPublicAccountPolicies(persistentPublicAccountPolicies map[string][]policy.Policy) {
	caller.Check(2, allowedPersistentPackages)

	fdms.persistentPublicAccountPolicies = persistentPublicAccountPolicies
}

// SetPersistentPolicyUser will ensure that the provided PolicyUser is always set.
func (fdms *FakeDMS) SetPersistentPolicyUser(persistentPolicyUser *string) {
	caller.Check(2, allowedPersistentPackages)

	fdms.persistentPolicyUser = persistentPolicyUser
}

// SetPersistentDisablePolicyKeyVerification will disable key verification and sign any domain.
func (fdms *FakeDMS) SetPersistentDisablePolicyKeyVerification(persistentDisablePolicyKeyVerification *bool) {
	caller.Check(2, allowedPersistentPackages)
	fdms.persistentDisableDomainVerification = persistentDisablePolicyKeyVerification
}

// Ping pings the running FakeDMS server and returns an error if all is not well.
func (fdms *FakeDMS) Ping(ctx context.Context) error {
	resp, err := http.Get(fdms.URL + "/test/ping")
	if err != nil {
		return errors.Wrap(err, "ping request failed")
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		return errors.Errorf("ping gave %d response", resp.StatusCode)
	}
	return nil
}

// kill will kill the running cmd and wait for it to return.
// To be used only when other errors have occurred; otherwise use Stop().
func (fdms *FakeDMS) kill(ctx context.Context) {
	if err := fdms.cmd.Kill(); err != nil {
		testing.ContextLog(ctx, "Kill command failed: ", err)
	}
	<-fdms.done
}

// Stop will stop the FakeDMS and return once the command has exited.
func (fdms *FakeDMS) Stop(ctx context.Context) {
	fdms.clientConn.Close()
	resp, err := http.Get(fdms.URL + "/test/exit")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode == 200 {
			// FakeDMS will exit on its own.
			select {
			case <-fdms.done:
				testing.ContextLog(ctx, "FakeDMS is closed")
				return
			case <-time.After(1 * time.Second):
				testing.ContextLog(ctx, "Took more than 1 second to close FakeDMS")
			case <-ctx.Done():
				testing.ContextLog(ctx, "Test timed out: ", ctx.Err())
			}
		}
	}

	// FakeDMS will not exit on its own.
	fdms.kill(ctx)
}

// SendRemoteCommand sends a remote command request to the fake_dmserver.
func (fdms *FakeDMS) SendRemoteCommand(ctx context.Context, req *empb.SendRemoteCommandRequest) (*empb.SendRemoteCommandResponse, error) {
	if req.RemoteCommand.CommandId == nil {
		tmpID := fdms.internalRemoteCommandID
		req.RemoteCommand.CommandId = &tmpID
		fdms.internalRemoteCommandID++
	}
	return fdms.remoteCommandsServiceClient.SendRemoteCommand(ctx, req)
}

// WaitRemoteCommandResult waits for the result of the given remote command from the fake_dmserver.
func (fdms *FakeDMS) WaitRemoteCommandResult(ctx context.Context, commandID *int64) (*empb.WaitRemoteCommandResultResponse, error) {
	return fdms.remoteCommandsServiceClient.WaitRemoteCommandResult(ctx, &empb.WaitRemoteCommandResultRequest{
		CommandId: commandID,
	})
}
