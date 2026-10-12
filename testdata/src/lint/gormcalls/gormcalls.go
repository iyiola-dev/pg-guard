package gormcalls

import (
	"context"
	"database/sql"
	"fmt"

	"gorm.io/gorm"
)

type User struct {
	ID   int
	Name string
}

type Repo struct {
	db *gorm.DB
}

var global *gorm.DB

func conditions(ctx context.Context, db *gorm.DB, name string, ids []int) {
	var u User
	var users []User
	db.WithContext(ctx).Where("name = ?", name).First(&u)
	db.WithContext(ctx).Where("id IN ?", ids).Or("name LIKE ?", name+"%").Find(&users)
	db.WithContext(ctx).Where("name = ? AND", name).Find(&users) // want `\[invalid-sql\] invalid SQL syntax in WHERE fragment: syntax error at end of input`
	db.WithContext(ctx).Where("name", name).Find(&users)
	db.WithContext(ctx).Where(&User{Name: name}).Find(&users)
	db.WithContext(ctx).Where("name = @name", sql.Named("name", name)).Find(&users)
	db.WithContext(ctx).Where("name = @name", map[string]interface{}{"name": name}).Find(&users)
	db.WithContext(ctx).Not("deleted").Find(&users)
	db.WithContext(ctx).First(&u, "name = ?", name)
	db.WithContext(ctx).First(&u, "10")
	db.WithContext(ctx).First(&u, 10)
	db.WithContext(ctx).Find(&users, "name = ? OR", name) // want `\[invalid-sql\] invalid SQL syntax in WHERE fragment`
}

func clauses(ctx context.Context, db *gorm.DB, sub *gorm.DB) {
	var users []User
	var names []string
	db.WithContext(ctx).Select("name, count(*) AS total").Group("name").Having("count(*) > ?", 1).Order("total desc, name").Find(&users)
	db.WithContext(ctx).Joins("Company").Joins("JOIN emails ON emails.user_id = users.id AND emails.email = ?", "x").Find(&users)
	db.WithContext(ctx).Table("users AS u").Pluck("u.name", &names)
	db.WithContext(ctx).Table("(?) AS u", sub).Find(&users)
	db.WithContext(ctx).Where("amount > (?)", sub).Find(&users)
	db.WithContext(ctx).Model(&User{}).Update("name", gorm.Expr("upper(name) || ?", "!"))
	db.WithContext(ctx).Order("name desc,").Find(&users) // want `\[invalid-sql\] invalid SQL syntax in ORDER BY fragment`
}

func raw(ctx context.Context, db *gorm.DB, name string) {
	var users []User
	db.WithContext(ctx).Raw("SELECT * FROM users WHERE name = ?", name).Scan(&users)
	db.WithContext(ctx).Raw("SELECT * FROM users WHERE id IN ?", []int{1, 2}).Scan(&users)
	db.WithContext(ctx).Raw("SELECT * FROM users WHERE name = @name", sql.Named("name", name)).Scan(&users)
	db.WithContext(ctx).Exec("UPDATE users SET name = ? WHERE id = ?", name, 1)
	db.WithContext(ctx).Raw("SELEC * FROM users").Scan(&users) // want `\[invalid-sql\] invalid SQL syntax: syntax error at or near "SELEC"`
}

func injection(ctx context.Context, db *gorm.DB, name, column string) {
	var users []User
	db.WithContext(ctx).Where(fmt.Sprintf("name = '%s'", name)).Find(&users)                // want `\[unparameterized-query\] SQL built with fmt.Sprintf`
	db.WithContext(ctx).Raw("SELECT * FROM users WHERE name = '" + name + "'").Scan(&users) // want `\[unparameterized-query\] SQL built with string concatenation`
	db.WithContext(ctx).Order(column + " desc").Find(&users)                                // want `\[unparameterized-query\]`
	db.WithContext(ctx).Order(column).Find(&users)
}

func (r *Repo) missingContext(ctx context.Context, id int) {
	var u User
	r.db.First(&u, id)                // want `\[missing-context\] First\(\) runs without a context; chain WithContext\(ctx\) before it`
	r.db.Where("id = ?", id).Take(&u) // want `\[missing-context\] Take\(\) runs without a context`
	global.Raw("SELECT 1").Scan(&u)   // want `\[missing-context\] Scan\(\) runs without a context`
	global.Exec("DELETE FROM users")  // want `\[missing-context\] Exec\(\) runs without a context`

	r.db.WithContext(ctx).First(&u, id)
	tx := r.db.WithContext(ctx)
	tx.Where("id = ?", id).First(&u)
	r.db.Session(&gorm.Session{Context: ctx}).First(&u, id)
	r.db.Session(&gorm.Session{NewDB: true}).First(&u, id) // want `\[missing-context\] First\(\)`
}

func callbacks(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return tx.Create(&User{Name: "a"}).Error
	})
}

func unknownOrigin(db *gorm.DB) {
	var u User
	db.First(&u)
}

func opened() {
	db, _ := gorm.Open(nil)
	var u User
	db.First(&u) // want `\[missing-context\] First\(\)`
}

func loops(ctx context.Context, db *gorm.DB, ids []int) {
	var u User
	for _, id := range ids {
		db.WithContext(ctx).Where("id = ?", id).First(&u) // want `\[n-plus-one\] query executed inside loop body`
	}
}
