// +build windows
/*
 * Copyright (c) 2021, Psiphon Inc.
 * All rights reserved.
 *
 * This program is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * This program is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <http://www.gnu.org/licenses/>.
 *
 */
package tun

import (
	"fmt"
	"io"
	"sync"
	"sync/atomic"
	"syscall"

	"github.com/Psiphon-Labs/psiphon-tunnel-core/psiphon/common/errors"
	"golang.org/x/sys/windows"
)

// TODO/miro:
// - add a comment about files being opened in overlapped mode, or implement Windows
//   ReOpenFile function: https://docs.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-reopenfile
// - Fix incomplete offset increments (only using lower 32 bits currently)
// - Check we are returning the standard Reader/Writer errors so the caller can act accordingly

// NonblockingIO provides interruptible I/O for non-pollable
// and/or foreign file descriptors that can't use the netpoller
// available in os.OpenFile as of Go 1.9.
//
// A NonblockingIO wraps a file descriptor in an
// io.ReadWriteCloser interface. The underlying implementation
// uses select and a pipe to interrupt Read and Write calls that
// are blocked when Close is called.
//
// Read and write mutexes allow, for each operation, only one
// concurrent goroutine to call syscalls, preventing an unbounded
// number of OS threads from being created by blocked select
// syscalls.
type NonblockingIO struct {
	closed          int32
	ioFD            syscall.Handle
	readMutex       sync.Mutex
	writeMutex      sync.Mutex
	cancelObject    windows.Handle
	readOverlapped  windows.Overlapped
	writeOverlapped windows.Overlapped
}

// NewNonblockingIO creates a new NonblockingIO with the specified
// file descriptor, which is duplicated and set to nonblocking and
// close-on-exec.
func NewNonblockingIO(ioFD syscall.Handle) (*NonblockingIO, error) {
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	var err error
	newFD := ioFD

	// TODO/miro: re-add file handle duplication

	init := func(fd syscall.Handle) error {
		syscall.CloseOnExec(fd)
		return syscall.SetNonblock(fd, true)
	}
	err = init(newFD)
	if err != nil {
		return nil, errors.Trace(err)
	}

	// TODO/miro: naming events causes them to be re-used, make a note

	cancelObject, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}

	writeHEvent, err := windows.CreateEvent(nil, 1, 1, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	writeOverlapped := windows.Overlapped{
		Internal:     0,
		InternalHigh: 0,
		Offset:       0,
		OffsetHigh:   0,
		HEvent:       writeHEvent,
	}

	readHEvent, err := windows.CreateEvent(nil, 1, 1, nil)
	if err != nil {
		return nil, errors.Trace(err)
	}
	readOverlapped := windows.Overlapped{
		Internal:     0,
		InternalHigh: 0,
		Offset:       0,
		OffsetHigh:   0,
		HEvent:       readHEvent,
	}

	return &NonblockingIO{
		ioFD:            newFD,
		cancelObject:    cancelObject,
		readOverlapped:  readOverlapped,
		writeOverlapped: writeOverlapped,
	}, nil
}

// Read implements the io.Reader interface.
func (nio *NonblockingIO) Read(p []byte) (int, error) {
	nio.readMutex.Lock()
	defer nio.readMutex.Unlock()
	if atomic.LoadInt32(&nio.closed) != 0 {
		return 0, io.EOF
	}
	for {

		fmt.Println(errors.Tracef("Overlapped {internal:%v, internalHigh:%v, offset:%v, offsetHigh:%v}", nio.readOverlapped.Internal, nio.readOverlapped.InternalHigh, nio.readOverlapped.Offset, nio.readOverlapped.OffsetHigh))

		var done uint32

		err := windows.ReadFile(windows.Handle(nio.ioFD), p, &done, &nio.readOverlapped)
		if err != nil && err != windows.ERROR_IO_PENDING {
			return int(done), errors.Trace(err)
		}
		if err == nil {
			if done == 0 {
				continue
			}
			// TODO/miro: this seems is possible https://docs.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-readfile
			return int(done), errors.TraceNew("unexpected synchronous ReadFile result")
		}

		fmt.Println(errors.TraceNew("WaitForMultipleObjects"))

		events := []windows.Handle{nio.cancelObject, nio.readOverlapped.HEvent}
		signalledEventIndex, err := windows.WaitForMultipleObjects(events, false, windows.INFINITE)
		if err != nil {
			return int(done), errors.Trace(err)
		}
		i := signalledEventIndex - windows.WAIT_OBJECT_0
		if i == 0 {
			return 0, io.EOF
		}

		fmt.Println(errors.TraceNew("GetOverlappedResult"))

		// If end-of-file (EOF) is detected during asynchronous operations, the call to GetOverlappedResult
		// for that operation returns FALSE and GetLastError returns ERROR_HANDLE_EOF.
		err = windows.GetOverlappedResult(windows.Handle(nio.ioFD), &nio.readOverlapped, &done, false)
		if err != nil && err == windows.ERROR_IO_PENDING {
			continue
		}

		if err != nil && err != io.EOF && err != windows.ERROR_HANDLE_EOF {
			return int(done), errors.Trace(err)
		}
		fmt.Println(errors.Tracef("Read %d bytes, err %v", done, err))
		fmt.Println(errors.Tracef("OverlappedResult {internal:%v, internalHigh:%v, offset:%v, offsetHigh:%v}", nio.readOverlapped.Internal, nio.readOverlapped.InternalHigh, nio.readOverlapped.Offset, nio.readOverlapped.OffsetHigh))
		if done == 0 && err == nil {
			// https://godoc.org/io#Reader:
			// "Implementations of Read are discouraged from
			// returning a zero byte count with a nil error".
			continue
		}

		nio.readOverlapped.Offset += done

		return int(done), err // TODO/miro: cannot trace errors, such as EOF (check other error returns)
	}
}

// Write implements the io.Writer interface.
func (nio *NonblockingIO) Write(p []byte) (int, error) {
	nio.writeMutex.Lock()
	defer nio.writeMutex.Unlock()
	if atomic.LoadInt32(&nio.closed) != 0 {
		return 0, errors.TraceNew("file already closed")
	}
	n := 0
	t := len(p)
	for n < t {

		fmt.Println(errors.Tracef("OverlappedResult {internal:%v, internalHigh:%v, offset:%v, offsetHigh:%v}", nio.writeOverlapped.Internal, nio.writeOverlapped.InternalHigh, nio.writeOverlapped.Offset, nio.writeOverlapped.OffsetHigh))

		var done uint32

		err := windows.WriteFile(windows.Handle(nio.ioFD), p, &done, &nio.writeOverlapped)
		if err != nil && err != windows.ERROR_IO_PENDING {
			return n, errors.Trace(err)
		}
		if err == nil {
			n += int(done)
			if n < t {
				p = p[done:]
				continue
			}
			// TODO/miro: is this possible?
			return n, errors.TraceNew("unexpected synchronous ReadFile result")
		}

		events := []windows.Handle{nio.cancelObject, nio.writeOverlapped.HEvent}
		signalledEventIndex, err := windows.WaitForMultipleObjects(events, false, windows.INFINITE)
		if err != nil {
			return n, errors.Trace(err)
		}
		i := signalledEventIndex - windows.WAIT_OBJECT_0
		if i == 0 {
			return 0, errors.TraceNew("file has closed") // TODO/miro: is there a standard error to return here?
		}

		err = windows.GetOverlappedResult(windows.Handle(nio.ioFD), &nio.writeOverlapped, &done, false)
		if err != nil && err == windows.ERROR_IO_PENDING {
			continue
		}
		if err != nil {
			return n, errors.Trace(err)
		}

		fmt.Println(errors.Tracef("OverlappedResult {internal:%v, internalHigh:%v, offset:%v, offsetHigh:%v}", nio.writeOverlapped.Internal, nio.writeOverlapped.InternalHigh, nio.writeOverlapped.Offset, nio.writeOverlapped.OffsetHigh))

		nio.writeOverlapped.Offset += done

		// TODO/miro: compare with old code
		n += int(done)
		if n < t {
			p = p[done:]
		}
	}
	return n, nil
}

// IsClosed indicates whether the NonblockingIO is closed.
func (nio *NonblockingIO) IsClosed() bool {
	fmt.Println("Checking is closed...")
	return atomic.LoadInt32(&nio.closed) != 0
}

// Close implements the io.Closer interface.
func (nio *NonblockingIO) Close() error {
	fmt.Println("Closing...")
	if !atomic.CompareAndSwapInt32(&nio.closed, 0, 1) {
		return nil
	}

	// Interrupt any Reads/Writes blocked in Select.
	fmt.Println("Closing raising cancel signal")
	err := windows.SetEvent(nio.cancelObject)
	if err != nil {
		return errors.Trace(err)
	}

	// Lock to ensure concurrent Read/Writes have
	// exited and are no longer using the file
	// descriptors before closing the file descriptors.
	nio.readMutex.Lock()
	defer nio.readMutex.Unlock()
	nio.writeMutex.Lock()
	defer nio.writeMutex.Unlock()
	syscall.Close(nio.ioFD)
	fmt.Println("Closing closed...")
	return nil
}
