// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"io"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/dutcontrol"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DUTControlPort is a dutcontrol console port.
type DUTControlPort struct {
	stream      dutcontrol.DutControl_ConsoleClient
	data        <-chan *dutcontrol.ConsoleSerialData
	written     <-chan *dutcontrol.ConsoleSerialWriteResult
	readTimeout time.Duration
	unreadBuf   []byte
}

func (p *DUTControlPort) drainNonBlocking(discard bool) error {
	var err error
	for {
		select {
		case d, more := <-p.data:
			if d != nil && !discard {
				p.unreadBuf = append(p.unreadBuf, d.Data...)
			}
			if !more {
				return io.EOF
			}
			if err == nil && d != nil && d.Err != "" {
				err = errors.New(d.Err)
			}
		default:
			return err
		}
	}
}

// Read bytes into buffer and return number of bytes read.
// Bytes already written to the port shall be moved into buf, up to its size.
func (p *DUTControlPort) Read(ctx context.Context, buf []byte) (n int, err error) {
	// Consume all waiting ConsoleSerialData without blocking.
	err = p.drainNonBlocking(false)

	// If we have at least 1 byte unread, return without blocking.
	if len(p.unreadBuf) > 0 || err != nil {
		n := copy(buf, p.unreadBuf)
		p.unreadBuf = p.unreadBuf[n:]
		return n, err
	}

	// Nothing unread, so wait up to readTimeout to get at least 1 byte.
	ctx, cancel := context.WithTimeout(ctx, p.readTimeout)
	defer cancel()

	select {
	case d, more := <-p.data:
		if d != nil {
			p.unreadBuf = append(p.unreadBuf, d.Data...)
		}
		if !more {
			err = io.EOF
		} else if d != nil && d.Err != "" {
			err = errors.New(d.Err)
		}
		n := copy(buf, p.unreadBuf)
		p.unreadBuf = p.unreadBuf[n:]
		return n, err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Write bytes from buffer and return number of bytes written.
// It returns a non-nil error when n != len(b), nil otherwise.
func (p *DUTControlPort) Write(ctx context.Context, buf []byte) (int, error) {
	err := p.stream.Send(&dutcontrol.ConsoleRequest{
		Operation: &dutcontrol.ConsoleRequest_SerialWrite{
			SerialWrite: &dutcontrol.ConsoleSerialWrite{
				Data: buf,
			},
		},
	})

	if err != nil {
		return 0, err
	}

	select {
	case d, more := <-p.written:
		if !more {
			return 0, io.EOF
		}
		if d.Err != "" {
			err = errors.New(d.Err)
		}
		return int(d.N), err
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

const flushQuietWindow = 20 * time.Millisecond

// Flush discards unreadBuf and drains pending ConsoleSerialData chunks from
// the data channel. When readTimeout > 0, Flush continues draining until no
// new chunk arrives within min(20ms, readTimeout) (bounded by ctx) so that
// chunks already in flight from devboardsvc/HyperDebug are also discarded.
func (p *DUTControlPort) Flush(ctx context.Context) error {
	p.unreadBuf = nil
	if err := p.drainNonBlocking(true); err != nil || p.readTimeout <= 0 {
		return err
	}

	quietWindow := min(flushQuietWindow, p.readTimeout)
	timer := time.NewTimer(quietWindow)
	defer timer.Stop()

	var err error
	for {
		select {
		case d, more := <-p.data:
			if !more {
				return io.EOF
			}
			if err == nil && d != nil && d.Err != "" {
				err = errors.New(d.Err)
			}
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(quietWindow)
		case <-timer.C:
			return err
		case <-ctx.Done():
			if err != nil {
				return err
			}
			return ctx.Err()
		}
	}
}

// Close closes the port.
func (p *DUTControlPort) Close(ctx context.Context) error {
	testing.ContextLog(ctx, "Closing DUTControlPort")
	if err := p.stream.CloseSend(); err != nil {
		return err
	}
	for {
		select {
		case _, more := <-p.written:
			if !more {
				// Gorotine has closed the channel, meaning that it has also
				// finished writing transcript log to disk.
				return nil
			}
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}
