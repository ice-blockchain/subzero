// SPDX-License-Identifier: ice License 1.0

package connector

import (
	"strings"

	"github.com/cockroachdb/errors"
	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
)

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
			if strings.HasSuffix(dbErr.ConstraintName, "_pkey") {
				return errors.Wrap(ErrDuplicate, dbErr.ConstraintName)
			} else {
				column := strings.ReplaceAll(dbErr.ConstraintName, dbErr.TableName, "")
				column = strings.ReplaceAll(column, "_key", "")
				column = strings.ReplaceAll(column, "_", "")

				return errors.Wrap(ErrDuplicate, column)
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
		case pgerrcode.SerializationFailure:
			return ErrSerializationFailure
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
		case pgerrcode.ReadOnlySQLTransaction:
			return ErrReadOnly
		}
	}

	return err
}
