package main

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// 6 decimal places
const basePrecision = 1_000_000

// TSLAToken represents a rebasing token for TSLA shares
type TSLAToken struct {
	totalSupply      *big.Int
	balances         map[string]*big.Int
	rebaseMultiplier *big.Int
}

// NewTSLAToken creates a new TSLA token contract
func NewTSLAToken() *TSLAToken {
	return &TSLAToken{
		totalSupply:      big.NewInt(0),
		balances:         make(map[string]*big.Int),
		rebaseMultiplier: big.NewInt(1),
	}
}

// Mint creates new tokens based on off-chain TSLA shares
func (t *TSLAToken) Mint(address string, shares uint64) {
	// Convert shares to precise units (multiply by basePrecision)
	amount := big.NewInt(int64(shares))
	amount.Mul(amount, big.NewInt(basePrecision))

	if t.balances[address] == nil {
		t.balances[address] = big.NewInt(0)
	}
	t.balances[address].Add(t.balances[address], amount)
	t.totalSupply.Add(t.totalSupply, amount)
}

// Dividend represents a cash dividend payment
type Dividend struct {
	cashAmount *big.Int // Amount in cents (e.g., $1.00 = 100)
	sharePrice *big.Int // Current share price in cents
}

// Rebase adjusts token supply based on corporate actions
func (t *TSLAToken) Rebase(action interface{}) {
	switch v := action.(type) {
	case uint64:
		// Handle stock split
		multiplier := big.NewInt(int64(v))

		// Update all balances for split
		for address := range t.balances {
			balance := t.balances[address]
			newBalance := new(big.Int).Mul(balance, multiplier)
			t.balances[address] = newBalance
		}

		t.rebaseMultiplier = multiplier

	case Dividend:
		// Let's use higher precision (10^6 = 1M) to handle small numbers
		precisionFactor := big.NewInt(basePrecision)

		// Convert cash dividend to equivalent shares at current price
		// ($1.50 / $100.00) = 0.015
		shareRatio := new(big.Int).Mul(precisionFactor, v.cashAmount)
		shareRatio.Div(shareRatio, v.sharePrice)

		// Update all balances for cash dividend
		for address := range t.balances {
			balance := t.balances[address]

			// Calculate dividend shares with proper precision
			dividendShares := new(big.Int).Mul(balance, shareRatio)
			dividendShares.Div(dividendShares, precisionFactor)

			// Add the dividend shares to the balance
			t.balances[address].Add(t.balances[address], dividendShares)
		}
	}
}

func main() {
	// Initialize token
	tslaToken := NewTSLAToken()

	// Mint tokens to your address based on off-chain TSLA shares
	reece := "0xREECE"
	tslaToken.Mint(reece, 10) // Mint 1 TSLA token

	fmt.Printf("Initial balance for %s: %s tokens\n", reece, formatTokens(tslaToken.balances[reece]))

	// Simulate a 2:1 stock split
	tslaToken.Rebase(uint64(2))
	fmt.Printf("Balance after 2:1 stock split for %s: %s tokens\n",
		reece, formatTokens(tslaToken.balances[reece]))

	// Simulate a $1.50 cash dividend when share price is $100.00
	dividend := Dividend{
		cashAmount: dollarsToCents("$1.50"),
		sharePrice: dollarsToCents("$100.00"),
	}
	tslaToken.Rebase(dividend)
	fmt.Printf("Balance after $1.50 dividend for %s: %s tokens\n",
		reece, formatTokens(tslaToken.balances[reece]))
}

// formatTokens converts the raw balance to a human-readable string with 6 decimal places
func formatTokens(raw *big.Int) string {
	whole := new(big.Int).Div(raw, big.NewInt(basePrecision))
	frac := new(big.Int).Mod(raw, big.NewInt(basePrecision))
	return fmt.Sprintf("%d.%06d", whole, frac)
}

func dollarsToCents(dollars interface{}) *big.Int {
	switch v := dollars.(type) {
	case float64:
		return big.NewInt(int64(v * 100))
	case float32:
		return big.NewInt(int64(v * 100))
	case int:
		return big.NewInt(int64(v * 100))
	case int64:
		return big.NewInt(v * 100)
	case uint:
		return big.NewInt(int64(v * 100))
	case uint64:
		return big.NewInt(int64(v * 100))
	case string:
		// Remove currency symbols and whitespace
		s := strings.TrimSpace(v)
		s = strings.TrimPrefix(s, "$")
		s = strings.ReplaceAll(s, ",", "")

		// Parse as float to handle decimal points
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			panic(fmt.Sprintf("Invalid dollar amount: %s", v))
		}

		// Convert to cents
		return big.NewInt(int64(f * 100))
	case *big.Int:
		return new(big.Int).Mul(v, big.NewInt(100))
	default:
		panic(fmt.Sprintf("Unsupported type for dollar amount: %T", dollars))
	}
}
