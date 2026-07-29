package mixin

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

const inspectMySQLTimeMixinCompatibilityQuery = "SELECT @@GLOBAL.sql_mode, @@SESSION.sql_mode"

var incompatibleMySQLTimeMixinModes = map[string]struct{}{
	"NO_ZERO_DATE": {},
	"TRADITIONAL":  {},
}

// MySQLTimeMixinCompatibility 描述当前 MySQL SQL Mode 与 TimeMixin 零时间默认值的兼容性。
type MySQLTimeMixinCompatibility struct {
	GlobalSQLMode            string
	SessionSQLMode           string
	IncompatibleGlobalModes  []string
	IncompatibleSessionModes []string
}

// Compatible 返回全局和当前会话 SQL Mode 是否都兼容 TimeMixin。
func (c MySQLTimeMixinCompatibility) Compatible() bool {
	return len(c.IncompatibleGlobalModes) == 0 && len(c.IncompatibleSessionModes) == 0
}

// InspectMySQLTimeMixinCompatibility 读取 MySQL 全局及当前会话 SQL Mode。
//
// 该函数只返回结构化检查结果，不修改数据库配置，也不记录日志。调用方应根据自身
// 启动策略决定如何提示不兼容模式。
func InspectMySQLTimeMixinCompatibility(
	ctx context.Context,
	db *sql.DB,
) (MySQLTimeMixinCompatibility, error) {
	if db == nil {
		return MySQLTimeMixinCompatibility{}, errors.New("inspect MySQL TimeMixin compatibility: nil database")
	}

	var result MySQLTimeMixinCompatibility
	if err := db.QueryRowContext(ctx, inspectMySQLTimeMixinCompatibilityQuery).Scan(
		&result.GlobalSQLMode,
		&result.SessionSQLMode,
	); err != nil {
		return MySQLTimeMixinCompatibility{}, err
	}

	result.IncompatibleGlobalModes = findIncompatibleMySQLTimeMixinModes(result.GlobalSQLMode)
	result.IncompatibleSessionModes = findIncompatibleMySQLTimeMixinModes(result.SessionSQLMode)

	return result, nil
}

func findIncompatibleMySQLTimeMixinModes(sqlMode string) []string {
	modes := strings.Split(sqlMode, ",")
	var incompatibleModes []string
	seen := make(map[string]struct{}, len(modes))

	for _, mode := range modes {
		normalizedMode := strings.ToUpper(strings.TrimSpace(mode))
		if _, incompatible := incompatibleMySQLTimeMixinModes[normalizedMode]; !incompatible {
			continue
		}
		if _, duplicated := seen[normalizedMode]; duplicated {
			continue
		}

		seen[normalizedMode] = struct{}{}
		incompatibleModes = append(incompatibleModes, normalizedMode)
	}

	return incompatibleModes
}
