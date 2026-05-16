package wire

import (
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/repositories"
	"gorm.io/gorm"
)

func provideAccountsGorm(db *gorm.DB) balance.AccountRepository {
	return repositories.NewAccount(db)
}

func provideLedgerGorm(db *gorm.DB) limit.DailyLedger {
	return repositories.NewLedger(db)
}

func provideOrdersGorm(db *gorm.DB) repositories.OrderRepository {
	return repositories.NewOrder(db)
}
