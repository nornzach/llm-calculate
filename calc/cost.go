// Package calc cost estimation for LLM API token usage.
package calc

import (
	"fmt"
	"math"
)

// CostResult holds the calculated costs for a single request, daily, and monthly periods.
type CostResult struct {
	SingleRequestUSD float64 `json:"single_request_usd"`
	DailyCostUSD     float64 `json:"daily_cost_usd"`
	MonthlyCostUSD   float64 `json:"monthly_cost_usd"`
}

// Cost calculates token-based API costs given input/output token counts and per-million pricing.
// inputTokens: number of input tokens in a single request
// outputTokens: number of output tokens in a single request
// inputPricePerMillion: price per million input tokens (USD)
// outputPricePerMillion: price per million output tokens (USD)
// requestsPerDay: number of requests per day
//
// Returns an error if any parameter is negative or non-finite (NaN, Inf).
// Returns a CostResult with costs calculated over three time horizons:
// - Single request
// - Daily (based on requestsPerDay)
// - Monthly (30 days)
func Cost(inputTokens, outputTokens, inputPricePerMillion, outputPricePerMillion, requestsPerDay float64) (*CostResult, error) {
	// Check for NaN and Inf first (before checking for negative values)
	for _, val := range []float64{inputTokens, outputTokens, inputPricePerMillion, outputPricePerMillion, requestsPerDay} {
		if math.IsNaN(val) || math.IsInf(val, 0) {
			return nil, fmt.Errorf("parameters must be finite numbers (got NaN or Inf)")
		}
	}

	// Validate all parameters are non-negative
	if inputTokens < 0 || outputTokens < 0 || inputPricePerMillion < 0 || outputPricePerMillion < 0 || requestsPerDay < 0 {
		return nil, fmt.Errorf("all parameters must be non-negative")
	}

	// Calculate cost per single request
	singleRequestCost := (inputTokens*inputPricePerMillion + outputTokens*outputPricePerMillion) / 1_000_000

	// Calculate daily cost (requests per day * single request cost)
	dailyCost := singleRequestCost * requestsPerDay

	// Calculate monthly cost (30 days)
	monthlyCost := dailyCost * 30

	return &CostResult{
		SingleRequestUSD: singleRequestCost,
		DailyCostUSD:     dailyCost,
		MonthlyCostUSD:   monthlyCost,
	}, nil
}
