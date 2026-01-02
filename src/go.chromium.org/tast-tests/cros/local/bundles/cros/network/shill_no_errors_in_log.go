// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"io"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Func:         ShillNoErrorsInLog,
		Desc:         "Checks that there are no unexpected error logs in net.log when shill restarts and becomes online",
		Contacts: []string{
			"cros-networking@google.com",
			"jiejiang@google.com",
		},
		Attr: []string{"group:mainline", "group:release-health", "release-health_network", "informational"},
	})
}

const shillJob = "shill"

// startShillAndWaitForNetworks does the actions that we are interested in.
// Shill should be stopped before calling this function and should be restarted
// after calling this function.
func startShillAndWaitForNetworks(ctx context.Context, s *testing.State) {
	if err := upstart.RestartJob(ctx, shillJob); err != nil {
		s.Fatal("Failed starting shill: ", err)
	}

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed creating shill manager proxy: ", err)
	}

	// Wait until a service is connected.
	connectedProps := map[string]interface{}{
		shillconst.ServicePropertyIsConnected: true,
	}
	if _, err := manager.WaitForServiceProperties(ctx, connectedProps, 30*time.Second); err != nil {
		s.Fatal("Failed to wait for connected service: ", err)
	}

	// Conditionally wait until a service to be online to cover the portal
	// detection process in the log. Since a failure can be due to a transient
	// environmental issue, do log instead of failing the test on failure.
	onlineProps := map[string]interface{}{
		shillconst.ServicePropertyState: shillconst.ServiceStateOnline,
	}
	if _, err := manager.WaitForServiceProperties(ctx, onlineProps, 15*time.Second); err != nil {
		s.Log("Failed to wait for online service: ", err)
	}

	if ethernetAvailable, err := manager.IsAvailable(ctx, shill.TechnologyEthernet); err != nil {
		s.Fatal("Error calling IsAvailable: ", err)
	} else if !ethernetAvailable {
		s.Fatal("Ethernet not available")
	}

	// Wait for WiFi to come up if it is available.
	if wifiAvailable, err := manager.IsAvailable(ctx, shill.TechnologyWifi); err != nil {
		s.Fatal("Error calling IsAvailable: ", err)
	} else if !wifiAvailable {
		s.Log("WiFi not available")
	}

	if err := upstart.StopJob(ctx, shillJob); err != nil {
		s.Fatal("Failed stopping shill: ", err)
	}
}

func ShillNoErrorsInLog(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// We lose connectivity along the way here, and if that races with the
	// recover_duts network-recovery hooks, it may interrupt us. Lock the hook
	// before shill restarted.
	unlock, err := network.LockCheckNetworkHook(ctx)
	if err != nil {
		s.Fatal("Failed to lock check network hook: ", err)
	}
	defer unlock()

	if err := upstart.StopJob(ctx, shillJob); err != nil {
		s.Fatal("Failed stopping shill: ", err)
	}
	defer upstart.RestartJob(cleanupCtx, shillJob)

	// Save the net.log into the test folder to make debugging easier.
	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	sr, err := syslog.NewReader(ctx, syslog.SourcePath(syslog.NetLogFile), syslog.Severities(syslog.Err))
	if err != nil {
		s.Fatal("Failed to open the net log file: ", err)
	}
	defer sr.Close()
	startShillAndWaitForNetworks(ctx, s)
	endTime := time.Now()

	type subEntry struct {
		Program  string
		FileName string
		Message  string
	}
	var subEntries []subEntry

	for {
		e, err := sr.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.Fatal("Failed to read the network log: ", err)
		}
		if e.Timestamp.After(endTime) {
			break
		}
		subEntries = append(subEntries, subEntry{e.Program, syslog.ExtractFileName(*e), e.Content})
	}

	allowedEntries := shillconst.InitializeAllowedEntries()
	var unexpected []subEntry
	for _, e := range subEntries {
		allowed := false
		for i, r := range allowedEntries {
			if r.Program != e.Program || r.FileName != e.FileName {
				continue
			}
			if matched, _ := regexp.MatchString(r.MessageRegex, e.Message); matched {
				allowedEntries[i].Counter++
				allowed = true
				break
			}
		}
		if !allowed {
			unexpected = append(unexpected, e)
		}
	}

	for _, r := range allowedEntries {
		s.Log(r.Counter, " * ", r.Program, ":", r.FileName, ":", r.MessageRegex)
	}

	if len(unexpected) != 0 {
		s.Log("Unexpected errors: ")
		const maxLines = 3
		msglines := len(unexpected)
		if msglines > maxLines {
			msglines = maxLines
		}
		msgs := make([]string, msglines)
		for n, e := range unexpected {
			s.Log(e)
			if n < msglines {
				msgs[n] = e.Message
			}
		}
		msg := strings.Join(msgs, ", ")
		if msglines != len(unexpected) {
			s.Fatalf("Unexpected error lines, %v/%v: %v", msglines, len(unexpected), msg)
		} else {
			s.Fatalf("Unexpected error lines(%v): %v", msglines, msg)
		}
	}
}
