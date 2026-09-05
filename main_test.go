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

func TestScreenshotScenarios(t *testing.T) {
	mustLoad()
	m := findModel("qwen--qwen3.8-27b")
	if m == nil || m.Heads != 24 || m.ParamSource != "safetensors" || m.Revision == "" {
		t.Fatalf("Qwen catalog metadata is incomplete: %+v", m)
	}
	w := []calc.WorkloadBucket{{Context: 8192, Output: 512, Share: 1}}
	plans := calc.Planner(hws, *m, calc.PlanOpts{TargetTPM: 1e6, QuantOnly: "fp4"}, w, 16, calc.Opts{})
	if len(plans) == 0 {
		t.Fatal("Qwen NVFP4 at 1M TPM must have a candidate in the real hardware catalog")
	}
	for _, p := range plans {
		if (p.Support != "supported" && p.Support != "conditional") || p.TPM < 1e6 {
			t.Fatalf("invalid or insufficient plan: %+v", p)
		}
	}
	rec := calc.Recommend(hws, models, *m, w, calc.RecommendOpts{TargetTPM: 6000, Conc: 16}, calc.Opts{})
	if len(rec.Picks) == 0 {
		t.Fatal("Qwen default prescription must not be empty")
	}
	for _, id := range []string{"deepseek-r1", "qwen3-32b", "qwen3-next-80b-a3b", "gpt-oss-120b"} {
		model := findModel(id)
		if model == nil {
			t.Fatalf("missing curated model %s", id)
		}
		if _, reason, valid := calc.ModelSupport(*model, calc.Opts{}); !valid {
			t.Errorf("curated model %s blocked by missing metadata: %s", id, reason)
		}
	}
	bad := *m
	bad.Heads = 0
	if _, reason, valid := calc.ModelSupport(bad, calc.Opts{}); valid || reason == "" {
		t.Fatal("missing query heads must explain the data issue without fabricating geometry")
	}
	perf := calc.ThroughputWorkload(*findHW("rtx2080"), *findModel("llama-3.1-70b"), calc.QuantByID("q4km"),
		[]calc.WorkloadBucket{{Context: 4096, Output: 512, Share: 1}}, 4, 4, calc.Opts{})
	if perf.Fit || perf.Deployable || perf.Mem.Weights <= perf.Mem.Cap {
		t.Fatalf("4x8GB cannot hold 70B Q4_K_M weights: %+v", perf)
	}
	perf = calc.ThroughputWorkload(*findHW("rtx4090"), *findModel("llama-3.1-8b"), calc.QuantByID("q4km"),
		[]calc.WorkloadBucket{{Context: 4096, Output: 512, Share: 1}}, 4, 1, calc.Opts{})
	if !perf.EstimateValid || !perf.Fit || perf.SingleTPS <= 0 {
		t.Fatalf("the default performance example must be calculable: %+v", perf)
	}
}
