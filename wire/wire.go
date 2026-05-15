//go:build wireinject
// +build wireinject

package wire

import (
	googlewire "github.com/google/wire"
	"github.com/pantakan/intergold-validator/domain/module/balance"
	"github.com/pantakan/intergold-validator/domain/module/limit"
	"github.com/pantakan/intergold-validator/domain/module/orderval"
	"github.com/pantakan/intergold-validator/domain/module/quote"
	"github.com/pantakan/intergold-validator/domain/service/processor"
	validatorsvc "github.com/pantakan/intergold-validator/domain/service/validator"
	"gorm.io/gorm"
)

var moduleSet = googlewire.NewSet(
	orderval.DefaultConfig,
	orderval.New,
	quote.DefaultConfig,
	quote.New,
	balance.New,
	limit.DefaultConfig,
	limit.New,
	validatorsvc.New,
)

func InitValidatorService() (*validatorsvc.Service, error) {
	googlewire.Build(
		provideAccounts,
		provideMarket,
		provideLedger,
		moduleSet,
	)
	return nil, nil
}

func InitValidatorServiceGorm(db *gorm.DB) (*validatorsvc.Service, error) {
	googlewire.Build(
		provideAccountsGorm,
		provideMarket,
		provideLedgerGorm,
		moduleSet,
	)
	return nil, nil
}

func InitProcessorService(db *gorm.DB) (*processor.Service, error) {
	googlewire.Build(
		provideAccountsGorm,
		provideMarket,
		provideLedgerGorm,
		moduleSet,
		processor.New,
	)
	return nil, nil
}
