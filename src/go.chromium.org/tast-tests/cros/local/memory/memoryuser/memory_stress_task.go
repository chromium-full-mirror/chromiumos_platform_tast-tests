// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memoryuser

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// MemoryStressUnit creates a Chrome tab that allocates memory like the
// platform.MemoryStressBasic test.
type MemoryStressUnit struct {
	url      string
	tconn    *browser.TestConn
	cooldown time.Duration
	tabID    int // ID of new tab, so we can query for liveness later
}

// Run creates a Chrome tab that allocates memory, then waits for the provided
// cooldown.
func (st *MemoryStressUnit) Run(ctx context.Context, br *browser.Browser) error {
	conn, err := br.NewConn(ctx, st.url)
	if err != nil {
		return errors.New("failed to open MemoryStressUnit page")
	}
	// Leaving the debug connection for a target attached results in
	// PageDiscardingHelper treating the tab as protected. So we close the
	// connection after verifying that the allocation complete.
	defer conn.Close()

	// Because chrome.tabs is not available on the conn, query active tabs
	// assuming there's only one window so only one active tab, and the active tab is
	// the newly created tab, in order to get its TabID.
	tconn, err := br.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the connection to the test extension")
	}
	st.tconn = tconn

	if err := tconn.Call(ctx, &st.tabID, `async () => {
	  const tabs = await tast.promisify(chrome.tabs.query)({active: true});
	  if (tabs.length !== 1) {
	    throw new Error("unexpected number of active tabs: got " + tabs.length)
	  }
	  return tabs[0].id;
	}`); err != nil {
		return errors.Wrap(err, "cannot get tab id for the new tab")
	}

	// Wait for allocation to complete.
	const expr = "document.hasOwnProperty('out') == true"
	if err := conn.WaitForExprFailOnErr(ctx, expr); err != nil {
		return errors.Wrap(err, "unexpected error waiting for allocation")
	}
	if st.cooldown > 0 {
		// GoBigSleepLint we sleep here to throttle allocation of memory. Allocating
		// as fast as possible can cause instability.
		if err := testing.Sleep(ctx, st.cooldown); err != nil {
			return errors.Wrap(err, "failed to sleep for cooldown")
		}
	}
	return nil
}

// Close closes the memory stress allocation tab.
func (st *MemoryStressUnit) Close(ctx context.Context, br *browser.Browser) error {
	if err := st.tconn.Call(ctx, nil, `async (url) => {
		const query = tast.promisify(chrome.tabs.query);
		const remove = tast.promisify(chrome.tabs.remove);
		const tabs = await query({ url });
		// Works for any number of tabs, even if it will usually be 1.
		await Promise.all(tabs.map(t => remove(t.id)));
	}`, st.url); err != nil {
		return errors.Wrapf(err, "failed to close tab %q", st.url)
	}
	return nil
}

// StillAlive checks if a tab is still present and undiscarded.
func (st *MemoryStressUnit) StillAlive(ctx context.Context, br *browser.Browser) bool {
	// Discarded tab may be reported by "No tab with id" error by JS, or
	// "discarded" property.
	var discarded bool
	if err := st.tconn.Call(ctx, &discarded, `async (id) => {
	  try {
	    const tab = await tast.promisify(chrome.tabs.get)(id);
	    return tab.discarded;
	  } catch (e) {
	    if (e.message.startsWith("No tab with id: "))
	      return true;
	    throw e;
	  }
	}`, st.tabID); err != nil {
		return false
	}
	return !discarded
}

// FillChromeOSMemory launches memory stress tabs until one is killed, filling
// up memory in ChromeOS.
func FillChromeOSMemory(ctx context.Context, dataFileSystem http.FileSystem, br *browser.Browser, unitMiB int, ratio float32) (func(context.Context) error, error) {
	server := NewMemoryStressServer(dataFileSystem)
	var units []*MemoryStressUnit
	cleanup := func(ctx context.Context) error {
		var res error
		for _, unit := range units {
			if err := unit.Close(ctx, br); err != nil {
				testing.ContextLogf(ctx, "Failed to close MemoryStressUnit: %s", err)
				if res == nil {
					res = err
				}
			}
		}
		server.Close()
		return res
	}
	for i := 0; ; i++ {
		const tabOpenCooldown = 2 * time.Second
		unit := server.NewMemoryStressUnit(unitMiB, ratio, tabOpenCooldown)
		units = append(units, unit)
		if err := unit.Run(ctx, br); err != nil {
			return cleanup, errors.Wrapf(err, "failed to run MemoryStressUnit %q", unit.url)
		}
		for _, unit := range units {
			if !unit.StillAlive(ctx, br) {
				testing.ContextLogf(ctx, "FillChromeOSMemory started %d units of %d MiB before first kill", len(units), unitMiB)
				return cleanup, nil
			}
		}
	}
}

// MemoryStressTask wraps MemoryStressUnit to conform to the MemoryTask and
// KillableTask interfaces.
type MemoryStressTask struct{ MemoryStressUnit }

// MemoryStressTask is a MemoryTask.
var _ MemoryTask = (*MemoryStressTask)(nil)

// MemoryStressTask is a KillableTask.
var _ KillableTask = (*MemoryStressTask)(nil)

// String returns a friendly name for the task.
func (st *MemoryStressTask) String() string {
	return "Chrome Memory Stress Basic"
}

// NeedVM is false because we do not need Crostini.
func (st *MemoryStressTask) NeedVM() bool {
	return false
}

// Run creates a Chrome tab that allocates memory, then waits for the provided
// cooldown.
func (st *MemoryStressTask) Run(ctx context.Context, testEnv *TestEnv) error {
	return st.MemoryStressUnit.Run(ctx, testEnv.br)
}

// Close closes the memory stress allocation tab.
func (st *MemoryStressTask) Close(ctx context.Context, testEnv *TestEnv) {
	st.MemoryStressUnit.Close(ctx, testEnv.br)
}

// StillAlive returns false if the tab has been discarded, or was never opened.
func (st *MemoryStressTask) StillAlive(ctx context.Context, testEnv *TestEnv) bool {
	return st.MemoryStressUnit.StillAlive(ctx, testEnv.br)
}

// MemoryStressServer is an http server that hosts the html and js needed to
// create MemoryStressTasks.
type MemoryStressServer struct {
	server *httptest.Server
	nextID int
}

// Resources needed by MemoryStressServer to create MemoryStressTasks.
const (
	AllocPageFilename  = "memory_stress.html"
	JavascriptFilename = "memory_stress.js"
)

// NewMemoryStressServer creates a server that can create MemoryStressTasks.
// Close() should be called after use.
func NewMemoryStressServer(dataFileSystem http.FileSystem) *MemoryStressServer {
	return &MemoryStressServer{
		server: httptest.NewServer(http.FileServer(dataFileSystem)),
	}
}

// NewMemoryStressUnit creates a new MemoryStressUnit.
// allocMiB - The amount of memory the tab will allocate.
// ratio    - How compressible the allocated memory will be.
// cooldown - How long to wait after allocating before returning.
func (s *MemoryStressServer) NewMemoryStressUnit(allocMiB int, ratio float32, cooldown time.Duration) *MemoryStressUnit {
	url := fmt.Sprintf("%s/%s?alloc=%d&ratio=%.3f&id=%d", s.server.URL, AllocPageFilename, allocMiB, ratio, s.nextID)
	s.nextID++
	return &MemoryStressUnit{
		url:      url,
		cooldown: cooldown,
	}
}

// NewMemoryStressTask creates a new MemoryStressTask.
// allocMiB - The amount of memory the tab will allocate.
// ratio    - How compressible the allocated memory will be.
// cooldown - How long to wait after allocating before returning.
func (s *MemoryStressServer) NewMemoryStressTask(allocMiB int, ratio float32, cooldown time.Duration) *MemoryStressTask {
	return &MemoryStressTask{*s.NewMemoryStressUnit(allocMiB, ratio, cooldown)}
}

// Close shuts down the http server.
func (s *MemoryStressServer) Close() {
	s.server.Close()
}
