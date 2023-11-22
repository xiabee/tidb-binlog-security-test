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
	gosql "database/sql"
	"fmt"

	pkgsql "github.com/pingcap/tidb-binlog/pkg/sql"

	"github.com/pingcap/errors"
)

const (
	// StatusNormal means server quit normally, data <= ts is synced to downstream
	StatusNormal int = 0
	// StatusRunning means server running or quit abnormally, part of data may or may not been synced to downstream
	StatusRunning int = 1
)

// Checkpoint is able to save and load checkpoints
type Checkpoint interface {
	Save(ts int64, status int) error
	Load() (ts int64, status int, err error)
}

type dbCheckpoint struct {
	database  string
	table     string
	db        *gosql.DB
	topicName string
}

// NewCheckpoint creates a Checkpoint
func NewCheckpoint(db *gosql.DB, topicName string) (Checkpoint, error) {
	cp := &dbCheckpoint{
		db:        db,
		database:  "tidb_binlog",
		table:     "arbiter_checkpoint",
		topicName: topicName,
	}

	if err := cp.createSchemaIfNeed(); err != nil {
		return nil, errors.Trace(err)
	}

	return cp, nil
}

func (c *dbCheckpoint) createSchemaIfNeed() error {
	sql := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", pkgsql.QuoteName(c.database))
	_, err := c.db.Exec(sql)
	if err != nil {
		return errors.Trace(err)
	}

	sql = fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s(
		topic_name VARCHAR(255) PRIMARY KEY, ts BIGINT NOT NULL, status INT NOT NULL)`,
		pkgsql.QuoteSchema(c.database, c.table))
	_, err = c.db.Exec(sql)
	if err != nil {
		return errors.Trace(err)
	}

	return nil
}

// Save saves the ts and status
func (c *dbCheckpoint) Save(ts int64, status int) error {
	sql := fmt.Sprintf("REPLACE INTO %s(topic_name, ts, status) VALUES(?,?,?)",
		pkgsql.QuoteSchema(c.database, c.table))
	_, err := c.db.Exec(sql, c.topicName, ts, status)
	if err != nil {
		return errors.Annotatef(err, "exec fail: '%s', args: %s %d, %d", sql, c.topicName, ts, status)
	}

	return nil
}

// Load return ts and status, if no record in checkpoint, return err = errors.NotFoundf
func (c *dbCheckpoint) Load() (ts int64, status int, err error) {
	sql := fmt.Sprintf("SELECT ts, status FROM %s WHERE topic_name = ?",
		pkgsql.QuoteSchema(c.database, c.table))

	row := c.db.QueryRow(sql, c.topicName)

	err = row.Scan(&ts, &status)
	if err != nil {
		if errors.Cause(err) == gosql.ErrNoRows {
			return 0, 0, errors.NotFoundf("no checkpoint for: %s", c.topicName)
		}
		return 0, 0, errors.Trace(err)
	}

	return
}
