// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/dutcontrol"
)

func TestDUTControlPortFlushDropsUnreadAndQueuedChunks(t *testing.T) {
	ctx := context.Background()
	ch := make(chan *dutcontrol.ConsoleSerialData, 4)
	port := &DUTControlPort{
		data:        ch,
		readTimeout: 50 * time.Millisecond,
		unreadBuf:   []byte("stale-unread"),
	}

	ch <- &dutcontrol.ConsoleSerialData{Data: []byte("chunk1")}
	ch <- &dutcontrol.ConsoleSerialData{Data: []byte("chunk2")}

	if err := port.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if len(port.unreadBuf) != 0 {
		t.Fatalf("unreadBuf not cleared: got %q", port.unreadBuf)
	}
	if len(ch) != 0 {
		t.Fatalf("channel not drained: %d items remaining", len(ch))
	}

	// Queue fresh data after Flush and verify Read sees only the post-Flush data.
	ch <- &dutcontrol.ConsoleSerialData{Data: []byte("after-flush")}
	buf := make([]byte, 32)
	n, err := port.Read(ctx, buf)
	if err != nil {
		t.Fatalf("Read after Flush failed: %v", err)
	}
	if !bytes.Equal(buf[:n], []byte("after-flush")) {
		t.Fatalf("Read after Flush = %q, want %q", buf[:n], "after-flush")
	}
}

func TestDUTControlPortFlushEmptyChannel(t *testing.T) {
	ctx := context.Background()
	ch := make(chan *dutcontrol.ConsoleSerialData, 1)
	port := &DUTControlPort{
		data: ch,
	}

	if err := port.Flush(ctx); err != nil {
		t.Fatalf("Flush on empty channel returned error: %v", err)
	}
}

func TestDUTControlPortFlushClosedChannelReturnsEOF(t *testing.T) {
	ctx := context.Background()
	ch := make(chan *dutcontrol.ConsoleSerialData, 2)
	ch <- &dutcontrol.ConsoleSerialData{Data: []byte("before-close")}
	close(ch)

	port := &DUTControlPort{
		data:      ch,
		unreadBuf: []byte("unread"),
	}

	if err := port.Flush(ctx); err != io.EOF {
		t.Fatalf("Flush on closed channel = %v, want io.EOF", err)
	}
	if len(port.unreadBuf) != 0 {
		t.Fatalf("unreadBuf not cleared on closed channel: got %q", port.unreadBuf)
	}
}

func TestDUTControlPortChunkErrorReturnedByReadAndFlush(t *testing.T) {
	ctx := context.Background()

	// Verify Read returns chunk error alongside data.
	chRead := make(chan *dutcontrol.ConsoleSerialData, 2)
	chRead <- &dutcontrol.ConsoleSerialData{Data: []byte("abc"), Err: "uart framing error"}
	portRead := &DUTControlPort{
		data:        chRead,
		readTimeout: 50 * time.Millisecond,
	}
	buf := make([]byte, 16)
	n, err := portRead.Read(ctx, buf)
	if err == nil || err.Error() != "uart framing error" {
		t.Fatalf("Read error = %v, want 'uart framing error'", err)
	}
	if string(buf[:n]) != "abc" {
		t.Fatalf("Read data = %q, want 'abc'", buf[:n])
	}

	// Verify Flush returns chunk error and discards queued data.
	chFlush := make(chan *dutcontrol.ConsoleSerialData, 2)
	chFlush <- &dutcontrol.ConsoleSerialData{Data: []byte("abc"), Err: "uart overrun"}
	portFlush := &DUTControlPort{
		data:        chFlush,
		readTimeout: 50 * time.Millisecond,
	}
	if err := portFlush.Flush(ctx); err == nil || err.Error() != "uart overrun" {
		t.Fatalf("Flush error = %v, want 'uart overrun'", err)
	}
}

func TestDUTControlPortFlushDrainsInFlightChunksDuringQuietWindow(t *testing.T) {
	ctx := context.Background()
	ch := make(chan *dutcontrol.ConsoleSerialData, 4)
	port := &DUTControlPort{
		data:        ch,
		readTimeout: 100 * time.Millisecond,
	}

	go func() {
		<-time.After(5 * time.Millisecond)
		ch <- &dutcontrol.ConsoleSerialData{Data: []byte("in-flight-1")}
		<-time.After(5 * time.Millisecond)
		ch <- &dutcontrol.ConsoleSerialData{Data: []byte("in-flight-2")}
	}()

	if err := port.Flush(ctx); err != nil {
		t.Fatalf("Flush failed: %v", err)
	}
	if len(ch) != 0 {
		t.Fatalf("expected in-flight chunks to be drained, got %d remaining", len(ch))
	}
}
