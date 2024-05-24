// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ti50

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/serial"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/dutcontrol"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// ConsoleUart is the rawuart port.
	ConsoleUart = "console"
	// EcUart is the raw uart name for EC.
	EcUart = "EC"
	// UartBaud is the baud rate to use to access all raw uarts.
	UartBaud = 115200

	// qSize is the channel size for the data and write receive channels, it should be large enough to not ever block on writes so that one channel does not block the other.
	qSize = 100000
)

// DUTControlRawUARTPortOpener opens a raw UART port through the dutcontrol grpc client.
//
// Example:
// conn, err := grpc.DialContext(ctx, hostPort, grpc.WithInsecure())
//
//	if err != nil {
//	    return nil, err
//	}
//
// defer conn.Close(ctx)
// client := dutcontrol.NewDutControlClient(conn)
// opener := &DUTControlRawUARTPortOpener(client, "console", 115200, 1024, 200 * time.Millisecond)
// port, err := opener.OpenPort(ctx)
type DUTControlRawUARTPortOpener struct {
	// The dutcontrol grpc client.
	Client dutcontrol.DutControlClient
	// The uart number or an alias defined in the OpenTitanTool conf json file.
	Uart string
	// The baud rate.
	Baud int
	// The max size of received serial data.
	DataLen int
	// The timeout for a read operation.
	ReadTimeout time.Duration
	// The name of a log file for all data in the receive direction.
	LogName string
}

// DUTControlCCDPortOpener opens a CCD serial interface through the dutcontrol grpc client.
type DUTControlCCDPortOpener struct {
	// The dutcontrol grpc client.
	Client dutcontrol.DutControlClient
	// The uart number or an alias defined in the OpenTitanTool conf json file.
	Ep dutcontrol.CCDSerialEndPoint
	// The baud rate.
	Baud int
	// The max size of received serial data.
	DataLen int
	// The timeout for a read operation.
	ReadTimeout time.Duration
	// The name of a log file for all data in the receive direction.
	LogName string
}

// openDUTControlConsole opens a console and returns its data and write result receive channels.
func openDUTControlConsole(stream dutcontrol.DutControl_ConsoleClient, dir, filename string, req *dutcontrol.ConsoleRequest) (<-chan *dutcontrol.ConsoleSerialData, <-chan *dutcontrol.ConsoleSerialWriteResult, error) {
	var logfile *os.File
	if filename != "" {
		f, err := os.OpenFile(filepath.Join(dir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return nil, nil, errors.Wrap(err, "error opening log file")
		}
		logfile = f
	}
	if err := stream.Send(req); err != nil {
		return nil, nil, errors.Wrap(err, "send request")
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, nil, errors.Wrap(err, "recv open")
	}
	open := resp.GetOpen()
	if open == nil {
		return nil, nil, errors.New("open response is nil")
	}
	if open.Err != "" {
		return nil, nil, errors.New(string(open.Err))
	}
	data := make(chan *dutcontrol.ConsoleSerialData, qSize)
	write := make(chan *dutcontrol.ConsoleSerialWriteResult, qSize)
	go func() {
		addTs := true
	Loop:
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				testing.ContextLog(stream.Context(), "Dutcontrol recv EOF")
				break
			} else if err != nil {
				break
			}
			switch op := resp.Type.(type) {
			case *dutcontrol.ConsoleResponse_SerialData:
				if logfile != nil {
					ts := time.Now().UTC()
					tspf := []byte(ts.Format("2006-01-02T15:04:05.000Z") + " ")
					data := bytes.ReplaceAll(op.SerialData.Data, []byte("\x00"), []byte("\\x00"))
					for _, line := range bytes.SplitAfter(data, []byte("\n")) {
						if len(line) == 0 {
							break
						}
						if addTs {
							line = append(tspf, line...)
						}
						addTs = bytes.HasSuffix(line, []byte("\n"))
						logfile.Write(line)
					}
				}
				if len(data) == qSize {
					testing.ContextLog(stream.Context(), "WARNING: Dutcontrol data queue full, could block future operations")
				}
				data <- op.SerialData
			case *dutcontrol.ConsoleResponse_SerialWrite:
				if len(write) == qSize {
					testing.ContextLog(stream.Context(), "WARNING: Dutcontrol write queue full, could block future operations")
				}
				write <- op.SerialWrite
			default:
				testing.ContextLog(stream.Context(), "Dutcontrol recv error, unknown message type: ", op)
				break Loop
			}
		}
		if logfile != nil {
			logfile.Close()
		}
		close(data)
		close(write)
	}()
	return data, write, nil
}

// OpenPort opens and returns the port.
func (c *DUTControlRawUARTPortOpener) OpenPort(ctx context.Context) (serial.Port, error) {
	stream, err := c.Client.Console(ctx)
	if err != nil {
		return nil, err
	}
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get directory for saving files")
	}

	data, write, err := openDUTControlConsole(stream,
		dir,
		c.LogName,
		&dutcontrol.ConsoleRequest{
			Operation: &dutcontrol.ConsoleRequest_Open{
				Open: &dutcontrol.ConsoleOpen{
					Type: &dutcontrol.ConsoleOpen_RawUart{RawUart: &dutcontrol.ConsoleOpenRawUART{Uart: c.Uart, Baud: int32(c.Baud), DataLen: int32(c.DataLen)}},
				},
			}})
	if err != nil {
		return nil, err
	}

	return &DUTControlPort{stream, data, write, c.ReadTimeout, nil}, nil
}

// OpenPort opens and returns the port.
func (c *DUTControlCCDPortOpener) OpenPort(ctx context.Context) (serial.Port, error) {
	stream, err := c.Client.Console(ctx)
	if err != nil {
		return nil, err
	}
	dir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get directory for saving files")
	}

	data, write, err := openDUTControlConsole(stream,
		dir,
		c.LogName,
		&dutcontrol.ConsoleRequest{
			Operation: &dutcontrol.ConsoleRequest_Open{
				Open: &dutcontrol.ConsoleOpen{
					Type: &dutcontrol.ConsoleOpen_CcdSerial{CcdSerial: &dutcontrol.ConsoleOpenCCDSerial{Ep: c.Ep, Baud: int32(c.Baud), DataLen: int32(c.DataLen)}},
				},
			}})
	if err != nil {
		return nil, err
	}

	return &DUTControlPort{stream, data, write, c.ReadTimeout, nil}, nil
}
