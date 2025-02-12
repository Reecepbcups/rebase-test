package main

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
)

// 6 decimal places
const basePrecision = 1_000_000

// StockToken represents a rebasing token for any stock
type StockToken struct {
	ticker           string
	totalSupply      *big.Int
	balances         map[string]*big.Int
	rebaseMultiplier *big.Int
	sharePrice       *big.Int // in cents
}

// NewStockToken creates a new stock token contract
func NewStockToken(ticker string) *StockToken {
	return &StockToken{
		ticker:           ticker,
		totalSupply:      big.NewInt(0),
		balances:         make(map[string]*big.Int),
		rebaseMultiplier: big.NewInt(1),
		sharePrice:       dollarsToCents("$100.00"), // Initial price
	}
}

// Mint creates new tokens based on off-chain TSLA shares
func (t *StockToken) Mint(address string, shares uint64) {
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
func (t *StockToken) Rebase(action interface{}) {
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

		divAmt, _ := v.cashAmount.Float64()
		sharePrice, _ := v.sharePrice.Float64()
		divYield := divAmt / sharePrice
		fmt.Printf("\nSimulating $%.2f dividend at share price of $%.2f (Yield: %0.2f%%)...\n", divAmt/100, sharePrice/100, divYield*100)

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

// OndoWrappedStock represents a non-rebasing wrapper token
type OndoWrappedStock struct {
	ticker       string
	totalSupply  *big.Int
	balances     map[string]*big.Int
	exchangeRate *big.Int
}

// NewOndoWrappedStock creates a new wrapper token contract
func NewOndoWrappedStock(ticker string) *OndoWrappedStock {
	return &OndoWrappedStock{
		ticker:       fmt.Sprintf("ow%s", ticker),
		totalSupply:  big.NewInt(0),
		balances:     make(map[string]*big.Int),
		exchangeRate: big.NewInt(basePrecision),
	}
}

// Wrap converts TSLA tokens to owTSLA tokens
func (ow *OndoWrappedStock) Wrap(st *StockToken, from string, amount *big.Int) {
	if st.balances[from].Cmp(amount) < 0 {
		panic("Insufficient TSLA balance")
	}

	// Calculate owTSLA amount based on current exchange rate
	owAmount := new(big.Int).Mul(amount, big.NewInt(basePrecision))
	owAmount.Div(owAmount, ow.exchangeRate)

	// Transfer TSLA to wrapper contract
	st.balances[from].Sub(st.balances[from], amount)
	if st.balances[ow.ticker] == nil {
		st.balances[ow.ticker] = big.NewInt(0)
	}
	st.balances[ow.ticker].Add(st.balances[ow.ticker], amount)

	// Mint owTSLA to user
	if ow.balances[from] == nil {
		ow.balances[from] = big.NewInt(0)
	}
	ow.balances[from].Add(ow.balances[from], owAmount)
	ow.totalSupply.Add(ow.totalSupply, owAmount)
}

// Unwrap converts owTSLA tokens back to TSLA tokens
func (ow *OndoWrappedStock) Unwrap(st *StockToken, to string, owAmount *big.Int) {
	// Check the balance of the contract
	contractAddr := "0xCONTRACT"
	if ow.balances[contractAddr] == nil || ow.balances[contractAddr].Cmp(owAmount) < 0 {
		panic(fmt.Sprintf("Insufficient owTSLA balance for %s", contractAddr))
	}

	// Calculate TSLA amount based on current exchange rate
	tslaAmount := new(big.Int).Mul(owAmount, ow.exchangeRate)
	tslaAmount.Div(tslaAmount, big.NewInt(basePrecision))

	// Burn owTSLA from contract
	ow.balances[contractAddr].Sub(ow.balances[contractAddr], owAmount)
	ow.totalSupply.Sub(ow.totalSupply, owAmount)

	// Transfer TSLA from wrapper contract to recipient
	st.balances[ow.ticker].Sub(st.balances[ow.ticker], tslaAmount)
	if st.balances[to] == nil {
		st.balances[to] = big.NewInt(0)
	}
	st.balances[to].Add(st.balances[to], tslaAmount)
}

// UpdateExchangeRate recalculates the exchange rate after rebases
func (ow *OndoWrappedStock) UpdateExchangeRate(tsla *StockToken) {
	if ow.totalSupply.Sign() == 0 {
		return // No tokens wrapped, keep exchange rate as is
	}

	// New exchange rate = (TSLA balance in wrapper * basePrecision) / owTSLA total supply
	ow.exchangeRate = new(big.Int).Mul(tsla.balances[ow.ticker], big.NewInt(basePrecision))
	ow.exchangeRate.Div(ow.exchangeRate, ow.totalSupply)
}

func (ow *OndoWrappedStock) Transfer(from, to string, amount *big.Int) {
	if ow.balances[from].Cmp(amount) < 0 {
		panic("Insufficient balance")
	}

	if ow.balances[to] == nil {
		ow.balances[to] = big.NewInt(0)
	}

	ow.balances[from].Sub(ow.balances[from], amount)
	ow.balances[to].Add(ow.balances[to], amount)
}

// Interact handles token transfers, automatically wrapping if sending to a contract
func (t *StockToken) Interact(from, to string, amount *big.Int, ows *OndoWrappedStock) {
	fmt.Printf("Transferring %s%s from %s to %s\n", formatTokens(amount), t.ticker, from, to)

	// Check if recipient is a contract
	if strings.HasPrefix(to, "0xCONTRACT") {
		// Auto-wrap and transfer
		fmt.Println("Auto-wrapping tokens for contract interaction...")
		ows.Wrap(t, from, amount)

		// Calculate wrapped amount based on current exchange rate
		wrappedAmount := new(big.Int).Mul(amount, big.NewInt(basePrecision))
		wrappedAmount.Div(wrappedAmount, ows.exchangeRate)

		// Transfer wrapped tokens to contract
		ows.Transfer(from, to, wrappedAmount)
		return
	}

	// Regular transfer for non-contract addresses
	if t.balances[from].Cmp(amount) < 0 {
		panic("Insufficient balance")
	}

	if t.balances[to] == nil {
		t.balances[to] = big.NewInt(0)
	}

	t.balances[from].Sub(t.balances[from], amount)
	t.balances[to].Add(t.balances[to], amount)
}

// Claim unwraps and transfers tokens from contract to user
func (ow *OndoWrappedStock) Claim(st *StockToken, from, to string, wrappedAmount *big.Int) {
	if !strings.HasPrefix(from, "0xCONTRACT") {
		panic("Can only claim from contract addresses")
	}

	fmt.Printf("Claiming %s wrapped tokens...\n", formatTokens(wrappedAmount))

	// Check contract's wrapped token balance
	if ow.balances[from] == nil || ow.balances[from].Cmp(wrappedAmount) < 0 {
		fmt.Printf("Attempting to claim more than available. Max available: %s\n",
			formatTokens(ow.balances[from]))
		wrappedAmount = new(big.Int).Set(ow.balances[from])
	}

	// Calculate underlying amount based on exchange rate
	underlyingAmount := new(big.Int).Mul(wrappedAmount, ow.exchangeRate)
	underlyingAmount.Div(underlyingAmount, big.NewInt(basePrecision))

	fmt.Printf("This will receive %s underlying tokens at current exchange rate of %s\n",
		formatTokens(underlyingAmount),
		formatTokens(ow.exchangeRate))

	// Unwrap tokens directly to recipient
	ow.Unwrap(st, to, wrappedAmount)
}

// Helper to display balances and values
func displayBalances(st *StockToken, ow *OndoWrappedStock, userAddr, contractAddr string) {
	fmt.Printf("\nShare price: $%.2f\n", float64(st.sharePrice.Int64())/100)

	// User's base token balance
	baseBalance := formatTokens(st.balances[userAddr])
	baseValue := new(big.Int).Mul(st.balances[userAddr], st.sharePrice)
	baseValue.Div(baseValue, big.NewInt(basePrecision))
	fmt.Printf("%s balance: %s tokens ($%.2f)\n",
		st.ticker,
		baseBalance,
		float64(baseValue.Int64())/100)

	// Wrapper contract's base token balance
	wrapperBalance := formatTokens(st.balances[ow.ticker])
	wrapperValue := new(big.Int).Mul(st.balances[ow.ticker], st.sharePrice)
	wrapperValue.Div(wrapperValue, big.NewInt(basePrecision))
	fmt.Printf("%s balance in wrapper: %s tokens ($%.2f)\n",
		st.ticker,
		wrapperBalance,
		float64(wrapperValue.Int64())/100)

	// Contract's wrapped token balance
	wrappedBalance := formatTokens(ow.balances[contractAddr])
	wrappedValue := new(big.Int).Mul(ow.balances[contractAddr], st.sharePrice)
	wrappedValue.Mul(wrappedValue, ow.exchangeRate)
	wrappedValue.Div(wrappedValue, big.NewInt(basePrecision*basePrecision))
	fmt.Printf("%s balance of contract: %s tokens ($%.2f)\n",
		ow.ticker,
		wrappedBalance,
		float64(wrappedValue.Int64())/100)

	fmt.Printf("Exchange rate: %s\n", formatTokens(ow.exchangeRate))
}

func main() {
	// Initialize tokens
	stockToken := NewStockToken("TSLA")
	owStock := NewOndoWrappedStock("TSLA")

	reece := "0xREECE"
	contract := "0xCONTRACT"
	stockToken.Mint(reece, 10)

	sharePrice := float64(stockToken.sharePrice.Int64()) / 100
	dollarValueOfBalance := (float64(stockToken.balances[reece].Int64()) / basePrecision) * sharePrice
	fmt.Printf("Initial %s balance for %s: %s tokens ($%.2f)\n", stockToken.ticker, reece, formatTokens(stockToken.balances[reece]), dollarValueOfBalance)

	// Interact with contract (will auto-wrap)
	fmt.Println("\nInteracting with contract...")
	transferAmount := new(big.Int).Mul(big.NewInt(5), big.NewInt(basePrecision))
	stockToken.Interact(reece, contract, transferAmount, owStock)

	fmt.Println("\nAfter contract interaction:")
	displayBalances(stockToken, owStock, reece, contract)

	// Simulate a 2:1 stock split
	fmt.Println("\nSimulating 2:1 stock split...")
	stockToken.sharePrice.Div(stockToken.sharePrice, big.NewInt(2)) // Halve the price
	stockToken.Rebase(uint64(2))
	owStock.UpdateExchangeRate(stockToken)

	fmt.Println("\nAfter stock split:")
	displayBalances(stockToken, owStock, reece, contract)

	// Simulate a $1.50 dividend
	dividend := Dividend{
		cashAmount: dollarsToCents("$1.50"),
		sharePrice: stockToken.sharePrice,
	}
	stockToken.Rebase(dividend)
	owStock.UpdateExchangeRate(stockToken)

	fmt.Println("\nAfter dividend:")
	displayBalances(stockToken, owStock, reece, contract)

	// Claim wrapped tokens
	fmt.Println("\nClaiming tokens from contract...")
	claimAmount := new(big.Int).Mul(big.NewInt(1), big.NewInt(basePrecision))
	owStock.Claim(stockToken, contract, reece, claimAmount)

	fmt.Println("\nAfter claiming:")
	displayBalances(stockToken, owStock, reece, contract)
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
