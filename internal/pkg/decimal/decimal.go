package decimal

import (
	"github.com/shopspring/decimal"
)

// Zero is the zero value for convenience.
var Zero = decimal.Zero

// NewFromInt creates a decimal from an integer.
func NewFromInt(val int64) decimal.Decimal {
	return decimal.NewFromInt(val)
}

// NewFromFloat creates a decimal from a float64 string representation.
// Use NewFromString for exact values.
func NewFromFloat(val float64) decimal.Decimal {
	return decimal.NewFromFloat(val)
}

// NewFromString creates a decimal from a string.
func NewFromString(val string) (decimal.Decimal, error) {
	return decimal.NewFromString(val)
}

// MustNewFromString creates a decimal from a string, panicking on error.
func MustNewFromString(val string) decimal.Decimal {
	return decimal.RequireFromString(val)
}

// GreaterThan returns true if a > b.
func GreaterThan(a, b decimal.Decimal) bool {
	return a.GreaterThan(b)
}

// Sum returns the sum of the given decimals.
func Sum(vals ...decimal.Decimal) decimal.Decimal {
	result := decimal.Zero
	for _, v := range vals {
		result = result.Add(v)
	}
	return result
}

// IsPositive returns true if val > 0.
func IsPositive(val decimal.Decimal) bool {
	return val.IsPositive()
}

// IsZero returns true if val == 0.
func IsZero(val decimal.Decimal) bool {
	return val.IsZero()
}
