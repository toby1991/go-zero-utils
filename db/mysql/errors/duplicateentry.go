package errors

import (
	"github.com/go-sql-driver/mysql"
	"errors"
)

func IsDuplicateEntry(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}
