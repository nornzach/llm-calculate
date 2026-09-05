package main

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"testing"

	"llmcalc/calc"
)

func TestEnrichModelPreservesCuratedInferenceData(t *testing.T) {
	dst := calc.Model{ID: "qwen", Params: 7, Layers: 28, Src: "curated", Conf: "official"}
	src := calc.Model{
		ID:           "qwen",
		Params:       8,
		Layers:       32,
		Architecture: "Qwen2ForCausalLM",
		DType:        "bfloat16",
		SourceURL:    "https://huggingface.co/Qwen/Qwen2.5-7B-Instruct",
		License:      "apache-2.0",
		Official:     true,
	}

	enrichModel(&dst, src)

	if dst.Params != 7 || dst.Layers != 28 || dst.Src != "curated" || dst.Conf != "official" {
		t.Fatalf("curated inference data was overwritten: %+v", dst)
	}
}

func TestValidStackRejectsUnknownOptions(t *testing.T) {
	if !validStack("vllm", "none", "fp16") {
		t.Fatal("known inference stack must be accepted")
	}
	for _, bad := range [][3]string{{"bogus", "none", "fp16"}, {"vllm", "bogus", "fp16"}, {"vllm", "none", "int4"}} {
		if validStack(bad[0], bad[1], bad[2]) {
			t.Fatalf("unknown inference option must be rejected: %v", bad)
		}
	}
}

func TestSanitizeWorkloadRejectsInvalidBuckets(t *testing.T) {
	valid := []calc.WorkloadBucket{{Context: 8192, Output: 512, Share: 0.8, PrefixHit: 0.2}, {Context: 65536, Output: 1024, Share: 0.2}}
	if got, err := sanitizeWorkload(valid); err != nil || len(got) != 2 {
		t.Fatalf("valid workload rejected: got=%v err=%v", got, err)
	}
	invalid := [][]calc.WorkloadBucket{
		nil,
		{{Context: 511, Output: 512, Share: 1}},
		{{Context: 8192, Output: 0, Share: 1}},
		{{Context: 8192, Output: 512, Share: 0}},
		{{Context: 8192, Output: 512, Share: 1, PrefixHit: 0.91}},
	}
	for _, workload := range invalid {
		if _, err := sanitizeWorkload(workload); err == nil {
			t.Errorf("invalid workload accepted: %+v", workload)
		}
	}
}

func TestWriteJSONRejectsNonFiniteNumbers(t *testing.T) {
	response := httptest.NewRecorder()
	writeJSON(response, map[string]float64{"bad": math.Inf(1)})
	if response.Code != 500 || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("non-finite response must fail before writing partial JSON: status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestValidatePlanOptionsRejectsInconsistentBounds(t *testing.T) {
	for _, po := range []calc.PlanOpts{
		{TargetTPM: -1},
		{MinTOS: -1},
		{Objective: "unknown"},
		{Queue: true, MaxQ: 15},
	} {
		if err := validatePlanOptions(po, 16); err == nil {
			t.Errorf("invalid planner options accepted: %+v", po)
		}
	}
	if err := validatePlanOptions(calc.PlanOpts{TargetTPM: 6000, Objective: "cost", Queue: true, MaxQ: 16}, 16); err != nil {
		t.Fatalf("valid planner bounds rejected: %v", err)
	}
}
