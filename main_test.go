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
	}

	enrichModel(&dst, src)

	if dst.Params != 7 || dst.Layers != 28 || dst.Src != "curated" || dst.Conf != "official" {
		t.Fatalf("curated inference data was overwritten: %+v", dst)
	}
	if dst.Architecture != src.Architecture || dst.DType != src.DType || dst.SourceURL != src.SourceURL || dst.License != src.License {
		t.Fatalf("platform metadata was not merged: %+v", dst)
	}
}
