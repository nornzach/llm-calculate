package main

import (
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
	if dst.Architecture != src.Architecture || dst.DType != src.DType || dst.SourceURL != src.SourceURL || dst.License != src.License || !dst.Official {
		t.Fatalf("platform metadata was not merged: %+v", dst)
	}
}

func TestFetchedCatalogContainsCurrentOfficialModels(t *testing.T) {
	b, err := embedded.ReadFile("data/models_hf.json")
	if err != nil {
		t.Fatal(err)
	}
	models, err := calc.LoadModels(b)
	if err != nil {
		t.Fatal(err)
	}
	required := map[string]bool{
		"qwen--qwen3.5-35b-a3b": false,
		"qwen--qwen3.8-27b":     false,
	}
	for _, model := range models {
		if !model.Official {
			t.Fatalf("unverified repository remains in fetched catalog: %s", model.ID)
		}
		if _, ok := required[model.ID]; ok {
			required[model.ID] = model.Architecture != "" && model.DType != ""
		}
	}
	for id, complete := range required {
		if !complete {
			t.Fatalf("required official model is missing metadata: %s", id)
		}
	}
}

func TestEmbeddedHardwareCatalogIsStructurallyValid(t *testing.T) {
	b, err := embedded.ReadFile("data/hardware.json")
	if err != nil {
		t.Fatal(err)
	}
	hardware, err := calc.LoadHW(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(hardware) != 121 {
		t.Fatalf("unexpected hardware catalog size: %d", len(hardware))
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
