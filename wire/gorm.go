package wire

import (
	"github.com/pantakan/intergold-validator/domain/module/balance"
	"github.com/pantakan/intergold-validator/domain/module/limit"
	"github.com/pantakan/intergold-validator/domain/repositories"
	"gorm.io/gorm"
)

func provideAccountsGorm(db *gorm.DB) balance.AccountRepository {
	return repositories.NewAccount(db)
}

func provideLedgerGorm(db *gorm.DB) limit.DailyLedger {
	return repositories.NewLedger(db)
}
