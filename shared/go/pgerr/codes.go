package pgerr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// PostgreSQL error codes.
// See: https://www.postgresql.org/docs/current/errcodes-appendix.html
const (
	UniqueViolation     = "23505"
	ForeignKeyViolation = "23503"
)

// IsCode returns true if err is a PgError with the given code.
func IsCode(err error, code string) bool {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok {
		return pgErr.Code == code
	}
	return false
}
