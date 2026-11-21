package main

import (
	"encoding/json"
	"math"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"llmcalc/calc"
)

func TestBinaryUpdateIgnoresStaleWorkingDirectoryCatalog(t *testing.T) {
	// Reproduce a binary-only deployment with a pre-metadata-refresh JSON
	// left in its working directory. The released catalog must still win.
	b, err := embedded.ReadFile("data/models_hf.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog []calc.Model
	if err := json.Unmarshal(b, &catalog); err != nil {
		t.Fatal(err)
	}
	var old calc.Model
	for _, m := range catalog {
		if m.ID == "qwen--qwen3.8-27b" {
			old = m
			break
		}
	}
	if old.ID == "" {
		t.Fatal("screenshot model missing from bundled catalog")
	}
	old.Heads, old.Revision, old.ParamSource = 0, "", ""
	t.Chdir(t.TempDir())
	if err := os.Mkdir("data", 0755); err != nil {
		t.Fatal(err)
	}
	stale, _ := json.Marshal([]calc.Model{old})
	if err := os.WriteFile(filepath.Join("data", "models_hf.json"), stale, 0644); err != nil {
		t.Fatal(err)
	}
	mustLoad("")
	m := findModel(old.ID)
	if m == nil || m.Heads != 24 || m.Revision == "" || m.ParamSource != "safetensors" {
		t.Fatalf("working-directory JSON overrode the released catalog: %+v", m)
	}
	w := []calc.WorkloadBucket{{Context: 8192, Output: 512, Share: 1}}
	result := calc.Recommend(hws, models, *m, w, calc.RecommendOpts{TargetTPM: 6000, Conc: 16, Objectives: "cost,tos"}, calc.Opts{})
	if len(result.Pareto) == 0 {
		t.Fatal("stale files must not break the screenshot's default map scenario")
	}
}

func TestExplicitCatalogOverride(t *testing.T) {
	b, err := embedded.ReadFile("data/models.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog []calc.Model
	if err := json.Unmarshal(b, &catalog); err != nil {
		t.Fatal(err)
	}
	catalog[0].ID = "explicit-override"
	valid, _ := json.Marshal(catalog[:1])
	for _, filename := range []string{"models_hf.json", "models_modelscope.json"} {
		for _, tc := range []struct{ name, body string }{
			{"missing", ""}, {"malformed", "{"}, {"invalid-model", `[{"id":"broken"}]`}, {"valid", string(valid)},
		} {
			t.Run(filename+"/"+tc.name, func(t *testing.T) {
				dir := t.TempDir()
				if tc.body != "" {
					if err := os.WriteFile(filepath.Join(dir, filename), []byte(tc.body), 0644); err != nil {
						t.Fatal(err)
					}
				}
				mustLoad(dir)
				if tc.name != "valid" {
					if m := findModel("qwen--qwen3.8-27b"); m == nil || m.Heads != 24 {
						t.Fatal("missing or malformed override must preserve the bundled catalog")
					}
				} else if findModel("explicit-override") == nil {
					t.Fatal("explicit catalog directory was ignored")
				}
			})
		}
	}
}

func TestAPIContracts(t *testing.T) {
	mustLoad("")
	handler := newHandler()
	request := func(path, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		method := "GET"
		if body != "" {
			method = "POST"
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(method, path, strings.NewReader(body)))
		if response.Code != status || (strings.HasPrefix(path, "/api/") && !json.Valid(response.Body.Bytes())) {
			t.Fatalf("%s %s: want %d, got %d: %.400s", method, path, status, response.Code, response.Body.String())
		}
		return response
	}
	for _, path := range []string{"/", "/index.html", "/api/hardware", "/api/models", "/api/quants", "/api/engines", "/api/specs", "/api/quick"} {
		request(path, "", 200)
	}
	workload := `"workload":[{"context":4096,"output":512,"share":1}]`
	perfBody := `{"hw":"h200","model":"llama-3.1-8b","quant":"fp16","batch":4,` + workload + `}`
	var result struct {
		Perf  calc.Perf `json:"perf"`
		Curve []struct {
			B      int     `json:"b"`
			Single float64 `json:"single"`
			Used   float64 `json:"used"`
		} `json:"curve"`
	}
	json.Unmarshal(request("/api/perf", perfBody, 200).Body.Bytes(), &result)
	if !result.Perf.EstimateValid || !result.Perf.Fit {
		t.Fatalf("valid performance rejected: %+v", result.Perf)
	}
	matched := false
	for _, pt := range result.Curve {
		if pt.B == 4 {
			matched = true
			if pt.Single != result.Perf.SingleTPS || pt.Used != result.Perf.Mem.P999Total {
				t.Fatal("sweep and current calculation disagree")
			}
		}
	}
	if !matched {
		t.Fatal("sweep omitted current concurrency")
	}
	request("/api/fit", `{"hw":"h200","models":["llama-3.1-8b"],"batch":4,`+workload+`}`, 200)
	var plans []calc.Plan
	json.Unmarshal(request("/api/plan", `{"model":"llama-3.1-8b","tpm":6000,"conc":4,"quant_only":"fp16","advanced":{"bw_util":0.3},`+workload+`}`, 200).Body.Bytes(), &plans)
	if len(plans) == 0 {
		t.Fatal("planner returned no candidates")
	}
	for _, p := range plans {
		if p.Accuracy != "scenario" || p.TPM < 6000 {
			t.Fatalf("lost planner assumptions or capacity: %+v", p)
		}
	}
	request("/api/recommend", `{"model":"llama-3.1-8b","tpm":6000,"conc":4,"objectives":"cost,tos",`+workload+`}`, 200)
	card := `{"direction":"card","hw":"h200","cards":1,"conc":4,` + workload
	base := request("/api/recommend", card+`}`, 200).Body.String()
	stale := request("/api/recommend", card+`,"tpm":1e20,"tos":1e6,"queue":true,"maxq":1}`, 200).Body.String()
	if base != stale {
		t.Fatal("disabled model-direction controls leaked into fixed hardware results")
	}
	for _, bad := range []struct {
		path, body string
		status     int
	}{
		{"/api/perf", strings.Replace(perfBody, "h200", "missing", 1), 404},
		{"/api/perf", strings.Replace(perfBody, "fp16", "unknown", 1), 400},
		{"/api/perf", strings.Replace(perfBody, "4096", "-1", 1), 400},
		{"/api/perf", strings.Replace(perfBody, `"batch":4`, `"eng":"unknown"`, 1), 400},
		{"/api/perf", strings.Replace(perfBody, `"batch":4`, `"kvq":"int4"`, 1), 400},
		{"/api/recommend", `{"direction":"invalid"}`, 400},
		{"/api/recommend", `{"objectives":"banana"}`, 400},
		{"/api/recommend", `{"objectives":"cost,tos,avail"}`, 400},
		{"/api/recommend", `{"direction":"card","hw":"missing"}`, 404},
		{"/api/recommend", `{"direction":"card","hw":"h200","cards":9}`, 400},
		{"/api/plan", `{"model":"llama-3.1-8b","conc":16,"queue":true,"maxq":8,` + workload + `}`, 400},
	} {
		request(bad.path, bad.body, bad.status)
	}
	for _, path := range []string{"/api/fit", "/api/perf", "/api/plan", "/api/recommend"} {
		for _, body := range []string{`{`, `{} {}`, `{"typo":1}`, `{"model":"` + strings.Repeat("a", 1<<20) + `"}`} {
			request(path, body, 400)
		}
	}
	custom := sanitizeCustom(&calc.Model{Params: 8, Hidden: 4096, Heads: 32, Dim: 128, Layers: 32, KVT: "mha", KVH: 8}, "en")
	if custom.KVH != 32 {
		t.Fatal("MHA must have one KV head for every query head")
	}
	b, _ := json.Marshal(custom)
	json.Unmarshal(request("/api/perf", `{"hw":"h200","custom":`+string(b)+`,"batch":1,`+workload+`}`, 200).Body.Bytes(), &result)
	if !result.Perf.EstimateValid || result.Perf.Accuracy != "scenario" {
		t.Fatalf("custom MHA scenario is not usable: valid=%v accuracy=%s reason=%s", result.Perf.EstimateValid, result.Perf.Accuracy, result.Perf.SupportReason)
	}
}

func TestHomepageCalcNote(t *testing.T) {
	index, err := inlineIndex()
	if err != nil {
		t.Fatal(err)
	}
	html := string(index)
	for _, id := range []string{"calc-note", "calc-disclaimer"} {
		if strings.Count(html, `id="`+id+`"`) != 1 {
			t.Fatalf("homepage must contain exactly one paragraph with id %q", id)
		}
	}

	mapStart := strings.Index(html, `<section class="view on" id="v-map">`)
	fitStart := strings.Index(html, `<section class="view" id="v-fit">`)
	if mapStart < 0 || fitStart <= mapStart {
		t.Fatal("homepage map section boundaries are missing or out of order")
	}
	home := html[mapStart:fitStart]

	note, noteAt, ok := paragraphByID(home, "calc-note")
	if !ok {
		t.Fatal("local computation note must be inside the homepage map section")
	}
	if openTag := note[:strings.Index(note, ">")+1]; !strings.Contains(openTag, `class="calc-note"`) || !strings.Contains(openTag, `role="note"`) {
		t.Fatal("local computation note must be a calc-note read-only paragraph")
	}
	if text, ok := paragraphText(note); !ok || text != "结果由本地计算得出，不上传输入。" {
		t.Fatalf("local computation note text = %q", text)
	}

	disclaimer, _, ok := paragraphByID(home, "calc-disclaimer")
	if !ok {
		t.Fatal("the earlier read-only disclaimer must stay in the homepage map section")
	}
	if openTag := disclaimer[:strings.Index(disclaimer, ">")+1]; !strings.Contains(openTag, `class="calc-note"`) || !strings.Contains(openTag, `role="note"`) {
		t.Fatal("the earlier disclaimer must keep its read-only paragraph styling")
	}
	if text, ok := paragraphText(disclaimer); !ok || !strings.HasPrefix(text, "本页为只读提示：") {
		t.Fatalf("the earlier disclaimer lost its text: %q", text)
	}

	// A result card opens with this marker. Check the closing tag of every
	// occurrence so the note cannot be moved into or above any of them.
	const cardOpen = `<div class="chart mapcard"`
	cards := 0
	for search := 0; ; {
		rel := strings.Index(home[search:], cardOpen)
		if rel < 0 {
			break
		}
		cardAt := search + rel
		cardEnd := closingDivEnd(home, cardAt)
		if cardEnd < 0 {
			t.Fatalf("result card at offset %d has no closing div", cardAt)
		}
		if noteAt < cardEnd {
			t.Fatalf("local computation note must follow every result card (card at %d ends at %d)", cardAt, cardEnd)
		}
		cards++
		search = cardEnd
	}
	if cards == 0 {
		t.Fatal("homepage map section has no result cards")
	}
	for _, cardID := range []string{"mapScatterCard", "mapHeatCard"} {
		if strings.Count(home, `id="`+cardID+`"`) != 1 {
			t.Fatalf("homepage result card %q must have a unique id", cardID)
		}
	}

	// Actual visibility and horizontal overflow at 390px/1280px are browser
	// checks; assert the CSS hooks that let the note wrap at narrow widths.
	for _, style := range []string{".calc-note {", "display: block; max-width: 100%;", "overflow-wrap: anywhere;", "white-space: normal;"} {
		if !strings.Contains(html, style) {
			t.Fatalf("calc-note CSS is missing %q", style)
		}
	}
}

// paragraphByID returns the paragraph containing the unique id and its offset.
func paragraphByID(html, id string) (string, int, bool) {
	marker := `id="` + id + `"`
	if strings.Count(html, marker) != 1 {
		return "", 0, false
	}
	at := strings.Index(html, marker)
	open := strings.LastIndex(html[:at], "<p")
	close := strings.Index(html[at:], "</p>")
	if open < 0 || close < 0 {
		return "", 0, false
	}
	close += at + len("</p>")
	return html[open:close], open, true
}

func paragraphText(element string) (string, bool) {
	open := strings.Index(element, ">")
	if open < 0 || !strings.HasSuffix(element, "</p>") {
		return "", false
	}
	return element[open+1 : len(element)-len("</p>")], true
}

// closingDivEnd returns the offset just after the </div> matching the <div> at start.
func closingDivEnd(html string, start int) int {
	depth := 0
	for i := start; i < len(html); {
		open := strings.Index(html[i:], "<div")
		close := strings.Index(html[i:], "</div>")
		if close < 0 {
			return -1
		}
		if open >= 0 && open < close {
			j := i + open
			if j+len("<div") < len(html) {
				switch html[j+len("<div")] {
				case ' ', '\t', '\n', '\r', '>':
					depth++
					i = j + len("<div")
					continue
				}
			}
			i = j + len("<div")
			continue
		}
		depth--
		i += close + len("</div>")
		if depth == 0 {
			return i
		}
	}
	return -1
}

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

func TestFullCatalogCalculationContracts(t *testing.T) {
	mustLoad("")
	checked, valid, limited, metadataValid := 0, 0, 0, 0
	check := func(h calc.HW, m calc.Model, q calc.Quant) {
		t.Helper()
		p := calc.ThroughputWorkload(h, m, q, []calc.WorkloadBucket{{Context: 512, Output: 128, Share: 1}}, 1, 1, calc.Opts{})
		checked++
		if _, err := json.Marshal(p); err != nil {
			t.Fatalf("non-finite result %s/%s/%s: %v", h.ID, m.ID, q.ID, err)
		}
		if p.EstimateValid {
			valid++
			if p.SingleTPS <= 0 || p.Support == "unknown" || p.Support == "unsupported" {
				t.Fatalf("invalid performance contract: %s/%s/%s", h.ID, m.ID, q.ID)
			}
		} else {
			limited++
			if p.SingleTPS != 0 || p.AggTPS != 0 || p.TPMMixed != 0 || p.SupportReason == "" {
				t.Fatalf("invalid estimate leaked speed: %s/%s/%s", h.ID, m.ID, q.ID)
			}
		}
		if p.Deployable && (!p.Fit || !p.EstimateValid || p.Support != "supported") {
			t.Fatal("deployability contract violated")
		}
	}
	for _, m := range models {
		if _, _, ok := calc.ModelSupport(m, calc.Opts{}); ok {
			metadataValid++
		}
		for _, id := range []string{"fp16", "fp8", "int8", "int4", "fp4", "q4km"} {
			check(*findHW("h200"), m, calc.QuantByID(id))
		}
	}
	for _, h := range hws {
		for _, id := range []string{"llama-3.1-8b", "llama-3.1-70b", "qwen--qwen3.8-27b", "deepseek-r1", "qwen3-next-80b-a3b", "gpt-oss-120b"} {
			m := findModel(id)
			if m == nil {
				t.Fatalf("missing audit model: %s", id)
			}
			for _, q := range calc.Quants {
				check(h, *m, q)
			}
		}
	}
	t.Logf("catalog: %d models, %d hardware; %d configurations checked, %d valid estimates, %d limited", len(models), len(hws), checked, valid, limited)
	t.Logf("model metadata: %d usable, %d limited", metadataValid, len(models)-metadataValid)
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
	mustLoad("")
	m := findModel("qwen--qwen3.8-27b")
	if m == nil || m.Heads != 24 || m.ParamSource != "safetensors" || m.Revision == "" {
		t.Fatalf("Qwen catalog metadata is incomplete: %+v", m)
	}
	if fp8 := findModel("qwen--qwen3.8-27b-fp8"); fp8 == nil || fp8.FixedQuantID() != "fp8" {
		t.Fatalf("serialized Qwen FP8 checkpoint must lock its actual weight format: %+v", fp8)
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
	mapRec := calc.Recommend(hws, models, *m, w, calc.RecommendOpts{
		TargetTPM: 6000, Conc: 16, Objectives: "cost,tos", Limit: 50,
	}, calc.Opts{})
	if len(mapRec.Picks) == 0 || len(mapRec.Pareto) == 0 {
		t.Fatal("Qwen map must have cost/speed candidates")
	}
	paired := calc.Recommend(hws, models, *findModel("llama-3.1-8b"), w,
		calc.RecommendOpts{TargetTPM: 6000, Conc: 4, Objectives: "cost,avail", Limit: 12}, calc.Opts{})
	redundant := false
	for _, item := range paired.Pareto {
		redundant = redundant || item.Plan.Replicas > 1
	}
	if !redundant {
		t.Fatal("display limit must preserve the availability endpoint instead of filling every slot with equal-cost variants")
	}
	for _, id := range []string{"h200", "b200"} {
		cell := calc.FitMatrix(*findHW(id), []calc.Model{*m}, 1, w, 16, calc.Opts{})[0].Cells[0]
		if cell.Fit == 0 || cell.Support != "conditional" || cell.TPS <= 0 {
			t.Fatalf("Qwen FP16 map cell on %s must retain capacity and conditional support: %+v", id, cell)
		}
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
