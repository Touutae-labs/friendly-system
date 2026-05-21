package txutil

import (
	"context"

	"gorm.io/gorm"
)

type contextKey struct{}

// WithTx injects a GORM transaction into the context so repository methods
// can participate in a caller-owned transaction without *gorm.DB leaking into
// domain port interfaces.
func WithTx(ctx context.Context, tx *gorm.DB) context.Context {
	return context.WithValue(ctx, contextKey{}, tx)
}

// FromContext extracts a GORM transaction from the context.
// Falls back to the provided default db when none is present.
func FromContext(ctx context.Context, db *gorm.DB) *gorm.DB {
	if tx, ok := ctx.Value(contextKey{}).(*gorm.DB); ok && tx != nil {
		return tx
	}
	return db
}
