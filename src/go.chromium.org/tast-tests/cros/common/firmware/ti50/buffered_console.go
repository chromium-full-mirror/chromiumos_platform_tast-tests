// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/serial"
	"go.chromium.org/tast/core/errors"
)

// BufferedConsole represents a UART console that can be read or written to.
type BufferedConsole struct {
	readBuf    []byte
	readBufLen int
	portOpener serial.PortOpener
	port       serial.Port
}

// NewBufferedConsole returns a new buffered console.
func NewBufferedConsole(bufMax int, portOpener serial.PortOpener) *BufferedConsole {
	return &BufferedConsole{
		readBuf:    make([]byte, bufMax),
		portOpener: portOpener,
	}
}

// Open opens the console port.
func (c *BufferedConsole) Open(ctx context.Context) error {
	if c.port != nil {
		return errors.New("BufferedConsole already open")
	}
	p, err := c.portOpener.OpenPort(ctx)
	if err != nil {
		return err
	}
	c.port = p
	return nil
}

// IsOpen returns true iff the port is open.
func (c *BufferedConsole) IsOpen() bool {
	return c.port != nil
}

// Close closes the console port.
func (c *BufferedConsole) Close(ctx context.Context) error {
	// Pull everything from console we can and write to file, but ignore error if encountered
	_ = c.ClearInput(ctx)

	if c.port != nil {
		err := c.port.Close(ctx)
		c.port = nil
		if err != nil {
			return err
		}
	}
	return nil
}

func (c *BufferedConsole) readSerial(ctx context.Context) error {
	if c.port == nil {
		return errors.New("BufferedConsole not open")
	}
	if c.readBufLen == len(c.readBuf) {
		return errors.New("buffer full")
	}
	n, err := c.port.Read(ctx, c.readBuf[c.readBufLen:])
	c.readBufLen += n
	if err != nil {
		return errors.Wrap(err, "port read error")
	}
	if n == 0 {
		return errors.New("read nothing")
	}
	return nil
}

// ReadSerialSubmatch reads from the port until one regex is matched.  In case multiple
// regexs match, the first (leftmost) one will be returned.
func (c *BufferedConsole) ReadSerialSubmatch(ctx context.Context, regularExpressions ...*regexp.Regexp) (whichRegularExpression int, output [][]byte, err error) {
	for {
		for i := 0; i < len(regularExpressions); i++ {
			re := regularExpressions[i]
			indices := re.FindSubmatchIndex(c.readBuf[:c.readBufLen])
			if indices != nil {
				buf := make([]byte, indices[1])
				copy(buf, c.readBuf[:indices[1]])
				c.readBufLen = copy(c.readBuf, c.readBuf[indices[1]:c.readBufLen])
				return i, re.FindSubmatch(buf), nil
			}
		}
		err := c.readSerial(ctx)
		if err != nil {
			// Report the last regex since the first regex is most likely the
			// regex to detect unexpected panics and resets.
			return 0, nil, errors.Wrapf(err, "(wanted %s)", regularExpressions[len(regularExpressions)-1])
		}
	}
}

// ReadSerialBytes reads from the serial port until number of bytes have been read.
func (c *BufferedConsole) ReadSerialBytes(ctx context.Context, size int) (output []byte, err error) {
	for {
		if c.readBufLen >= size {
			buf := make([]byte, size)
			copy(buf, c.readBuf)
			// Remove the buffered data we are sending to caller
			c.readBufLen = copy(c.readBuf, c.readBuf[size:c.readBufLen])
			return buf, nil
		}
		// Try to read from port since we don't have enough data yet
		err := c.readSerial(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get specified number of bytes")
		}
	}
}

// ClearInput clears any pending input that hasn't been read yet.
func (c *BufferedConsole) ClearInput(ctx context.Context) error {
	ctx2, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	err := c.readSerial(ctx2)
	c.readBufLen = 0
	if errors.Is(err, context.DeadlineExceeded) {
		// Return nil if it was our ctx2 that timed out.
		return ctx.Err()
	}
	return err
}

// WriteSerial writes to the serial port.
func (c *BufferedConsole) WriteSerial(ctx context.Context, b []byte) error {
	if c.port == nil {
		return errors.New("BufferedConsole not open")
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
	c.readBufLen = 0
	if c.port != nil {
		err := c.port.Flush(ctx)
		if err != nil {
			return err
		}
	}
	return nil
}
