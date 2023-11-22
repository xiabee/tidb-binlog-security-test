// Copyright 2019 PingCAP, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package storage

import (
	"math/rand"
	"os"
	"path"
	"strconv"
	"syscall"
	"time"

	fuzz "github.com/google/gofuzz"
	"github.com/pingcap/check"
	pb "github.com/pingcap/tipb/go-binlog"
)

func init() {
	rand.Seed(time.Now().Unix())
}

type VlogSuit struct{}

var _ = check.Suite(&VlogSuit{})

func randRequest() *request {
	var ts int64
	f := fuzz.New().NumElements(1, 20).NilChance(0)
	f.Fuzz(&ts)
	binlog := pb.Binlog{
		StartTs: ts,
		Tp:      pb.BinlogType_Prewrite,
	}
	payload, err := binlog.Marshal()
	if err != nil {
		panic(err)
	}
	return &request{
		startTS: ts,
		payload: payload,
		tp:      pb.BinlogType_Prewrite,
	}
}

func newVlog(c *check.C) *valueLog {
	return newVlogWithOptions(c, DefaultOptions())
}

func newVlogWithOptions(c *check.C, options *Options) *valueLog {
	var err error

	dir := path.Join(os.TempDir(), strconv.Itoa(rand.Int()))
	c.Log("use dir: ", dir)
	err = os.Mkdir(dir, 0777)
	c.Assert(err, check.IsNil)

	vlog, err := newValueLog(dir, options)
	c.Assert(err, check.IsNil)

	return vlog
}

func (vs *VlogSuit) TestOpenEmpty(c *check.C) {
	vlog := newVlog(c)
	defer os.RemoveAll(vlog.dirPath)
}

func (vs *VlogSuit) TestSingleWriteRead(c *check.C) {
	vlog := newVlog(c)
	defer os.RemoveAll(vlog.dirPath)

	req := randRequest()
	err := vlog.write([]*request{req})
	c.Assert(err, check.IsNil)

	payload, err := vlog.readValue(req.valuePointer)
	c.Assert(err, check.IsNil)

	c.Assert(req.payload, check.DeepEquals, payload, check.Commentf("data read back not equal"))
}

func (vs *VlogSuit) TestBatchWriteRead(c *check.C) {
	testBatchWriteRead(c, 1, DefaultOptions())

	testBatchWriteRead(c, 128, DefaultOptions())

	// set small valueLogFileSize, so we can test multi log file case
	testBatchWriteRead(c, 1024, DefaultOptions().WithValueLogFileSize(3000))
}

func testBatchWriteRead(c *check.C, reqNum int, options *Options) {
	vlog := newVlogWithOptions(c, options)
	defer os.RemoveAll(vlog.dirPath)

	reqs := make([]*request, 0, reqNum)
	for i := 0; i < reqNum; i++ {
		reqs = append(reqs, randRequest())
	}

	err := vlog.write(reqs)
	c.Assert(err, check.IsNil)

	for _, req := range reqs {
		payload, err := vlog.readValue(req.valuePointer)
		c.Assert(err, check.IsNil)

		c.Assert(req.payload, check.DeepEquals, payload, check.Commentf("data read back not equal"))
	}

	// test scan start at the middle point of request
	idx := len(reqs) / 2
	err = vlog.scan(reqs[idx].valuePointer, func(vp valuePointer, record *Record) error {
		c.Assert(record.payload, check.DeepEquals, reqs[idx].payload, check.Commentf("data read back not equal"))
		idx++
		return nil
	})
	c.Assert(err, check.IsNil)
}

func (vs *VlogSuit) TestCloseAndOpen(c *check.C) {
	vlog := newVlogWithOptions(c, DefaultOptions().WithValueLogFileSize(100))
	defer os.RemoveAll(vlog.dirPath)

	dirPath := vlog.dirPath
	opt := vlog.opt

	n := 10
	reqs := make([]*request, 0, n*3)
	batch := make([]*request, 0, 3)
	for i := 0; i < n; i++ {
		// close and open back every time
		var err = vlog.close()
		c.Assert(err, check.IsNil)

		vlog, err = newValueLog(dirPath, opt)
		c.Assert(err, check.IsNil)

		batch = batch[:0]
		// write a few request
		for j := 0; j < 3; j++ {
			req := randRequest()
			batch = append(batch, req)
		}
		err = vlog.write(batch)
		c.Assert(err, check.IsNil)
		reqs = append(reqs, batch...)
	}

	c.Log("reqs len: ", len(reqs))

	for _, req := range reqs {
		payload, err := vlog.readValue(req.valuePointer)
		c.Assert(err, check.IsNil)

		c.Assert(req.payload, check.DeepEquals, payload, check.Commentf("data read back not equal"))
	}

}

func (vs *VlogSuit) TestGCTS(c *check.C) {
	vlog := newVlogWithOptions(c, DefaultOptions().WithValueLogFileSize(2048))
	defer os.RemoveAll(vlog.dirPath)

	payload := make([]byte, 128)
	requests := make([]*request, 100)
	// 100 * 128 > 2048 guarantees that multiple log files are created
	for i := 0; i < len(requests); i++ {
		requests[i] = &request{
			startTS: int64(i),
			tp:      pb.BinlogType_Prewrite,
			payload: payload,
		}
	}
	err := vlog.write(requests)
	c.Assert(err, check.IsNil)
	pointers := make([]valuePointer, 0, len(requests))
	for _, req := range requests {
		pointers = append(pointers, req.valuePointer)
	}

	before := len(vlog.filesMap)
	c.Logf("before log file num: %d", before)

	vlog.gcLock.Lock()

	gcDone := make(chan struct{})
	go func() {
		// The following call should block waiting for the gcLock
		vlog.gcTS(90)
		close(gcDone)
	}()

	after := len(vlog.filesMap)
	c.Logf("after log file num: %d", after)
	c.Assert(after, check.Equals, before, check.Commentf("gc is not prevented"))

	vlog.gcLock.Unlock()
	<-gcDone
	after = len(vlog.filesMap)
	c.Logf("after log file num: %d", after)
	c.Assert(after, check.Less, before, check.Commentf("no file is deleted"))

	// ts 0 has been gc
	_, err = vlog.readValue(pointers[0])
	c.Assert(err, check.NotNil)

	// ts 91 should not be gc
	_, err = vlog.readValue(pointers[91])
	c.Assert(err, check.IsNil)
}

type ValuePointerSuite struct{}

var _ = check.Suite(&ValuePointerSuite{})

func (vps *ValuePointerSuite) TestValuePointerMarshalBinary(c *check.C) {
	var vp valuePointer
	fuzz := fuzz.New()
	fuzz.Fuzz(&vp)

	var expect valuePointer
	data, err := vp.MarshalBinary()
	c.Assert(err, check.IsNil)

	err = expect.UnmarshalBinary(data)
	c.Assert(err, check.IsNil)

	c.Assert(vp, check.Equals, expect)
}

// Test when no disk space write fail
// and should recover after disk space are free up
// set file size resource limit to make it write fail like no disk space
func (vs *VlogSuit) TestNoSpace(c *check.C) {
	dir := c.MkDir()
	c.Log("use dir: ", dir)

	var origRlimit syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_FSIZE, &origRlimit)
	c.Assert(err, check.IsNil)

	// set file size limit to be 20k
	err = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 20 * 1024, Max: origRlimit.Max})
	c.Assert(err, check.IsNil)

	defer func() {
		err = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &origRlimit)
		c.Assert(err, check.IsNil)
	}()

	vlog, err := newValueLog(dir, DefaultOptions())
	c.Assert(err, check.IsNil)

	// Size of the encoded record should be 1024 + headerLength = 1040
	payload := make([]byte, 1024)
	req := &request{
		payload: payload,
	}

	// Enough space for 19 * 1040
	for i := 0; i < 19; i++ {
		err = vlog.write([]*request{req})
		c.Assert(err, check.IsNil)
	}

	// failed because only 20k space available and may write a incomplete record
	err = vlog.write([]*request{req})
	c.Assert(err, check.NotNil)

	// increase file size limit to have enough space for one more request
	err = syscall.Setrlimit(syscall.RLIMIT_FSIZE, &syscall.Rlimit{Cur: 20*1024 + 1040, Max: origRlimit.Max})
	c.Assert(err, check.IsNil)

	// should write success now
	err = vlog.write([]*request{req})
	c.Assert(err, check.IsNil)

	// read back normally
	_, err = vlog.readValue(req.valuePointer)
	c.Assert(err, check.IsNil)

	err = vlog.close()
	c.Assert(err, check.IsNil)
}
