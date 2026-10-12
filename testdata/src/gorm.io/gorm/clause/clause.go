// Package clause is a minimal stand-in for gorm.io/gorm/clause.
package clause

type Expr struct {
	SQL  string
	Vars []interface{}
}

type Column struct {
	Name string
}
