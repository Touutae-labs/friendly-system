//go:build wireinject
// +build wireinject

package di

import (
	"github.com/google/wire"
	"gorm.io/gorm"

	"github.com/Touutae-labs/friendly-system/internal/configurations"
	controller "github.com/Touutae-labs/friendly-system/internal/controllers"
	"github.com/Touutae-labs/friendly-system/internal/modules/balance"
	"github.com/Touutae-labs/friendly-system/internal/modules/limit"
	"github.com/Touutae-labs/friendly-system/internal/modules/orderval"
	"github.com/Touutae-labs/friendly-system/internal/modules/quote"
	"github.com/Touutae-labs/friendly-system/internal/server"
	"github.com/Touutae-labs/friendly-system/internal/service/processor"
	validatorsvc "github.com/Touutae-labs/friendly-system/internal/service/validator"
)

// Application holds all top-level components of the application.
type Application struct {
	Server *server.Server
	DB     *gorm.DB
}

var serverSet = wire.NewSet(
	wire.FieldsOf(new(configurations.Config), "Server"),
	provideServerConfig,
	provideServerAndRegisterHandlers,
)

var controllersSet = wire.NewSet(
	controller.NewControllers,
	controller.NewHealthController,
	controller.NewOrderController,
)

var servicesSet = wire.NewSet(
	validatorsvc.New,
	processor.New,
)

var modulesSet = wire.NewSet(
	provideOrderValConfig,
	orderval.New,
	provideQuoteConfig,
	quote.New,
	balance.New,
	provideLimitConfig,
	limit.New,
)

var repositoriesSet = wire.NewSet(
	provideAccountsGorm,
	provideLedgerGorm,
	provideOrdersGorm,
)

var dbSet = wire.NewSet(
	wire.FieldsOf(new(configurations.Config), "Database"),
	provideDB,
)

func initialize(cfg configurations.Config, title server.ServerTitle, version server.ServerVersion) (*Application, func(), error) {
	wire.Build(
		wire.Struct(new(Application), "*"),
		provideMarket,
		dbSet,
		serverSet,
		controllersSet,
		servicesSet,
		modulesSet,
		repositoriesSet,
	)
	return nil, func() {}, nil
}

// Initialize is the main entrypoint for dependency injection.
func Initialize(cfg configurations.Config, title server.ServerTitle, version server.ServerVersion) (*server.Server, func(), error) {
	app, cleanup, err := initialize(cfg, title, version)
	if err != nil {
		return nil, nil, err
	}
	return app.Server, cleanup, nil
}

// InitializeApp returns the full Application including DB access (useful for seeding in tests).
func InitializeApp(cfg configurations.Config, title server.ServerTitle, version server.ServerVersion) (*Application, func(), error) {
	return initialize(cfg, title, version)
}
