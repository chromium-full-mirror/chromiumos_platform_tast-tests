// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting to missive.
package secagentd

import (
	"context"
	"strconv"
	"time"

	rep "chromiumos/reporting"
	xdr "chromiumos/xdr/secagentd"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdcommon"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentddbusmonitor"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdprocfsscraper"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/secagentd/secagentdupstart"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ProcessEvents,
		Desc: "Checks that Process XDR events are correctly being reported",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com",
		},
		// ChromeOS > Security > ChromeOS Enterprise Security
		BugComponent: "b:1208373",
		Attr:         []string{"group:mainline", "group:enterprise-reporting"},
		Timeout:      4 * time.Minute,
		SoftwareDeps: []string{"bpf"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// makeProcessForPid fills the provided Process and Namespaces proto for the
// given process pid and sets ppid to the pid of the parent process.
func makeProcessForPid(ctx context.Context, pid uint64, p *xdr.Process, ns *xdr.Namespaces, ppid *uint64) error {
	if err := secagentdprocfsscraper.FillNamespaces(pid, ns); err != nil {
		return err
	}
	var err error
	if *ppid, err = secagentdprocfsscraper.FillProc(pid, p); err != nil {
		return err
	}
	cmdline, err := secagentdprocfsscraper.GetCmdLine(ctx, pid)
	if err != nil {
		return err
	}
	p.Commandline = proto.String(cmdline)
	p.Image = &xdr.FileImage{}
	if err := secagentdprocfsscraper.FillImage(pid, ns.GetMntNs(), p.GetImage()); err != nil {
		return err
	}
	return nil
}

// makeExpectedExec fills in the provided ProcessExecEvent proto for the given
// process pid. Proto contents reflect what we expect secagentd to emit as the
// Exec event for the given process. Except for any UUIDs which are random and
// unpredictable.
func makeExpectedExec(ctx context.Context, pid uint64, exec *xdr.ProcessExecEvent) error {
	exec.SpawnProcess = &xdr.Process{}
	exec.SpawnNamespaces = &xdr.Namespaces{}
	var ppid, gpid, ggpid uint64
	if err := makeProcessForPid(ctx, pid, exec.GetSpawnProcess(), exec.GetSpawnNamespaces(), &ppid); err != nil {
		return err
	}

	exec.Process = &xdr.Process{}
	if err := makeProcessForPid(ctx, ppid, exec.GetProcess(), &xdr.Namespaces{}, &gpid); err != nil {
		return err
	}

	exec.ParentProcess = &xdr.Process{}
	if err := makeProcessForPid(ctx, gpid, exec.GetParentProcess(), &xdr.Namespaces{}, &ggpid); err != nil {
		return err
	}

	return nil
}

// makeExpectedTerminate fills in the provided ProcessTerminateEvent
// with content from the given ProcessExecEvent. The contents reflect what we
// expect secagentd to emit as the Terminate event for the given process.
func makeExpectedTerminate(exec *xdr.ProcessExecEvent, term *xdr.ProcessTerminateEvent) {
	term.Process = exec.GetSpawnProcess()
	term.ParentProcess = exec.GetProcess()
}

// copyUUID copies ProcessUuid from one proto to another if present.
func copyUUID(from, to *xdr.Process) {
	if from != nil {
		to.ProcessUuid = proto.String(from.GetProcessUuid())
	}
}

func checkProcessEventWatcher(s *testing.State, ew *dbusutil.EventWatcher) ([]*xdr.ProcessExecEvent, []*xdr.ProcessTerminateEvent) {
	event, ok := <-ew.Events()
	if !ok {
		s.Fatal("Timed out waiting for expected events")
	}
	if len(event.Arguments) == 0 {
		return nil, nil
	}
	arg, ok := event.Arguments[0].([]byte)
	if !ok {
		return nil, nil
	}
	enq := &rep.EnqueueRecordRequest{}
	if err := proto.Unmarshal(arg, enq); err != nil {
		s.Fatal("Failed to unmarshal an EnqueueRecordRequest: ", err)
	}

	if enq.GetRecord().GetDestination() != rep.Destination_CROS_SECURITY_PROCESS {
		return nil, nil
	}
	pe := &xdr.XdrProcessEvent{}
	if err := proto.Unmarshal(enq.GetRecord().GetData(), pe); err != nil {
		s.Fatal("Failed to unmarshal data for a CROS_SECURITY_PROCESS record: ", err)
	}

	var bExecs []*xdr.ProcessExecEvent
	var bTerminates []*xdr.ProcessTerminateEvent
	for _, v := range pe.GetBatchedEvents() {
		if v.GetProcessExec() != nil {
			bExecs = append(bExecs, v.GetProcessExec())
		}
		if v.GetProcessTerminate() != nil {
			bTerminates = append(bTerminates, v.GetProcessTerminate())
		}

		if err := secagentdcommon.CheckCommon(v.GetCommon()); err != nil {
			s.Error("Invalid common field: ", err)
		}
	}

	return bExecs, bTerminates
}

// ProcessEvents runs a toy program, scrapes expected process and ancestral
// information from procfs, and verifies it against the events emitted by
// secagentd over dbus.
func ProcessEvents(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer func(ctx context.Context) {
		secagentdupstart.RestartSecagentd(ctx)
		cancel()
	}(cleanupCtx)

	const batchIntervalS = 5
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	agentPid, err := secagentdupstart.RestartSecagentd(ctx,
		upstart.WithArg("SECAGENTD_LOG_LEVEL", "-1"),
		upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"),
		upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true"),
		upstart.WithArg("PLUGIN_BATCH_INTERVAL_S_FOR_TESTING", strconv.Itoa(batchIntervalS)))
	if err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}

	ew, cancel, err := secagentddbusmonitor.SetupDbusWatcherWithTimeout(ctx, agentPid, 45*time.Second)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}
	defer cancel()

	if err := secagentdprocfsscraper.WaitForBpfMaps(ctx, agentPid); err != nil {
		s.Fatal("Failed to verify secagentd is ready to test: ", err)
	}
	// Launch a primer command to definitely set the "first seen exec" time
	// in secagentd. This will help avoid false positives for "meta_first_seen"
	// in terminate events.
	primeCmd := testexec.CommandContext(ctx, "/bin/yes")
	if err := primeCmd.Start(); err != nil {
		s.Fatalf("Error starting %q: %v ", primeCmd, err)
	}
	// Guarantee that "first seen exec" time is at least two seconds
	// before we exec the command we want to monitor.
	// Justification: If exec time is equal to first seen exec time
	// then meta_first_seen will be true in the terminate. This is a false
	// positive and while it is acceptable in practice, it will result in
	// test failures.
	// GoBigSleepLint: Using poll makes no sense here, we want to create a time
	// separation so there is no condition to poll for.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
	if err := primeCmd.Kill(); err != nil {
		s.Fatalf("Failed to kill %q: %v", primeCmd, err)
	}
	// Don't check the error here because it will likely just say
	// "signal: Killed"
	primeCmd.Wait()

	// Launch a long running process and scrape procfs.
	cmd := testexec.CommandContext(ctx, "/bin/yes")
	if err := cmd.Start(); err != nil {
		s.Fatalf("Error starting %q: %v ", cmd, err)
	}
	expExec := xdr.ProcessExecEvent{}
	if err := makeExpectedExec(ctx, uint64(cmd.Process.Pid), &expExec); err != nil {
		s.Fatal("Failed to make expected ProcessExec proto: ", err)
	}
	expTerm := xdr.ProcessTerminateEvent{}
	makeExpectedTerminate(&expExec, &expTerm)
	expPid := expExec.GetSpawnProcess().GetCanonicalPid()

	if err := cmd.Kill(); err != nil {
		s.Fatalf("Failed to kill %q: %v", cmd, err)
	}
	// Don't check the error here because it will likely just say
	// "signal: Killed"
	cmd.Wait()

	s.Log("Expected Exec: ", expExec.String())
	s.Log("Expected Terminate: ", expTerm.String())

	execFound, terminateFound := false, false

	for {
		bExecs, bTerminates := checkProcessEventWatcher(s, ew)

		if bExecs != nil {
			for _, exec := range bExecs {
				if exec != nil && exec.GetSpawnProcess() != nil && exec.GetSpawnProcess().GetCanonicalPid() == expPid {
					execFound = true
					// Copy the random UUIDs so that proto.Equal() is happy.
					copyUUID(exec.GetSpawnProcess(), expExec.SpawnProcess)
					copyUUID(exec.GetProcess(), expExec.Process)
					copyUUID(exec.GetParentProcess(), expExec.ParentProcess)
					// Copy over the terminate timestamp if present.
					if exec.TerminateTimestampUs != nil {
						expExec.TerminateTimestampUs = proto.Int64(exec.GetTerminateTimestampUs())
					}
					// The spawned process is guaranteed to be seen for the first
					// time. The rest of the hierarchy depends on tast
					// implementation details so we skip checking those.
					expExec.GetSpawnProcess().MetaFirstAppearance = proto.Bool(true)
					expExec.GetProcess().MetaFirstAppearance = exec.GetProcess().MetaFirstAppearance
					expExec.GetParentProcess().MetaFirstAppearance = exec.GetParentProcess().MetaFirstAppearance
					if !proto.Equal(&expExec, exec) {
						s.Log("Actual ProcessExec: ", exec.String())
						s.Log("Expected ProcessExec: ", expExec.String())
						s.Errorf("Found a ProcessExec event for pid %d but its contents failed to match", expPid)
					}
				}
			}
		}

		if bTerminates != nil {
			for _, terminate := range bTerminates {
				if terminate != nil && terminate.GetProcess() != nil && terminate.GetProcess().GetCanonicalPid() == expPid {
					terminateFound = true
					copyUUID(terminate.GetProcess(), expTerm.Process)
					copyUUID(terminate.GetParentProcess(), expTerm.ParentProcess)
					// We definitely saw the exec events already so this isn't the
					// first appearance of either process.
					expTerm.GetProcess().MetaFirstAppearance = proto.Bool(false)
					expTerm.GetParentProcess().MetaFirstAppearance = proto.Bool(false)
					if !proto.Equal(&expTerm, terminate) {
						s.Log("Actual ProcessTerminate: ", terminate.String())
						s.Log("Expected ProcessTerminate: ", expTerm.String())
						s.Errorf("Found a ProcessTerminate event for pid %d but its contents failed to match", expPid)
					}
				}
			}
		}
		if execFound {
			break
		}
	}

	if !execFound {
		s.Errorf("Failed to find a matching ProcessExec event for pid %d", expPid)
	}

	if terminateFound {
		// Coalescing is best effort and based on timing. Err on the
		// side of not flaking the test.
		s.Logf("Found uncoalesced ProcessExit event for pid %d", expPid)
	}
}
