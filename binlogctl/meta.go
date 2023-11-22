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

package binlogctl

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pingcap/errors"
	"github.com/pingcap/log"
	"github.com/pingcap/tidb-binlog/pkg/flags"
	"github.com/pingcap/tidb-binlog/pkg/util"
	pd "github.com/tikv/pd/client"
	"go.uber.org/zap"
)

var newPDClientFunc = pd.NewClient

// GenerateMetaInfo generates Meta from pd
func GenerateMetaInfo(cfg *Config) error {
	if err := os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return errors.Trace(err)
	}

	// get newest ts from pd
	commitTS, err := GetTSO(cfg)
	if err != nil {
		log.Error("get tso failed", zap.Error(err))
		return errors.Trace(err)
	}

	// generate meta file
	metaFileName := path.Join(cfg.DataDir, "savepoint")
	err = saveMeta(metaFileName, commitTS, cfg.TimeZone)
	return errors.Trace(err)
}

// GetTSO gets ts from pd
func GetTSO(cfg *Config) (int64, error) {
	ectdEndpoints, err := flags.ParseHostPortAddr(cfg.EtcdURLs)
	if err != nil {
		return 0, errors.Trace(err)
	}

	pdCli, err := newPDClientFunc(ectdEndpoints, pd.SecurityOption{
		CAPath:   cfg.SSLCA,
		CertPath: cfg.SSLCert,
		KeyPath:  cfg.SSLKey,
	})
	if err != nil {
		return 0, errors.Trace(err)
	}

	return util.GetTSO(pdCli)
}

// Meta contains commit TS that can be used to specifies the location of the synchronized data
type Meta struct {
	CommitTS int64 `toml:"commitTS" json:"commitTS"`
}

// String returns the string of Meta
func (m *Meta) String() string {
	return fmt.Sprintf("commitTS: %d", m.CommitTS)
}

// saveMeta saves current tso in meta file.
func saveMeta(metaFileName string, ts int64, timeZone string) error {
	meta := &Meta{CommitTS: ts}

	var buf bytes.Buffer
	e := toml.NewEncoder(&buf)
	err := e.Encode(meta)
	if err != nil {
		return errors.Annotatef(err, "save meta %+v into %s", meta, metaFileName)
	}

	if timeZone != "" {
		t := util.TSOToRoughTime(ts)
		location, err1 := time.LoadLocation(timeZone)
		if err1 != nil {
			log.Warn("fail to load location", zap.String("time zone", timeZone), zap.Error(err1))
		} else {
			buf.WriteString(t.UTC().String())
			buf.WriteByte('\n')
			buf.WriteString(t.In(location).String())
		}
	}

	err = util.WriteFileAtomic(metaFileName, buf.Bytes(), 0644)
	if err != nil {
		return errors.Annotatef(err, "save meta %+v into %s", meta, metaFileName)
	}

	log.Info("save meta", zap.Stringer("meta", meta))
	return nil
}
