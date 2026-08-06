package decimal

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestNewFromInt(t *testing.T) {
	d := NewFromInt(42)
	if !d.Equal(decimal.NewFromInt(42)) {
		t.Errorf("NewFromInt(42) = %s, want 42", d)
	}
}

func TestNewFromString_Success(t *testing.T) {
	d, err := NewFromString("3.14159")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !d.Equal(decimal.NewFromFloat(3.14159)) {
		t.Errorf("got %s, want 3.14159", d)
	}
}

func TestNewFromString_Invalid(t *testing.T) {
	_, err := NewFromString("not-a-number")
	if err == nil {
		t.Error("expected error for invalid string")
	}
}

func TestMustNewFromString(t *testing.T) {
	d := MustNewFromString("123.456")
	if !d.Equal(decimal.NewFromFloat(123.456)) {
		t.Errorf("got %s, want 123.456", d)
	}
}

func TestMustNewFromString_Panics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid string")
		}
	}()
	MustNewFromString("!!!")
}

func TestGreaterThan(t *testing.T) {
	if !GreaterThan(decimal.NewFromInt(5), decimal.NewFromInt(3)) {
		t.Error("5 should be > 3")
	}
	if GreaterThan(decimal.NewFromInt(3), decimal.NewFromInt(5)) {
		t.Error("3 should not be > 5")
	}
	if GreaterThan(decimal.NewFromInt(3), decimal.NewFromInt(3)) {
		t.Error("3 should not be > 3")
	}
}

func TestSum(t *testing.T) {
	sum := Sum(decimal.NewFromInt(1), decimal.NewFromInt(2), decimal.NewFromInt(3))
	if !sum.Equal(decimal.NewFromInt(6)) {
		t.Errorf("Sum(1,2,3) = %s, want 6", sum)
	}
}

func TestSum_Empty(t *testing.T) {
	sum := Sum()
	if !sum.IsZero() {
		t.Errorf("Sum() = %s, want 0", sum)
	}
}

func TestIsPositive(t *testing.T) {
	if !IsPositive(decimal.NewFromInt(1)) {
		t.Error("1 should be positive")
	}
	if IsPositive(decimal.NewFromInt(0)) {
		t.Error("0 should not be positive")
	}
	if IsPositive(decimal.NewFromInt(-1)) {
		t.Error("-1 should not be positive")
	}
}

func TestIsZero(t *testing.T) {
	if !IsZero(decimal.Zero) {
		t.Error("Zero should be zero")
	}
	if IsZero(decimal.NewFromInt(1)) {
		t.Error("1 should not be zero")
	}
}

func TestZero(t *testing.T) {
	if !Zero.IsZero() {
		t.Error("Zero constant should be zero")
	}
}
