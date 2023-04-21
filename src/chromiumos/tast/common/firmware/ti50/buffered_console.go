// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"os"
	"path/filepath"
	"regexp"

	"chromiumos/tast/common/firmware/serial"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// BufferedConsole represents a UART console that can be read or written to.
type BufferedConsole struct {
	filename              string
	targetBufferUnread    []byte
	targetBufferUnreadLen int
	portOpener            serial.PortOpener
	port                  serial.Port
	logfile               *os.File
}

// NewBufferedConsole returns a new buffered console.
func NewBufferedConsole(filename string, bufMax int, portOpener serial.PortOpener) *BufferedConsole {
	return &BufferedConsole{
		filename:           filename,
		targetBufferUnread: make([]byte, bufMax),
		portOpener:         portOpener,
	}
}

// Open opens the console port.
func (c *BufferedConsole) Open(ctx context.Context) error {
	if c.port != nil {
		return nil
	}
	p, err := c.portOpener.OpenPort(ctx)
	if err != nil {
		return err
	}
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		c.port.Close(ctx)
		return errors.New("failed to get directory for saving files")
	}
	f, err := os.OpenFile(filepath.Join(dir, "andreiboard.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		c.port.Close(ctx)
		return err
	}
	c.port = p
	c.logfile = f
	return nil
}

// IsOpen returns true iff the port is open.
func (c *BufferedConsole) IsOpen() bool {
	return c.port != nil
}

// Close closes the console port.
func (c *BufferedConsole) Close(ctx context.Context) error {
	if c.logfile != nil {
		c.logfile.Close()
		c.logfile = nil
	}
	if c.port != nil {
		err := c.port.Close(ctx)
		c.port = nil
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *BufferedConsole) appendToLogFile(ctx context.Context, buf []byte) error {
	if c.logfile == nil {
		return errors.New("Logfile not opened")
	}
	_, err := c.logfile.Write(buf)
	return err
}

// ReadSerialSubmatch reads from the serial port until regex is matched.
func (c *BufferedConsole) ReadSerialSubmatch(ctx context.Context, re *regexp.Regexp) (output [][]byte, err error) {
	if err := c.Open(ctx); err != nil {
		return nil, errors.Wrap(err, "port open error")
	}

	buf := make([]byte, len(c.targetBufferUnread))
	total := copy(buf, c.targetBufferUnread[:c.targetBufferUnreadLen])
	for {
		indices := re.FindSubmatchIndex(buf[:total])
		if indices != nil {
			c.targetBufferUnreadLen = copy(c.targetBufferUnread, buf[indices[1]:total])
			return re.FindSubmatch(buf[:total]), nil
		}
		if total == len(c.targetBufferUnread) {
			c.targetBufferUnreadLen = copy(c.targetBufferUnread, buf)
			return nil, errors.Errorf("buffer is full (wanted %s)", re)
		}
		current, err := c.port.Read(ctx, buf[total:])
		if current > 0 {
			if err := c.appendToLogFile(ctx, buf[total:total+current]); err != nil {
				testing.ContextLog(ctx, "Log file error: ", err)
			}
		}
		total += current
		if err != nil {
			c.targetBufferUnreadLen = copy(c.targetBufferUnread, buf[:total])
			return nil, errors.Wrapf(err, "port read error (wanted %s)", re)
		}
		if current == 0 {
			break
		}
	}

	c.targetBufferUnreadLen = copy(c.targetBufferUnread, buf[:total])
	return nil, errors.New("failed to find match")
}

// ReadSerialBytes reads from the serial port until number of bytes have been read.
func (c *BufferedConsole) ReadSerialBytes(ctx context.Context, size int) (output []byte, err error) {
	if err := c.Open(ctx); err != nil {
		return nil, errors.Wrap(err, "port open error")
	}

	for {
		if c.targetBufferUnreadLen >= size {
			buf := make([]byte, size)
			copy(buf, c.targetBufferUnread)
			// Remove the buffered data we are sending to caller
			c.targetBufferUnreadLen = copy(c.targetBufferUnread, c.targetBufferUnread[size:c.targetBufferUnreadLen])
			return buf, nil
		}
		// Try to read from port since we don't have enough data yet
		current, err := c.port.Read(ctx, c.targetBufferUnread[c.targetBufferUnreadLen:])
		if current > 0 {
			if err := c.appendToLogFile(ctx, c.targetBufferUnread[c.targetBufferUnreadLen:c.targetBufferUnreadLen+current]); err != nil {
				testing.ContextLog(ctx, "Log file error: ", err)
			}
		}
		c.targetBufferUnreadLen += current
		if err != nil {
			return nil, errors.Wrap(err, "failed to get specified number of bytes")
		}
		if current == 0 {
			return nil, errors.New("failed to get specified number of bytes")
		}
	}
}

// ClearInput clears any pending input that hasn't been read yet.
func (c *BufferedConsole) ClearInput(ctx context.Context) error {
	c.targetBufferUnreadLen = 0
	if err := c.Open(ctx); err != nil {
		return errors.Wrap(err, "port open error")
	}
	for {
		// Try to read from port since we don't have enough data yet
		current, err := c.port.Read(ctx, c.targetBufferUnread)
		if current > 0 {
			if err := c.appendToLogFile(ctx, c.targetBufferUnread[:current]); err != nil {
				testing.ContextLog(ctx, "Log file error: ", err)
			}
		}
		if err != nil {
			return errors.Wrap(err, "failed to clear input")
		}
		if current == 0 {
			return nil
		}
	}
}

// WriteSerial writes to the serial port.
func (c *BufferedConsole) WriteSerial(ctx context.Context, b []byte) error {
	if err := c.Open(ctx); err != nil {
		return err
	}
	n, err := c.port.Write(ctx, b)

	if err != nil {
		return err
	}

	if n != len(b) {
		return errors.Errorf("not all bytes written, got %d, want %d", n, len(b))
	}
	return nil
}

// FlushSerial flushes un-read/written chars.
func (c *BufferedConsole) FlushSerial(ctx context.Context) error {
	c.targetBufferUnreadLen = 0
	if c.port != nil {
		err := c.port.Flush(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}
