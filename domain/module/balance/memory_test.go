package balance_test

import (
	"context"
	"sync"
	"testing"

	"github.com/Touutae-labs/friendly-system/domain/module/balance"
	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestMemory_ConcurrentReadWrite(t *testing.T) {
	repo := balance.NewMemory(map[string]decimal.Decimal{"C001": d("100")})
	const goroutines = 50
	const perGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				if i%2 == 0 {
					repo.Set("C001", decimal.NewFromInt(int64(i*j)))
				} else {
					_, _ = repo.Balance(context.Background(), "C001")
				}
			}
		}()
	}
	wg.Wait()
}
