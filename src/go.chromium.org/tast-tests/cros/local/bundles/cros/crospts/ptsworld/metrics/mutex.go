// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

const mutexTitle = "BenchmarkMutex"

// mutexCrosboltNameTable maps the PTS description to crosbolt name.
var mutexCrosboltNameTable = map[string]string{
	"Benchmark: Shared Mutex Lock Shared":        "Shared_Mutex_Lock_Shared",
	"Benchmark: Mutex Lock Unlock spinlock":      "Mutex_Lock_Unlock_spinlock",
	"Benchmark: Mutex Lock Unlock std::mutex":    "Mutex_Lock_Unlock_std_mutex",
	"Benchmark: Semaphore Release And Acquire":   "Semaphore_Release_And_Acquire",
	"Benchmark: Mutex Lock Unlock pthread_mutex": "Mutex_Lock_Unlock_pthread_mutex",
}
