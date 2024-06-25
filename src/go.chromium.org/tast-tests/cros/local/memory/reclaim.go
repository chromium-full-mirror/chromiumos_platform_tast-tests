// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Reclaimer eats memory and reclaims them.
type Reclaimer struct {
	buffers [][]byte
	ctx     context.Context
}

// PostMmapCallback is type of callback after unix.Mmap().
type PostMmapCallback func([]byte) error

// NewReclaimer returns a new instance of Reclaimer.
func NewReclaimer(ctx context.Context) *Reclaimer {
	return &Reclaimer{
		buffers: make([][]byte, 0),
		ctx:     ctx,
	}
}

// allocate eats memory by the given mmap() parameters.  It reads smaps_rollup
// (see ReadSmapsRollup()) and makes sure `keyName` increases after the memory
// allocation.  Otherwise, it returns an error about the allocation failure.
func (r *Reclaimer) allocate(fd, size, prot, flags int, keyName string, postMmapCallback PostMmapCallback) error {
	if size <= 0 {
		return errors.Errorf("invalid size %d", size)
	}

	prev, err := ReadSmapsRollup()
	if err != nil {
		return err
	}

	buf, err := unix.Mmap(fd, 0, size, prot, flags)
	if err != nil {
		return err
	}

	if postMmapCallback != nil {
		err = postMmapCallback(buf)
		if err != nil {
			return err
		}
	}

	if prot&unix.PROT_WRITE != 0 {
		for i := 0; i < size; i += len(randomPage) {
			copy(buf[i:], randomPage[:])
		}
	}

	// Give the system some time to allocate memory.
	// However, this is unreliable.
	err = unix.Msync(buf, unix.MS_SYNC)
	if err != nil {
		return err
	}
	unix.Sync()

	err = testing.Poll(r.ctx, func(ctx context.Context) error {
		cur, err := ReadSmapsRollup()
		if err != nil {
			return err
		}

		if prev[keyName] >= cur[keyName] {
			return errors.Errorf("failed to allocate %s memory: %d >= %d", keyName, prev[keyName], cur[keyName])
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
	if err != nil {
		return err
	}

	r.buffers = append(r.buffers, buf)
	return nil
}

// AllocateAnonymous allocates anonymous memory.
func (r *Reclaimer) AllocateAnonymous(size int) error {
	return r.allocate(-1, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS, "Anonymous", nil)
}

// AllocateShmem allocates shared memory.
func (r *Reclaimer) AllocateShmem(size int) error {
	return r.allocate(-1, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED|unix.MAP_ANONYMOUS, "Pss_Shmem", nil)
}

// isTHPEnabled parses /sys/kernel/mm/transparent_hugepage/enabled.  Return
// true if THP is enabled; otherwise, false.
func isTHPEnabled() bool {
	// A pair of brackets to indicate current setting.
	// Example to parse:
	//   always [madvise] never
	re := regexp.MustCompile(`\[(\w+)\]`)
	thp, err := os.ReadFile("/sys/kernel/mm/transparent_hugepage/enabled")
	if err != nil {
		return false
	}

	// If current THP setting is "never", give up to allocate huge pages.
	m := re.FindStringSubmatch(string(thp))
	if len(m) != 2 || m[1] == "never" {
		return false
	}

	return true
}

// AllocateAnonHugePages allocates anonymous huge page memory.
func (r *Reclaimer) AllocateAnonHugePages(size int) error {
	if !isTHPEnabled() {
		return nil
	}

	return r.allocate(-1, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_PRIVATE|unix.MAP_ANONYMOUS, "AnonHugePages", func(buf []byte) error {
		return unix.Madvise(buf, unix.MADV_HUGEPAGE)
	})
}

// AllocateFileMem allocates file-backed memory.
func (r *Reclaimer) AllocateFileMem(size int) error {
	tmpfile, err := os.CreateTemp("/usr/local/tmp", "")
	if err != nil {
		return err
	}
	defer os.Remove(tmpfile.Name())

	err = tmpfile.Close()
	if err != nil {
		return err
	}

	fd, err := unix.Open(tmpfile.Name(), unix.O_RDWR|unix.O_CREAT|unix.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer unix.Close(fd)

	err = unix.Ftruncate(fd, int64(size))
	if err != nil {
		return err
	}

	return r.allocate(fd, size, unix.PROT_READ|unix.PROT_WRITE, unix.MAP_SHARED, "Pss_File", nil)
}

// reclaim reclaims for each memory type (i.e. "anon", "file", and "shmem") via
// procfs.  It reads smaps_rollup (see ReadSmapsRollup()) and makes sure
// `keyName` decreases after the reclaim.  Otherwise, it returns an error about
// the reclaim failure.
func (r *Reclaimer) reclaim(name, keyName string) error {
	prev, err := ReadSmapsRollup()
	if err != nil {
		return err
	}

	err = os.WriteFile("/proc/self/reclaim", []byte(name), 0)
	if err != nil {
		return err
	}

	// Give the system some time to reclaim memory.
	// However, this is unreliable.
	unix.Sync()

	return testing.Poll(r.ctx, func(ctx context.Context) error {
		cur, err := ReadSmapsRollup()
		if err != nil {
			return err
		}

		if prev[keyName] <= cur[keyName] {
			return errors.Errorf("failed to reclaim %s memory: %d <= %d", keyName, prev[keyName], cur[keyName])
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second})
}

// ReclaimAnonymous reclaims anonymous memory.
func (r *Reclaimer) ReclaimAnonymous() error {
	return r.reclaim("anon", "Anonymous")
}

// ReclaimAnonHugePages reclaims anonymous huge pages.
func (r *Reclaimer) ReclaimAnonHugePages() error {
	if !isTHPEnabled() {
		return nil
	}

	return r.reclaim("anon", "AnonHugePages")
}

// ReclaimFile reclaims file-backed memory.
func (r *Reclaimer) ReclaimFile() error {
	return r.reclaim("file", "Pss_File")
}

// ReclaimShmem reclaims shared memory.
func (r *Reclaimer) ReclaimShmem() error {
	return r.reclaim("shmem", "Pss_Shmem")
}

// FreeAll releases all allocated buffers.
func (r *Reclaimer) FreeAll() {
	for _, buf := range r.buffers {
		unix.Munmap(buf)
	}
}

// ReadSmapsRollup reads /proc/self/smaps_rollup and parses key-values in a map.
// Example content:
//
//	Pss_Dirty:           192 kB
//	Pss_Anon:            192 kB
//	Pss_File:            243 kB
//	Pss_Shmem:             0 kB
//
// It returns a map:
//
//	m["Pss_Dirty"] = 192
//	m["Pss_Anon"] = 192
//	m["Pss_File"] = 243
//	m["Pss_Shmem"] = 0
func ReadSmapsRollup() (map[string]int, error) {
	file, err := os.Open("/proc/self/smaps_rollup")
	if err != nil {
		return nil, errors.Wrap(err, "failed to open")
	}
	defer file.Close()

	m := make(map[string]int)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		kv := strings.Split(scanner.Text(), ":")

		v, err := strconv.Atoi(strings.Fields(kv[1])[0])
		if err != nil {
			return nil, errors.Wrap(err, "failed to atoi")
		}

		m[kv[0]] = v
	}

	return m, scanner.Err()
}
