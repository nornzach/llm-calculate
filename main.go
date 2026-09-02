// LLM 推理计算器：Go 单二进制，内嵌数据与网页，无外部依赖。
package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"

	"llmcalc/calc"
)

//go:embed web data
var embedded embed.FS

var (
	hws    []calc.HW
	models []calc.Model
)

func mustLoad() {
	b, err := embedded.ReadFile("data/hardware.json")
	if err != nil {
		log.Fatal(err)
	}
	hws, err = calc.LoadHW(b)
	if err != nil {
		log.Fatal(err)
	}
	b, err = embedded.ReadFile("data/models.json")
	if err != nil {
		log.Fatal(err)
	}
	models, err = calc.LoadModels(b)
	if err != nil {
		log.Fatal(err)
	}
	for i := range models {
		models[i].Src = "curated"
	}
	// 稀疏注意力补丁：DSA 模型的选中 token 数（HF config 无此字段，采集会丢，兜底）
	sparsePatch := map[string]float64{
		"deepseek-ai--deepseek-v3.2-exp":      2048,
		"deepseek-ai--deepseek-v3.2-speciale": 2048,
		"qwen--qwen3.8-flash-next":            2048,
		"zai-org--glm-5.3":                    2048,
	}
	patchSparse := func(ms []calc.Model) {
		for i := range ms {
			if ms[i].Sparse == 0 {
				if v, ok := sparsePatch[ms[i].ID]; ok {
					ms[i].Sparse = v
				}
			}
		}
	}
	patchSparse(models)
	// 开发时优先读取磁盘，便于刷新模型库；独立二进制回退内嵌数据。
	index := make(map[string]int, len(models))
	for i := range models {
		index[models[i].ID] = i
	}
	for _, path := range []string{"data/models_hf.json", "data/models_modelscope.json"} {
		fb, err := os.ReadFile(path)
		if err != nil {
			fb, err = embedded.ReadFile(path)
		}
		if err != nil {
			continue
		}
		fetched, err := calc.LoadModels(fb)
		if err != nil {
			log.Printf("%s 加载失败: %v", path, err)
			continue
		}
		added := 0
		for _, m := range fetched {
			if i, ok := index[m.ID]; ok {
				enrichModel(&models[i], m)
				continue
			}
			models = append(models, m)
			index[m.ID] = len(models) - 1
			added++
		}
		log.Printf("%s 合并 %d 个模型（总计 %d）", path, added, len(models))
	}
	patchSparse(models)
}

func enrichModel(dst *calc.Model, src calc.Model) {
	if dst.ModelType == "" {
		dst.ModelType = src.ModelType
	}
	if dst.Architecture == "" {
		dst.Architecture = src.Architecture
	}
	if dst.DType == "" {
		dst.DType = src.DType
	}
	if dst.RopeTheta == 0 {
		dst.RopeTheta = src.RopeTheta
	}
	if dst.CheckpointGB == 0 {
		dst.CheckpointGB = src.CheckpointGB
	}
	if dst.NativeQuant == "" {
		dst.NativeQuant = src.NativeQuant
	}
	if dst.SourceURL == "" {
		dst.SourceURL = src.SourceURL
	}
	if dst.Downloads == 0 {
		dst.Downloads = src.Downloads
	}
	if dst.License == "" {
		dst.License = src.License
	}
	if len(dst.Tasks) == 0 {
		dst.Tasks = src.Tasks
	}
	if dst.CreatedAt == "" {
		dst.CreatedAt = src.CreatedAt
	}
	if dst.UpdatedAt == "" {
		dst.UpdatedAt = src.UpdatedAt
	}
	if src.Official {
		dst.Official = true
	}
}

func findHW(id string) *calc.HW {
	for i := range hws {
		if hws[i].ID == id {
			return &hws[i]
		}
	}
	return nil
}

func findModel(id string) *calc.Model {
	for i := range models {
		if models[i].ID == id {
			return &models[i]
		}
	}
	return nil
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"error":%q}`, msg)
}

type fitReq struct {
	HW    string `json:"hw"`
	N     int    `json:"n"`
	Ctx   int    `json:"ctx"`
	Batch int    `json:"batch"`
	Eng   string `json:"eng"`
	Spec  string `json:"spec"`
	KVQ   string `json:"kvq"`
}

type perfReq struct {
	HW       string    `json:"hw"`
	N        int       `json:"n"`
	Model    string    `json:"model"`
	Quant    string    `json:"quant"`
	Ctx      int       `json:"ctx"`
	Batch    int       `json:"batch"`
	Eng      string    `json:"eng"`
	Spec     string    `json:"spec"`
	KVQ      string    `json:"kvq"`
	Hit      float64   `json:"hit"`
	OutLen   int       `json:"outlen"`
	Advanced calc.Opts `json:"advanced"`
}

type planReq struct {
	Model  string      `json:"model"`
	Custom *calc.Model `json:"custom"` // 自定义假想模型（优先于 model id）
	calc.PlanOpts
	Ctx      int       `json:"ctx"`
	Conc     int       `json:"conc"`
	Eng      string    `json:"eng"`
	Spec     string    `json:"spec"`
	KVQ      string    `json:"kvq"`
	Hit      float64   `json:"hit"`
	Advanced calc.Opts `json:"advanced"`
}

// clampF64 限制浮点范围（0 值给默认）。
func clampF64(v, lo, hi, def float64) float64 {
	if v < lo {
		return def
	}
	if v > hi {
		return hi
	}
	return v
}

// sanitizeCustom 校验/兜底用户输入的假想模型参数，避免除零与离谱值。
func sanitizeCustom(m *calc.Model) calc.Model {
	clampF := func(v, lo, hi, def float64) float64 {
		if v <= 0 {
			return def
		}
		if v < lo {
			return lo
		}
		if v > hi {
			return hi
		}
		return v
	}
	m.Params = clampF(m.Params, 0.1, 5000, 70)
	m.EncoderParams = clampF64(m.EncoderParams, 0, m.Params, 0)
	textParams := m.Params - m.EncoderParams
	if textParams < 0.1 {
		textParams = 0.1
	}
	if m.MoE {
		m.Active = clampF(m.Active, 0.05, textParams, textParams*0.1)
	} else {
		m.Active = textParams
	}
	m.Layers = clamp(m.Layers, 1, 256, 80)
	m.Hidden = clampF(m.Hidden, 256, 65536, 8192)
	m.Intermediate = clampF64(m.Intermediate, 0, 262144, 0)
	m.MoEIntermediate = clampF64(m.MoEIntermediate, 0, 262144, 0)
	if m.KVT != "mha" && m.KVT != "mla" {
		m.KVT = "gqa"
	}
	m.KVH = clamp(m.KVH, 1, 256, 8)
	m.Dim = clamp(m.Dim, 16, 1024, 128)
	if m.KVT == "mla" {
		m.MLA = clampF(m.MLA, 64, 4096, 576)
	}
	if m.KVLayers > m.Layers || m.KVLayers < 0 {
		m.KVLayers = 0
	}
	if m.StateMB < 0 {
		m.StateMB = 0
	} else if m.StateMB > 1e6 {
		m.StateMB = 1e6
	}
	if m.Experts < 0 {
		m.Experts = 0
	} else if m.Experts > 4096 {
		m.Experts = 4096
	}
	if m.TopK < 0 || m.TopK > m.Experts {
		m.TopK = 0
	}
	m.SharedExperts = clamp(m.SharedExperts, 0, m.Experts, 0)
	m.MoELayers = clamp(m.MoELayers, 0, m.Layers, 0)
	m.MTPHeads = clamp(m.MTPHeads, 0, 32, 0)
	m.MTP = m.MTP || m.MTPHeads > 0
	m.Multimodal = m.Multimodal || m.EncoderParams > 0
	m.Ctx = clamp(m.Ctx, 512, 1048576, 131072)
	kvLayers := m.KVLayers
	if kvLayers == 0 {
		kvLayers = m.Layers
	}
	if m.LocalLayers < 0 || m.LocalLayers > kvLayers || m.Window <= 0 {
		m.LocalLayers, m.Window = 0, 0
	} else if m.Window > m.Ctx {
		m.Window = m.Ctx
	}
	if m.Sparse < 0 {
		m.Sparse = 0
	} else if m.Sparse > float64(m.Ctx) {
		m.Sparse = float64(m.Ctx)
	}
	if m.Name == "" {
		m.Name = "自定义模型"
	}
	m.ID, m.Org, m.Year, m.Conf, m.Src = "custom", "自定义", 2026, "reported", "custom"
	return *m
}

func clamp(v, lo, hi, def int) int {
	if v <= 0 {
		return def
	}
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func main() {
	addr := flag.String("addr", ":8317", "listen address")
	flag.Parse()
	mustLoad()

	webFS, err := fs.Sub(embedded, "web")
	if err != nil {
		log.Fatal(err)
	}

	mux := http.NewServeMux()
	mux.Handle("/", http.FileServer(http.FS(webFS)))

	mux.HandleFunc("GET /api/hardware", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, hws)
	})
	mux.HandleFunc("GET /api/models", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, models)
	})
	mux.HandleFunc("GET /api/quants", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, calc.Quants)
	})
	mux.HandleFunc("GET /api/engines", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, calc.Engines)
	})
	mux.HandleFunc("GET /api/specs", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, calc.SpecMethods)
	})
	mux.HandleFunc("GET /api/quick", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, calc.QuickTable(hws))
	})

	mux.HandleFunc("POST /api/fit", func(w http.ResponseWriter, r *http.Request) {
		var req fitReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		h := findHW(req.HW)
		if h == nil {
			writeErr(w, 404, "unknown hardware")
			return
		}
		n := clamp(req.N, 1, 8, 1)
		ctx := clamp(req.Ctx, 512, 1048576, 8192)
		batch := clamp(req.Batch, 1, 256, 8)
		o := calc.Opts{Engine: req.Eng, Spec: req.Spec, KVQuant: req.KVQ}
		writeJSON(w, calc.FitMatrix(*h, models, n, ctx, batch, o))
	})

	mux.HandleFunc("POST /api/perf", func(w http.ResponseWriter, r *http.Request) {
		var req perfReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		h := findHW(req.HW)
		m := findModel(req.Model)
		if h == nil || m == nil {
			writeErr(w, 404, "unknown hardware or model")
			return
		}
		n := clamp(req.N, 1, 8, 1)
		ctx := clamp(req.Ctx, 512, 1048576, 8192)
		batch := clamp(req.Batch, 1, 256, 8)
		q := calc.QuantByID(req.Quant)
		o := req.Advanced
		o.Engine, o.Spec, o.KVQuant = req.Eng, req.Spec, req.KVQ
		o.HitRate = clampF64(req.Hit, 0, 0.9, 0)
		o.OutLen = clamp(req.OutLen, 16, 8192, 512)
		p := calc.Throughput(*h, *m, q, ctx, batch, n, o)
		// 并发扫描曲线
		type pt struct {
			B   int     `json:"b"`
			Agg float64 `json:"agg"`
		}
		var curve []pt
		for _, b := range []int{1, 2, 4, 8, 16, 32, 64, 128} {
			pp := calc.Throughput(*h, *m, q, ctx, b, n, o)
			curve = append(curve, pt{B: b, Agg: pp.AggTPS})
		}
		writeJSON(w, map[string]any{"perf": p, "curve": curve})
	})

	mux.HandleFunc("POST /api/plan", func(w http.ResponseWriter, r *http.Request) {
		var req planReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, 400, err.Error())
			return
		}
		var m calc.Model
		if req.Custom != nil {
			m = sanitizeCustom(req.Custom)
		} else {
			found := findModel(req.Model)
			if found == nil {
				writeErr(w, 404, "unknown model")
				return
			}
			m = *found
		}
		ctx := clamp(req.Ctx, 512, 1048576, 8192)
		conc := clamp(req.Conc, 1, 256, 16)
		o := req.Advanced
		o.Engine, o.Spec, o.KVQuant = req.Eng, req.Spec, req.KVQ
		o.HitRate = clampF64(req.Hit, 0, 0.9, 0)
		o.OutLen = clamp(req.OutLen, 16, 8192, 512)
		writeJSON(w, calc.Planner(hws, m, req.PlanOpts, ctx, conc, o))
	})

	// 简单日志
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			log.Printf("%s %s", r.Method, r.URL.Path)
		}
		mux.ServeHTTP(w, r)
	})

	fmt.Printf("LLM 推理计算器 → http://localhost%s  (硬件 %d · 模型 %d)\n", *addr, len(hws), len(models))
	log.Fatal(http.ListenAndServe(*addr, handler))
}
