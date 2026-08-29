package setting

import "context"

// Entry is a single system_settings row. Value is a JSON-encoded value.
type Entry struct {
	Key   string
	Value []byte
}

// Repository defines system_settings data access.
type Repository interface {
	All(ctx context.Context) ([]Entry, error)
	Get(ctx context.Context, keys ...string) ([]Entry, error)
	Upsert(ctx context.Context, entries []Entry) error
}
