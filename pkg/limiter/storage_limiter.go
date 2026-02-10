package limiter

import (
	"time"

	internallimiter "github.com/SmitUplenchwar2687/Chrono/internal/limiter"
	"github.com/SmitUplenchwar2687/Chrono/pkg/clock"
	chronostorage "github.com/SmitUplenchwar2687/Chrono/pkg/storage"
)

// NewStorageLimiter creates a limiter backed by a pkg/storage backend.
func NewStorageLimiter(storage chronostorage.Storage, rate int, window time.Duration, c clock.Clock) (*StorageLimiter, error) {
	return internallimiter.NewStorageLimiter(storage, rate, window, c)
}
