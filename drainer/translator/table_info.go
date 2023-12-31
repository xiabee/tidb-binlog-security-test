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

package translator

import "github.com/pingcap/tidb/parser/model"

// TableInfoGetter is used to get table info by table id of TiDB
type TableInfoGetter interface {
	TableByID(id int64) (info *model.TableInfo, ok bool)
	SchemaAndTableName(id int64) (string, string, bool)
	CanAppendDefaultValue(id int64, schemaVersion int64) bool
	// IsDroppingColumn(id int64) bool
	TableBySchemaVersion(id int64, schemaVersion int64) (info *model.TableInfo, ok bool)
}
