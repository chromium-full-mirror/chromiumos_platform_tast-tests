// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting to missive.
package secagentd

import (
	"context"
	"time"

	"google.golang.org/protobuf/proto"

	rep "chromiumos/reporting"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/bundles/cros/secagentd/secagentddbusmonitor"
	"chromiumos/tast/local/bundles/cros/secagentd/secagentdprocfsscraper"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
	xdr "chromiumos/xdr/reporting"
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
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"bpf"},
	})
}

// makeProcessForPid fills the provided Process and Namespaces proto for the
// given process pid and sets ppid to the pid of the parent process.
func makeProcessForPid(pid uint64, p *xdr.Process, ns *xdr.Namespaces, ppid *uint64) error {
	if err := secagentdprocfsscraper.FillNamespaces(pid, ns); err != nil {
		return err
	}
	var err error
	if *ppid, err = secagentdprocfsscraper.FillProcStatus(pid, p); err != nil {
		return err
	}
	cmdline, err := secagentdprocfsscraper.GetCmdLine(pid)
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
func makeExpectedExec(pid uint64, exec *xdr.ProcessExecEvent) error {
	exec.SpawnProcess = &xdr.Process{}
	exec.SpawnNamespaces = &xdr.Namespaces{}
	var ppid, gpid, ggpid uint64
	if err := makeProcessForPid(pid, exec.GetSpawnProcess(), exec.GetSpawnNamespaces(), &ppid); err != nil {
		return err
	}

	exec.Process = &xdr.Process{}
	if err := makeProcessForPid(ppid, exec.GetProcess(), &xdr.Namespaces{}, &gpid); err != nil {
		return err
	}

	exec.ParentProcess = &xdr.Process{}
	if err := makeProcessForPid(gpid, exec.GetParentProcess(), &xdr.Namespaces{}, &ggpid); err != nil {
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

// ProcessEvents runs a toy program, scrapes expected process and ancestral
// information from procfs, and verifies it against the events emitted by
// secagentd over dbus.
func ProcessEvents(ctx context.Context, s *testing.State) {
	// Restart secagentd and have it ignore policy and not wait for the first
	// agent event to be enqueued successfully.
	if err := upstart.RestartJob(ctx, "secagentd", upstart.WithArg("BYPASS_POLICY_FOR_TESTING", "true"), upstart.WithArg("BYPASS_ENQ_OK_WAIT_FOR_TESTING", "true")); err != nil {
		s.Fatal("Failed to restart secagentd: ", err)
	}

	stop, err := secagentddbusmonitor.SetupDbusMonitor(ctx)
	if err != nil {
		s.Fatal("Failed to setup dbus monitoring: ", err)
	}

	// Launch a long running process and scrape procfs.
	cmd := testexec.CommandContext(ctx, "/bin/yes")
	if err := cmd.Start(); err != nil {
		s.Fatalf("Error starting %q: %v ", cmd, err)
	}
	expExec := xdr.ProcessExecEvent{}
	if err := makeExpectedExec(uint64(cmd.Process.Pid), &expExec); err != nil {
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

	// Small grace period for the events to be processed and emitted by
	// secagentd.
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Collect the log of EnqueueRecord dbus calls to Missived.
	calledMethods, err := stop()
	if err != nil {
		s.Fatal("Failed to capture EnqueueRecord dbus calls to missived: ", err)
	}
	s.Logf("secagentd enqueued %d events", len(calledMethods))

	execFound, terminateFound := false, false
	for _, method := range calledMethods {
		if len(method.Arguments) == 0 {
			continue
		}
		arg, ok := method.Arguments[0].([]byte)
		if !ok {
			continue
		}
		enq := &rep.EnqueueRecordRequest{}
		if err := proto.Unmarshal(arg, enq); err != nil {
			s.Fatal("Failed to unmarshal an EnqueueRecordRequest")
		}
		if enq.GetRecord().GetDestination() != rep.Destination_CROS_SECURITY_PROCESS {
			continue
		}
		pe := &xdr.XdrProcessEvent{}
		if err := proto.Unmarshal(enq.GetRecord().GetData(), pe); err != nil {
			s.Fatal("Failed to unmarshal data for a CROS_SECURITY_PROCESS record")
		}
		exec := pe.GetProcessExec()
		if exec != nil && exec.GetSpawnProcess() != nil && exec.GetSpawnProcess().GetCanonicalPid() == expPid {
			execFound = true
			// Copy the random UUIDs so that proto.Equal() is happy.
			copyUUID(exec.GetSpawnProcess(), expExec.SpawnProcess)
			copyUUID(exec.GetProcess(), expExec.Process)
			copyUUID(exec.GetParentProcess(), expExec.ParentProcess)
			if !proto.Equal(&expExec, exec) {
				s.Log("Actual ProcessExec: ", exec.String())
				s.Log("Expected ProcessExec: ", expExec.String())
				s.Errorf("Found a ProcessExec event for pid %d but its contents failed to match", expPid)
			}
		}
		terminate := pe.GetProcessTerminate()
		if terminate != nil && terminate.GetProcess() != nil && terminate.GetProcess().GetCanonicalPid() == expPid {
			terminateFound = true
			copyUUID(terminate.GetProcess(), expTerm.Process)
			copyUUID(terminate.GetParentProcess(), expTerm.ParentProcess)
			if !proto.Equal(&expTerm, terminate) {
				s.Log("Actual ProcessTerminate: ", terminate.String())
				s.Log("Expected ProcessTerminate: ", expTerm.String())
				s.Errorf("Found a ProcessTerminate event for pid %d but its contents failed to match", expPid)
			}
		}
	}
	if !execFound {
		s.Errorf("Failed to find a matching ProcessExec event for pid %d", expPid)
	}
	if !terminateFound {
		s.Errorf("Failed to find a matching ProcessExit event for pid %d", expPid)
	}
}
