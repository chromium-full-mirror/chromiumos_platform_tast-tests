// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"fmt"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/crosserverutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
)

const (
	hibernateConfirmationPath = "/tmp/hibernate_confirmation"
	hibernateCyclesVars       = "cycles"
	hibernateCyclesDefault    = 5
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Hibernate,
		Desc:         "Verifies that system comes back after hibernation",
		BugComponent: "b:1361410",
		Contacts:     []string{"chromeos-platform-power@google.com", "vovoy@google.com"},
		SoftwareDeps: []string{"reboot", "no_qemu"},
		// Allow for a larger number of cycles for stress testing. In case of a hang the
		// test will time out on one of the shorter context specific timeouts.
		Timeout: 24 * time.Hour,
		Vars:    []string{hibernateCyclesVars, tape.ServiceAccountVar},
	})
}

func getDmesg(ctx context.Context, d *dut.DUT) (string, error) {
	out, err := d.Conn().CommandContext(ctx, "dmesg").Output()
	if err != nil {
		return "", errors.Wrap(err, "failed to get dmesg")
	}
	return string(out[:]), nil
}

// checkDmesgFsCorruption returns error if any of the file system corruption pattern is in the DUT
// dmesg.
func checkDmesgFsCorruption(ctx context.Context, s *testing.State, dmesg string) error {
	fileCorruptionPatterns := []string{
		"space map common: bitmap check failed:",
		"sm_bitmap validator check failed",
		"metadata operation 'dm_pool_alloc_data_block' failed",
		"aborting current metadata transaction",
		"integrity: Error on flusing disk cache",
		"provision_block: alloc_data_block() failed",
		"Buffer I/O error on device",
		"error count since last fsck",
		"submitting bio failed at sector",
		"bad block bitmap checksum",
		"block bitmap corrupt",
		"block bitmap and bg descriptor inconsistent",
		"ext4_journal_check_start:83: Detected aborted journal",
		"I/O error while writing superblock",
		"blk_update_request: I/O error",
		"Buffer I/O error on device",
		"blk_update_request: I/O error",
	}

	re := regexp.MustCompile(strings.Join(fileCorruptionPatterns, "|"))
	match := re.FindString(dmesg)
	if match != "" {
		return errors.Errorf("found file corruption strings: %s", match)
	}
	s.Log("No file corruption strings found")
	return nil
}

// checkDmesgSuspendResumeConfirmation returns nil if all the suspend/resume confirmation patterns
// are in the DUT dmesg. Returns error otherwise.
func checkDmesgSuspendResumeConfirmation(ctx context.Context, s *testing.State, dmesg string) error {
	confirmationPatterns := []string{
		// suspend
		"Freezing user space processes ...",
		"PM: end freeze of devices complete",
		"Disabling non-boot CPUs ...",
		// restore
		"PM: restore of devices complete",
		"Restarting tasks ...",
	}

	for _, pattern := range confirmationPatterns {
		if !strings.Contains(dmesg, pattern) {
			return errors.Errorf("Confirmation pattern not found in dmesg: %s", pattern)
		}
	}
	return nil
}

// writeNumberToHibernateConfirmation write a number to the hibernate confirmation file.
func writeNumberToHibernateConfirmation(ctx context.Context, s *testing.State, number uint32) error {
	s.Logf("Write number %d to hibernate confirmation", number)

	command := fmt.Sprintf("echo %d > %s", number, hibernateConfirmationPath)

	dut := s.DUT()
	_, err := dut.Conn().CommandContext(ctx, "bash", "-c", command).Output()
	if err != nil {
		return errors.Wrap(err, "failed to write to hibernate confirmation")
	}

	return nil
}

// readNumberFromHibernateConfirmation read and parse the hibernate confirmation file.
func readNumberFromHibernateConfirmation(ctx context.Context, s *testing.State) (uint32, error) {
	command := fmt.Sprintf("cat %s", hibernateConfirmationPath)
	dut := s.DUT()
	out, err := dut.Conn().CommandContext(ctx, "bash", "-c", command).Output()
	if err != nil {
		return 0, errors.Wrap(err, "failed to read hibernate confirmation")
	}
	str := string(out[:])
	str = strings.ReplaceAll(str, "\n", "")
	result, err := strconv.ParseUint(str, 10, 32)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to parse hibernate confirmation, content: %s", str)
	}
	return uint32(result), nil

}

// hibernateResume hibernate and resume the DUT.
func hibernateResume(ctx context.Context, s *testing.State) error {
	commandCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	command := "hiberman hibernate -r"
	dut := s.DUT()
	out, err := dut.Conn().CommandContext(commandCtx, "bash", "-c", command).CombinedOutput()
	s.Logf("hiberman output: %s", out)
	if err != nil {
		if strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			// The command is expected to time out if hibernate was successful.
			return nil
		}
		return errors.Wrap(err, "failed to hibernate")
	}
	return errors.New("\"hiberman hibernate -r\" should disconnect the DUT")
}

func login(ctx context.Context, s *testing.State, account *tape.OwnedTestAccount, client *crosserverutil.Client, reuseSession, keepState bool) error {
	// Start Chrome with the username and password.
	cs := pb.NewChromeServiceClient(client.Conn)
	if _, err := cs.New(ctx, &pb.NewRequest{
		LoginMode: pb.LoginMode_LOGIN_MODE_GAIA_LOGIN,
		Credentials: &pb.NewRequest_Credentials{
			Username: account.Username,
			Password: account.Password,
		},
		TryReuseSession: reuseSession,
		KeepState:       keepState,
	}); err != nil {
		return errors.Wrap(err, "failed to log into chrome on DUT")
	}
	return nil
}

func openTabs(ctx context.Context, s *testing.State, client *crosserverutil.Client) ([]string, error) {
	s.Log("Open tabs")
	numConns := 3
	url := "about:blank"
	targetIds := make([]string, numConns)

	// Create new Chrome connections pointing to the same url.
	svc := pb.NewConnServiceClient(client.Conn)
	for i := 0; i < numConns; i++ {
		res, err := svc.NewConn(ctx, &pb.NewConnRequest{Url: url})
		if err != nil {
			return nil, errors.Wrapf(err, "failed to open page %v", url)
		}
		targetIds[i] = res.TargetId
	}
	return targetIds, nil
}

func loginAndOpenTabs(ctx context.Context, s *testing.State, account *tape.OwnedTestAccount) ([]string, error) {
	client, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to the GRPC server on the DUT")
	}
	defer client.Close(ctx)

	if err := login(ctx, s, account, client, false, false); err != nil {
		return nil, errors.Wrap(err, "failed to login")
	}

	targetIds, err := openTabs(ctx, s, client)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open tabs")
	}
	return targetIds, nil
}

// verifyTargetIds ensures that NewConnForTarget can access existing chrome connections through targetIds.
func verifyTargetIds(ctx context.Context, s *testing.State, client *crosserverutil.Client, targetIds []string) error {
	s.Log("Verify target IDs")

	// Verify connecting to target IDs.
	svc := pb.NewConnServiceClient(client.Conn)
	for _, targetID := range targetIds {
		s.Log("Connecting to target (120 seconds timeout): ", targetID)
		timeoutCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
		defer cancel()
		newConn, err := svc.NewConnForTarget(timeoutCtx, &pb.NewConnForTargetRequest{TargetId: targetID})
		if err != nil {
			return errors.Wrapf(err, "failed when calling NewConnForTargetRequest for %v", targetID)
		}
		s.Log("Connected to target: ", targetID)
		activateTargetRequest := &pb.ActivateTargetRequest{
			Id: newConn.Id,
		}
		_, err = svc.ActivateTarget(ctx, activateTargetRequest)
		if err != nil {
			return errors.Wrapf(err, "failed when calling ActivateTarget for %v", activateTargetRequest)
		}
	}
	return nil
}

func loginToResume(ctx context.Context, s *testing.State, account *tape.OwnedTestAccount) error {
	client, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the GRPC server on the DUT")
	}
	defer client.Close(ctx)

	// Login with 60 seconds timeout.
	timeoutCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	if err := login(timeoutCtx, s, account, client, false, true); err != nil {
		if strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			return nil
		}
		return errors.Wrap(err, "unexpected login error")
	}
	return nil
}

func loginAndVerifyTargetIds(ctx context.Context, s *testing.State, account *tape.OwnedTestAccount, targetIds []string) error {
	client, err := crosserverutil.GetGRPCClient(ctx, s.DUT())
	if err != nil {
		return errors.Wrap(err, "failed to connect to the GRPC server on the DUT")
	}
	defer client.Close(ctx)

	if err := login(ctx, s, account, client, true, true); err != nil {
		return errors.Wrap(err, "failed to login")
	}

	if err := verifyTargetIds(ctx, s, client, targetIds); err != nil {
		return errors.Wrap(err, "failed to verify target IDs")
	}

	return nil
}

// hibernateIteration runs a cycle of hibernation and resume.
func hibernateIteration(ctx context.Context, s *testing.State, account *tape.OwnedTestAccount) {
	dut := s.DUT()

	// Reboot to ensure system is in a consistent state.
	s.Log("Starting reboot")
	if err := dut.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}
	s.Log("Reboot completed")

	// Login and open tabs.
	targetIds, err := loginAndOpenTabs(ctx, s, account)
	if err != nil {
		s.Fatalf("Failed to login and open tabs: %o", err)
	}
	s.Log("Target ids: ", targetIds)

	// Search dmesg for file system corruption.
	dmesgBeforeHibernation, err := getDmesg(ctx, dut)
	if err != nil {
		s.Fatalf("Failed to get dmesg: %o", err)
	}
	if err := checkDmesgFsCorruption(ctx, s, dmesgBeforeHibernation); err != nil {
		s.Fatalf("Failed to check dmesg file corruptions: %o", err)
	}

	// Write random number to tmpfs
	randomNumber := rand.Uint32()
	if err := writeNumberToHibernateConfirmation(ctx, s, randomNumber); err != nil {
		s.Fatalf("Failed to write hibernate confirmation file: %o", err)
	}

	// Hibernate and reboot.
	s.Log("Starting hibernation")
	if err := hibernateResume(ctx, s); err != nil {
		s.Fatalf("Failed to Hibernate: %o", err)
	}

	// Wait for alive.
	s.Log("Waiting for alive")
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 2*time.Minute)
	defer cancelWaitConnect()
	if err := dut.WaitConnect(waitConnectCtx); err != nil {
		s.Fatalf("Failed to reconnect: %o", err)
	}

	// Login to trigger resume.
	s.Log("Login to resume")
	if err := loginToResume(ctx, s, account); err != nil {
		s.Fatalf("Failed to resume: %o", err)
	}

	// Reconnect as successful resume resets the connection.
	if err := dut.Connect(ctx); err != nil {
		s.Fatalf("Failed to reconnect to DUT: %o", err)
	}

	// Verify target ids.
	s.Log("Login again and verify target ids")
	if err := loginAndVerifyTargetIds(ctx, s, account, targetIds); err != nil {
		s.Fatalf("Failed to login and verify target ids: %o", err)
	}

	// Check the hibernate confirmation file.
	number, err := readNumberFromHibernateConfirmation(ctx, s)
	if err != nil {
		s.Fatalf("Failed to read hibernate confirmation file: %o", err)
	}
	if randomNumber != number {
		s.Fatalf("The numbers in hibernate confirmation mismatch. write: %d, read: %d", randomNumber, number)
	}
	s.Log("Checking hibernate confirmation file succeeded")

	// Get dmesg after resume.
	dmesgAfterResume, err := getDmesg(ctx, dut)
	if err != nil {
		s.Fatalf("Failed to get dmesg: %o", err)
	}

	// Check dmesg suspend resume patterns.
	if err := checkDmesgSuspendResumeConfirmation(ctx, s, dmesgAfterResume); err != nil {
		s.Fatalf("Failed to Check suspend resume patterns in dmesg: %o", err)
	}
	s.Log("Checking dmesg suspend resume confirmations succeeded")

	// Search dmesg for file system corruption.
	if err := checkDmesgFsCorruption(ctx, s, dmesgAfterResume); err != nil {
		s.Fatalf("Failed to check dmesg file corruptions: %o", err)
	}
}

func Hibernate(ctx context.Context, s *testing.State) {
	// cycles: hibernate cycles to run.
	cycles := hibernateCyclesDefault
	if v, ok := s.Var(hibernateCyclesVars); ok {
		newCycles, err := strconv.Atoi(v)
		if err != nil {
			s.Fatalf("Failed to parse %s from string %s", hibernateCyclesVars, v)
		}
		cycles = newCycles
	}

	// Getting test account.
	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	const timeoutPerCycle int32 = 300 // 300 seconds.
	timeout := timeoutPerCycle * int32(cycles)
	// Create an account manager and lease a test account for the duration of the test.
	accManager, account, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(ctx)

	for i := 1; i <= cycles; i++ {
		hibernateCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()

		hibernateIteration(hibernateCtx, s, account)
		s.Logf("Hibernate attempt %d complete", i)
	}
}
