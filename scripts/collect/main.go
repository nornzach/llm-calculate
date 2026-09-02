// 从 Hugging Face 与 ModelScope 采集可部署的大语言模型。
// 用法：go run ./scripts/collect -source hf|modelscope -all -min-year 2023
//
// -all 只采集已核实发布机构的完整 LLM / 多模态 LLM 仓库，避免社区改名、
// 剪枝和二次量化淹没官方模型。写入前与同范围旧库合并，并原子替换文件。
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type hfEntry struct {
	ID             string   `json:"id"`
	Downloads      int64    `json:"downloads"`
	LastModified   string   `json:"lastModified"`
	CreatedAt      string   `json:"createdAt"`
	Tags           []string `json:"tags"`
	PipelineTag    string   `json:"pipeline_tag"`
	Provider       string   `json:"-"`
	ParameterCount int64    `json:"-"`
	FileSize       int64    `json:"-"`
	License        string   `json:"-"`
	Tasks          []string `json:"-"`
	Official       bool     `json:"-"`
	Config         struct {
		Architectures []string `json:"architectures"`
		ModelType     string   `json:"model_type"`
	} `json:"config"`
	Safetensors *struct {
		Total      int64            `json:"total"`
		Parameters map[string]int64 `json:"parameters"`
	} `json:"safetensors"`
	Siblings []struct {
		RFilename string `json:"rfilename"`
		Size      int64  `json:"size"`
	} `json:"siblings"`
}

type msEntry struct {
	ID           string   `json:"id"`
	Downloads    int64    `json:"downloads"`
	CreatedAt    string   `json:"created_at"`
	LastModified string   `json:"last_modified"`
	FileSize     int64    `json:"file_size"`
	Params       int64    `json:"params"`
	License      string   `json:"license"`
	Tasks        []string `json:"tasks"`
	Tags         []string `json:"tags"`
}

func (e msEntry) catalogEntry() hfEntry {
	return hfEntry{
		ID: e.ID, Downloads: e.Downloads, CreatedAt: e.CreatedAt, LastModified: e.LastModified,
		Tags: e.Tags, Provider: "modelscope", ParameterCount: e.Params, FileSize: e.FileSize,
		License: e.License, Tasks: e.Tasks,
	}
}

type msListResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Models     []msEntry `json:"models"`
		TotalCount int       `json:"total_count"`
	} `json:"data"`
}

type msDetailResponse struct {
	Success bool    `json:"success"`
	Data    msEntry `json:"data"`
}

type msFilesResponse struct {
	Data struct {
		Files []struct {
			Path string `json:"Path"`
			Size int64  `json:"Size"`
		} `json:"Files"`
	} `json:"Data"`
}

type hfQuantConfig struct {
	Method       string                  `json:"quant_method"`
	Format       string                  `json:"format"`
	ConfigGroups map[string]hfQuantGroup `json:"config_groups"`
}

type hfQuantGroup struct {
	Format  string `json:"format"`
	Weights struct {
		NumBits int    `json:"num_bits"`
		Type    string `json:"type"`
	} `json:"weights"`
}

type hfConfig struct {
	ModelType            string    `json:"model_type"`
	Architectures        []string  `json:"architectures"`
	TorchDType           string    `json:"torch_dtype"`
	DType                string    `json:"dtype"`
	RopeTheta            float64   `json:"rope_theta"`
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
	UseSlidingWindow     *bool     `json:"use_sliding_window"`
	SlidingWindowPattern int       `json:"sliding_window_pattern"`
	LinearAttn           *struct {
		FullAttnLayers []int `json:"full_attn_layers"`
		NumHeads       int   `json:"num_heads"`
		HeadDim        int   `json:"head_dim"`
	} `json:"linear_attn_config"`
	QuantConfig *hfQuantConfig `json:"quantization_config"`
}

type outModel struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Org             string   `json:"org"`
	Year            int      `json:"year"`
	Params          float64  `json:"params"`
	Active          float64  `json:"active"`
	Layers          int      `json:"layers"`
	Hidden          float64  `json:"hidden"`
	ModelType       string   `json:"model_type,omitempty"`
	Architecture    string   `json:"architecture,omitempty"`
	DType           string   `json:"dtype,omitempty"`
	RopeTheta       float64  `json:"rope_theta,omitempty"`
	Intermediate    float64  `json:"intermediate,omitempty"`
	MoEIntermediate float64  `json:"moe_intermediate,omitempty"`
	KVT             string   `json:"kvt"`
	KVH             int      `json:"kvh"`
	Dim             int      `json:"dim"`
	MLA             float64  `json:"mla,omitempty"`
	KVLayers        int      `json:"kvlayers,omitempty"`
	LocalLayers     int      `json:"local_layers,omitempty"`
	Window          int      `json:"window,omitempty"`
	StateMB         float64  `json:"state_mb,omitempty"`
	Experts         int      `json:"experts,omitempty"`
	TopK            int      `json:"topk,omitempty"`
	SharedExperts   int      `json:"shared_experts,omitempty"`
	MoELayers       int      `json:"moe_layers,omitempty"`
	MTP             bool     `json:"mtp,omitempty"`
	MTPHeads        int      `json:"mtp_heads,omitempty"`
	Sparse          float64  `json:"sparse,omitempty"`
	Ctx             int      `json:"ctx"`
	MoE             bool     `json:"moe"`
	Multimodal      bool     `json:"multimodal,omitempty"`
	Conf            string   `json:"conf"`
	Official        bool     `json:"official,omitempty"`
	Src             string   `json:"src"`
	CheckpointGB    float64  `json:"checkpoint_gb,omitempty"`
	NativeQuant     string   `json:"native_quant,omitempty"`
	SourceURL       string   `json:"source_url,omitempty"`
	Downloads       int64    `json:"downloads"`
	License         string   `json:"license,omitempty"`
	Tasks           []string `json:"tasks,omitempty"`
	CreatedAt       string   `json:"created_at,omitempty"`
	UpdatedAt       string   `json:"updated_at,omitempty"`
	Notes           string   `json:"notes,omitempty"`
}

var skipRe = regexp.MustCompile(`(?i)(lora|-ft\b|finetune|checkpoint|merge|obliterat|abliterat|uncensored|heretic|derestricted|dspark|speculat|medusa|eagle|-mtpv?\d*\b)`)
var packagedRe = regexp.MustCompile(`(?i)(?:^|[-_.])(awq|gptq|gguf|fp8|fp4|nvfp4|int4|int8|bnb|quant(?:ized)?|compressed|pruned?)(?:[-_.]|$)`)

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
	"qwen/qwen3.5-35b-a3b":                3,
	"stepfun-ai/step-3.5-flash":           11,
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
	_, err := getJSONPage(c, url, v)
	return err
}

var nextLinkRe = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func getJSONPage(c *http.Client, url string, v any) (string, error) {
	var lastErr error
	for attempt := range 4 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt*attempt) * 700 * time.Millisecond)
		}
		resp, err := c.Get(url)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == http.StatusOK {
			err = json.NewDecoder(resp.Body).Decode(v)
			resp.Body.Close()
			if err != nil {
				return "", err
			}
			if match := nextLinkRe.FindStringSubmatch(resp.Header.Get("Link")); len(match) == 2 {
				return match[1], nil
			}
			return "", nil
		}
		resp.Body.Close()
		lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
		if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
			return "", lastErr
		}
	}
	return "", lastErr
}

const defaultPublishers = "Qwen,deepseek-ai,moonshotai,zai-org,ZhipuAI,mistralai,MiniMaxAI,MiniMax,nvidia,microsoft,google,openai,stepfun-ai,inclusionAI,internlm,Shanghai_AI_Laboratory,ByteDance-Seed,OpenGVLab,baidu,Tencent-Hunyuan,XiaomiMiMo,CohereLabs,ai21labs,openbmb,OpenBMB,THUDM,01-ai,TeleAI,Tele-AI,arcee-ai,swiss-ai,LiquidAI,ibm-granite,allenai,HuggingFaceTB,tiiuae,ornith-ai,meta-llama,stabilityai,Salesforce,databricks,BAAI,ModelBest"

var officialPipelines = []string{
	"text-generation",
	"image-text-to-text",
	"text2text-generation",
	"visual-question-answering",
	"image-to-text",
	"any-to-any",
	"audio-text-to-text",
}

func publisherSet(csv string) map[string]bool {
	out := map[string]bool{}
	for _, publisher := range strings.Split(csv, ",") {
		if publisher = strings.TrimSpace(publisher); publisher != "" {
			out[strings.ToLower(publisher)] = true
		}
	}
	return out
}

func fetchModelScopeList(c *http.Client, host, owner, sortBy string, maxItems int) ([]hfEntry, error) {
	if maxItems > 3000 {
		maxItems = 3000
	}
	pageSize := min(50, maxItems)
	pages := (maxItems + pageSize - 1) / pageSize
	out := make([]hfEntry, 0, maxItems)
	for page := range pages {
		q := url.Values{
			"page_number": {"1"},
			"page_size":   {strconv.Itoa(pageSize)},
			"filter.task": {"text-generation"},
		}
		q.Set("page_number", strconv.Itoa(page+1))
		if owner != "" {
			q.Set("owner", owner)
		}
		if sortBy == "downloads" {
			q.Set("sort", "downloads")
		}
		var response msListResponse
		if err := getJSON(c, host+"/openapi/v1/models?"+q.Encode(), &response); err != nil {
			return out, err
		}
		if !response.Success {
			return out, fmt.Errorf("ModelScope 返回失败")
		}
		for _, e := range response.Data.Models {
			out = append(out, e.catalogEntry())
		}
		if len(response.Data.Models) < pageSize || len(out) >= response.Data.TotalCount {
			break
		}
	}
	return out, nil
}

func main() {
	source := flag.String("source", "hf", "数据源：hf 或 modelscope")
	base := flag.String("base", "", "数据源站点；默认使用官方 Hugging Face 或 ModelScope")
	limit := flag.Int("limit", 150, "非全量模式最多解析的模型数量")
	minParams := flag.Float64("min-params", 0.1, "最小参数量（B）")
	orgs := flag.String("orgs", "", "已核实发布机构（逗号分隔）；-all 未指定时使用内置列表")
	only := flag.String("only", "", "只采集指定仓库（逗号分隔完整 id），结果合并进 out 而非覆盖")
	minYear := flag.Int("min-year", 2023, "只采集该年份及之后创建的模型")
	all := flag.Bool("all", false, "分页采集已核实机构的全部 LLM / 多模态 LLM 仓库")
	refresh := flag.Bool("refresh", false, "重新解析已入库模型；默认只解析新增仓库")
	out := flag.String("out", "", "输出文件；默认按数据源写入 data/models_hf.json 或 data/models_modelscope.json")
	flag.Parse()

	provider := strings.ToLower(*source)
	host := strings.TrimRight(*base, "/")
	switch provider {
	case "hf":
		if host == "" {
			host = os.Getenv("HF_ENDPOINT")
		}
		if host == "" {
			host = "https://huggingface.co"
		}
		if *out == "" {
			*out = "data/models_hf.json"
		}
	case "modelscope":
		if host == "" {
			host = os.Getenv("MODELSCOPE_ENDPOINT")
		}
		if host == "" {
			host = "https://modelscope.cn"
		}
		if *out == "" {
			*out = "data/models_modelscope.json"
		}
	default:
		fmt.Fprintln(os.Stderr, "-source 只支持 hf 或 modelscope")
		os.Exit(2)
	}
	authors := strings.TrimSpace(*orgs)
	if *all && authors == "" {
		authors = defaultPublishers
	}
	officialPublishers := publisherSet(defaultPublishers)
	publishers := publisherSet(authors)
	c := &http.Client{Timeout: 30 * time.Second}

	// 两个列表：全站热门（downloads）+ 最新发布（createdAt），合并去重。
	// 新模型下载量低，单靠热门榜永远收不进来。
	var entries []hfEntry
	seenID := map[string]bool{}
	fetchList := func(sortBy string, n int, minDl int64) {
		var list []hfEntry
		var err error
		if provider == "modelscope" {
			list, err = fetchModelScopeList(c, host, "", sortBy, n)
		} else {
			listURL := fmt.Sprintf("%s/api/models?pipeline_tag=text-generation&sort=%s&direction=-1&limit=%d", host, sortBy, n)
			err = getJSON(c, listURL, &list)
		}
		if err != nil {
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
		fmt.Printf("sort=%s 纳入 %d 条\n", sortBy, added)
	}
	// 机构定向：按任务分页拉取，避免全站社区衍生仓库挤掉官方发布。
	fetchAuthor := func(author, pipeline, sortBy string, n int) {
		added := 0
		if provider == "modelscope" {
			list, err := fetchModelScopeList(c, host, author, sortBy, n)
			if err != nil {
				fmt.Fprintf(os.Stderr, "owner=%s sort=%s 拉取失败: %v\n", author, sortBy, err)
				return
			}
			for _, e := range list {
				if seenID[e.ID] {
					continue
				}
				seenID[e.ID] = true
				entries = append(entries, e)
				added++
			}
		} else {
			next := fmt.Sprintf("%s/api/models?author=%s&pipeline_tag=%s&sort=%s&direction=-1&limit=%d", host, url.QueryEscape(author), url.QueryEscape(pipeline), sortBy, n)
			for next != "" {
				var list []hfEntry
				var err error
				next, err = getJSONPage(c, next, &list)
				if err != nil {
					fmt.Fprintf(os.Stderr, "author=%s sort=%s 拉取失败: %v\n", author, sortBy, err)
					break
				}
				for _, e := range list {
					if seenID[e.ID] {
						continue
					}
					seenID[e.ID] = true
					entries = append(entries, e)
					added++
				}
				if !*all {
					break
				}
			}
		}
		fmt.Printf("author=%s task=%s sort=%s 纳入 %d 条\n", author, pipeline, sortBy, added)
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
			if provider == "modelscope" {
				var response msDetailResponse
				if err := getJSON(c, host+"/openapi/v1/models/"+id, &response); err != nil {
					fmt.Fprintf(os.Stderr, "%s 信息拉取失败: %v\n", id, err)
					continue
				}
				e = response.Data.catalogEntry()
			} else {
				if err := getJSON(c, host+"/api/models/"+id, &e); err != nil {
					fmt.Fprintf(os.Stderr, "%s 信息拉取失败: %v\n", id, err)
					continue
				}
				e.ID = id
			}
			e.Official = officialPublishers[strings.ToLower(strings.SplitN(id, "/", 2)[0])]
			entries = append(entries, e)
		}
		fmt.Printf("only 模式：%d 个指定仓库\n", len(entries))
	} else {
		if authors != "" {
			for _, o := range strings.Split(authors, ",") {
				o = strings.TrimSpace(o)
				if o == "" {
					continue
				}
				if *all && provider == "hf" {
					for _, pipeline := range officialPipelines {
						fetchAuthor(o, pipeline, "createdAt", 1000)
					}
				} else if *all {
					fetchAuthor(o, "text-generation", "createdAt", 3000)
				} else {
					fetchAuthor(o, "text-generation", "createdAt", 40)
					fetchAuthor(o, "text-generation", "downloads", 40)
				}
			}
		}
		if !*all {
			fetchList("downloads", *limit*3, 0)
			fetchList("createdAt", *limit*3, 0)
		}
	}
	if len(entries) < 20 && *only == "" {
		fmt.Fprintln(os.Stderr, "列表条目过少（疑似限流），保留旧数据退出")
		os.Exit(2)
	}
	fmt.Printf("合计 %d 条，开始过滤与解析…\n", len(entries))

	var old []outModel
	if ob, err := os.ReadFile(*out); err == nil {
		json.Unmarshal(ob, &old)
	}
	if *all && len(publishers) > 0 {
		old = keepPublishers(old, publishers)
	}
	oldIDs := make(map[string]bool, len(old))
	for _, m := range old {
		oldIDs[m.ID] = true
	}

	type result struct {
		m   outModel
		ok  bool
		err string
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	resCh := make(chan result, len(entries))
	seen := 0

	for _, e := range entries {
		slash := strings.IndexByte(e.ID, '/')
		if slash < 1 {
			continue
		}
		e.Official = officialPublishers[strings.ToLower(e.ID[:slash])]
		if !e.Official {
			continue
		}
		if !*all && seen >= *limit && *only == "" {
			break
		}
		name := e.ID[slash+1:]
		if *only == "" && (skipRe.MatchString(name) || skipOrg[strings.ToLower(e.ID[:slash])]) {
			continue
		}
		if *minYear > 0 && len(e.CreatedAt) >= 4 {
			if y, _ := strconv.Atoi(e.CreatedAt[:4]); y > 0 && y < *minYear {
				continue
			}
		}
		id := strings.ToLower(strings.ReplaceAll(e.ID, "/", "--"))
		if *all && !*refresh && oldIDs[id] {
			continue
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
	fresh := len(models)
	models, carried := mergeModels(models, old)
	sort.Slice(models, func(i, j int) bool {
		pi := models[i].NativeQuant != "" && models[i].NativeQuant != "fp16" || packagedRe.MatchString(models[i].Name)
		pj := models[j].NativeQuant != "" && models[j].NativeQuant != "fp16" || packagedRe.MatchString(models[j].Name)
		if pi != pj {
			return !pi
		}
		return models[i].Downloads > models[j].Downloads
	})
	fmt.Printf("新解析 %d 个，沿用旧数据 %d 个，合计 %d 个\n", fresh, carried, len(models))

	if len(models) < 10 && *only == "" {
		fmt.Fprintf(os.Stderr, "仅解析出 %d 个（疑似限流），保留旧数据退出\n", len(models))
		os.Exit(2)
	}
	b, _ := json.MarshalIndent(models, "", "  ")
	b = append(b, '\n')
	tmp := *out + ".tmp"
	if err := os.WriteFile(tmp, b, 0644); err != nil {
		fmt.Fprintln(os.Stderr, "写入临时文件失败:", err)
		os.Exit(1)
	}
	if err := os.Rename(tmp, *out); err != nil {
		os.Remove(tmp)
		fmt.Fprintln(os.Stderr, "原子替换失败:", err)
		os.Exit(1)
	}
	fmt.Printf("入库 %d 个模型 → %s\n", len(models), *out)
	for why, n := range fail {
		fmt.Printf("  跳过 %d 个：%s\n", n, why)
	}
}

// mergeModels 只用新数据更新同 ID 条目，永远不因本次限流、筛选或解析失败删除旧数据。
func mergeModels(fresh, old []outModel) ([]outModel, int) {
	seen := make(map[string]bool, len(fresh)+len(old))
	for _, m := range fresh {
		seen[m.ID] = true
	}
	carried := 0
	for _, m := range old {
		if seen[m.ID] {
			continue
		}
		fresh = append(fresh, m)
		seen[m.ID] = true
		carried++
	}
	return fresh, carried
}

func keepPublishers(models []outModel, publishers map[string]bool) []outModel {
	kept := models[:0]
	for _, m := range models {
		if publishers[strings.ToLower(m.Org)] && !skipRe.MatchString(m.Name) {
			m.Official = true
			kept = append(kept, m)
		}
	}
	return kept
}

func modelScopeCheckpointSize(c *http.Client, host, id string) float64 {
	var response msFilesResponse
	filesURL := host + "/api/v1/models/" + id + "/repo/files?Revision=master&Recursive=true"
	if err := getJSON(c, filesURL, &response); err != nil {
		return 0
	}
	var total int64
	for _, file := range response.Data.Files {
		if strings.HasSuffix(strings.ToLower(file.Path), ".safetensors") {
			total += file.Size
		}
	}
	return float64(total) / 1e9
}

func tagValue(tags []string, prefix string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, prefix) {
			return strings.TrimPrefix(tag, prefix)
		}
	}
	return ""
}

func parseOne(c *http.Client, host string, e hfEntry, minParams float64) (outModel, bool, string) {
	var m outModel
	var notePre string
	provider := e.Provider
	if provider == "" {
		provider = "hf"
	}
	rawBase, sourceURL, sourceLabel := host+"/"+e.ID+"/resolve/main/", "https://huggingface.co/"+e.ID, "HF"
	if provider == "modelscope" {
		rawBase = host + "/models/" + e.ID + "/resolve/master/"
		sourceURL = host + "/models/" + e.ID
		sourceLabel = "ModelScope"
	} else if e.Safetensors == nil || e.Safetensors.Total == 0 {
		// HF 详情接口给出与存储 dtype 无关的 safetensors 精确张量数。
		var info hfEntry
		if err := getJSON(c, host+"/api/models/"+e.ID, &info); err == nil {
			info.Official = e.Official
			e = info
		}
	}
	var cfg hfConfig
	if err := getJSON(c, rawBase+"config.json", &cfg); err != nil {
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
	slidingEnabled := cfg.UseSlidingWindow == nil || *cfg.UseSlidingWindow
	if slidingEnabled && cfg.SlidingWindow > 0 && localLayers == 0 && (kvLayers == 0 || kvLayers == cfg.Layers) {
		kvLayers = cfg.Layers
		localLayers = cfg.Layers
		if cfg.SlidingWindowPattern > 1 {
			localLayers -= cfg.Layers / cfg.SlidingWindowPattern
		}
	}
	kvLayers = min(kvLayers, cfg.Layers)
	localLayers = min(localLayers, kvLayers)
	window := cfg.SlidingWindow
	if localLayers == 0 || window <= 0 {
		localLayers = 0
		window = 0
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

	// checkpoint_gb 只统计 safetensors payload，不把 tokenizer、文档或重复格式计入显存。
	var idx struct {
		Metadata struct {
			TotalSize float64 `json:"total_size"`
		} `json:"metadata"`
	}
	getJSON(c, rawBase+"model.safetensors.index.json", &idx)
	checkpointGB := idx.Metadata.TotalSize / 1e9
	if checkpointGB == 0 {
		if provider == "modelscope" {
			checkpointGB = modelScopeCheckpointSize(c, host, e.ID)
		} else {
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
	}
	nativeQuant, bytesPer := "fp16", 2.0
	quantStorage := false
	if qc := cfg.QuantConfig; qc != nil {
		format := strings.ToLower(qc.Method + " " + qc.Format)
		quantStorage = qc.Format != "" || len(qc.ConfigGroups) > 0
		bits, weightType := 0, ""
		for _, group := range qc.ConfigGroups {
			format += " " + strings.ToLower(group.Format)
			if group.Weights.NumBits > 0 {
				bits, weightType = group.Weights.NumBits, strings.ToLower(group.Weights.Type)
			}
		}
		switch {
		case strings.Contains(format, "mxfp4"):
			nativeQuant, bytesPer = "mxfp4", 0.5
		case strings.Contains(format, "nvfp4") || strings.Contains(format, "fp4"):
			nativeQuant, bytesPer = "fp4", 0.5
		case strings.Contains(format, "fp8"):
			nativeQuant, bytesPer = "fp8", 1
		case bits > 0 && bits <= 4 && weightType == "float":
			nativeQuant, bytesPer = "fp4", 0.5
		case bits > 0 && bits <= 4:
			nativeQuant, bytesPer = "int4", 0.55
		case bits > 0 && bits <= 8:
			nativeQuant, bytesPer = "int8", 1.05
		case strings.Contains(format, "int8") || strings.Contains(format, "w8a8"):
			nativeQuant, bytesPer = "int8", 1.05
		case strings.Contains(format, "awq") || strings.Contains(format, "gptq") || strings.Contains(format, "int4"):
			nativeQuant, bytesPer = "int4", 0.55
			quantStorage = true
		}
	}
	var params float64
	if e.ParameterCount > 0 {
		params = float64(e.ParameterCount) / 1e9
	} else if e.Safetensors != nil && e.Safetensors.Total > 0 {
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
	params = math.Round(params*10) / 10
	// 只有包含具体打包格式/分组的量化配置能证明仓库存储格式；否则 payload/参数量优先，
	// 避免把仅声明运行时能力的 BF16 权重误标为 FP8。
	if checkpointGB > 0 && !quantStorage {
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
	if !moe {
		moeLayers = 0
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
	architecture := ""
	if len(cfg.Architectures) > 0 {
		architecture = cfg.Architectures[0]
	} else if len(e.Config.Architectures) > 0 {
		architecture = e.Config.Architectures[0]
	}
	dtype := cfg.TorchDType
	if dtype == "" {
		dtype = cfg.DType
	}
	if dtype == "" && e.Safetensors != nil {
		var largest int64
		for stored, count := range e.Safetensors.Parameters {
			if count <= largest {
				continue
			}
			switch strings.ToUpper(stored) {
			case "BF16":
				dtype, largest = "bfloat16", count
			case "F16", "FP16":
				dtype, largest = "float16", count
			case "F32", "FP32":
				dtype, largest = "float32", count
			}
		}
	}
	license := e.License
	if license == "" {
		license = tagValue(e.Tags, "license:")
	}
	tasks := e.Tasks
	if len(tasks) == 0 && e.PipelineTag != "" {
		tasks = []string{e.PipelineTag}
	}
	m = outModel{
		ID:   strings.ToLower(strings.ReplaceAll(e.ID, "/", "--")),
		Name: parts[1], Org: parts[0], Year: year,
		Params: params, Active: active, Layers: cfg.Layers, Hidden: cfg.Hidden,
		ModelType: cfg.ModelType, Architecture: architecture, DType: dtype, RopeTheta: cfg.RopeTheta,
		Intermediate: cfg.Intermediate, MoEIntermediate: cfg.MoeIntermediate,
		KVT: kvt, KVH: kvh, Dim: dim, MLA: mla, KVLayers: kvLayers, LocalLayers: localLayers, Window: window, StateMB: stateMB,
		Experts: experts, TopK: topk, SharedExperts: sharedExperts, MoELayers: moeLayers,
		Ctx: ctx, MoE: moe, Multimodal: multimodal, Sparse: officialSparse[strings.ToLower(e.ID)], Conf: "fetched", Official: e.Official, Src: provider,
		MTP: cfg.NextN > 0 || cfg.MTP > 0, MTPHeads: max(cfg.NextN, cfg.MTP),
		CheckpointGB: checkpointGB, NativeQuant: nativeQuant, SourceURL: sourceURL, Downloads: e.Downloads,
		License: license, Tasks: tasks, CreatedAt: e.CreatedAt, UpdatedAt: e.LastModified,
		Notes: notePre + activeNote + fmt.Sprintf("%s 采集 · %s · 下载 %.1fM", sourceLabel, e.ID, float64(e.Downloads)/1e6),
	}
	return m, true, ""
}
