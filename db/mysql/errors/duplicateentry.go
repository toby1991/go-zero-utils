package errors

import (
	"errors"
	"github.com/go-sql-driver/mysql"
)

func IsDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}
