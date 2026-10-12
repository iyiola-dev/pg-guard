// Package gorm is a minimal stand-in for gorm.io/gorm with the same method
// signatures.
package gorm

import (
	"context"
	"database/sql"

	"gorm.io/gorm/clause"
)

type Dialector interface{}

type Option interface{}

type Session struct {
	NewDB   bool
	Context context.Context
}

type DB struct {
	Error        error
	RowsAffected int64
}

func Open(dialector Dialector, opts ...Option) (*DB, error) { return &DB{}, nil }

func Expr(expr string, args ...interface{}) clause.Expr { return clause.Expr{SQL: expr, Vars: args} }

func (db *DB) WithContext(ctx context.Context) *DB                             { return db }
func (db *DB) Session(config *Session) *DB                                     { return db }
func (db *DB) Debug() *DB                                                      { return db }
func (db *DB) Model(value interface{}) *DB                                     { return db }
func (db *DB) Table(name string, args ...interface{}) *DB                      { return db }
func (db *DB) Select(query interface{}, args ...interface{}) *DB               { return db }
func (db *DB) Where(query interface{}, args ...interface{}) *DB                { return db }
func (db *DB) Or(query interface{}, args ...interface{}) *DB                   { return db }
func (db *DB) Not(query interface{}, args ...interface{}) *DB                  { return db }
func (db *DB) Joins(query string, args ...interface{}) *DB                     { return db }
func (db *DB) InnerJoins(query string, args ...interface{}) *DB                { return db }
func (db *DB) Group(name string) *DB                                           { return db }
func (db *DB) Having(query interface{}, args ...interface{}) *DB               { return db }
func (db *DB) Order(value interface{}) *DB                                     { return db }
func (db *DB) Limit(limit int) *DB                                             { return db }
func (db *DB) Offset(offset int) *DB                                           { return db }
func (db *DB) Raw(sql string, values ...interface{}) *DB                       { return db }
func (db *DB) Exec(sql string, values ...interface{}) *DB                      { return db }
func (db *DB) First(dest interface{}, conds ...interface{}) *DB                { return db }
func (db *DB) Take(dest interface{}, conds ...interface{}) *DB                 { return db }
func (db *DB) Last(dest interface{}, conds ...interface{}) *DB                 { return db }
func (db *DB) Find(dest interface{}, conds ...interface{}) *DB                 { return db }
func (db *DB) FirstOrCreate(dest interface{}, conds ...interface{}) *DB        { return db }
func (db *DB) Create(value interface{}) *DB                                    { return db }
func (db *DB) Save(value interface{}) *DB                                      { return db }
func (db *DB) Update(column string, value interface{}) *DB                     { return db }
func (db *DB) Updates(values interface{}) *DB                                  { return db }
func (db *DB) Delete(value interface{}, conds ...interface{}) *DB              { return db }
func (db *DB) Count(count *int64) *DB                                          { return db }
func (db *DB) Scan(dest interface{}) *DB                                       { return db }
func (db *DB) Pluck(column string, dest interface{}) *DB                       { return db }
func (db *DB) Row() *sql.Row                                                   { return nil }
func (db *DB) Rows() (*sql.Rows, error)                                        { return nil, nil }
func (db *DB) Begin(opts ...*sql.TxOptions) *DB                                { return db }
func (db *DB) Commit() *DB                                                     { return db }
func (db *DB) Rollback() *DB                                                   { return db }
func (db *DB) Transaction(fc func(tx *DB) error, opts ...*sql.TxOptions) error { return fc(db) }
