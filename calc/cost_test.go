package calc

import (
	"math"
	"testing"
)

func TestCostNormalCalculation(t *testing.T) {
	tests := []struct {
		name                string
		inputTokens         float64
		outputTokens        float64
		inputPrice          float64
		outputPrice         float64
		requestsPerDay      float64
		expectedSingle      float64
		expectedDaily       float64
		expectedMonthly     float64
	}{
		{
			name:            "Basic pricing",
			inputTokens:     100,
			outputTokens:    200,
			inputPrice:      0.5,
			outputPrice:     1.5,
			requestsPerDay:  1000,
			expectedSingle:  (100*0.5 + 200*1.5) / 1_000_000,
			expectedDaily:   (100*0.5 + 200*1.5) / 1_000_000 * 1000,
			expectedMonthly: (100*0.5 + 200*1.5) / 1_000_000 * 1000 * 30,
		},
		{
			name:            "High volume requests",
			inputTokens:     1000,
			outputTokens:    2000,
			inputPrice:      1.0,
			outputPrice:     2.0,
			requestsPerDay:  10000,
			expectedSingle:  (1000*1.0 + 2000*2.0) / 1_000_000,
			expectedDaily:   (1000*1.0 + 2000*2.0) / 1_000_000 * 10000,
			expectedMonthly: (1000*1.0 + 2000*2.0) / 1_000_000 * 10000 * 30,
		},
		{
			name:            "Single request per day",
			inputTokens:     500,
			outputTokens:    1000,
			inputPrice:      0.3,
			outputPrice:     0.6,
			requestsPerDay:  1,
			expectedSingle:  (500*0.3 + 1000*0.6) / 1_000_000,
			expectedDaily:   (500*0.3 + 1000*0.6) / 1_000_000 * 1,
			expectedMonthly: (500*0.3 + 1000*0.6) / 1_000_000 * 1 * 30,
		},
		{
			name:            "Only input tokens",
			inputTokens:     500,
			outputTokens:    0,
			inputPrice:      1.0,
			outputPrice:     0,
			requestsPerDay:  100,
			expectedSingle:  (500 * 1.0) / 1_000_000,
			expectedDaily:   (500 * 1.0) / 1_000_000 * 100,
			expectedMonthly: (500 * 1.0) / 1_000_000 * 100 * 30,
		},
		{
			name:            "Only output tokens",
			inputTokens:     0,
			outputTokens:    500,
			inputPrice:      0,
			outputPrice:     2.0,
			requestsPerDay:  50,
			expectedSingle:  (500 * 2.0) / 1_000_000,
			expectedDaily:   (500 * 2.0) / 1_000_000 * 50,
			expectedMonthly: (500 * 2.0) / 1_000_000 * 50 * 30,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Cost(test.inputTokens, test.outputTokens, test.inputPrice, test.outputPrice, test.requestsPerDay)
			if err != nil {
				t.Fatalf("Cost() returned unexpected error: %v", err)
			}
			if result == nil {
				t.Fatal("Cost() returned nil result")
			}

			if !floatAlmostEqual(result.SingleRequestUSD, test.expectedSingle, 1e-12) {
				t.Errorf("SingleRequestUSD: got %g, want %g", result.SingleRequestUSD, test.expectedSingle)
			}
			if !floatAlmostEqual(result.DailyCostUSD, test.expectedDaily, 1e-9) {
				t.Errorf("DailyCostUSD: got %g, want %g", result.DailyCostUSD, test.expectedDaily)
			}
			if !floatAlmostEqual(result.MonthlyCostUSD, test.expectedMonthly, 1e-7) {
				t.Errorf("MonthlyCostUSD: got %g, want %g", result.MonthlyCostUSD, test.expectedMonthly)
			}
		})
	}
}

func TestCostEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		inputTokens    float64
		outputTokens   float64
		inputPrice     float64
		outputPrice    float64
		requestsPerDay float64
	}{
		{
			name:           "Zero tokens",
			inputTokens:    0,
			outputTokens:   0,
			inputPrice:     1.0,
			outputPrice:    1.0,
			requestsPerDay: 100,
		},
		{
			name:           "Zero requests per day",
			inputTokens:    100,
			outputTokens:   200,
			inputPrice:     0.5,
			outputPrice:    1.5,
			requestsPerDay: 0,
		},
		{
			name:           "Zero pricing",
			inputTokens:    1000,
			outputTokens:   2000,
			inputPrice:     0,
			outputPrice:    0,
			requestsPerDay: 1000,
		},
		{
			name:           "All zeros",
			inputTokens:    0,
			outputTokens:   0,
			inputPrice:     0,
			outputPrice:    0,
			requestsPerDay: 0,
		},
		{
			name:           "Large numbers",
			inputTokens:    1e6,
			outputTokens:   1e6,
			inputPrice:     10,
			outputPrice:    20,
			requestsPerDay: 1e4,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Cost(test.inputTokens, test.outputTokens, test.inputPrice, test.outputPrice, test.requestsPerDay)
			if err != nil {
				t.Fatalf("Cost() returned unexpected error for edge case: %v", err)
			}
			if result == nil {
				t.Fatal("Cost() returned nil result for edge case")
			}

			// All results should be non-negative and finite
			if result.SingleRequestUSD < 0 || math.IsNaN(result.SingleRequestUSD) || math.IsInf(result.SingleRequestUSD, 0) {
				t.Errorf("SingleRequestUSD invalid: %g", result.SingleRequestUSD)
			}
			if result.DailyCostUSD < 0 || math.IsNaN(result.DailyCostUSD) || math.IsInf(result.DailyCostUSD, 0) {
				t.Errorf("DailyCostUSD invalid: %g", result.DailyCostUSD)
			}
			if result.MonthlyCostUSD < 0 || math.IsNaN(result.MonthlyCostUSD) || math.IsInf(result.MonthlyCostUSD, 0) {
				t.Errorf("MonthlyCostUSD invalid: %g", result.MonthlyCostUSD)
			}
		})
	}
}

func TestCostInvalidInputs(t *testing.T) {
	tests := []struct {
		name            string
		inputTokens     float64
		outputTokens    float64
		inputPrice      float64
		outputPrice     float64
		requestsPerDay  float64
		shouldError     bool
		errorContains   string
	}{
		{
			name:           "Negative input tokens",
			inputTokens:    -100,
			outputTokens:   200,
			inputPrice:     0.5,
			outputPrice:    1.5,
			requestsPerDay: 100,
			shouldError:    true,
			errorContains:  "non-negative",
		},
		{
			name:           "Negative output tokens",
			inputTokens:    100,
			outputTokens:   -200,
			inputPrice:     0.5,
			outputPrice:    1.5,
			requestsPerDay: 100,
			shouldError:    true,
			errorContains:  "non-negative",
		},
		{
			name:           "Negative input price",
			inputTokens:    100,
			outputTokens:   200,
			inputPrice:     -0.5,
			outputPrice:    1.5,
			requestsPerDay: 100,
			shouldError:    true,
			errorContains:  "non-negative",
		},
		{
			name:           "Negative output price",
			inputTokens:    100,
			outputTokens:   200,
			inputPrice:     0.5,
			outputPrice:    -1.5,
			requestsPerDay: 100,
			shouldError:    true,
			errorContains:  "non-negative",
		},
		{
			name:           "Negative requests per day",
			inputTokens:    100,
			outputTokens:   200,
			inputPrice:     0.5,
			outputPrice:    1.5,
			requestsPerDay: -100,
			shouldError:    true,
			errorContains:  "non-negative",
		},
		{
			name:           "NaN input tokens",
			inputTokens:    math.NaN(),
			outputTokens:   200,
			inputPrice:     0.5,
			outputPrice:    1.5,
			requestsPerDay: 100,
			shouldError:    true,
			errorContains:  "finite",
		},
		{
			name:           "Inf input tokens",
			inputTokens:    math.Inf(1),
			outputTokens:   200,
			inputPrice:     0.5,
			outputPrice:    1.5,
			requestsPerDay: 100,
			shouldError:    true,
			errorContains:  "finite",
		},
		{
			name:           "NaN output price",
			inputTokens:    100,
			outputTokens:   200,
			inputPrice:     0.5,
			outputPrice:    math.NaN(),
			requestsPerDay: 100,
			shouldError:    true,
			errorContains:  "finite",
		},
		{
			name:           "Inf requests per day",
			inputTokens:    100,
			outputTokens:   200,
			inputPrice:     0.5,
			outputPrice:    1.5,
			requestsPerDay: math.Inf(-1),
			shouldError:    true,
			errorContains:  "finite",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Cost(test.inputTokens, test.outputTokens, test.inputPrice, test.outputPrice, test.requestsPerDay)

			if test.shouldError {
				if err == nil {
					t.Fatal("Cost() expected error but got none")
				}
				if test.errorContains != "" && !contains(err.Error(), test.errorContains) {
					t.Errorf("Error message does not contain %q: %v", test.errorContains, err)
				}
			} else {
				if err != nil {
					t.Fatalf("Cost() unexpected error: %v", err)
				}
				if result == nil {
					t.Fatal("Cost() returned nil result")
				}
			}
		})
	}
}

func TestCostMonthlyCalculation(t *testing.T) {
	// Verify that monthly cost is exactly 30 times the daily cost
	result, err := Cost(100, 200, 0.5, 1.5, 1000)
	if err != nil {
		t.Fatalf("Cost() returned error: %v", err)
	}

	expectedMonthly := result.DailyCostUSD * 30
	if !floatAlmostEqual(result.MonthlyCostUSD, expectedMonthly, 1e-10) {
		t.Errorf("Monthly cost should be exactly 30 * daily: got %g, want %g", result.MonthlyCostUSD, expectedMonthly)
	}
}

func TestCostDailyCalculation(t *testing.T) {
	// Verify that daily cost is exactly requestsPerDay times single request cost
	result, err := Cost(100, 200, 0.5, 1.5, 1000)
	if err != nil {
		t.Fatalf("Cost() returned error: %v", err)
	}

	expectedDaily := result.SingleRequestUSD * 1000
	if !floatAlmostEqual(result.DailyCostUSD, expectedDaily, 1e-12) {
		t.Errorf("Daily cost should be exactly requestsPerDay * single: got %g, want %g", result.DailyCostUSD, expectedDaily)
	}
}

// Helper function to compare floats with tolerance
func floatAlmostEqual(a, b, tolerance float64) bool {
	if a == b {
		return true
	}
	diff := math.Abs(a - b)
	if math.IsNaN(diff) {
		return false
	}
	return diff <= tolerance
}

// Helper function to check if error message contains substring
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
