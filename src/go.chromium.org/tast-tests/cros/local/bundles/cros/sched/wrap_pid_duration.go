// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sched

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const minDurationToWrapPIDSpace = 5 * time.Second

const rotateThreadScript = `import queue
import threading
import time

q = queue.Queue(maxsize=1024)

def thread_fn():
	q.put(threading.current_thread())

def reap_thread_fn():
	while True:
		thread = q.get()
		thread.join()

# Use another thread to reap threads to avoid extra conditional branches in the
# busy main loop.
reap_thread = threading.Thread(target=reap_thread_fn, daemon=True)
reap_thread.start()

timeout_event = threading.Event()
def timeout_fn():
	# If it takes minDurationToWrapPIDSpace + 1 to wrap PID space, it is large
	# enough.
	time.sleep(%f)
	timeout_event.set()

previous_id = reap_thread.native_id

start_time_ns = time.monotonic_ns()

# Use another thread to monitor timeout to avoid checking monotonic time in the
# busy main loop.
threading.Thread(target=timeout_fn, daemon=True).start()

pid_wrap_observed = False

while True:
	thread = threading.Thread(target=thread_fn, daemon=True)
	thread.start()
	if timeout_event.is_set():
		break
	elif not pid_wrap_observed:
		pid_wrap_observed = thread.native_id <= previous_id
	elif thread.native_id >= reap_thread.native_id or thread.native_id <= previous_id:
		break
	previous_id = thread.native_id

end_time_ns = time.monotonic_ns()

print(end_time_ns - start_time_ns)
`

func init() {
	testing.AddTest(&testing.Test{
		Func:         WrapPidDuration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure how long it takes to wrap the PID space by spawning threads",
		Contacts:     []string{"cros-core-systems-perf@google.com", "kawasin@google.com"},
		BugComponent: "b:167279", // ChromeOS > Platform > baseOS > Performance
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational"},
		Timeout:      40 * time.Second,
	})
}

func runPythonScript(ctx context.Context, script []byte) (string, error) {
	cmd := exec.CommandContext(ctx, "python")
	out := new(strings.Builder)

	cmd.Stdout = out

	pipe, err := cmd.StdinPipe()
	if err != nil {
		return "", errors.Wrap(err, "failed to create stdin pipe")
	}
	defer pipe.Close()

	n, err := pipe.Write(script)
	if err != nil {
		return "", errors.Wrap(err, "failed to write to stdin")
	} else if n != len(script) {
		return "", errors.New("failed to write all bytes to stdin")
	} else if err = pipe.Close(); err != nil {
		return "", errors.Wrap(err, "failed to close stdin")
	}

	if err := cmd.Start(); err != nil {
		return "", errors.Wrap(err, "failed to start python")
	}

	if err := cmd.Wait(); err != nil {
		return "", errors.Wrap(err, "failed to wait for python")
	}

	return out.String(), nil
}

func WrapPidDuration(ctx context.Context, s *testing.State) {
	// Python is not the best language to measure the duration. However Python is
	// the only available language to examine because Go does not allow spawning
	// threads and CGO is not allowed in tast test.
	output, err := runPythonScript(ctx, []byte(fmt.Sprintf(rotateThreadScript, minDurationToWrapPIDSpace.Seconds()+1)))
	if err != nil {
		s.Fatal("run python script: ", err)
	}

	durationNs, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil {
		s.Fatal("parse duration: ", err)
	}

	duration := time.Duration(durationNs) * time.Nanosecond
	if duration <= minDurationToWrapPIDSpace {
		s.Fatal("took too short to wrap PID space: ", duration)
	}

	s.Logf("Took %v to wrap PID space", duration)
}
