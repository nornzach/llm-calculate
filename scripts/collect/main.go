// 从 HuggingFace 采集热门 text-generation 模型，生成 data/models_hf.json。
// 用法：go run ./scripts/collect [-limit 150] [-out data/models_hf.json]
//
// 策略：按下载量与机构取样 → 过滤量化/衍生仓库 → 拉 config.json 解析结构 →
// 以 HF safetensors.total 读取与存储精度无关的实际张量参数量。
// 产物 conf=fetched、src=hf；服务器加载时人工收录（models.json）优先。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type hfEntry struct {
	ID           string   `json:"id"`
	Downloads    int64    `json:"downloads"`
	LastModified string   `json:"lastModified"`
	CreatedAt    string   `json:"createdAt"`
	Tags         []string `json:"tags"`
	Safetensors  *struct {
		Total      int64            `json:"total"`
		Parameters map[string]int64 `json:"parameters"`
	} `json:"safetensors"`
	Siblings []struct {
		RFilename string `json:"rfilename"`
		Size      int64  `json:"size"`
	} `json:"siblings"`
}

type hfConfig struct {
	ModelType            string    `json:"model_type"`
	Layers               int       `json:"num_hidden_layers"`
	Hidden               float64   `json:"hidden_size"`
	Intermediate         float64   `json:"intermediate_size"`
	MoeIntermediate      float64   `json:"moe_intermediate_size"`
	Heads                int       `json:"num_attention_heads"`
	KVHeads              int       `json:"num_key_value_heads"`
	HeadDim              int       `json:"head_dim"`
	MaxPos               int       `json:"max_position_embeddings"`
	KvLoraRank           float64   `json:"kv_lora_rank"`
	QkRopeDim            float64   `json:"qk_rope_head_dim"`
	NRouted              int       `json:"n_routed_experts"`
	NumRouted            int       `json:"num_routed_experts"`
	NumExperts           int       `json:"num_experts"`
	NumLocal             int       `json:"num_local_experts"`
	ExpertsPerToken      int       `json:"experts_per_token"`
	NumSelectedExperts   int       `json:"num_selected_experts"`
	NShared              int       `json:"n_shared_experts"`
	NumShared            int       `json:"num_shared_experts"`
	FirstKDense          int       `json:"first_k_dense_replace"`
	MoeLayerFreq         int       `json:"moe_layer_freq"`
	MoeLayerFrequency    int       `json:"moe_layer_frequency"`
	ExpertLayerPeriod    int       `json:"expert_layer_period"`
	ExpertLayerOffset    int       `json:"expert_layer_offset"`
	Topk                 int       `json:"num_experts_per_tok"`
	TopkAlt              int       `json:"num_experts_per_token"`
	MoeTopk              int       `json:"moe_topk"`
	Vocab                float64   `json:"vocab_size"`
	NextN                int       `json:"num_nextn_predict_layers"`
	MTP                  int       `json:"mtp_num_hidden_layers"`
	LinearVHeads         int       `json:"linear_num_value_heads"`
	LinearKDim           int       `json:"linear_key_head_dim"`
	LinearVDim           int       `json:"linear_value_head_dim"`
	TextConfig           *hfConfig `json:"text_config"`
	LayerTypes           []string  `json:"layer_types"`
	FullAttnEvery        int       `json:"full_attention_interval"`
	SlidingWindow        int       `json:"sliding_window"`
	SlidingWindowPattern int       `json:"sliding_window_pattern"`
	LinearAttn           *struct {
		FullAttnLayers []int `json:"full_attn_layers"`
		NumHeads       int   `json:"num_heads"`
		HeadDim        int   `json:"head_dim"`
	} `json:"linear_attn_config"`
	QuantConfig *struct {
		Method string `json:"quant_method"`
	} `json:"quantization_config"`
}

type outModel struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Org             string  `json:"org"`
	Year            int     `json:"year"`
	Params          float64 `json:"params"`
	Active          float64 `json:"active"`
	Layers          int     `json:"layers"`
	Hidden          float64 `json:"hidden"`
	ModelType       string  `json:"model_type,omitempty"`
	Intermediate    float64 `json:"intermediate,omitempty"`
	MoEIntermediate float64 `json:"moe_intermediate,omitempty"`
	KVT             string  `json:"kvt"`
	KVH             int     `json:"kvh"`
	Dim             int     `json:"dim"`
	MLA             float64 `json:"mla,omitempty"`
	KVLayers        int     `json:"kvlayers,omitempty"`
	LocalLayers     int     `json:"local_layers,omitempty"`
	Window          int     `json:"window,omitempty"`
	StateMB         float64 `json:"state_mb,omitempty"`
	Experts         int     `json:"experts,omitempty"`
	TopK            int     `json:"topk,omitempty"`
	SharedExperts   int     `json:"shared_experts,omitempty"`
	MoELayers       int     `json:"moe_layers,omitempty"`
	MTP             bool    `json:"mtp,omitempty"`
	MTPHeads        int     `json:"mtp_heads,omitempty"`
	Sparse          float64 `json:"sparse,omitempty"`
	Ctx             int     `json:"ctx"`
	MoE             bool    `json:"moe"`
	Multimodal      bool    `json:"multimodal,omitempty"`
	Conf            string  `json:"conf"`
	Src             string  `json:"src"`
	CheckpointGB    float64 `json:"checkpoint_gb,omitempty"`
	NativeQuant     string  `json:"native_quant,omitempty"`
	SourceURL       string  `json:"source_url,omitempty"`
	Downloads       int64   `json:"downloads"`
	Notes           string  `json:"notes,omitempty"`
}

var skipRe = regexp.MustCompile(`(?i)(gguf|awq|gptq|exl2|exl3|bnb|int4|int8|fp8|fp4|nvfp4|w4a16|w8a8|mlx|onnx|ggml|lora|-ft\b|finetune|quant|distill|checkpoint|merge|obliterat|abliterat|uncensored|heretic|derestricted|dspark|speculat|medusa|eagle)`)

// 搬运/微调/草稿模型组织：全量采集时跳过（-only 点名不受限）
var skipOrg = map[string]bool{
	"unsloth": true, "mlx-community": true, "nousresearch": true, "z-lab": true,
	"incoai": true, "radixark": true, "lsx-uniwue": true, "hmellor": true,
	"huggyllama": true, "codellama": true, "tinyllama": true, "eleutherai": true,
	"ilyagusev": true, "yentinglin": true, "typhoon-ai": true, "dphn": true,
	"tiger-lab": true, "davidau": true, "hauhaucs": true, "huihui-ai": true,
	"jackrong": true, "prithivmlmods": true, "quanttrio": true, "jica98": true,
	"trl-internal-testing": true,
	"0xsero":               true, "obliteratus": true,
}
var sizeRe = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*[bB](?:[_-]|$|[aA]\d)`)
var sizeRe2 = regexp.MustCompile(`(?i)[_-](\d+(?:\.\d+)?)[bB](?:[_-]|$)`)
var activeRe = regexp.MustCompile(`(?i)(?:^|[-_])[ae](\d+(?:\.\d+)?)b(?:[-_]|$)`)
var officialActive = map[string]float64{
	"moonshotai/kimi-k3":                  104,
	"moonshotai/kimi-k2-instruct":         32,
	"moonshotai/kimi-k2-instruct-0905":    32,
	"moonshotai/kimi-k2-thinking":         32,
	"moonshotai/kimi-k2-base":             32,
	"openai/gpt-oss-120b":                 5.1,
	"openai/gpt-oss-safeguard-120b":       5.1,
	"openai/gpt-oss-20b":                  3.6,
	"openai/gpt-oss-safeguard-20b":        3.6,
	"deepseek-ai/deepseek-v3":             37,
	"deepseek-ai/deepseek-v3-0324":        37,
	"deepseek-ai/deepseek-v3.1":           37,
	"deepseek-ai/deepseek-v3.1-base":      37,
	"deepseek-ai/deepseek-v3.1-terminus":  37,
	"deepseek-ai/deepseek-v3.2":           37,
	"deepseek-ai/deepseek-v3.2-exp":       37,
	"deepseek-ai/deepseek-v3.2-exp-base":  37,
	"deepseek-ai/deepseek-v3.2-speciale":  37,
	"deepseek-ai/deepseek-r1":             37,
	"deepseek-ai/deepseek-r1-0528":        37,
	"deepseek-ai/deepseek-r1-zero":        37,
	"qwen/qwen3.8-flash-next":             6,
	"zai-org/glm-5.3":                     40,
	"zai-org/glm-5.3-flash":               18,
	"mistralai/mistral-small-4-119b-2603": 6.5,
}
var officialCtx = map[string]int{
	"mistralai/mistral-small-4-119b-2603": 262144,
}
var officialSparse = map[string]float64{
	"deepseek-ai/deepseek-v3.2-exp":      2048,
	"deepseek-ai/deepseek-v3.2-speciale": 2048,
	"qwen/qwen3.8-flash-next":            2048,
	"zai-org/glm-5.3":                    2048,
}

func getJSON(c *http.Client, url string, v any) error {
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * 700 * time.Millisecond)
		}
		resp, err := c.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 200 {
			err = json.NewDecoder(resp.Body).Decode(v)
			resp.Body.Close()
			return err
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		if resp.StatusCode != 429 && resp.StatusCode < 500 {
			return lastErr // 4xx（非限流）直接放弃
		}
	}
	return lastErr
}

func main() {
	base := flag.String("base", "", "HF 站点（默认 https://huggingface.co，可用 HF_ENDPOINT 环境变量覆盖）")
	limit := flag.Int("limit", 150, "拉取的热门模型数量")
	minParams := flag.Float64("min-params", 1.0, "最小参数量（B）")
	orgs := flag.String("orgs", "", "重点机构（逗号分隔）：额外按 author 拉最新+最热，如 Qwen,deepseek-ai")
	only := flag.String("only", "", "只采集指定仓库（逗号分隔完整 id），结果合并进 out 而非覆盖")
	minYear := flag.Int("min-year", 0, "只保留该年份之后创建的模型（如 2024）")
	out := flag.String("out", "data/models_hf.json", "输出文件")
	flag.Parse()

	host := *base
	if host == "" {
		host = os.Getenv("HF_ENDPOINT")
	}
	if host == "" {
		host = "https://huggingface.co"
	}
	c := &http.Client{Timeout: 20 * time.Second}

	// 两个列表：全站热门（downloads）+ 最新发布（createdAt），合并去重。
	// 新模型下载量低，单靠热门榜永远收不进来。
	var entries []hfEntry
	seenID := map[string]bool{}
	fetchList := func(sort string, n int, minDl int64) {
		var list []hfEntry
		url := fmt.Sprintf("%s/api/models?pipeline_tag=text-generation&sort=%s&direction=-1&limit=%d", host, sort, n)
		if err := getJSON(c, url, &list); err != nil {
			fmt.Fprintln(os.Stderr, "拉取列表失败:", err)
			return
		}
		added := 0
		for _, e := range list {
			if seenID[e.ID] || e.Downloads < minDl {
				continue
			}
			seenID[e.ID] = true
			entries = append(entries, e)
			added++
		}
		fmt.Printf("sort=%s 纳入 %d 条\n", sort, added)
	}
	// 机构定向：author 维度拉最新发布 + 最热，保证新模型（如下载量还低的
	// 新一代旗舰）不被全站榜漏掉。
	fetchAuthor := func(author, sort string, n int) {
		var list []hfEntry
		url := fmt.Sprintf("%s/api/models?author=%s&pipeline_tag=text-generation&sort=%s&direction=-1&limit=%d", host, author, sort, n)
		if err := getJSON(c, url, &list); err != nil {
			fmt.Fprintf(os.Stderr, "author=%s sort=%s 拉取失败: %v\n", author, sort, err)
			return
		}
		added := 0
		for _, e := range list {
			if seenID[e.ID] {
				continue
			}
			seenID[e.ID] = true
			entries = append(entries, e)
			added++
		}
		fmt.Printf("author=%s sort=%s 纳入 %d 条\n", author, sort, added)
	}
	// -only 模式：不拉榜单，直接按完整 id 逐个点名采集。
	if *only != "" {
		entries = entries[:0]
		for _, id := range strings.Split(*only, ",") {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			var e hfEntry
			if err := getJSON(c, host+"/api/models/"+id, &e); err != nil {
				fmt.Fprintf(os.Stderr, "%s 信息拉取失败: %v\n", id, err)
				continue
			}
			e.ID = id
			entries = append(entries, e)
		}
		fmt.Printf("only 模式：%d 个指定仓库\n", len(entries))
	} else {
		if *orgs != "" {
			for _, o := range strings.Split(*orgs, ",") {
				o = strings.TrimSpace(o)
				if o == "" {
					continue
				}
				fetchAuthor(o, "createdAt", 40)
				fetchAuthor(o, "downloads", 40)
			}
		}
		fetchList("downloads", *limit*3, 0)
		fetchList("createdAt", *limit*4, 2000)
	}
	if len(entries) < 20 && *only == "" {
		fmt.Fprintln(os.Stderr, "列表条目过少（疑似限流），保留旧数据退出")
		os.Exit(2)
	}
	fmt.Printf("合计 %d 条，开始过滤与解析…\n", len(entries))

	type result struct {
		m   outModel
		ok  bool
		err string
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	resCh := make(chan result, len(entries))
	seen := 0

	for _, e := range entries {
		if seen >= *limit && *only == "" {
			break
		}
		name := e.ID[strings.IndexByte(e.ID, '/')+1:]
		if *only == "" && (skipRe.MatchString(name) || skipOrg[strings.ToLower(e.ID[:strings.IndexByte(e.ID, '/')])]) {
			continue
		}
		if *minYear > 0 && len(e.CreatedAt) >= 4 {
			if y, _ := strconv.Atoi(e.CreatedAt[:4]); y > 0 && y < *minYear {
				continue
			}
		}
		seen++
		wg.Add(1)
		go func(e hfEntry) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			m, ok, why := parseOne(c, host, e, *minParams)
			resCh <- result{m, ok, why}
		}(e)
	}
	go func() { wg.Wait(); close(resCh) }()

	var models []outModel
	fail := map[string]int{}
	for r := range resCh {
		if r.ok {
			models = append(models, r.m)
		} else {
			fail[r.err]++
		}
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Downloads > models[j].Downloads })

	// 点名更新与旧库合并；全量更新直接重建，避免已被过滤的陈旧/衍生仓库永久残留。
	carried := 0
	if *only != "" {
		var old []outModel
		if ob, err := os.ReadFile(*out); err == nil {
			json.Unmarshal(ob, &old)
		}
		pos := map[string]int{}
		for i, m := range models {
			pos[m.ID] = i
		}
		for _, o := range old {
			org := strings.ToLower(o.Org)
			if _, dup := pos[o.ID]; dup || skipRe.MatchString(o.Name) || skipOrg[org] {
				continue
			}
			models = append(models, o)
			carried++
		}
	}
	fmt.Printf("新解析 %d 个，沿用旧数据 %d 个，合计 %d 个\n", len(models)-carried, carried, len(models))

	if len(models) < 10 && *only == "" {
		fmt.Fprintf(os.Stderr, "仅解析出 %d 个（疑似限流），保留旧数据退出\n", len(models))
		os.Exit(2)
	}
	b, _ := json.MarshalIndent(models, "", "  ")
	if err := os.WriteFile(*out, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "写入失败:", err)
		os.Exit(1)
	}
	fmt.Printf("入库 %d 个模型 → %s\n", len(models), *out)
	for why, n := range fail {
		fmt.Printf("  跳过 %d 个：%s\n", n, why)
	}
}

func parseOne(c *http.Client, host string, e hfEntry, minParams float64) (outModel, bool, string) {
	var m outModel
	var notePre string
	// 模型详情接口给出与存储 dtype 无关的 safetensors 精确张量数；列表接口不返回该字段。
	if e.Safetensors == nil || e.Safetensors.Total == 0 {
		var info hfEntry
		if err := getJSON(c, host+"/api/models/"+e.ID, &info); err == nil {
			e = info
		}
	}
	var cfg hfConfig
	if err := getJSON(c, host+"/"+e.ID+"/resolve/main/config.json", &cfg); err != nil {
		return m, false, "无 config.json"
	}
	// 多模态包装配置：下潜到文本塔，同时保留包装层量化信息。
	multimodal := false
	if (cfg.Layers == 0 || cfg.Hidden == 0) && cfg.TextConfig != nil {
		outerQuant := cfg.QuantConfig
		cfg = *cfg.TextConfig
		if cfg.QuantConfig == nil {
			cfg.QuantConfig = outerQuant
		}
		multimodal = true
	}
	if cfg.Layers == 0 || cfg.Hidden == 0 {
		return m, false, "config 缺少层数/hidden"
	}
	// 混合 attention：full/sliding/local 层持有逐 token KV；linear 层只有固定 recurrent state。
	kvLayers, localLayers := 0, 0
	if len(cfg.LayerTypes) > 0 {
		for _, typ := range cfg.LayerTypes {
			t := strings.ToLower(typ)
			if strings.Contains(t, "linear") || !strings.Contains(t, "attention") {
				continue
			}
			kvLayers++
			if strings.Contains(t, "sliding") || strings.Contains(t, "local") {
				localLayers++
			}
		}
	} else if cfg.LinearAttn != nil && len(cfg.LinearAttn.FullAttnLayers) > 0 {
		kvLayers = len(cfg.LinearAttn.FullAttnLayers)
	} else if cfg.FullAttnEvery > 0 {
		kvLayers = cfg.Layers / cfg.FullAttnEvery
	}
	if cfg.SlidingWindow > 0 && localLayers == 0 && (kvLayers == 0 || kvLayers == cfg.Layers) {
		kvLayers = cfg.Layers
		localLayers = cfg.Layers
		if cfg.SlidingWindowPattern > 1 {
			localLayers -= cfg.Layers / cfg.SlidingWindowPattern
		}
	}
	stateMB := 0.0
	linearLayers := cfg.Layers - kvLayers
	if kvLayers > 0 && linearLayers > 0 {
		vHeads, kDim, vDim := cfg.LinearVHeads, cfg.LinearKDim, cfg.LinearVDim
		if cfg.LinearAttn != nil {
			if vHeads == 0 {
				vHeads = cfg.LinearAttn.NumHeads
			}
			if kDim == 0 {
				kDim = cfg.LinearAttn.HeadDim
			}
			if vDim == 0 {
				vDim = cfg.LinearAttn.HeadDim
			}
		}
		if vHeads > 0 && kDim > 0 && vDim > 0 {
			stateMB = float64(linearLayers*vHeads*kDim*vDim*2) / 1e6
		}
	}

	// 参数量优先使用 HF 的 safetensors.total；checkpoint_gb 记录实际
	// safetensors payload，只有选择原生量化格式时才可直接用于显存。
	var idx struct {
		Metadata struct {
			TotalSize float64 `json:"total_size"`
		} `json:"metadata"`
	}
	getJSON(c, host+"/"+e.ID+"/resolve/main/model.safetensors.index.json", &idx)
	checkpointGB := idx.Metadata.TotalSize / 1e9
	if checkpointGB == 0 {
		var info hfEntry
		if err := getJSON(c, host+"/api/models/"+e.ID+"?blobs=true", &info); err == nil {
			var total int64
			for _, f := range info.Siblings {
				if strings.HasSuffix(strings.ToLower(f.RFilename), ".safetensors") {
					total += f.Size
				}
			}
			checkpointGB = float64(total) / 1e9
		}
	}
	nativeQuant, bytesPer := "fp16", 2.0
	if cfg.QuantConfig != nil {
		switch strings.ToLower(cfg.QuantConfig.Method) {
		case "mxfp4":
			nativeQuant, bytesPer = "mxfp4", 0.5
		case "fp4", "nvfp4":
			nativeQuant, bytesPer = "fp4", 0.5
		case "fp8":
			nativeQuant, bytesPer = "fp8", 1
		case "int8", "w8a8", "int8_quanto":
			nativeQuant, bytesPer = "int8", 1
		case "compressed-tensors":
			nativeQuant, bytesPer = "compressed", 1
		}
	}
	var params float64
	if e.Safetensors != nil && e.Safetensors.Total > 0 {
		params = float64(e.Safetensors.Total) / 1e9
	} else if idx.Metadata.TotalSize > 0 {
		params = idx.Metadata.TotalSize / bytesPer / 1e9
	} else {
		name := e.ID[strings.IndexByte(e.ID, '/')+1:]
		if s := sizeRe.FindStringSubmatch(name); len(s) > 1 {
			params, _ = strconv.ParseFloat(s[1], 64)
		} else if s := sizeRe2.FindStringSubmatch(name); len(s) > 1 {
			params, _ = strconv.ParseFloat(s[1], 64)
		}
	}
	if params < minParams || params > 3000 {
		return m, false, "参数量越界或未知"
	}
	params = float64(int(params*10)) / 10
	// 配置里的 quantization_config 可能只描述运行时能力而非仓库存储格式。
	// safetensors payload / 参数量可直接判定实际位宽族，避免把 BF16 权重当 FP8。
	if checkpointGB > 0 {
		bpp := checkpointGB / params
		switch {
		case bpp >= 1.45:
			nativeQuant = "fp16"
		case bpp >= 0.72:
			if nativeQuant != "int8" {
				nativeQuant = "fp8"
			}
		default:
			switch nativeQuant {
			case "mxfp4", "fp4":
			default:
				nativeQuant = "int4"
			}
		}
	}

	// 注意力结构
	kvt, kvh, dim, mla := "mha", cfg.KVHeads, cfg.HeadDim, 0.0
	if cfg.KvLoraRank > 0 {
		kvt = "mla"
		mla = cfg.KvLoraRank + cfg.QkRopeDim
	} else if cfg.KVHeads > 0 && cfg.KVHeads < cfg.Heads {
		kvt = "gqa"
	}
	if kvh == 0 && cfg.Heads > 0 {
		// 有的仓库 config 里 num_key_value_heads 为 null（如 Nemotron 系），
		// 按 GQA-8 兜底估算并标注，比当 MHA 更接近现实。
		kvh = cfg.Heads / 8
		if kvh < 1 {
			kvh = 1
		}
		kvt = "gqa"
		notePre = "KV 头数缺省按 GQA-8 估算；"
	}
	if dim == 0 && cfg.Heads > 0 {
		dim = int(cfg.Hidden) / cfg.Heads
	}
	if kvh == 0 || dim == 0 {
		return m, false, "KV 头/维度未知"
	}

	// MoE
	moe := false
	active := params
	experts, topk := cfg.NRouted, cfg.Topk
	if experts == 0 {
		experts = cfg.NumRouted
	}
	if experts == 0 {
		experts = cfg.NumExperts
	}
	if experts == 0 {
		experts = cfg.NumLocal
	}
	if topk == 0 {
		topk = cfg.TopkAlt
	}
	if topk == 0 {
		topk = cfg.MoeTopk
	}
	if topk == 0 {
		topk = cfg.ExpertsPerToken
	}
	if topk == 0 {
		topk = cfg.NumSelectedExperts
	}
	sharedExperts := cfg.NShared
	if sharedExperts == 0 {
		sharedExperts = cfg.NumShared
	}
	moeLayers := cfg.Layers
	if cfg.FirstKDense > 0 && cfg.FirstKDense < moeLayers {
		moeLayers -= cfg.FirstKDense
	}
	freq := cfg.MoeLayerFreq
	if freq == 0 {
		freq = cfg.MoeLayerFrequency
	}
	if freq > 1 {
		moeLayers = (moeLayers + freq - 1) / freq
	}
	if cfg.ExpertLayerPeriod > 1 {
		moeLayers = 0
		for layer := cfg.ExpertLayerOffset; layer < cfg.Layers; layer += cfg.ExpertLayerPeriod {
			moeLayers++
		}
	}
	activeNote := ""
	if experts > 1 {
		moe = true
		if topk == 0 {
			topk = 8
		}
		name := e.ID[strings.IndexByte(e.ID, '/')+1:]
		if v, ok := officialActive[strings.ToLower(e.ID)]; ok {
			active = v
			activeNote = "激活参数取官方模型卡；"
		} else {
			expertDim := cfg.MoeIntermediate
			if expertDim == 0 {
				expertDim = cfg.Intermediate
			}
			routed := float64(moeLayers*experts) * 3 * cfg.Hidden * expertDim / 1e9
			if routed > 0 && routed < params {
				active = params - routed + routed*float64(topk)/float64(experts)
				activeNote = "激活参数由 MoE 结构推导；"
			} else if s := activeRe.FindStringSubmatch(name); len(s) > 1 {
				active, _ = strconv.ParseFloat(s[1], 64)
				activeNote = "激活参数取模型名；"
			} else {
				active = params * (0.04 + 0.9*float64(topk)/float64(experts))
				activeNote = "激活参数为启发式估算；"
			}
			active = math.Round(active*10) / 10
		}
	}

	ctx := cfg.MaxPos
	if ctx <= 0 {
		ctx = 8192
	}
	if v, ok := officialCtx[strings.ToLower(e.ID)]; ok {
		ctx = v
	}
	year := 2024
	if len(e.CreatedAt) >= 4 {
		year, _ = strconv.Atoi(e.CreatedAt[:4])
	} else if len(e.LastModified) >= 4 {
		year, _ = strconv.Atoi(e.LastModified[:4])
	}

	parts := strings.SplitN(e.ID, "/", 2)
	m = outModel{
		ID:   strings.ToLower(strings.ReplaceAll(e.ID, "/", "--")),
		Name: parts[1], Org: parts[0], Year: year,
		Params: params, Active: active, Layers: cfg.Layers, Hidden: cfg.Hidden,
		ModelType: cfg.ModelType, Intermediate: cfg.Intermediate, MoEIntermediate: cfg.MoeIntermediate,
		KVT: kvt, KVH: kvh, Dim: dim, MLA: mla, KVLayers: kvLayers, LocalLayers: localLayers, Window: cfg.SlidingWindow, StateMB: stateMB,
		Experts: experts, TopK: topk, SharedExperts: sharedExperts, MoELayers: moeLayers,
		Ctx: ctx, MoE: moe, Multimodal: multimodal, Sparse: officialSparse[strings.ToLower(e.ID)], Conf: "fetched", Src: "hf",
		MTP: cfg.NextN > 0 || cfg.MTP > 0, MTPHeads: max(cfg.NextN, cfg.MTP),
		CheckpointGB: checkpointGB, NativeQuant: nativeQuant, SourceURL: host + "/" + e.ID, Downloads: e.Downloads,
		Notes: notePre + activeNote + fmt.Sprintf("HF 采集 · %s · 下载 %.1fM", e.ID, float64(e.Downloads)/1e6),
	}
	return m, true, ""
}
