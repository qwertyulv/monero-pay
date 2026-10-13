package main

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// AtomicFactor defines the number of atomic units in one XMR.
var AtomicFactor = big.NewInt(1000000000000)

// ConvertUSDToAtomic converts USD cents to XMR atomic units using exact rational arithmetic.
// usdCents is the integer USD amount in cents, such as 1050 for $10.50.
// xmrUsdRate is the USD price of one XMR, such as 150.25.
func ConvertUSDToAtomic(usdCents int64, xmrUsdRate float64) (uint64, error) {
	if usdCents <= 0 {
		return 0, fmt.Errorf("invalid USD amount: %d", usdCents)
	}
	if xmrUsdRate <= 0 {
		return 0, fmt.Errorf("invalid XMR rate: %f", xmrUsdRate)
	}

	xmrUsdRateStr := fmt.Sprintf("%.8f", xmrUsdRate)

	// Use big.Rat to avoid repeated floating-point arithmetic during conversion.
	rateRat, ok := new(big.Rat).SetString(xmrUsdRateStr)
	if !ok || rateRat.Sign() <= 0 {
		return 0, fmt.Errorf("invalid XMR rate parsing: %s", xmrUsdRateStr)
	}

	// Convert the integer USD cents value into a dollar-denominated rational number.
	dollarsRat := big.NewRat(usdCents, 100)

	// XMR Amount Rat = USD Dollars / Rate
	xmrAmountRat := new(big.Rat).Quo(dollarsRat, rateRat)

	// Atomic Amount Rat = XMR Amount * 10^12
	atomicFactorRat := new(big.Rat).SetInt(AtomicFactor)
	atomicRat := new(big.Rat).Mul(xmrAmountRat, atomicFactorRat)

	// Round upward so the merchant does not lose value through fractional atomic-unit rounding.
	num := atomicRat.Num()
	denom := atomicRat.Denom()

	quotient := new(big.Int).Quo(num, denom)
	remainder := new(big.Int).Rem(num, denom)

	if remainder.Sign() > 0 {
		quotient.Add(quotient, big.NewInt(1)) // Apply ceiling rounding.
	}

	if !quotient.IsUint64() {
		return 0, fmt.Errorf("atomic calculation overflow")
	}

	return quotient.Uint64(), nil
}

// AtomicToXMRString formats an atomic XMR amount without floating-point conversion.
func AtomicToXMRString(amount uint64) string {
	whole := amount / 1_000_000_000_000
	fraction := amount % 1_000_000_000_000
	if fraction == 0 {
		return strconv.FormatUint(whole, 10)
	}
	frac := fmt.Sprintf("%012d", fraction)
	frac = strings.TrimRight(frac, "0")
	return strconv.FormatUint(whole, 10) + "." + frac
}

const atomicUnitsPerXMR uint64 = 1_000_000_000_000
const maxSQLiteInteger uint64 = 1<<63 - 1

// ParseXMRToAtomic converts a plain decimal XMR amount into atomic units without floating-point math.
// It accepts up to 12 decimal places and caps values at SQLite's signed 64-bit integer range.
func ParseXMRToAtomic(value string) (uint64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, fmt.Errorf("amount_xmr is required")
	}
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		return 0, fmt.Errorf("amount_xmr must be a positive decimal number")
	}
	if strings.ContainsAny(value, "eE") {
		return 0, fmt.Errorf("amount_xmr must use plain decimal notation, for example 0.05")
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || parts[0] == "" {
		return 0, fmt.Errorf("amount_xmr must use plain decimal notation, for example 0.05")
	}
	for _, digit := range parts[0] {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("amount_xmr contains an invalid character")
		}
	}
	whole, err := strconv.ParseUint(parts[0], 10, 64)
	if err != nil || whole > maxSQLiteInteger/atomicUnitsPerXMR {
		return 0, fmt.Errorf("amount_xmr is too large")
	}

	var fraction uint64
	if len(parts) == 2 {
		decimal := parts[1]
		if len(decimal) == 0 || len(decimal) > 12 {
			return 0, fmt.Errorf("amount_xmr supports at most 12 decimal places")
		}
		for _, digit := range decimal {
			if digit < '0' || digit > '9' {
				return 0, fmt.Errorf("amount_xmr contains an invalid character")
			}
		}
		padded := decimal + strings.Repeat("0", 12-len(decimal))
		fraction, err = strconv.ParseUint(padded, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid amount_xmr fraction")
		}
	}

	atomic := whole * atomicUnitsPerXMR
	if fraction > maxSQLiteInteger-atomic {
		return 0, fmt.Errorf("amount_xmr is too large for the invoice database")
	}
	atomic += fraction
	if atomic == 0 {
		return 0, fmt.Errorf("amount_xmr must be at least one atomic unit")
	}
	return atomic, nil
}
