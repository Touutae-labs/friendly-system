//go:build wireinject
// +build wireinject

package wire

import (
	googlewire "github.com/google/wire"
	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/Touutae-labs/friendly-system/domain/module/limit"
	"github.com/Touutae-labs/friendly-system/domain/module/orderval"
	"github.com/Touutae-labs/friendly-system/domain/module/quote"
	"github.com/Touutae-labs/friendly-system/domain/service/processor"
	validatorsvc "github.com/Touutae-labs/friendly-system/domain/service/validator"
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
		provideOrdersGorm,
		moduleSet,
		processor.New,
	)
	return nil, nil
}
