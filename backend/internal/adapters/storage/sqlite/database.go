package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
)

type sqlDialect string

const (
	sqliteDialect   sqlDialect = "sqlite"
	postgresDialect sqlDialect = "postgres"
)

type database struct {
	raw     *sql.DB
	dialect sqlDialect
}

type transaction struct {
	raw     *sql.Tx
	dialect sqlDialect
}

func (d *database) Close() error { return d.raw.Close() }

func (d *database) PingContext(ctx context.Context) error { return d.raw.PingContext(ctx) }

func (d *database) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return d.raw.ExecContext(ctx, bindQuery(d.dialect, query), args...)
}

func (d *database) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return d.raw.QueryContext(ctx, bindQuery(d.dialect, query), args...)
}

func (d *database) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return d.raw.QueryRowContext(ctx, bindQuery(d.dialect, query), args...)
}

func (d *database) BeginTx(ctx context.Context, options *sql.TxOptions) (*transaction, error) {
	tx, err := d.raw.BeginTx(ctx, options)
	if err != nil {
		return nil, err
	}
	return &transaction{raw: tx, dialect: d.dialect}, nil
}

func (t *transaction) ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return t.raw.ExecContext(ctx, bindQuery(t.dialect, query), args...)
}

func (t *transaction) QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error) {
	return t.raw.QueryContext(ctx, bindQuery(t.dialect, query), args...)
}

func (t *transaction) QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row {
	return t.raw.QueryRowContext(ctx, bindQuery(t.dialect, query), args...)
}

func (t *transaction) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return t.raw.PrepareContext(ctx, bindQuery(t.dialect, query))
}

func (t *transaction) Commit() error   { return t.raw.Commit() }
func (t *transaction) Rollback() error { return t.raw.Rollback() }

func bindQuery(dialect sqlDialect, query string) string {
	if dialect != postgresDialect {
		return query
	}
	var result strings.Builder
	result.Grow(len(query) + 16)
	parameter := 1
	inSingleQuote := false
	for index := 0; index < len(query); index++ {
		character := query[index]
		if character == '\'' {
			result.WriteByte(character)
			if inSingleQuote && index+1 < len(query) && query[index+1] == '\'' {
				result.WriteByte(query[index+1])
				index++
				continue
			}
			inSingleQuote = !inSingleQuote
			continue
		}
		if character == '?' && !inSingleQuote {
			result.WriteString(fmt.Sprintf("$%d", parameter))
			parameter++
			continue
		}
		result.WriteByte(character)
	}
	return result.String()
}
