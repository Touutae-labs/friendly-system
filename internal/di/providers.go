package di

import (
	"fmt"

	"github.com/glebarez/sqlite"
	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/Touutae-labs/friendly-system/internal/configurations"
	controller "github.com/Touutae-labs/friendly-system/internal/controllers"
	"github.com/Touutae-labs/friendly-system/internal/handler"
	"github.com/Touutae-labs/friendly-system/internal/models"
	"github.com/Touutae-labs/friendly-system/internal/modules/limit"
	"github.com/Touutae-labs/friendly-system/internal/modules/orderval"
	"github.com/Touutae-labs/friendly-system/internal/modules/quote"
	"github.com/Touutae-labs/friendly-system/internal/repositories"
	"github.com/Touutae-labs/friendly-system/internal/server"
)

func provideDB(cfg configurations.DatabaseConfig) (*gorm.DB, func(), error) {
	path := cfg.Path
	if path == "" {
		path = ":memory:"
	}
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("open db %s: %w", path, err)
	}
	if err := db.AutoMigrate(models.AllModels()...); err != nil {
		return nil, nil, fmt.Errorf("migrate: %w", err)
	}
	cleanup := func() {
		if sqlDB, e := db.DB(); e == nil {
			_ = sqlDB.Close()
		}
	}
	return db, cleanup, nil
}

func provideMarket() quote.MarketPriceProvider {
	return quote.NewFixed(decimal.RequireFromString("42000"))
}

func provideOrderValConfig(cfg configurations.Config) (orderval.Config, error) {
	c := orderval.DefaultConfig()
	if cfg.OrderVal.QuantityIncrement != "" {
		c.QuantityIncrement = decimal.RequireFromString(cfg.OrderVal.QuantityIncrement)
	}
	if cfg.OrderVal.MaxQuantity != "" {
		c.MaxQuantity = decimal.RequireFromString(cfg.OrderVal.MaxQuantity)
	}
	if cfg.OrderVal.MaxPrice != "" {
		c.MaxPrice = decimal.RequireFromString(cfg.OrderVal.MaxPrice)
	}
	if cfg.OrderVal.MaxCustomerIDLength > 0 {
		c.MaxCustomerIDLength = cfg.OrderVal.MaxCustomerIDLength
	}
	return c, c.Validate()
}

func provideQuoteConfig(cfg configurations.Config) (quote.Config, error) {
	c := quote.DefaultConfig()
	if cfg.Quote.PriceTolerance != "" {
		c.PriceTolerance = decimal.RequireFromString(cfg.Quote.PriceTolerance)
	}
	if cfg.Quote.SpreadMargin != "" {
		c.SpreadMargin = decimal.RequireFromString(cfg.Quote.SpreadMargin)
	}
	return c, c.Validate()
}

func provideLimitConfig(cfg configurations.Config) (limit.Config, error) {
	c := limit.DefaultConfig()
	if cfg.Limit.DailyLimit != "" {
		c.DailyLimit = decimal.RequireFromString(cfg.Limit.DailyLimit)
	}
	return c, c.Validate()
}

func provideServerConfig(cfg configurations.ServerConfig, title server.ServerTitle, version server.ServerVersion) server.ServerConfig {
	return server.ServerConfig{
		Title:            title,
		Version:          version,
		Port:             cfg.Port,
		MaxPayloadSizeKB: cfg.MaxPayloadSizeKB,
		TimeoutSeconds:   cfg.TimeoutSeconds,
		BaseURL:          cfg.BaseURL,
	}
}

func provideServerAndRegisterHandlers(cfg server.ServerConfig, c *controller.Controllers) *server.Server {
	srv := server.NewServer(cfg)
	handler.RegisterHandlers(*srv.API, c)
	return srv
}

func provideAccountsGorm(db *gorm.DB) repositories.AccountRepository {
	return repositories.NewAccount(db)
}

func provideLedgerGorm(db *gorm.DB) repositories.DailyLedger {
	return repositories.NewLedger(db)
}

func provideOrdersGorm(db *gorm.DB) repositories.OrderRepository {
	return repositories.NewOrder(db)
}
