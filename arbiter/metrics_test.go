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

package arbiter

import (
	. "github.com/pingcap/check"
	"github.com/pingcap/errors"
)

type instanceNameSuite struct{}

var _ = Suite(&instanceNameSuite{})

func (s *instanceNameSuite) TestShouldRetUnknown(c *C) {
	orig := getHostname
	defer func() {
		getHostname = orig
	}()
	getHostname = func() (string, error) {
		return "", errors.New("host")
	}

	n := instanceName(9090)
	c.Assert(n, Equals, "unknown")
}

func (s *instanceNameSuite) TestShouldUseHostname(c *C) {
	orig := getHostname
	defer func() {
		getHostname = orig
	}()
	getHostname = func() (string, error) {
		return "kendoka", nil
	}

	n := instanceName(9090)
	c.Assert(n, Equals, "kendoka_9090")
}
