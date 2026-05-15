package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/Touutae-labs/friendly-system/domain/repositories"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func main() {
	dbPath := flag.String("db", "./data.db", "path to SQLite database (schema must be pre-applied)")
	flag.Parse()

	db, err := gorm.Open(sqlite.Open(*dbPath), &gorm.Config{})
	if err != nil {
		log.Fatalf("open %s: %v", *dbPath, err)
	}

	accounts := []repositories.AccountModel{
		{CustomerID: "C001", Balance: "1000000"},
		{CustomerID: "C002", Balance: "500"},
		{CustomerID: "C003", Balance: "5000000"},
	}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"balance"}),
	}).Create(&accounts).Error; err != nil {
		log.Fatalf("seed accounts: %v", err)
	}

	today := time.Now().Format("2006-01-02")
	dailyTotal := repositories.DailyTotalModel{CustomerID: "C001", Day: today, Total: "4"}
	if err := db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "customer_id"}, {Name: "day"}},
		DoUpdates: clause.AssignmentColumns([]string{"total"}),
	}).Create(&dailyTotal).Error; err != nil {
		log.Fatalf("seed daily total: %v", err)
	}

	fmt.Printf("seeded %d accounts + 1 daily total (C001=4 baht-weight on %s) to %s\n",
		len(accounts), today, *dbPath)
}
