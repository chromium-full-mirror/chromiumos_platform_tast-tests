// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/memory"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MemoryReclaim,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Eat memory and reclaim via procfs",
		Contacts:     []string{"chromeos-memory@google.com"},
		BugComponent: "b:167286",
		Attr:         []string{"group:mainline", "informational"},
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{},
	})

}

const fourMB = 1024 * 1024 * 4

func testAnonymous(r *memory.Reclaimer, s *testing.State) error {
	err := r.AllocateAnonymous(fourMB)
	if err != nil {
		return err
	}

	err = r.ReclaimAnonymous()
	if err != nil {
		return err
	}

	return nil
}

func testAnonHugePages(r *memory.Reclaimer, s *testing.State) error {
	err := r.AllocateAnonHugePages(fourMB)
	if err != nil {
		return err
	}

	err = r.ReclaimAnonymous()
	if err != nil {
		return err
	}

	return nil
}

func testShmem(r *memory.Reclaimer, s *testing.State) error {
	err := r.AllocateShmem(fourMB)
	if err != nil {
		return err
	}

	err = r.ReclaimShmem()
	if err != nil {
		return err
	}

	return nil
}

func testFileMem(r *memory.Reclaimer, s *testing.State) error {
	err := r.AllocateFileMem(fourMB)
	if err != nil {
		return err
	}

	err = r.ReclaimFile()
	if err != nil {
		return err
	}

	return nil
}

func MemoryReclaim(ctx context.Context, s *testing.State) {
	r := memory.NewReclaimer(ctx)

	testCases := []struct {
		name string
		fp   func(*memory.Reclaimer, *testing.State) error
	}{
		{
			name: "testAnonymous",
			fp:   testAnonymous,
		},
		{
			name: "testAnonHugePages",
			fp:   testAnonHugePages,
		},
		{
			name: "testShmem",
			fp:   testShmem,
		},
		{
			name: "testFileMem",
			fp:   testFileMem,
		},
	}

	for _, t := range testCases {
		err := t.fp(r, s)
		if err != nil {
			s.Fatalf("Failed to %s: %v", t.name, err)
		}
	}

	r.FreeAll()
}
