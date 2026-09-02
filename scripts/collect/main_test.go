package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseOneUsesTensorCountForQuantizedCheckpoint(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/acme/Model-120B-A3B", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"acme/Model-120B-A3B","createdAt":"2026-01-01","config":{"architectures":["AcmeForConditionalGeneration"]},"safetensors":{"total":120000000000,"parameters":{"BF16":120000000000}}}`))
	})
	mux.HandleFunc("/acme/Model-120B-A3B/resolve/main/config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"num_hidden_layers":32,"hidden_size":4096,"num_attention_heads":32,"num_key_value_heads":8,"head_dim":128,"num_experts":64,"num_experts_per_tok":4,"max_position_embeddings":131072,"quantization_config":{"quant_method":"fp8"}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	m, ok, why := parseOne(srv.Client(), srv.URL, hfEntry{ID: "acme/Model-120B-A3B", Official: true}, 1)
	if !ok {
		t.Fatalf("parseOne failed: %s", why)
	}
	if m.Params != 120 || m.Active != 3 {
		t.Fatalf("parameter metadata should be dtype-independent and parse active size: %+v", m)
	}
	if !m.MoE || m.Experts != 64 || m.TopK != 4 {
		t.Fatalf("MoE routing metadata missing: %+v", m)
	}
	if !m.Official || m.Architecture != "AcmeForConditionalGeneration" || m.DType != "bfloat16" {
		t.Fatalf("official provenance or API metadata was lost: %+v", m)
	}
}

func TestParseOneDetectsSlidingWindowLayers(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models/acme/GemmaLike", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"acme/GemmaLike","createdAt":"2026-01-01","safetensors":{"total":4000000000}}`))
	})
	mux.HandleFunc("/acme/GemmaLike/resolve/main/config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"num_hidden_layers":6,"hidden_size":2048,"num_attention_heads":16,"num_key_value_heads":8,"head_dim":128,"max_position_embeddings":131072,"sliding_window":1024,"layer_types":["sliding_attention","sliding_attention","sliding_attention","sliding_attention","sliding_attention","full_attention"]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	m, ok, why := parseOne(srv.Client(), srv.URL, hfEntry{ID: "acme/GemmaLike"}, 1)
	if !ok {
		t.Fatalf("parseOne failed: %s", why)
	}
	if m.KVLayers != 6 || m.LocalLayers != 5 || m.Window != 1024 || m.StateMB != 0 {
		t.Fatalf("sliding-window structure missing: %+v", m)
	}
}

func TestSkipDerivedInferenceCheckpoints(t *testing.T) {
	for _, name := range []string{
		"DeepSeek-V4-DSpark",
		"Gemma4-27B-OBLITERATED",
		"Model-EAGLE3",
		"Model-Medusa",
		"Model-Speculative-Head",
	} {
		if !skipRe.MatchString(name) {
			t.Errorf("应过滤辅助/衍生 checkpoint %q", name)
		}
	}
	if skipRe.MatchString("Qwen3.8-2.4T-A95B") || skipRe.MatchString("Qwen2.5-7B-AWQ") {
		t.Error("不应过滤正式基础或官方量化模型")
	}
}

func TestPackagedCheckpointsSortAfterCanonicalModels(t *testing.T) {
	for _, name := range []string{"Qwen3.8-27B-FP8", "Qwen2.5-7B-AWQ", "Model-GGUF"} {
		if !packagedRe.MatchString(name) {
			t.Errorf("应降低封装量化 checkpoint 的优先级 %q", name)
		}
	}
	if packagedRe.MatchString("Qwen3.8-27B") {
		t.Error("正式基础模型不应被当作封装量化 checkpoint")
	}
}

func TestOfficialDiscoveryIncludesMultimodalAndDropsCommunityRepos(t *testing.T) {
	hasMultimodal := false
	for _, pipeline := range officialPipelines {
		hasMultimodal = hasMultimodal || pipeline == "image-text-to-text"
	}
	if !hasMultimodal {
		t.Fatal("official discovery must include multimodal LLMs")
	}

	models := []outModel{
		{ID: "qwen--qwen3.5-35b-a3b", Org: "Qwen"},
		{ID: "community--qwen3.5-35b-a3b-gguf", Org: "community"},
	}
	got := keepPublishers(models, publisherSet("Qwen"))
	if len(got) != 1 || got[0].Org != "Qwen" || !got[0].Official {
		t.Fatalf("official cleanup kept the wrong repositories: %+v", got)
	}
}

func TestParseOneUsesStructureBeforeRoundedModelName(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/acme/QwenLike-30B-A3B/resolve/main/config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"model_type":"qwen_moe","num_hidden_layers":48,"hidden_size":2048,"moe_intermediate_size":768,"num_attention_heads":32,"num_key_value_heads":4,"head_dim":128,"num_experts":128,"num_experts_per_tok":8,"max_position_embeddings":262144}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := hfEntry{ID: "acme/QwenLike-30B-A3B", Safetensors: &struct {
		Total      int64            `json:"total"`
		Parameters map[string]int64 `json:"parameters"`
	}{Total: 30_500_000_000}}
	m, ok, why := parseOne(srv.Client(), srv.URL, e, 1)
	if !ok {
		t.Fatalf("parseOne failed: %s", why)
	}
	if m.Active != 3.3 || m.MoELayers != 48 || m.MoEIntermediate != 768 {
		t.Fatalf("结构推导应得到 3.3B，而不是模型名四舍五入的 3B: %+v", m)
	}
}

func TestParseOneInfersStoredPrecisionFromPayload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/acme/RuntimeFP8/resolve/main/config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"num_hidden_layers":32,"hidden_size":4096,"num_attention_heads":32,"num_key_value_heads":8,"head_dim":128,"quantization_config":{"quant_method":"fp8"}}`))
	})
	mux.HandleFunc("/acme/RuntimeFP8/resolve/main/model.safetensors.index.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"metadata":{"total_size":200000000000}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := hfEntry{ID: "acme/RuntimeFP8", Safetensors: &struct {
		Total      int64            `json:"total"`
		Parameters map[string]int64 `json:"parameters"`
	}{Total: 100_000_000_000}}
	m, ok, why := parseOne(srv.Client(), srv.URL, e, 1)
	if !ok {
		t.Fatalf("parseOne failed: %s", why)
	}
	if m.NativeQuant != "fp16" || m.CheckpointGB != 200 {
		t.Fatalf("200GB/100B 参数应识别为 BF16/FP16 payload，而非运行时 FP8: %+v", m)
	}
}

func TestParseOneCollectsSingleShardPayload(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/acme/SingleShard/resolve/main/config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"num_hidden_layers":32,"hidden_size":4096,"num_attention_heads":32,"num_key_value_heads":8,"head_dim":128}`))
	})
	mux.HandleFunc("/api/models/acme/SingleShard", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"siblings":[{"rfilename":"model.safetensors","size":16000000000},{"rfilename":"tokenizer.json","size":1234}]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := hfEntry{ID: "acme/SingleShard", Safetensors: &struct {
		Total      int64            `json:"total"`
		Parameters map[string]int64 `json:"parameters"`
	}{Total: 8_000_000_000}}
	m, ok, why := parseOne(srv.Client(), srv.URL, e, 1)
	if !ok {
		t.Fatalf("parseOne failed: %s", why)
	}
	if m.CheckpointGB != 16 || m.NativeQuant != "fp16" {
		t.Fatalf("单文件 safetensors payload 未采集: %+v", m)
	}
}

func TestMergeModelsNeverDeletesOldEntries(t *testing.T) {
	fresh := []outModel{{ID: "qwen/new", Params: 8}, {ID: "qwen/shared", Params: 14}}
	old := []outModel{{ID: "qwen/old", Params: 7}, {ID: "qwen/shared", Params: 9}}

	got, carried := mergeModels(fresh, old)
	if carried != 1 || len(got) != 3 {
		t.Fatalf("merge must preserve old unique entries: carried=%d models=%+v", carried, got)
	}
	for _, m := range got {
		if m.ID == "qwen/shared" && m.Params != 14 {
			t.Fatalf("fresh metadata must replace the matching old entry: %+v", m)
		}
	}
}

func TestParseOneCollectsModelScopeMetadata(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/models/acme/MS-7B/resolve/master/config.json", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"architectures":["AcmeForCausalLM"],"torch_dtype":"bfloat16","model_type":"acme","num_hidden_layers":32,"hidden_size":4096,"intermediate_size":11008,"num_attention_heads":32,"num_key_value_heads":8,"head_dim":128,"max_position_embeddings":32768,"sliding_window":32768,"use_sliding_window":false}`))
	})
	mux.HandleFunc("/api/v1/models/acme/MS-7B/repo/files", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"Data":{"Files":[{"Path":"model-1.safetensors","Size":8000000000},{"Path":"model-2.safetensors","Size":6000000000},{"Path":"tokenizer.json","Size":1000}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	e := hfEntry{
		ID: "acme/MS-7B", Provider: "modelscope", ParameterCount: 494_032_768,
		CreatedAt: "2024-01-02T00:00:00Z", LastModified: "2025-03-04T00:00:00Z",
		License: "apache-2.0", Tasks: []string{"text-generation"},
	}
	m, ok, why := parseOne(srv.Client(), srv.URL, e, 0.1)
	if !ok {
		t.Fatalf("parseOne failed: %s", why)
	}
	if m.Src != "modelscope" || m.Params != 0.5 || m.CheckpointGB != 14 {
		t.Fatalf("ModelScope source, rounded params, or payload metadata missing: %+v", m)
	}
	if m.Architecture != "AcmeForCausalLM" || m.DType != "bfloat16" || m.License != "apache-2.0" {
		t.Fatalf("ModelScope inference metadata missing: %+v", m)
	}
	if m.LocalLayers != 0 || m.Window != 0 {
		t.Fatalf("disabled sliding window must not change KV layout: %+v", m)
	}
}
