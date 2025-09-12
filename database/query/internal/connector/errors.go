// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"regexp"
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
)

type DuplicateError struct {
	Constraint string
	Object     string
	Original   error
}

type sanitizedError struct {
	Original error
	Message  string
}

var (
	reUser        = regexp.MustCompile("user=[^\\s`]+")
	reDatabase    = regexp.MustCompile("database=[^\\s`]+")
	reDbname      = regexp.MustCompile("dbname=[^\\s`]+")
	rePassword    = regexp.MustCompile("password=[^\\s`]+")
	reURIUserInfo = regexp.MustCompile(`://[^/]+@`) // postgres://user:pass@host
)

func (e DuplicateError) Error() string {
	msg := "duplicate value violates unique constraint"
	if e.Constraint != "" {
		msg += ": " + e.Constraint
	}

	if e.Object != "" {
		msg += " (object: " + e.Object + ")"
	}

	return msg
}

func (e DuplicateError) Unwrap() error { return e.Original }

func (e sanitizedError) Error() string { return e.Message }
func (e sanitizedError) Unwrap() error { return e.Original }

func sanitizeErr(err error) error {
	if err == nil {
		return nil
	}
	msg := sanitizeError(err)
	if msg == err.Error() {
		return err
	}

	return sanitizedError{Original: err, Message: msg}
}

func sanitizeError(err error) string {
	if err == nil {
		return ""
	}

	return sanitizeDSN(err.Error())
}

func sanitizeDSN(s string) string {
	if s == "" {
		return s
	}
	s = reUser.ReplaceAllString(s, "user=***")
	s = reDatabase.ReplaceAllString(s, "database=***")
	s = reDbname.ReplaceAllString(s, "dbname=***")
	s = rePassword.ReplaceAllString(s, "password=***")
	s = reURIUserInfo.ReplaceAllString(s, "://***@")

	return s
}

func parseError(err error) error {
	var dbErr *Error

	if err == nil {
		return nil
	} else if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}

	if errors.As(err, &dbErr) {
		switch dbErr.SQLState() {
		case pgerrcode.UniqueViolation:
			dupErr := DuplicateError{Constraint: dbErr.ConstraintName, Original: err, Object: dbErr.Detail}

			if strings.HasSuffix(dbErr.ConstraintName, "_pkey") {
				return errors.Join(errors.Wrap(ErrDuplicate, dbErr.ConstraintName), dupErr)
			} else {
				column := strings.ReplaceAll(dbErr.ConstraintName, dbErr.TableName, "")
				column = strings.ReplaceAll(column, "_key", "")
				column = strings.ReplaceAll(column, "_", "")

				return errors.Join(errors.Wrap(ErrDuplicate, column), dupErr)
			}
		case pgerrcode.ForeignKeyViolation:
			column := strings.ReplaceAll(dbErr.ConstraintName, dbErr.TableName, "")
			column = strings.ReplaceAll(column, "_fkey", "")
			column = strings.ReplaceAll(column, "_", "")
			if strings.Contains(dbErr.Detail, "is still referenced from table") {
				return errors.Wrap(ErrRelationInUse, column)
			}
			return errors.Wrap(ErrRelationNotFound, column)
		case pgerrcode.CheckViolation:
			column := strings.ReplaceAll(dbErr.ConstraintName, dbErr.TableName, "")
			column = strings.ReplaceAll(column, "_check", "")
			column = strings.ReplaceAll(column, "_", "")

			return errors.Wrap(ErrCheckFailed, column)
		case pgerrcode.SerializationFailure, pgerrcode.DeadlockDetected:
			return errors.WithDetail(ErrSerializationFailure, dbErr.Error())
		case pgerrcode.InFailedSQLTransaction:
			return ErrTxAborted
		case pgerrcode.AmbiguousFunction:
			return ErrOperatorError
		case pgerrcode.UndefinedTable:
			return errors.Wrap(ErrRelationNotFound, dbErr.TableName)
		case pgerrcode.ExclusionViolation:
			return ErrExclusionViolation
		case pgerrcode.RaiseException:
			return errors.Join(ErrException, err)
		case pgerrcode.CharacterNotInRepertoire:
			return ErrInvalidData
		case pgerrcode.SyntaxError:
			return errors.WithDetail(ErrInternal, dbErr.Error())
		case pgerrcode.ReadOnlySQLTransaction, pgerrcode.FeatureNotSupported:
			return errors.WithDetail(ErrReadOnly, dbErr.Error())
		}
	}

	return sanitizeErr(err)
}
