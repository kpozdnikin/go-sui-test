package gormdb

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

type DBOperator interface {
	First(dest interface{}, conds ...interface{}) *gorm.DB
	Create(value interface{}) *gorm.DB
	Update(column string, value interface{}) *gorm.DB
	Where(query interface{}, args ...interface{}) *gorm.DB
	Model(value interface{}) *gorm.DB
}

type DbProvider interface {
	WithContext(ctx context.Context) DBOperator
}

type DefaultGormDB struct {
	db *gorm.DB
}

func NewDefaultGormDB(db *gorm.DB) DbProvider {
	return &DefaultGormDB{db: db}
}

func (g *DefaultGormDB) WithContext(ctx context.Context) DBOperator {
	return g.db.WithContext(ctx)
}

// NewDB creates a new instance of database connection using GORM
func NewDB(dialector gorm.Dialector, cfg *gorm.Config) (*gorm.DB, error) {
	db, err := gorm.Open(dialector, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Auto-migrate the schema
	if err := db.AutoMigrate(&ChirpTransactionModel{}); err != nil {
		return nil, fmt.Errorf("failed to migrate database: %w", err)
	}

	return db, nil
}
