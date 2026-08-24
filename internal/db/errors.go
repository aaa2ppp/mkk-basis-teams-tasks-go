package db

import (
	"errors"
	"fmt"

	"aaa2ppp/teams-tasks/internal/model"

	"github.com/go-sql-driver/mysql"
)

func MapMySqlError(err error) error {
	if mysqlErr, ok := errors.AsType[*mysql.MySQLError](err); ok {
		if mysqlErr.Number == 1062 {
			return fmt.Errorf("%w: %v", model.ErrConflict, mysqlErr.Message)
		}
	}
	return err
}
