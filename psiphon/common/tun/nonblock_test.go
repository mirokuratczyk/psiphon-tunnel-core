// +build darwin linux windows

/*
 * Copyright (c) 2017, Psiphon Inc.
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
	"bytes"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// TODO/miro:
// - Use duplex named pipe or socket?
// - Fix race between reader and writer go routines

func TestNonblockingIO(t *testing.T) {

	// Exercise NonblockingIO Read/Write/Close concurrency
	// and interruption by opening a socket pair and relaying
	// data in both directions. Each side has a reader and a
	// writer, for a total of four goroutines performing
	// concurrent I/O.
	//
	// Reader/writer peers use a common PRNG seed to generate
	// the same stream of bytes to the reader can check that
	// the writer sent the expected stream of bytes.
	//
	// The test is repeated for a number of iterations. For
	// half the iterations, th test wait only for the midpoint
	// of communication, so the Close calls will interrupt
	// active readers and writers. For the other half, wait
	// for the endpoint, so the readers have received all the
	// expected data from the writers and are waiting to read
	// EOF.

	iterations := 10
	maxIO := 32768
	messages := 100 // TODO/miro: was 1000

	for iteration := 0; iteration < iterations; iteration++ {

		fmt.Printf("\n\nIteration %d\n\n\n", iteration)

		var r syscall.Handle
		var w syscall.Handle
		saAttr := syscall.SecurityAttributes{Length: 0}
		saAttr.Length = uint32(unsafe.Sizeof(saAttr))
		saAttr.InheritHandle = 1
		saAttr.SecurityDescriptor = 0

		// TODO/miro: pipe is unidirectional
		// TOOD/miro: Asynchronous (overlapped) read and write operations are not supported by anonymous pipes (https://docs.microsoft.com/en-us/windows/win32/ipc/anonymous-pipe-operations)
		// err := syscall.CreatePipe(&r, &w, &saAttr, 0 /* default buffer size */)
		// if err != nil {
		// 	t.Fatalf("CreatePipe failed: %s", err)
		// 	return
		// }

		w, err := syscall.CreateFile(syscall.StringToUTF16Ptr("nonblock_test.txt"), syscall.GENERIC_WRITE, syscall.FILE_SHARE_READ, nil, syscall.CREATE_ALWAYS, syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAG_OVERLAPPED, 0)
		if err != nil {
			t.Fatalf("CreateFile failed: %s", err)
		}
		r, err = syscall.CreateFile(syscall.StringToUTF16Ptr("nonblock_test.txt"), syscall.GENERIC_READ, syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL|syscall.FILE_FLAG_OVERLAPPED, 0)
		if err != nil {
			t.Fatalf("CreateFile failed: %s", err)
		}

		fds := []syscall.Handle{r, w}

		nio0, err := NewNonblockingIO(fds[0])
		if err != nil {
			t.Fatalf("NewNonblockingIO failed: %s", err)
		}

		nio1, err := NewNonblockingIO(fds[1])
		if err != nil {
			t.Fatalf("NewNonblockingIO failed: %s", err)
		}

		// TODO/miro: need to duplicate file handles in NewNonblockingIO
		// TODO/miro: closes sockets
		// syscall.Close(fds[0])
		// syscall.Close(fds[1])

		readers := new(sync.WaitGroup)
		readersMidpoint := new(sync.WaitGroup)
		readersEndpoint := new(sync.WaitGroup)
		writers := new(sync.WaitGroup)

		reader := func(r io.Reader, h syscall.Handle, isClosed func() bool, seed int) {
			time.Sleep(time.Second * 1) // TODO/miro: hack to keep writer ahead of reader since there is no buffering
			defer readers.Done()

			PRNG := rand.New(rand.NewSource(int64(seed)))

			expectedData := make([]byte, maxIO)
			data := make([]byte, maxIO)

			midpointWaitDone := false

			for i := 0; i < messages; i++ {
				if i%(messages/10) == 0 {
					fmt.Printf("#%d: %d/%d\n", seed, i, messages)
				}
				if i == messages/2 {
					readersMidpoint.Done()
					midpointWaitDone = true
				}
				n := int(1 + PRNG.Int31n(int32(maxIO)))
				PRNG.Read(expectedData[:n])
				n, err := io.ReadFull(r, data[:n])
				if err != nil {
					if isClosed() {
						if !midpointWaitDone {
							readersMidpoint.Done()
						}
						readersEndpoint.Done()
						return
					}
					t.Errorf("io.ReadFull failed: %s", err)
					if !midpointWaitDone {
						readersMidpoint.Done()
					}
					readersEndpoint.Done()
					return
				}
				if !bytes.Equal(expectedData[:n], data[:n]) {
					t.Errorf("bytes.Equal failed")
					if !midpointWaitDone {
						readersMidpoint.Done()
					}
					readersEndpoint.Done()
					return
				}
			}

			readersEndpoint.Done()

			n, err := r.Read(data)
			for n == 0 && err == nil {
				n, err = r.Read(data)
			}
			// TODO/miro: this occasionally fails meaning there is unexpected
			// extra data to read

			if n != 0 || (err != io.EOF && err != syscall.ERROR_HANDLE_EOF) {
				t.Errorf("expected io.EOF failed: n %d, err %v", n, err)
				if !midpointWaitDone {
					readersMidpoint.Done()
				}
				return
			}
		}

		writer := func(w io.Writer, h syscall.Handle, isClosed func() bool, seed int) {
			defer writers.Done()

			PRNG := rand.New(rand.NewSource(int64(seed)))

			data := make([]byte, maxIO)

			for i := 0; i < messages; i++ {
				n := int(1 + PRNG.Int31n(int32(maxIO)))
				PRNG.Read(data[:n])
				m, err := w.Write(data[:n])
				if err != nil {
					if isClosed() {
						return
					}
					t.Errorf("w.Write failed: %s", err)
					return
				}
				if m != n {
					t.Errorf("w.Write failed: unexpected number of bytes written")
					return
				}
			}
		}

		isClosed := func() bool {
			c := nio0.IsClosed() || nio1.IsClosed()
			return c
		}

		readers.Add(1)
		readersMidpoint.Add(1)
		readersEndpoint.Add(1)
		go reader(nio0, r, isClosed, 0)
		// go reader(nio1, isClosed, 1)

		writers.Add(1)
		// go writer(nio0, isClosed, 1)
		go writer(nio1, w, isClosed, 0)

		fmt.Println("Waiting for midpoint...")
		readersMidpoint.Wait()

		if iteration%2 == 0 {
			fmt.Println("Waiting for endpoint...")
			readersEndpoint.Wait()
		}

		fmt.Println("Closing nio...")
		nio0.Close()
		nio1.Close()

		fmt.Println("Closing writers...")
		writers.Wait()
		fmt.Println("Closing readers...")
		readers.Wait()
	}
}
