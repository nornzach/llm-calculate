// Package calc 实现 LLM 推理计算器的显存可行性、decode/prefill
// 一阶 roofline 估算和反向部署规划。输出是容量筛选值，不是实测基准。
package calc

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
)

// ---------- 数据模型 ----------

type Link struct {
	T   string  `json:"t"`   // none | bridge | nvlink | xgmi | ethernet | hccs | unified | pcie
	B   float64 `json:"b"`   // 卡间互联总带宽 GB/s
	Dom int     `json:"dom"` // 全互联域最大卡数
}

type HW struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Vendor    string   `json:"vendor"`
	Cls       string   `json:"cls"` // consumer | workstation | datacenter | supernode | unified_soc | edge | sram_asic
	Arch      string   `json:"arch"`
	VRAM      float64  `json:"vram"` // GB；unified_soc 为物理内存
	BW        float64  `json:"bw"`   // 显存带宽 GB/s
	Link      Link     `json:"link"`
	Prec      []string `json:"prec"`
	TF        float64  `json:"tf"`                // dense FP16/BF16 tensor TFLOPS
	TF8       float64  `json:"tf8,omitempty"`     // dense FP8 tensor TFLOPS；0=按量化倍率估算
	TF4       float64  `json:"tf4,omitempty"`     // dense FP4 tensor TFLOPS；0=按量化倍率估算
	TFInt8    float64  `json:"tf_int8,omitempty"` // dense INT8 tensor TFLOPS；0=按 FP16 倍率估算
	TDP       float64  `json:"tdp"`
	CNY       float64  `json:"cny"` // 参考价（二手/整机），0=未知/仅云
	Conf      string   `json:"conf"`
	Unified   bool     `json:"unified,omitempty"`
	Svc       bool     `json:"svc,omitempty"`
	Notes     string   `json:"notes,omitempty"`
	SourceURL string   `json:"source_url,omitempty"`
}

type Model struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	Org             string  `json:"org"`
	Year            int     `json:"year"`
	Params          float64 `json:"params"` // 总参数 B
	Active          float64 `json:"active"` // 每 token 激活参数 B（dense = params）
	Layers          int     `json:"layers"`
	Hidden          float64 `json:"hidden"`
	ModelType       string  `json:"model_type,omitempty"`
	Architecture    string  `json:"architecture,omitempty"`
	DType           string  `json:"dtype,omitempty"`
	RopeTheta       float64 `json:"rope_theta,omitempty"`
	Intermediate    float64 `json:"intermediate,omitempty"`
	MoEIntermediate float64 `json:"moe_intermediate,omitempty"`
	KVT             string  `json:"kvt"` // mha | gqa | mla
	KVH             int     `json:"kvh"`
	Dim             int     `json:"dim"`
	MLA             float64 `json:"mla"` // MLA latent 维度（如 512+64）
	// KVLayers：混合注意力中持有逐 token KV 的层数；0 表示全部层。
	KVLayers int `json:"kvlayers,omitempty"`
	// LocalLayers/Window：持有滑动窗口 KV 的局部注意力层数及窗口 token 数。
	LocalLayers int `json:"local_layers,omitempty"`
	Window      int `json:"window,omitempty"`
	// StateMB：所有线性注意力层每请求的 FP16 recurrent state（MB），与上下文长度无关。
	StateMB float64 `json:"state_mb,omitempty"`
	// Experts/TopK 用于估算一个 decode batch 实际触达的不同专家数。
	Experts       int  `json:"experts,omitempty"`
	TopK          int  `json:"topk,omitempty"`
	SharedExperts int  `json:"shared_experts,omitempty"`
	MoELayers     int  `json:"moe_layers,omitempty"`
	MTP           bool `json:"mtp,omitempty"`
	MTPHeads      int  `json:"mtp_heads,omitempty"`
	// Sparse：稀疏注意力每个 query 选择的 token 数；0 表示稠密注意力。
	Sparse        float64  `json:"sparse,omitempty"`
	Ctx           int      `json:"ctx"`
	MoE           bool     `json:"moe"`
	EncoderParams float64  `json:"encoder_params,omitempty"` // 非自回归视觉/音频 encoder 参数 B
	Multimodal    bool     `json:"multimodal,omitempty"`
	Conf          string   `json:"conf"` // official | reported | fetched
	Official      bool     `json:"official,omitempty"`
	Src           string   `json:"src,omitempty"`
	CheckpointGB  float64  `json:"checkpoint_gb,omitempty"` // 原仓库 safetensors payload GB
	NativeQuant   string   `json:"native_quant,omitempty"`  // 原仓库权重格式
	SourceURL     string   `json:"source_url,omitempty"`
	Downloads     int64    `json:"downloads,omitempty"`
	License       string   `json:"license,omitempty"`
	Tasks         []string `json:"tasks,omitempty"`
	CreatedAt     string   `json:"created_at,omitempty"`
	UpdatedAt     string   `json:"updated_at,omitempty"`
	Notes         string   `json:"notes,omitempty"`
}

// Quant 权重量化档位。权重精度（W）与计算/激活精度（A）分开标注。
// W4A16 主要减少容量和带宽；只有 W8A8/W4A4 才能直接套对应低精度峰值。
// KV cache 量化是独立维度（Opts.KVQuant）。
type Quant struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Bytes float64 `json:"bytes"`
	Eta   float64 `json:"eta"`  // decode 带宽利用率
	W     string  `json:"w"`    // 权重精度标签
	A     string  `json:"a"`    // 激活/计算精度标签
	Mul   float64 `json:"mul"`  // prefill 算力倍率（需 Need 硬件路径）
	Need  string  `json:"need"` // 硬件 prec 要求（"" = 无原生路径）
	Fam   string  `json:"fam"`  // std | gguf | mlx | exl
	Main  bool    `json:"main"` // 是否进入 fit 矩阵列
	Note  string  `json:"note"`
}

var Quants = []Quant{
	// ---- 数据中心/通用 ----
	{ID: "fp16", Name: "FP16/BF16", Bytes: 2.0, Eta: 0.50, W: "16bit", A: "FP16", Mul: 1, Need: "fp16", Fam: "std", Main: true, Note: "基准精度"},
	{ID: "fp8", Name: "FP8·W8A8", Bytes: 1.0, Eta: 0.62, W: "8bit", A: "FP8", Mul: 2, Need: "fp8", Fam: "std", Main: true, Note: "Ada/Hopper+ 权重激活双 FP8，prefill 2×"},
	{ID: "int8", Name: "INT8·W8A8", Bytes: 1.05, Eta: 0.60, W: "8bit", A: "INT8", Mul: 2, Need: "int8", Fam: "std", Note: "SmoothQuant 系，Ampere+ INT8 tensor 2×"},
	{ID: "int4", Name: "INT4·W4A16", Bytes: 0.55, Eta: 0.60, W: "4bit", A: "FP16", Mul: 1, Need: "int4", Fam: "std", Main: true, Note: "AWQ/GPTQ/QAT 等权重量化：省显存与带宽；硬件内核决定实际收益"},
	{ID: "fp4", Name: "FP4·NVFP4", Bytes: 0.55, Eta: 0.62, W: "4bit", A: "FP4", Mul: 4, Need: "fp4", Fam: "std", Main: true, Note: "Blackwell 全 FP4 管线，prefill 4×；其余卡仅省显存"},
	{ID: "mxfp4", Name: "FP4·MXFP4", Bytes: 0.55, Eta: 0.58, W: "4bit", A: "FP16/MXFP8", Mul: 1, Need: "fp4", Fam: "std", Main: true, Note: "MXFP4 权重；激活精度和未量化张量由检查点决定，不等于通用 W4A4"},
	// ---- GGUF（llama.cpp 生态）----
	{ID: "q8", Name: "GGUF·Q8_0", Bytes: 1.06, Eta: 0.52, W: "8bit", A: "FP16", Mul: 1, Fam: "gguf", Note: "近无损，CPU/Metal/CUDA 通吃"},
	{ID: "q6", Name: "GGUF·Q6_K", Bytes: 0.83, Eta: 0.53, W: "6bit", A: "FP16", Mul: 1, Fam: "gguf", Note: "质量/体积甜点"},
	{ID: "q4km", Name: "GGUF·Q4_K_M", Bytes: 0.60, Eta: 0.55, W: "4bit", A: "FP16", Mul: 1, Fam: "gguf", Main: true, Note: "最常见的 GGUF 档位（~4.85bpw）"},
	{ID: "iq2", Name: "GGUF·IQ2_XXS", Bytes: 0.31, Eta: 0.42, W: "2bit", A: "FP16", Mul: 1, Fam: "gguf", Note: "极限 2bit：能跑 R1 的最小体积，精度损失明显"},
	// ---- MLX（Apple 专用）----
	{ID: "mlx8", Name: "MLX·8bit", Bytes: 1.06, Eta: 0.58, W: "8bit", A: "FP16", Mul: 1, Fam: "mlx", Note: "Apple Silicon 原生，统一内存零拷贝"},
	{ID: "mlx4", Name: "MLX·4bit", Bytes: 0.55, Eta: 0.55, W: "4bit", A: "FP16", Mul: 1, Fam: "mlx", Note: "Apple Silicon 原生主力档"},
	// ---- ExLlama（消费卡低并发）----
	{ID: "exl3", Name: "EXL3·4.25bpw", Bytes: 0.56, Eta: 0.60, W: "4bit", A: "FP16", Mul: 1, Fam: "exl", Note: "ExLlamaV3，1~4 并发速度王"},
}

// MainQuants fit 矩阵展示的列（其余档位仅在下拉框中可选）。
func MainQuants() []Quant {
	var out []Quant
	for _, q := range Quants {
		if q.Main {
			out = append(out, q)
		}
	}
	return out
}

func LookupQuant(id string) (Quant, bool) {
	for _, q := range Quants {
		if q.ID == id {
			return q, true
		}
	}
	return Quant{}, false
}

func QuantByID(id string) Quant {
	if q, ok := LookupQuant(id); ok {
		return q
	}
	return Quants[0]
}

// FixedQuantID 返回预量化仓库锁定的权重格式。FP16/BF16 基础仓库仍可模拟再量化。
func (m Model) FixedQuantID() string {
	if m.NativeQuant == "" || m.NativeQuant == "fp16" {
		return ""
	}
	for _, q := range Quants {
		if q.ID == m.NativeQuant {
			return q.ID
		}
	}
	return ""
}

func (m Model) effectiveQuant(q Quant) (Quant, bool) {
	if id := m.FixedQuantID(); id != "" {
		return QuantByID(id), true
	}
	return q, false
}

// ---------- 推理栈：框架 / 推测解码 / KV 量化 / 缓存 ----------

// Engine 保留框架兼容性和显存底座。EtaMul/Flops/StepMs/SchedK 是场景参数；
// 缺少同机同模型同 workload 的校准数据前保持相同，避免虚构框架排名。
type Engine struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	EtaMul  float64  `json:"eta_mul"`
	Flops   float64  `json:"flops"`
	StepMs  float64  `json:"step_ms"`
	SchedK  float64  `json:"sched_k"`
	FwMem   float64  `json:"fw_mem"`            // 估算的框架常驻显存 GB
	Vendors []string `json:"vendors,omitempty"` // 空 = 全平台
	Note    string   `json:"note"`
}

var Engines = []Engine{
	{ID: "auto", Name: "自动选型", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 1.5,
		Note: "按量化格式和硬件厂商选择运行时"},
	{ID: "vllm", Name: "vLLM", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 1.5,
		Vendors: []string{"nvidia", "amd", "intel", "huawei", "hygon", "metax", "mthreads", "enflame", "biren", "iluvatar", "kunlunxin", "cambricon"},
		Note:    "PagedAttention、continuous batching；非主线硬件通常依赖厂商插件或分支"},
	{ID: "sglang", Name: "SGLang", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 1.6,
		Vendors: []string{"nvidia", "amd", "huawei", "hygon"},
		Note:    "RadixAttention、PD/EP/DP 等服务能力；可用组合须按版本核对"},
	{ID: "trtllm", Name: "TensorRT-LLM", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 1.8,
		Vendors: []string{"nvidia"},
		Note:    "NVIDIA CUDA 推理栈；内核和功能须按 GPU 代际及版本核对"},
	{ID: "llamacpp", Name: "llama.cpp", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 1.2,
		Note: "GGUF 跨 CPU、Metal、CUDA、HIP 等后端"},
	{ID: "mlx", Name: "MLX", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 1.0,
		Vendors: []string{"apple"},
		Note:    "Apple Silicon 统一内存运行时"},
	{ID: "exllama", Name: "ExLlamaV3", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 1.2,
		Vendors: []string{"nvidia"},
		Note:    "NVIDIA GPU 的 EXL3 量化运行时"},
	{ID: "lmdeploy", Name: "LMDeploy", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 1.5,
		Vendors: []string{"nvidia"},
		Note:    "TurboMind/vLLM 后端；此处仅列已核对的 NVIDIA 路径"},
	{ID: "mindie", Name: "MindIE", EtaMul: 1, Flops: 0.45, StepMs: 1, SchedK: 512, FwMem: 2.0,
		Vendors: []string{"huawei"},
		Note:    "昇腾官方推理引擎"},
}

// EngineOK 该框架是否原生支持此硬件。厂商未知（空）时视为兼容。
func (e Engine) EngineOK(h HW) bool {
	if len(e.Vendors) == 0 || h.Vendor == "" {
		return true
	}
	for _, v := range e.Vendors {
		if v == h.Vendor {
			return true
		}
	}
	return false
}

// resolveEngine 处理 auto：量化家族优先（GGUF→llama.cpp，MLX→MLX，EXL→ExLlama），
// 否则按硬件厂商选默认框架。
func resolveEngine(id string, h HW, q Quant) Engine {
	if id == "" || id == "auto" {
		switch q.Fam {
		case "gguf":
			id = "llamacpp"
		case "mlx":
			id = "mlx"
		case "exl":
			id = "exllama"
		default:
			switch h.Vendor {
			case "apple":
				id = "mlx"
			case "huawei":
				id = "mindie"
			default:
				id = "vllm"
			}
		}
	}
	for _, e := range Engines {
		if e.ID == id && e.ID != "auto" {
			return e
		}
	}
	return Engines[1] // vllm
}

// SpecMethod 推测/并行解码方法。
// Tau = 平均接受长度（每步产出 token 数）；Ovh = 草稿+验证的每步时间开销；
// 单流净加速 gain(1) = Tau/(1+Ovh)；并发衰减 gain(b) = 1+(gain1-1)×max(Floor, 1-(b-1)/Bsat)，
// Floor 可为负（高并发反噬，如 EAGLE-3 b=32 实测 0.5×）。
type SpecMethod struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Tau   float64 `json:"tau"`
	Ovh   float64 `json:"ovh"`
	Bsat  float64 `json:"bsat"`
	Floor float64 `json:"floor"`
	MemGB float64 `json:"mem_gb"`
	Note  string  `json:"note"`
}

// 这些是可编辑的场景系数，不是跨模型、跨 workload 的保证值。除 lookahead 外，
// 用户选择某方法表示已有与目标模型匹配的草稿模型或预测头。
var SpecMethods = []SpecMethod{
	{ID: "none", Name: "关闭", Tau: 1, Bsat: 1, Note: "逐 token 自回归解码"},
	{ID: "mtp", Name: "MTP 原生多头", Tau: 1.9, Ovh: 0.06, Bsat: 80, Floor: 0.4,
		Note: "仅模型元数据明确含 MTP 头时生效；接受率和收益必须按 workload 校准"},
	{ID: "eagle3", Name: "EAGLE-3", Tau: 2.8, Ovh: 0.15, Bsat: 22, Floor: -0.35, MemGB: 1.0,
		Note: "需与目标模型匹配并训练的草稿头；系数仅为场景估算"},
	{ID: "medusa", Name: "Medusa", Tau: 2.5, Ovh: 0.10, Bsat: 20, Floor: -0.2, MemGB: 0.5,
		Note: "需目标模型的 Medusa 头；系数仅为场景估算"},
	{ID: "draft", Name: "草稿模型", Tau: 2.24, Ovh: 0.30, Bsat: 16, Floor: -0.2, MemGB: 0,
		Note: "假设有兼容小模型；暂按目标权重 5% 计草稿显存，收益须实测"},
	{ID: "lookahead", Name: "Lookahead", Tau: 1.9, Ovh: 0.08, Bsat: 24, Floor: 0, MemGB: 0,
		Note: "n-gram 零草稿成本；收益高度依赖代码/编辑类重复模式"},
	{ID: "dflash", Name: "DFlash", Tau: 6.5, Ovh: 0.30, Bsat: 64, Floor: 0.3, MemGB: 0.8,
		Note: "需匹配的块扩散草稿；论文均值不能直接当生产吞吐"},
	{ID: "dflash2", Name: "DFlash2", Tau: 4.5, Ovh: 0.30, Bsat: 36, Floor: 0.1, MemGB: 1.2,
		Note: "需匹配草稿检查点；模型卡数据不能直接当生产吞吐"},
}

func SpecByID(id string) SpecMethod {
	for _, s := range SpecMethods {
		if s.ID == id {
			return s
		}
	}
	return SpecMethods[0]
}

// gain 当前并发下的净加速比（含草稿开销与并发衰减）。
func (s SpecMethod) gain(batch int) float64 {
	if s.ID == "none" || s.Tau <= 1 {
		return 1
	}
	g1 := s.Tau / (1 + s.Ovh)
	d := 1 - float64(batch-1)/s.Bsat
	if d < s.Floor {
		d = s.Floor
	}
	g := 1 + (g1-1)*d
	if g < 0.3 {
		g = 0.3 // 反噬下限
	}
	return g
}

// Opts 同时承载部署、缓存、媒体和实测校准。零值保持原有简单模式。
type Opts struct {
	Engine  string  `json:"engine"`
	Spec    string  `json:"spec"`
	KVQuant string  `json:"kvq"`    // fp16 | fp8 | fp4
	HitRate float64 `json:"hit"`    // 命中的前缀 token 比例
	OutLen  int     `json:"outlen"` // 平均输出 token

	TP int `json:"tp,omitempty"` // tensor parallel；0=使用全部 cards
	PP int `json:"pp,omitempty"` // pipeline parallel
	EP int `json:"ep,omitempty"` // expert parallel
	CP int `json:"cp,omitempty"` // context/KV parallel

	WeightGB     float64 `json:"weight_gb,omitempty"`     // 实际加载权重 GB（整个副本）
	RuntimeGB    float64 `json:"runtime_gb,omitempty"`    // 每卡实测框架常驻 GB
	ActivationGB float64 `json:"activation_gb,omitempty"` // 每卡实测峰值 workspace GB
	AdapterGB    float64 `json:"adapter_gb,omitempty"`    // 整个副本加载的 adapter 权重 GB
	DraftGB      float64 `json:"draft_gb,omitempty"`      // 整个副本额外 draft/head 权重 GB
	MemUtil      float64 `json:"mem_util,omitempty"`      // 可用显存比例
	BWUtil       float64 `json:"bw_util,omitempty"`       // 实测 HBM 带宽利用率
	FlopsUtil    float64 `json:"flops_util,omitempty"`    // 实测 dense 峰值利用率
	LinkUtil     float64 `json:"link_util,omitempty"`     // 实测互联带宽利用率
	ScheduleMS   float64 `json:"schedule_ms,omitempty"`   // 每 decode step 实测调度开销

	KVOverhead   float64 `json:"kv_overhead,omitempty"` // block/allocator 容量系数
	KVOffload    float64 `json:"kv_offload,omitempty"`  // 卸载到 CPU/远端的 KV 比例
	OffloadBW    float64 `json:"offload_bw,omitempty"`  // 单卡有效回读 GB/s
	PrefillChunk int     `json:"prefill_chunk,omitempty"`
	MediaTokens  int     `json:"media_tokens,omitempty"` // 视觉/音频 encoder 输入 token
	RouterSkew   float64 `json:"router_skew,omitempty"`  // EP 最忙 rank / 平均负载
	SpecTau      float64 `json:"spec_tau,omitempty"`     // 实测每步接受 token
	SpecOvh      float64 `json:"spec_ovh,omitempty"`     // 实测 draft/verify 相对开销
	Lang         string  `json:"lang,omitempty"`         // en（默认）| zh；仅影响展示文本
	skipTrace    bool
}

func (o Opts) norm() Opts {
	if o.KVQuant == "" {
		o.KVQuant = "fp16"
	}
	o.HitRate = clamp(o.HitRate, 0, 0.9)
	if o.OutLen <= 0 {
		o.OutLen = 512
	}
	if o.PrefillChunk <= 0 {
		o.PrefillChunk = 8192
	}
	o.PrefillChunk = min(o.PrefillChunk, 1<<20)
	if o.KVOverhead <= 0 {
		o.KVOverhead = 1
	}
	o.KVOverhead = clamp(o.KVOverhead, 1, 2)
	o.KVOffload = clamp(o.KVOffload, 0, 1)
	if o.KVOffload > 0 && o.OffloadBW <= 0 {
		o.KVOffload = 0
	}
	o.MemUtil = clamp(o.MemUtil, 0, 1)
	o.BWUtil = clamp(o.BWUtil, 0, 1)
	o.FlopsUtil = clamp(o.FlopsUtil, 0, 1)
	o.LinkUtil = clamp(o.LinkUtil, 0, 1)
	o.WeightGB = math.Max(0, o.WeightGB)
	o.RuntimeGB = math.Max(0, o.RuntimeGB)
	o.ActivationGB = math.Max(0, o.ActivationGB)
	o.AdapterGB = math.Max(0, o.AdapterGB)
	o.DraftGB = math.Max(0, o.DraftGB)
	o.ScheduleMS = clamp(o.ScheduleMS, 0, 10_000)
	o.OffloadBW = clamp(o.OffloadBW, 0, 1_000_000)
	o.RouterSkew = clamp(o.RouterSkew, 1, 16)
	o.SpecTau = clamp(o.SpecTau, 0, 32)
	o.SpecOvh = clamp(o.SpecOvh, 0, 10)
	o.MediaTokens = max(0, o.MediaTokens)
	if o.Lang != "zh" {
		o.Lang = "en"
	}
	return o
}

// kvSupported 只接受当前计算器明确建模的运行时路径；不把“能存更小”
// 与“所选引擎能执行该 KV 格式”混为一谈。
func (o Opts) kvSupported(h HW, eng Engine) bool {
	switch o.KVQuant {
	case "", "fp16":
		return true
	case "fp8":
		return precHas(h, "fp8") && (eng.ID == "vllm" || eng.ID == "sglang" || eng.ID == "trtllm")
	case "fp4":
		return precHas(h, "fp4") && eng.ID == "sglang"
	default:
		return false
	}
}

// kvMemF 返回可执行 KV 格式的容量比；不支持的组合按 FP16 保守计算。
func (o Opts) kvMemF(h HW, eng Engine) float64 {
	if !o.kvSupported(h, eng) {
		return 1
	}
	switch o.KVQuant {
	case "fp8":
		return 0.5
	case "fp4":
		return 1 / 3.56
	default:
		return 1
	}
}

func (o Opts) kvReadF(h HW, eng Engine) float64 {
	return o.kvMemF(h, eng)
}

// LoadHW / LoadModels 解析嵌入的 JSON 数据。
func LoadHW(b []byte) ([]HW, error) {
	var v []HW
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(v))
	for _, h := range v {
		if h.ID == "" || h.Name == "" || h.Vendor == "" || seen[h.ID] {
			return nil, fmt.Errorf("invalid or duplicate hardware %q", h.ID)
		}
		seen[h.ID] = true
		if !h.Svc && (h.VRAM <= 0 || h.BW <= 0 || h.TF <= 0 || len(h.Prec) == 0) {
			return nil, fmt.Errorf("hardware %q lacks roofline inputs", h.ID)
		}
	}
	return v, nil
}

func LoadModels(b []byte) ([]Model, error) {
	var v []Model
	if err := json.Unmarshal(b, &v); err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(v))
	for _, m := range v {
		if m.ID == "" || m.Name == "" || m.Org == "" || seen[m.ID] {
			return nil, fmt.Errorf("invalid or duplicate model %q", m.ID)
		}
		seen[m.ID] = true
		if m.Params <= 0 || m.Active <= 0 || m.Active > m.Params || m.Layers <= 0 || m.Hidden <= 0 || m.Ctx <= 0 {
			return nil, fmt.Errorf("model %q has invalid core dimensions", m.ID)
		}
		if (m.KVT == "mla" && m.MLA <= 0) ||
			((m.KVT == "mha" || m.KVT == "gqa") && (m.KVH <= 0 || m.Dim <= 0)) {
			return nil, fmt.Errorf("model %q has invalid KV dimensions", m.ID)
		}
		if m.KVT != "mha" && m.KVT != "gqa" && m.KVT != "mla" {
			return nil, fmt.Errorf("model %q has unknown attention type %q", m.ID, m.KVT)
		}
		if m.KVLayers > m.Layers || m.LocalLayers > m.kvLayers() || (m.LocalLayers > 0 && m.Window <= 0) {
			return nil, fmt.Errorf("model %q has invalid attention layers", m.ID)
		}
		if m.NativeQuant != "" {
			if _, ok := LookupQuant(m.NativeQuant); !ok {
				return nil, fmt.Errorf("model %q has unknown native quant %q", m.ID, m.NativeQuant)
			}
		}
	}
	return v, nil
}

// ---------- 基础量 ----------

// Accel 返回该硬件上此量化档位是否有硬件快路径（否则仅省显存/带宽打折）。
func (h HW) Accel(q Quant) bool {
	switch q.Fam {
	case "mlx":
		return h.Vendor == "apple"
	case "exl":
		return h.Vendor == "nvidia"
	case "gguf":
		return false // 反量化路径，无原生加速
	}
	if q.ID == "fp4" {
		return h.Vendor == "nvidia" && precHas(h, "fp4")
	}
	if q.ID == "mxfp4" {
		return (h.Vendor == "nvidia" || h.Vendor == "amd") && precHas(h, "fp4")
	}
	if q.ID == "fp16" {
		return precHas(h, "fp16") || precHas(h, "bf16")
	}
	if q.Need != "" {
		return precHas(h, q.Need)
	}
	return false
}

// PeakTF 返回该量化路径可用的 dense 峰值；缺逐精度规格时使用架构倍率并标记为估算。
func (h HW) PeakTF(q Quant) float64 {
	if q.ID == "fp8" && h.TF8 > 0 {
		return h.TF8
	}
	if q.ID == "int8" && h.TFInt8 > 0 {
		return h.TFInt8
	}
	if q.ID == "fp4" && h.Accel(q) && h.TF4 > 0 {
		return h.TF4
	}
	if h.Accel(q) {
		return h.TF * q.Mul
	}
	return h.TF
}

func (h HW) peakExact(q Quant) bool {
	switch q.ID {
	case "fp8":
		return h.TF8 > 0
	case "int8":
		return h.TFInt8 > 0
	case "fp4":
		return h.Accel(q) && h.TF4 > 0
	}
	return q.Fam == "std" && h.TF > 0
}

type topology struct {
	tp, pp, ep, cp int
	valid          bool
}

func (o Opts) topology(cards int) topology {
	if cards < 1 {
		cards = 1
	}
	if o.TP == 0 && o.PP == 0 && o.EP == 0 && o.CP == 0 {
		return topology{tp: cards, pp: 1, ep: 1, cp: 1, valid: true}
	}
	t := topology{tp: max(1, o.TP), pp: max(1, o.PP), ep: max(1, o.EP), cp: max(1, o.CP)}
	t.valid = t.tp*t.pp*t.ep*t.cp == cards
	if !t.valid {
		t.tp, t.pp, t.ep, t.cp = cards, 1, 1, 1
	}
	return t
}

func (o Opts) topologyFor(m Model, cards int) topology {
	t := o.topology(cards)
	if t.ep > 1 && !m.MoE {
		return topology{tp: max(1, cards), pp: 1, ep: 1, cp: 1, valid: false}
	}
	return t
}

func (t topology) String() string {
	return fmt.Sprintf("TP%d · PP%d · EP%d · CP%d", t.tp, t.pp, t.ep, t.cp)
}

type weightParts struct {
	baseTotal, expertTotal   float64
	expertActive, expertRead float64
}

func (m Model) weights(batch int) weightParts {
	textParams := math.Max(0, m.Params-m.EncoderParams)
	textActive := math.Min(textParams, m.Active)
	p := weightParts{baseTotal: textParams}
	if !m.MoE || m.Experts <= m.TopK || m.TopK <= 0 {
		return p
	}
	perExpert := (textParams - textActive) / float64(m.Experts-m.TopK)
	if perExpert <= 0 {
		return p
	}
	p.expertTotal = float64(m.Experts) * perExpert
	p.baseTotal = math.Max(0, textParams-p.expertTotal)
	p.expertActive = float64(m.TopK) * perExpert
	unique := float64(m.Experts) * (1 - math.Pow(1-float64(m.TopK)/float64(m.Experts), float64(batch)))
	p.expertRead = math.Min(p.expertTotal, unique*perExpert)
	return p
}

func (o Opts) weightGB(m Model, q Quant) float64 {
	if o.WeightGB > 0 {
		return o.WeightGB
	}
	if m.CheckpointGB > 0 && m.NativeQuant == q.ID {
		return m.CheckpointGB
	}
	return m.Params * q.Bytes
}

func (o Opts) bwUtil(q Quant, eng Engine, h HW) float64 {
	if o.BWUtil > 0 {
		return o.BWUtil
	}
	v := q.Eta * eng.EtaMul
	if h.Unified {
		v *= 0.85
	}
	return v
}

func (o Opts) flopsUtil(eng Engine) float64 {
	if o.FlopsUtil > 0 {
		return o.FlopsUtil
	}
	return eng.Flops
}

func (o Opts) linkBW(h HW) float64 {
	bw := h.Link.B
	if bw <= 0 {
		bw = pcieBW
	}
	if o.LinkUtil > 0 {
		bw *= o.LinkUtil
	}
	return bw
}

func (o Opts) capGB(h HW, eng Engine) float64 {
	if o.MemUtil > 0 {
		return h.VRAM * o.MemUtil
	}
	return h.CapGB(eng)
}

func (o Opts) activationGB(m Model, ctx, batch int, t topology) float64 {
	if o.ActivationGB > 0 {
		return o.ActivationGB
	}
	tokens := float64(batch * min(ctx, o.PrefillChunk))
	// FlashAttention 不保存 O(n²) attention matrix；保留 residual、QKV/MLP workspace 的一阶上界。
	return tokens * m.Hidden * 2 * 4 / 1e9 / float64(t.tp*t.cp)
}

// kvLayers 返回持有逐 token KV 的 attention 层数。
func (m Model) kvLayers() int {
	if m.KVLayers > 0 {
		return m.KVLayers
	}
	return m.Layers
}

func (m Model) localLayers() int {
	if m.LocalLayers < 0 || m.LocalLayers > m.kvLayers() || m.Window <= 0 {
		return 0
	}
	return m.LocalLayers
}

func (m Model) kvLayerBytes() float64 {
	if m.KVT == "mla" {
		return m.MLA * 2
	}
	return 2 * float64(m.KVH) * float64(m.Dim) * 2
}

// KVTokBytes 返回所有 KV attention 层追加一个 token 的 FP16 K+V 字节数。
func (m Model) KVTokBytes() float64 {
	return float64(m.kvLayers()) * m.kvLayerBytes()
}

// KVBytes 返回一个请求在指定上下文下实际保留的 FP16 KV 字节数。
// full attention 保留全部上下文；sliding/local attention 只保留 Window。
func (m Model) KVBytes(ctx int) float64 {
	layers := m.kvLayers()
	local := m.localLayers()
	full := layers - local
	localCtx := ctx
	if local > 0 && localCtx > m.Window {
		localCtx = m.Window
	}
	return (float64(full*ctx) + float64(local*localCtx)) * m.kvLayerBytes()
}

// KVBatchBytes 返回 batch 个请求在共享前缀命中时实际驻留的 FP16 KV 字节。
// full-attention 前缀 block 只存一份；local-attention 仅共享仍落在滑窗内的尾部。
func (m Model) KVBatchBytes(ctx, batch int, hit float64) float64 {
	if ctx <= 0 || batch <= 0 {
		return 0
	}
	shared := min(ctx, int(float64(ctx)*clamp(hit, 0, 1)))
	local := m.localLayers()
	full := m.kvLayers() - local
	fullTokens := shared + (ctx-shared)*batch
	localCtx := ctx
	if local > 0 {
		localCtx = min(ctx, m.Window)
	}
	sharedLocal := min(shared, max(0, localCtx-(ctx-shared)))
	localTokens := sharedLocal + (localCtx-sharedLocal)*batch
	return (float64(full*fullTokens) + float64(local*localTokens)) * m.kvLayerBytes()
}

// kvRankFactor 是每个 TP rank 持有的逐 token KV 比例。KV head 少于 TP
// 时会复制，不能把 KV 无条件除以 TP；MLA latent cache 默认在 TP ranks 间复制。
func (m Model) kvRankFactor(tp int) float64 {
	if tp <= 1 || m.KVT == "mla" || m.KVH <= 0 {
		return 1
	}
	return math.Ceil(float64(m.KVH)/float64(tp)) / float64(m.KVH)
}

// CapGB 返回当前引擎的单卡可用显存预算。vLLM/SGLang/TRT 类服务引擎
// 按常用 0.90 配置；本地轻量运行时按 0.95；统一内存为系统保留更多空间。
func (h HW) CapGB(eng Engine) float64 {
	if h.Unified {
		return h.VRAM * 0.70
	}
	switch eng.ID {
	case "llamacpp", "mlx", "exllama":
		return h.VRAM * 0.95
	default:
		return h.VRAM * 0.90
	}
}

// MemDetail 五段式显存明细（每卡，TP 分片后）。
type MemDetail struct {
	Weights     float64 `json:"weights"`
	KV          float64 `json:"kv"`
	Fw          float64 `json:"fw"`
	Act         float64 `json:"act"`
	Adapter     float64 `json:"adapter"`
	OffloadedKV float64 `json:"offloaded_kv"`
	Sys         float64 `json:"sys"`
	Total       float64 `json:"total"`
	P999Total   float64 `json:"p999_total,omitempty"` // 按驻留时间加权的并发显存 P99.9 保护值
	Cap         float64 `json:"cap"`
	Budget      float64 `json:"budget"`   // 引擎可分配预算；Cap 为物理显存
	HeadPct     float64 `json:"head_pct"` // (Budget-已分配)/物理显存
	Fit         bool    `json:"fit"`
}

func Memory(h HW, m Model, q Quant, ctx, batch, cards int, o Opts) MemDetail {
	o = o.norm()
	q, _ = m.effectiveQuant(q)
	eng := resolveEngine(o.Engine, h, q)
	t := o.topologyFor(m, cards)
	spec := SpecByID(o.Spec)
	parts := m.weights(batch)
	weightTotal := o.weightGB(m, q)
	textParams := math.Max(0, m.Params-m.EncoderParams)
	encoderWeight := 0.0
	if m.Params > 0 {
		encoderWeight = weightTotal * m.EncoderParams / m.Params
	}
	textWeight := math.Max(0, weightTotal-encoderWeight)
	weights := encoderWeight/float64(t.tp) +
		textWeight*(parts.baseTotal/math.Max(textParams, 1e-9)/float64(t.tp*t.pp)+
			parts.expertTotal/math.Max(textParams, 1e-9)/float64(t.tp*t.ep*t.pp))

	kvRaw := m.KVBatchBytes(ctx, batch, o.HitRate) / 1e9 * o.kvMemF(h, eng) * o.KVOverhead *
		m.kvRankFactor(t.tp) / float64(t.pp*t.cp)
	offloadedKV := kvRaw * o.KVOffload
	state := m.StateMB / 1000 * float64(batch) / float64(t.tp*t.pp)
	kv := kvRaw - offloadedKV + state

	runtime := eng.FwMem
	if o.RuntimeGB > 0 {
		runtime = o.RuntimeGB
	}
	draft := o.DraftGB
	if draft <= 0 && (spec.ID != "mtp" || m.MTP || m.MTPHeads > 0) {
		draft = spec.MemGB
		if o.Spec == "draft" {
			draft = weightTotal * 0.05
		}
	}
	adapter := (o.AdapterGB + draft) / float64(t.tp*t.pp)
	act := o.activationGB(m, ctx, batch, t)
	allocated := weights + kv + runtime + act + adapter
	budget := o.capGB(h, eng)
	sys := math.Max(0, h.VRAM-budget)
	d := MemDetail{
		Weights: weights, KV: kv, Fw: runtime, Act: act, Adapter: adapter, OffloadedKV: offloadedKV,
		Sys: sys, Total: allocated + sys, Cap: h.VRAM, Budget: budget,
	}
	d.HeadPct = (budget - allocated) / h.VRAM
	d.Fit = allocated <= budget
	return d
}

// ---------- 吞吐 / 真实工作负载 ----------

// WorkloadBucket 是按请求到达数统计的工作负载桶；Share 会在计算时归一化。
type WorkloadBucket struct {
	Context   int     `json:"context"`
	Output    int     `json:"output"`
	Share     float64 `json:"share"`
	PrefixHit float64 `json:"prefix_hit"`
}

type WorkloadBucketPerf struct {
	Context     int     `json:"context"`
	Output      int     `json:"output"`
	Share       float64 `json:"share"`     // 请求到达占比
	Occupancy   float64 `json:"occupancy"` // share × request latency 后的在途占比
	PrefixHit   float64 `json:"prefix_hit"`
	SingleTPS   float64 `json:"single_tps"`
	TTFTms      float64 `json:"ttft_ms"`
	TPOTms      float64 `json:"tpot_ms"`
	ReqMs       float64 `json:"req_ms"`
	BatchMemory float64 `json:"batch_memory"`
	Fit         bool    `json:"fit"`
}

type WorkloadStats struct {
	Buckets      []WorkloadBucketPerf `json:"buckets"`
	MeanContext  float64              `json:"mean_context"`
	MeanOutput   float64              `json:"mean_output"`
	P95Context   int                  `json:"p95_context"`
	P99Context   int                  `json:"p99_context"`
	P999Context  int                  `json:"p999_context"`
	MaxContext   int                  `json:"max_context"`
	P95TTFTms    float64              `json:"p95_ttft_ms"`
	P99TTFTms    float64              `json:"p99_ttft_ms"`
	P999TTFTms   float64              `json:"p999_ttft_ms"`
	P95ReqMs     float64              `json:"p95_req_ms"`
	P99ReqMs     float64              `json:"p99_req_ms"`
	P999ReqMs    float64              `json:"p999_req_ms"`
	P95SingleTPS float64              `json:"p95_single_tps"`
}

type Perf struct {
	Fit             bool           `json:"fit"`
	Mem             MemDetail      `json:"mem"`
	SingleTPS       float64        `json:"single_tps"`
	AggTPS          float64        `json:"agg_tps"`
	PreTPS          float64        `json:"pre_tps"`   // 有效 prefill 速度 tok/s（当前上下文口径）
	ReqS            float64        `json:"req_s"`     // 稳态请求速率 req/s（当前并发）
	TPM             float64        `json:"tpm"`       // decode 输出 tok/min
	TPMMixed        float64        `json:"tpm_mixed"` // 混合 TPM：一分钟处理的输入+输出总 token
	TTFTms          float64        `json:"ttft_ms"`
	TPOTms          float64        `json:"tpot_ms"`
	ReqMs           float64        `json:"req_ms"` // 单请求时延（TTFT + (outlen-1)×TPOT）
	MaxBatch        int            `json:"max_batch"`
	Accel           bool           `json:"accel"`
	QuantID         string         `json:"quant"`
	QuantLocked     bool           `json:"quant_locked"`
	KVSupported     bool           `json:"kv_supported"`
	SpecApplied     bool           `json:"spec_applied"`
	EngName         string         `json:"eng_name"`
	EngOK           bool           `json:"eng_ok"` // 框架是否原生支持该硬件
	SpecName        string         `json:"spec_name"`
	Bottleneck      string         `json:"bottleneck"`        // memory | compute
	DecodeMemMs     float64        `json:"decode_mem_ms"`     // 每 decode step 的显存 roof
	DecodeComputeMs float64        `json:"decode_compute_ms"` // 每 decode step 的算力 roof
	CommMs          float64        `json:"comm_ms"`           // 每 decode step 的 TP collective
	ScheduleMs      float64        `json:"schedule_ms"`       // 每 decode step 的调度场景值
	OffloadMs       float64        `json:"offload_ms"`
	EncoderMs       float64        `json:"encoder_ms"`
	PeakTF          float64        `json:"peak_tf"`
	PeakExact       bool           `json:"peak_exact"`
	Accuracy        string         `json:"accuracy"` // analytical | calibrated
	Topology        string         `json:"topology"`
	TopologyOK      bool           `json:"topology_ok"`
	Trace           []TraceRow     `json:"trace"`
	Workload        *WorkloadStats `json:"workload,omitempty"`
	tPre            float64        // 纯 prefill 耗时 ms（内部复用，不序列化）
	reqSec          float64        // 单请求占用的串行服务预算（内部复用）
}

type TraceRow struct {
	K string `json:"k"`
	V string `json:"v"`
	N string `json:"n,omitempty"` // 注释
}

func tr(k, v, n string) TraceRow { return TraceRow{K: k, V: v, N: n} }

func localText(lang, en, zh string) string {
	if lang == "zh" {
		return zh
	}
	return en
}

func engineDisplay(e Engine, lang string) string {
	if e.ID == "auto" {
		return localText(lang, "Auto", "自动选型")
	}
	return e.Name
}

func engineDescription(e Engine, lang string) string {
	if lang == "zh" {
		return e.Note
	}
	switch e.ID {
	case "auto":
		return "Select the runtime from the quantization format and hardware vendor"
	case "vllm":
		return "PagedAttention and continuous batching; secondary hardware often requires vendor plugins or forks"
	case "sglang":
		return "RadixAttention and PD/EP/DP serving; verify supported combinations against the deployed version"
	case "trtllm":
		return "NVIDIA CUDA inference stack; kernels and features depend on GPU generation and version"
	case "llamacpp":
		return "GGUF runtime across CPU, Metal, CUDA, HIP, and other backends"
	case "mlx":
		return "Apple Silicon unified-memory runtime"
	case "exllama":
		return "EXL3 quantized runtime for NVIDIA GPUs"
	case "lmdeploy":
		return "TurboMind/vLLM backends; only verified NVIDIA paths are listed here"
	case "mindie":
		return "Official Ascend inference engine"
	default:
		return e.Note
	}
}

func quantDescription(q Quant, lang string) string {
	if lang == "zh" {
		return q.Note
	}
	switch q.ID {
	case "fp16":
		return "Baseline precision"
	case "fp8":
		return "FP8 weights and activations on Ada/Hopper or newer; 2× prefill compute path"
	case "int8":
		return "SmoothQuant-style INT8 tensor path on Ampere or newer"
	case "int4":
		return "AWQ/GPTQ/QAT weight-only quantization; saves memory and bandwidth, while kernel support controls speed"
	case "fp4":
		return "Blackwell FP4 pipeline; other GPUs only receive the storage benefit"
	case "mxfp4":
		return "MXFP4 weights; activation precision and unquantized tensors depend on the checkpoint"
	case "q8":
		return "Near-lossless GGUF format across CPU, Metal, and CUDA"
	case "q6":
		return "GGUF quality/size sweet spot"
	case "q4km":
		return "Common GGUF format at roughly 4.85 bits per weight"
	case "iq2":
		return "Extreme 2-bit GGUF format with material accuracy loss"
	case "mlx8":
		return "Native Apple Silicon 8-bit unified-memory format"
	case "mlx4":
		return "Primary native Apple Silicon 4-bit format"
	case "exl3":
		return "ExLlamaV3 format optimized for low-concurrency NVIDIA inference"
	default:
		return q.Note
	}
}

func specDisplay(s SpecMethod, lang string) string {
	switch s.ID {
	case "none":
		return localText(lang, "Off", "关闭")
	case "mtp":
		return localText(lang, "Native MTP heads", "MTP 原生多头")
	case "draft":
		return localText(lang, "Draft model", "草稿模型")
	default:
		return s.Name
	}
}

func specDescription(s SpecMethod, lang string) string {
	if lang == "zh" {
		return s.Note
	}
	switch s.ID {
	case "none":
		return "Standard one-token-at-a-time autoregressive decoding"
	case "mtp":
		return "Requires model metadata for MTP heads; acceptance and gain must be calibrated for the workload"
	case "eagle3":
		return "Requires a compatible trained draft head; built-in coefficients are scenario inputs only"
	case "medusa":
		return "Requires Medusa heads for the target model; built-in coefficients are scenario inputs only"
	case "draft":
		return "Assumes a compatible small model; draft memory defaults to 5% of target weights and gain requires measurements"
	case "lookahead":
		return "N-gram proposal without a draft model; gain depends heavily on repeated code or editing patterns"
	case "dflash":
		return "Requires a compatible block-diffusion draft; paper averages are not production throughput"
	case "dflash2":
		return "Requires a compatible draft checkpoint; model-card results are not production throughput"
	default:
		return s.Note
	}
}

const pcieBW = 25.0 // 无互联时 PCIe4 x16 有效带宽 GB/s

func tpCommMs(h HW, m Model, tokens float64, t topology, o Opts) float64 {
	if t.tp <= 1 {
		return 0
	}
	// 每层 attention/MLP 各一次 AllReduce；ring 每次传 2*(TP-1)/TP 份 payload。
	ring := 2 * float64(t.tp-1) / float64(t.tp)
	return 2 * ring * float64(m.Layers) * tokens * m.Hidden * 2 / 1e9 / o.linkBW(h) * 1000
}

func routerSkew(m Model, tokens float64, ep int, o Opts) float64 {
	skew := o.RouterSkew
	if m.TopK > 0 {
		assignments := math.Max(1, tokens*float64(m.TopK))
		skew = math.Max(skew, float64(ep)/math.Min(float64(ep), assignments))
	}
	return skew
}

func epCommMs(h HW, m Model, tokens float64, t topology, o Opts) float64 {
	if t.ep <= 1 || !m.MoE {
		return 0
	}
	layers := m.MoELayers
	if layers <= 0 {
		layers = m.Layers
	}
	// 每个 MoE 层按 TopK 路由做 dispatch + combine All-to-All。
	routes := float64(max(1, m.TopK))
	traffic := 2 * float64(t.ep-1) / float64(t.ep) * float64(layers) * tokens * routes * m.Hidden * 2 / 1e9
	return traffic * routerSkew(m, tokens, t.ep, o) / o.linkBW(h) * 1000
}

func cpCommMs(h HW, m Model, tokens float64, t topology, o Opts) float64 {
	if t.cp <= 1 {
		return 0
	}
	traffic := 2 * float64(t.cp-1) / float64(t.cp) * float64(m.kvLayers()) * tokens * m.Hidden * 2 / 1e9
	return traffic / o.linkBW(h) * 1000
}

func ppCommMs(h HW, m Model, tokens float64, t topology, o Opts) float64 {
	if t.pp <= 1 {
		return 0
	}
	traffic := float64(t.pp-1) * tokens * m.Hidden * 2 / 1e9
	return traffic / o.linkBW(h) * 1000
}

func commMs(h HW, m Model, tokens float64, t topology, o Opts) float64 {
	return tpCommMs(h, m, tokens, t, o) + epCommMs(h, m, tokens, t, o) +
		cpCommMs(h, m, tokens, t, o) + ppCommMs(h, m, tokens, t, o)
}

func activeWeightRead(m Model, batch int) float64 {
	p := m.weights(batch)
	if p.expertTotal > 0 {
		return p.baseTotal + p.expertRead
	}
	if m.MoE {
		return math.Min(math.Max(0, m.Params-m.EncoderParams), m.Active*float64(batch))
	}
	return math.Max(0, m.Params-m.EncoderParams)
}

// Throughput 计算 decode/prefill 的一阶 roofline 容量；cards 由 TP×PP×EP×CP 分解。
func Throughput(h HW, m Model, q Quant, ctx, batch, cards int, o Opts) Perf {
	o = o.norm()
	q, quantLocked := m.effectiveQuant(q)
	eng := resolveEngine(o.Engine, h, q)
	spec := SpecByID(o.Spec)
	t := o.topologyFor(m, cards)
	kvmf, kvrf := o.kvMemF(h, eng), o.kvReadF(h, eng)
	mem := Memory(h, m, q, ctx, batch, cards, o)
	topologyOK := t.valid
	p := Perf{
		Fit: mem.Fit, Mem: mem, QuantID: q.ID, QuantLocked: quantLocked,
		KVSupported: o.kvSupported(h, eng), Accel: h.Accel(q),
		EngName: engineDisplay(eng, o.Lang), EngOK: eng.EngineOK(h), SpecName: specDisplay(spec, o.Lang),
		PeakTF: h.PeakTF(q), PeakExact: h.peakExact(q),
		Accuracy: "analytical", Topology: t.String(), TopologyOK: topologyOK,
	}
	if o.BWUtil > 0 && o.FlopsUtil > 0 && o.ScheduleMS > 0 && (cards == 1 || o.LinkUtil > 0) {
		p.Accuracy = "calibrated"
	}

	parts := m.weights(batch)
	weightTotal := o.weightGB(m, q)
	bytesPerParam := weightTotal / math.Max(m.Params, 1e-9)
	skew := routerSkew(m, float64(batch), t.ep, o)
	activeW := activeWeightRead(m, batch)
	wGB := activeW * bytesPerParam / float64(t.tp)
	linearActive := math.Max(0, m.Active-m.EncoderParams)
	if parts.expertTotal > 0 {
		wGB = (parts.baseTotal + parts.expertRead*skew/float64(t.ep)) * bytesPerParam / float64(t.tp)
		linearActive = parts.baseTotal + parts.expertActive*skew/float64(t.ep)
	}
	wGB += o.AdapterGB / float64(t.tp)

	kvReadCtx := ctx
	if m.Sparse > 0 && float64(kvReadCtx) > m.Sparse {
		kvReadCtx = int(m.Sparse)
	}
	kvTotalGB := m.KVBytes(kvReadCtx) * float64(batch) / 1e9 * m.kvRankFactor(t.tp) /
		float64(t.cp) * kvrf * o.KVOverhead
	kvGPU := kvTotalGB * (1 - o.KVOffload)
	kvOffload := kvTotalGB * o.KVOffload
	stateGB := 2 * m.StateMB / 1000 * float64(batch) / float64(t.tp)
	eta := o.bwUtil(q, eng, h)
	tHBM := (wGB + kvGPU + stateGB) / (h.BW * eta) * 1000
	tOffload := 0.0
	if kvOffload > 0 {
		tOffload = kvOffload / o.OffloadBW * 1000
	}
	tMem := tHBM + tOffload

	peakTF := h.PeakTF(q)
	flopsUtil := o.flopsUtil(eng)
	attnL := m.kvLayers()
	localL := m.localLayers()
	fullL := attnL - localL
	fullKeys := float64(ctx)
	if m.Sparse > 0 {
		fullKeys = math.Min(fullKeys, m.Sparse)
	}
	localKeys := float64(ctx)
	if localL > 0 {
		localKeys = math.Min(localKeys, float64(m.Window))
	}
	if m.Sparse > 0 {
		localKeys = math.Min(localKeys, m.Sparse)
	}
	decodeF := 2*linearActive*1e9*float64(batch) +
		4*float64(batch)*m.Hidden*(float64(fullL)*fullKeys+float64(localL)*localKeys)/float64(t.cp)
	tCompute := decodeF / (peakTF * 1e12 * float64(t.tp) * flopsUtil) * 1000
	tpComm := tpCommMs(h, m, float64(batch), t, o)
	epComm := epCommMs(h, m, float64(batch), t, o)
	cpComm := cpCommMs(h, m, float64(batch), t, o)
	ppComm := ppCommMs(h, m, float64(batch), t, o)
	tComm := tpComm + epComm + cpComm + ppComm
	tFixed := eng.StepMs * (1 + float64(batch)/eng.SchedK)
	if o.ScheduleMS > 0 {
		tFixed = o.ScheduleMS
	}
	tStep := math.Max(tMem, tCompute) + tComm + tFixed
	p.Bottleneck = "memory"
	if tCompute > tMem {
		p.Bottleneck = "compute"
	} else if tOffload > tHBM && tOffload > tCompute {
		p.Bottleneck = "offload"
	}
	p.DecodeMemMs = round2(tMem)
	p.DecodeComputeMs = round2(tCompute)
	p.CommMs = round2(tComm)
	p.ScheduleMs = round2(tFixed)
	p.OffloadMs = round2(tOffload)

	specScenario := spec
	if o.SpecTau > 0 {
		specScenario.Tau = o.SpecTau
	}
	if o.SpecOvh > 0 {
		specScenario.Ovh = o.SpecOvh
	}
	g := specScenario.gain(batch)
	modelSpecOK := spec.ID != "mtp" || m.MTP || m.MTPHeads > 0
	specCalibrated := o.SpecTau > 0 && o.SpecOvh > 0
	specOK := spec.ID == "none" || (modelSpecOK && specCalibrated)
	if !specOK {
		g = 1
	}
	p.SpecApplied = spec.ID != "none" && specOK
	single := 1000 / tStep * g
	agg := float64(batch) * 1000 / tStep * g

	// Prefix 命中 P、待算 token N 时，dense causal QK+AV =
	// 2*L*D*N*(2*(P+N)-N)。CP 在 token 维度分摊 prefill。
	inEff := float64(ctx) * (1 - o.HitRate)
	preTokens := inEff / float64(t.cp)
	preSkew := routerSkew(m, math.Max(1, preTokens), t.ep, o)
	preActive := math.Max(0, m.Active-m.EncoderParams)
	preParts := m.weights(max(1, int(math.Min(inEff, 1e6))))
	preRead := activeWeightRead(m, max(1, int(math.Min(inEff, 1e6))))
	if preParts.expertTotal > 0 {
		preActive = preParts.baseTotal + preParts.expertActive*preSkew/float64(t.ep)
		preRead = preParts.baseTotal + preParts.expertRead*preSkew/float64(t.ep)
	}
	tLinCompute := 2 * preActive * preTokens / (peakTF * float64(t.tp) * flopsUtil)
	preWeightGB := preRead*bytesPerParam/float64(t.tp) + o.AdapterGB/float64(t.tp)
	tWeight := preWeightGB / (h.BW * eta) * 1000
	chunks := math.Max(1, math.Ceil(inEff/float64(o.PrefillChunk)))
	tLin := math.Max(tLinCompute, tWeight*chunks)

	var attnF float64
	if fullL > 0 {
		if m.Sparse > 0 {
			attnF += 4 * float64(fullL) * inEff * math.Min(float64(ctx), m.Sparse) * m.Hidden
		} else {
			attnF += 2 * float64(fullL) * inEff * (2*float64(ctx) - inEff) * m.Hidden
		}
	}
	if localL > 0 {
		window := math.Min(float64(ctx), float64(m.Window))
		if m.Sparse > 0 {
			window = math.Min(window, m.Sparse)
		}
		prefix := float64(ctx) - inEff
		ramp := math.Min(inEff, math.Max(0, window-prefix))
		keyPairs := ramp*(2*prefix+ramp)/2 + (inEff-ramp)*window
		attnF += 4 * float64(localL) * keyPairs * m.Hidden
	}
	tAttn := attnF / float64(t.cp) / (peakTF * 1e12 * float64(t.tp) * flopsUtil) * 1000
	tPreComm := commMs(h, m, preTokens, t, o)
	kvWriteGB := m.KVTokBytes() * inEff / 1e9 * m.kvRankFactor(t.tp) / float64(t.cp) * kvmf * o.KVOverhead
	tKVWrite := kvWriteGB * (1 - o.KVOffload) / (h.BW * eta) * 1000
	if o.KVOffload > 0 {
		tKVWrite += kvWriteGB * o.KVOffload / o.OffloadBW * 1000
	}
	tEncoder := 0.0
	if m.EncoderParams > 0 && o.MediaTokens > 0 {
		encoderGB := weightTotal * m.EncoderParams / math.Max(m.Params, 1e-9) / float64(t.tp)
		encoderCompute := 2 * m.EncoderParams * float64(o.MediaTokens) / (peakTF * float64(t.tp) * flopsUtil)
		encoderRead := encoderGB / (h.BW * eta) * 1000
		tEncoder = math.Max(encoderCompute, encoderRead)
	}
	p.EncoderMs = round2(tEncoder)
	preSchedule := chunks * eng.StepMs
	tPre := tLin + tAttn + tPreComm + tKVWrite + tEncoder + preSchedule

	p.SingleTPS = round1(single)
	p.AggTPS = round1(agg)
	p.TPM = round1(agg * 60)
	if tPre > 0 {
		p.PreTPS = round1(inEff / tPre * 1000)
	}
	decodeTokens := math.Max(0, float64(o.OutLen-1))
	if agg > 0 {
		p.reqSec = decodeTokens/agg + tPre/1000
		reqS := 1 / p.reqSec
		p.ReqS = round4(reqS)
		p.TPMMixed = round1(reqS * (float64(ctx) + float64(o.OutLen)) * 60)
	}
	p.TPOTms = round1(tStep / g)
	p.TTFTms = round1(tPre)
	p.ReqMs = round1(tPre + decodeTokens*tStep/g)
	p.tPre = tPre
	kvScale := m.kvRankFactor(t.tp) / float64(t.pp*t.cp) * kvmf * o.KVOverhead * (1 - o.KVOffload) / 1e9
	kvOne := m.KVBatchBytes(ctx, 1, o.HitRate) * kvScale
	kvPerReq := (m.KVBatchBytes(ctx, 2, o.HitRate)-m.KVBatchBytes(ctx, 1, o.HitRate))*kvScale +
		m.StateMB/1000/float64(t.tp*t.pp)
	actPerReq := mem.Act / float64(max(1, batch))
	fixed := mem.Weights + mem.Fw + mem.Adapter + math.Max(0, kvOne-kvPerReq)
	if perRequest := kvPerReq + actPerReq; perRequest > 0 {
		p.MaxBatch = max(0, int((mem.Budget-fixed)/perRequest))
	}

	if o.skipTrace {
		return p
	}

	peakLabel := localText(o.Lang, "vendor per-precision peak", "厂商逐精度峰值")
	if !p.PeakExact {
		peakLabel = localText(o.Lang, "estimated from FP16 peak and architecture ratio", "由 FP16 峰值和架构倍率估算")
	}
	weightNote := localText(o.Lang,
		fmt.Sprintf("%gB parameters × %.2f B/param (%s)", m.Params, q.Bytes, q.Name),
		fmt.Sprintf("%gB 参数 × %.2f B/param（%s）", m.Params, q.Bytes, q.Name))
	if o.WeightGB > 0 {
		weightNote = localText(o.Lang, "Using the measured loaded weight size", "使用用户输入的实际加载权重")
	} else if m.CheckpointGB > 0 && m.NativeQuant == q.ID {
		weightNote = localText(o.Lang, "Using the HF safetensors payload matching the native quantization", "使用 HF safetensors payload（匹配原生量化）")
	}
	p.Trace = []TraceRow{
		tr(localText(o.Lang, "Estimate status", "估算级别"), p.Accuracy,
			localText(o.Lang, "analytical is an uncalibrated roofline; calibrated means key measured utilization inputs were supplied", "analytical 为未校准 roofline；calibrated 表示已提供关键实测利用率")),
		tr(localText(o.Lang, "Parallel topology", "并行拓扑"), p.Topology,
			localText(o.Lang, fmt.Sprintf("%d cards; the product must match", cards), fmt.Sprintf("%d cards；乘积必须相等", cards))),
		tr(localText(o.Lang, "Inference engine", "推理框架"), engineDisplay(eng, o.Lang), engNote(eng, h, p.EngOK, p.Accuracy == "calibrated", o.Lang)),
		tr(localText(o.Lang, "Quantization path", "量化路径"), fmt.Sprintf("W%s · A%s · KV %s", q.W, q.A, strings.ToUpper(o.KVQuant)), quantNote(h, q, eng, peakTF/h.TF, o.Lang)),
		tr(localText(o.Lang, "Weight memory", "权重显存"), gb(mem.Weights), weightNote),
		tr(localText(o.Lang, "KV / state", "KV / 状态"), gb(mem.KV), kvNote(m, ctx, batch, t, o, kvrf)),
		tr(localText(o.Lang, "Per-card budget", "单卡预算"), fmt.Sprintf("%.1f / %.1f GB", mem.Budget, mem.Cap), capNote(h, eng, o)),
		tr(localText(o.Lang, "Decode-step reads", "decode 单步访存"), fmt.Sprintf("%.2f GB", wGB+kvGPU+stateGB), moeNote(m, activeW, o.Lang)+sparseReadNote(m, ctx, o.Lang)),
		tr("decode roofline", fmt.Sprintf("%.2f ms", math.Max(tMem, tCompute)),
			localText(o.Lang,
				fmt.Sprintf("max(memory %.2fms, compute %.2fms); peak %.0f TF (%s)", tMem, tCompute, peakTF, peakLabel),
				fmt.Sprintf("max(访存 %.2fms, 计算 %.2fms)；峰值 %.0f TF（%s）", tMem, tCompute, peakTF, peakLabel))),
		tr(localText(o.Lang, "Effective bandwidth", "有效带宽"), fmt.Sprintf("%.0f GB/s", h.BW*eta),
			localText(o.Lang, fmt.Sprintf("rated %.0f × η%.2f", h.BW, eta), fmt.Sprintf("标称 %.0f × η%.2f", h.BW, eta))),
		tr(localText(o.Lang, "Communication", "通信耗时"), fmt.Sprintf("%.2f ms", tComm),
			localText(o.Lang,
				fmt.Sprintf("%s; TP %.2f + EP %.2f + CP %.2f + PP %.2f ms", commNote(h, t, o), tpComm, epComm, cpComm, ppComm),
				fmt.Sprintf("%s；TP %.2f + EP %.2f + CP %.2f + PP %.2f ms", commNote(h, t, o), tpComm, epComm, cpComm, ppComm))),
		tr(localText(o.Lang, "Step time", "单步耗时"), fmt.Sprintf("%.2f ms", tStep),
			localText(o.Lang, fmt.Sprintf("roofline + communication + %.2fms scheduling (%s)", tFixed, eng.Name), fmt.Sprintf("roofline + 通信 + 调度 %.2fms（%s）", tFixed, eng.Name))),
	}
	if o.KVOffload > 0 {
		p.Trace = append(p.Trace, tr("KV offload", fmt.Sprintf("%.2f ms/step", tOffload),
			localText(o.Lang,
				fmt.Sprintf("%.0f%% of KV reread through a %.0f GB/s tier; %.1f GB external capacity", o.KVOffload*100, o.OffloadBW, mem.OffloadedKV),
				fmt.Sprintf("%.0f%% KV 经 %.0f GB/s 层级回读；外部容量 %.1f GB", o.KVOffload*100, o.OffloadBW, mem.OffloadedKV))))
	}
	if spec.ID != "none" {
		note := specNote(specScenario, batch, o.Lang)
		if !modelSpecOK {
			note = localText(o.Lang, "⚠ The model has no MTP-head metadata; acceleration is not applied", "⚠ 模型没有 MTP 头元数据，本次不应用加速")
		} else if !specCalibrated {
			note = localText(o.Lang, "⚠ Measured accepted tokens τ and draft/verify overhead are both required; acceleration is not applied", "⚠ 未同时填写实测接受 token τ 与 draft/verify 开销，本次不应用加速")
		}
		p.Trace = append(p.Trace,
			tr(localText(o.Lang, "Speculative decoding", "推测解码"), fmt.Sprintf("%s ×%.2f", specDisplay(spec, o.Lang), g), note),
			tr(localText(o.Lang, "Effective TPOT", "有效 TPOT"), fmt.Sprintf("%.2f ms", tStep/g),
				localText(o.Lang, fmt.Sprintf("%.2fms per step ÷ %.2f× scenario gain", tStep, g), fmt.Sprintf("单步 %.2fms ÷ 场景增益 ×%.2f", tStep, g))),
		)
	}
	p.Trace = append(p.Trace,
		tr(localText(o.Lang, "Prefill time", "prefill 耗时"), fmt.Sprintf("%.0f ms", tPre), prefillNote(m, ctx, o, tLin, tAttn, tPreComm)),
		tr(localText(o.Lang, "Prefill chunks", "prefill 分块"), fmt.Sprintf("%.0f × %d", float64(o.PrefillChunk), int(chunks)),
			localText(o.Lang, fmt.Sprintf("KV writes %.0fms; encoder %.0fms", tKVWrite, tEncoder), fmt.Sprintf("KV 写入 %.0fms；encoder %.0fms", tKVWrite, tEncoder))),
		tr(localText(o.Lang, "Prefill throughput", "prefill 速度"), fmt.Sprintf("%.0f tok/s", p.PreTPS),
			localText(o.Lang, fmt.Sprintf("%d uncached input tokens ÷ %.0f ms", int(inEff), tPre), fmt.Sprintf("未命中输入 %d tok ÷ %.0f ms", int(inEff), tPre))),
		tr(localText(o.Lang, "Request latency", "请求时延"), fmt.Sprintf("%.0f ms", p.ReqMs),
			localText(o.Lang, fmt.Sprintf("TTFT + %d subsequent tokens × TPOT", int(decodeTokens)), fmt.Sprintf("TTFT + %d 个后续 token × TPOT", int(decodeTokens)))),
		tr(localText(o.Lang, "Steady-state rate", "稳态速率"), fmt.Sprintf("%.2f req/s", p.ReqS),
			localText(o.Lang, fmt.Sprintf("decode budget %.0f/%.0f + %.2fs prefill budget", decodeTokens, agg, tPre/1000), fmt.Sprintf("后续 decode 预算 %.0f/%.0f + prefill 预算 %.2fs", decodeTokens, agg, tPre/1000))),
		tr(localText(o.Lang, "Mixed TPM", "混合 TPM"), fmt.Sprintf("%.0f tok/min", p.TPMMixed),
			localText(o.Lang, fmt.Sprintf("%.2f req/s × (%d raw input + %d output) × 60", p.ReqS, ctx, o.OutLen), fmt.Sprintf("%.2f req/s ×（%d 原始输入 + %d 输出）× 60", p.ReqS, ctx, o.OutLen))),
	)
	if !topologyOK {
		p.Trace = append(p.Trace, tr(localText(o.Lang, "⚠ Invalid topology", "⚠ 拓扑无效"), p.Topology,
			localText(o.Lang, "TP×PP×EP×CP must equal cards, and EP only applies to MoE; reverted to full TP", "TP×PP×EP×CP 必须等于 cards，且 EP 仅适用于 MoE；已回退全 TP")))
	}
	if m.Multimodal && m.EncoderParams == 0 {
		p.Trace = append(p.Trace, tr(localText(o.Lang, "⚠ Multimodal encoder", "⚠ 多模态 encoder"), localText(o.Lang, "Unknown parameter count", "参数量未知"),
			localText(o.Lang, "The text tower is modeled; media encoder TTFT is omitted", "文本塔可计算；媒体 encoder TTFT 未计入")))
	}
	if m.Ctx > 0 && ctx > m.Ctx {
		p.Trace = append(p.Trace, tr(localText(o.Lang, "⚠ Context extension", "⚠ 上下文外推"),
			localText(o.Lang, fmt.Sprintf("%dK > %dK native", ctx/1024, m.Ctx/1024), fmt.Sprintf("%dK > 原生 %dK", ctx/1024, m.Ctx/1024)),
			localText(o.Lang, "Requires YaRN / RoPE extension; long-context accuracy and stability may degrade", "需 YaRN / RoPE 外推，长文精度与稳定性可能下降")))
	}
	return p
}

type workloadRun struct {
	bucket    WorkloadBucket
	perf      Perf
	mem1      MemDetail
	mem2      MemDetail
	occupancy float64
}

func normalizeWorkload(workload []WorkloadBucket) []WorkloadBucket {
	out := make([]WorkloadBucket, 0, len(workload))
	total := 0.0
	for _, b := range workload {
		if b.Share <= 0 || b.Context <= 0 || b.Output <= 0 {
			continue
		}
		b.PrefixHit = clamp(b.PrefixHit, 0, 0.9)
		out = append(out, b)
		total += b.Share
	}
	if total <= 0 {
		return nil
	}
	for i := range out {
		out[i].Share /= total
	}
	return out
}

func workloadQuantile(runs []workloadRun, q float64, value func(workloadRun) float64) workloadRun {
	ordered := append([]workloadRun(nil), runs...)
	sort.Slice(ordered, func(i, j int) bool { return value(ordered[i]) < value(ordered[j]) })
	cumulative := 0.0
	for _, run := range ordered {
		cumulative += run.bucket.Share
		if cumulative >= q {
			return run
		}
	}
	return ordered[len(ordered)-1]
}

// ThroughputWorkload 按请求到达占比聚合各桶服务需求，并按请求驻留时间
// 重加权显存占用。P99.9 并发显存使用正态矩近似，同时保留单请求尾桶保护。
func ThroughputWorkload(h HW, m Model, q Quant, workload []WorkloadBucket, batch, cards int, o Opts) Perf {
	workload = normalizeWorkload(workload)
	if len(workload) == 0 {
		return Perf{}
	}
	batch = max(1, batch)
	runs := make([]workloadRun, len(workload))
	residenceTotal := 0.0
	for i, b := range workload {
		bo := o
		bo.skipTrace = o.skipTrace || i > 0
		bo.OutLen, bo.HitRate = b.Output, b.PrefixHit
		perf := Throughput(h, m, q, b.Context, batch, cards, bo)
		runs[i] = workloadRun{
			bucket: b,
			perf:   perf,
			mem1:   Memory(h, m, q, b.Context, 1, cards, bo),
			mem2:   Memory(h, m, q, b.Context, 2, cards, bo),
		}
		residenceTotal += b.Share * math.Max(perf.ReqMs, 0.1)
	}
	for i := range runs {
		runs[i].occupancy = runs[i].bucket.Share * math.Max(runs[i].perf.ReqMs, 0.1) / residenceTotal
	}

	p := runs[0].perf
	p.Workload = &WorkloadStats{}
	if !o.skipTrace {
		p.Workload.Buckets = make([]WorkloadBucketPerf, len(runs))
	}
	var meanCtx, meanOut, effectiveInput, preSeconds, reqSeconds, reqLatency, ttft float64
	var decodeTokens, singleSeconds, aggregateSeconds, decodeMem, decodeCompute, comm, schedule, offload, encoder float64
	maxProfileContext := 0
	for _, run := range runs {
		share := run.bucket.Share
		outTokens := math.Max(1, float64(run.bucket.Output-1))
		meanCtx += share * float64(run.bucket.Context)
		meanOut += share * float64(run.bucket.Output)
		maxProfileContext = max(maxProfileContext, run.bucket.Context)
		effectiveInput += share * float64(run.bucket.Context) * (1 - run.bucket.PrefixHit)
		preSeconds += share * run.perf.tPre / 1000
		reqSeconds += share * run.perf.reqSec
		reqLatency += share * run.perf.ReqMs
		ttft += share * run.perf.TTFTms
		decodeTokens += share * outTokens
		singleSeconds += share * outTokens / math.Max(run.perf.SingleTPS, 1e-9)
		aggregateSeconds += share * outTokens / math.Max(run.perf.AggTPS, 1e-9)
		decodeMem += share * outTokens * run.perf.DecodeMemMs
		decodeCompute += share * outTokens * run.perf.DecodeComputeMs
		comm += share * outTokens * run.perf.CommMs
		schedule += share * outTokens * run.perf.ScheduleMs
		offload += share * outTokens * run.perf.OffloadMs
		encoder += share * run.perf.EncoderMs
	}
	p.SingleTPS = round1(decodeTokens / math.Max(singleSeconds, 1e-9))
	p.AggTPS = round1(decodeTokens / math.Max(aggregateSeconds, 1e-9))
	p.PreTPS = round1(effectiveInput / math.Max(preSeconds, 1e-9))
	p.reqSec = reqSeconds
	reqRate := 1 / math.Max(reqSeconds, 1e-9)
	p.ReqS = round4(reqRate)
	p.TPM = round1(p.AggTPS * 60)
	p.TPMMixed = round1(reqRate * (meanCtx + meanOut) * 60)
	p.TTFTms = round1(ttft)
	p.TPOTms = round1(singleSeconds / math.Max(decodeTokens, 1e-9) * 1000)
	p.ReqMs = round1(reqLatency)
	p.tPre = preSeconds * 1000
	p.DecodeMemMs = round2(decodeMem / math.Max(decodeTokens, 1e-9))
	p.DecodeComputeMs = round2(decodeCompute / math.Max(decodeTokens, 1e-9))
	p.CommMs = round2(comm / math.Max(decodeTokens, 1e-9))
	p.ScheduleMs = round2(schedule / math.Max(decodeTokens, 1e-9))
	p.OffloadMs = round2(offload / math.Max(decodeTokens, 1e-9))
	p.EncoderMs = round2(encoder)
	p.Bottleneck = "memory"
	if p.DecodeComputeMs > p.DecodeMemMs {
		p.Bottleneck = "compute"
	} else if p.OffloadMs > p.DecodeMemMs-p.OffloadMs && p.OffloadMs > p.DecodeComputeMs {
		p.Bottleneck = "offload"
	}

	mix := MemDetail{Cap: runs[0].mem1.Cap, Budget: runs[0].mem1.Budget}
	var meanFirst, meanInc, secondFirst, secondInc, maxFirst, maxInc float64
	allOneFit := true
	for _, run := range runs {
		w := run.occupancy
		inc := math.Max(0, run.mem2.Total-run.mem1.Total)
		meanFirst += w * run.mem1.Total
		meanInc += w * inc
		secondFirst += w * run.mem1.Total * run.mem1.Total
		secondInc += w * inc * inc
		maxFirst = math.Max(maxFirst, run.mem1.Total)
		maxInc = math.Max(maxInc, inc)
		allOneFit = allOneFit && run.mem1.Fit
		scale := float64(batch - 1)
		mix.Weights += w * (run.mem1.Weights + scale*(run.mem2.Weights-run.mem1.Weights))
		mix.KV += w * (run.mem1.KV + scale*(run.mem2.KV-run.mem1.KV))
		mix.Fw += w * (run.mem1.Fw + scale*(run.mem2.Fw-run.mem1.Fw))
		mix.Act += w * (run.mem1.Act + scale*(run.mem2.Act-run.mem1.Act))
		mix.Adapter += w * (run.mem1.Adapter + scale*(run.mem2.Adapter-run.mem1.Adapter))
		mix.OffloadedKV += w * (run.mem1.OffloadedKV + scale*(run.mem2.OffloadedKV-run.mem1.OffloadedKV))
		mix.Sys += w * (run.mem1.Sys + scale*(run.mem2.Sys-run.mem1.Sys))
	}
	varFirst := math.Max(0, secondFirst-meanFirst*meanFirst)
	varInc := math.Max(0, secondInc-meanInc*meanInc)
	guardAt := func(b int) float64 {
		if b <= 0 {
			return 0
		}
		scale := float64(b - 1)
		mean := meanFirst + scale*meanInc
		guard := mean + 3.090232*math.Sqrt(varFirst+scale*varInc)
		return math.Min(guard, maxFirst+scale*maxInc)
	}
	mix.Total = meanFirst + float64(batch-1)*meanInc
	mix.P999Total = guardAt(batch)
	mix.HeadPct = (mix.Cap - mix.P999Total) / mix.Cap
	mix.Fit = allOneFit && mix.P999Total <= mix.Cap
	p.Mem, p.Fit = mix, mix.Fit
	if allOneFit {
		lo, hi := 0, 4096
		for lo < hi {
			mid := (lo + hi + 1) / 2
			if guardAt(mid) <= mix.Cap {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		p.MaxBatch = lo
	} else {
		p.MaxBatch = 0
	}

	ctx95 := workloadQuantile(runs, .95, func(r workloadRun) float64 { return float64(r.bucket.Context) })
	ctx99 := workloadQuantile(runs, .99, func(r workloadRun) float64 { return float64(r.bucket.Context) })
	ctx999 := workloadQuantile(runs, .999, func(r workloadRun) float64 { return float64(r.bucket.Context) })
	lat95 := workloadQuantile(runs, .95, func(r workloadRun) float64 { return r.perf.ReqMs })
	lat99 := workloadQuantile(runs, .99, func(r workloadRun) float64 { return r.perf.ReqMs })
	lat999 := workloadQuantile(runs, .999, func(r workloadRun) float64 { return r.perf.ReqMs })
	stats := p.Workload
	stats.MeanContext, stats.MeanOutput = round1(meanCtx), round1(meanOut)
	stats.P95Context, stats.P99Context, stats.P999Context, stats.MaxContext = ctx95.bucket.Context, ctx99.bucket.Context, ctx999.bucket.Context, maxProfileContext
	stats.P95TTFTms, stats.P99TTFTms, stats.P999TTFTms = lat95.perf.TTFTms, lat99.perf.TTFTms, lat999.perf.TTFTms
	stats.P95ReqMs, stats.P99ReqMs, stats.P999ReqMs = lat95.perf.ReqMs, lat99.perf.ReqMs, lat999.perf.ReqMs
	stats.P95SingleTPS = lat95.perf.SingleTPS
	if !o.skipTrace {
		for i, run := range runs {
			stats.Buckets[i] = WorkloadBucketPerf{
				Context: run.bucket.Context, Output: run.bucket.Output, Share: run.bucket.Share,
				Occupancy: run.occupancy, PrefixHit: run.bucket.PrefixHit, SingleTPS: run.perf.SingleTPS,
				TTFTms: run.perf.TTFTms, TPOTms: run.perf.TPOTms, ReqMs: run.perf.ReqMs,
				BatchMemory: run.perf.Mem.Total, Fit: run.perf.Fit,
			}
		}
	}
	if o.skipTrace {
		return p
	}

	metadata := min(4, len(p.Trace))
	p.Trace = append([]TraceRow(nil), p.Trace[:metadata]...)
	p.Trace = append(p.Trace,
		tr(localText(o.Lang, "Workload distribution", "工作负载分布"), fmt.Sprintf("%d buckets", len(runs)),
			localText(o.Lang,
				fmt.Sprintf("request-arrival weighted; mean %.0f input + %.0f output tokens", meanCtx, meanOut),
				fmt.Sprintf("按请求到达占比加权；平均输入 %.0f + 输出 %.0f token", meanCtx, meanOut))),
		tr(localText(o.Lang, "Context percentiles", "上下文分位"), fmt.Sprintf("P95 %dK · P99 %dK · P99.9 %dK · max %dK", stats.P95Context/1024, stats.P99Context/1024, stats.P999Context/1024, stats.MaxContext/1024),
			localText(o.Lang, "percentiles use request-arrival share; max preserves the rarest configured tail", "分位数按请求到达占比统计；max 保留配置中最稀有的尾部")),
		tr(localText(o.Lang, "Concurrent memory", "并发显存"), fmt.Sprintf("mean %.1f · P99.9 %.1f GB", mix.Total, mix.P999Total),
			localText(o.Lang, "occupancy weighted by request latency; P99.9 uses a normal moment approximation", "按请求时延重加权驻留占比；P99.9 使用正态矩近似")),
		tr(localText(o.Lang, "TTFT distribution", "TTFT 分布"), fmt.Sprintf("mean %.0f · P95 %.0f · P99 %.0f ms", p.TTFTms, stats.P95TTFTms, stats.P99TTFTms),
			localText(o.Lang, "each bucket is evaluated by the same prefill roofline", "每个桶使用同一 prefill roofline 独立计算")),
		tr(localText(o.Lang, "Request latency distribution", "请求时延分布"), fmt.Sprintf("mean %.0f · P95 %.0f · P99 %.0f ms", p.ReqMs, stats.P95ReqMs, stats.P99ReqMs),
			localText(o.Lang, "TTFT + bucket output length × TPOT", "TTFT + 各桶输出长度 × TPOT")),
		tr(localText(o.Lang, "Steady-state mixed rate", "混合稳态速率"), fmt.Sprintf("%.2f req/s · %.0f tok/min", p.ReqS, p.TPMMixed),
			localText(o.Lang, "harmonic aggregation of per-bucket serial service demand", "按各桶串行服务预算做调和聚合")),
	)
	return p
}

func engNote(eng Engine, h HW, ok, calibrated bool, lang string) string {
	if !ok {
		return localText(lang,
			fmt.Sprintf("⚠ %s does not list native %s support; performance is a generic baseline", eng.Name, h.Vendor),
			fmt.Sprintf("⚠ %s 未列出 %s 原生支持；性能数字仅为通用基线", eng.Name, h.Vendor))
	}
	if calibrated {
		return engineDescription(eng, lang) + localText(lang, "; using measurements from this deployment", "；使用当前部署实测校准参数")
	}
	return engineDescription(eng, lang) + localText(lang, "; performance coefficients are not calibrated against a matching benchmark", "；性能系数未做同条件基准校准")
}

// quantNote 量化路径说明：框架错配与 prefill 算力倍率。
func quantNote(h HW, q Quant, eng Engine, fmul float64, lang string) string {
	s := quantDescription(q, lang)
	switch q.Fam {
	case "gguf":
		if eng.ID != "llamacpp" {
			s = localText(lang, "⚠ GGUF support is limited in "+eng.Name+"; use llama.cpp; ", "⚠ GGUF 在 "+eng.Name+" 下支持有限，建议 llama.cpp；") + s
		}
	case "mlx":
		if eng.ID != "mlx" {
			s = localText(lang, "⚠ MLX quantization requires MLX on Apple Silicon; ", "⚠ MLX 量化需 MLX 框架（Apple Silicon）；") + s
		}
	case "exl":
		if eng.ID != "exllama" {
			s = localText(lang, "⚠ EXL3 requires ExLlamaV3; ", "⚠ EXL3 需 ExLlamaV3 框架；") + s
		}
	}
	if fmul > 1 {
		s += localText(lang, fmt.Sprintf("; prefill compute ×%.0f", fmul), fmt.Sprintf("；prefill 算力 ×%.0f", fmul))
	} else if q.Mul > 1 {
		s += localText(lang, "; this device has no "+strings.ToUpper(q.Need)+" path, so prefill is not accelerated", "；该卡无 "+strings.ToUpper(q.Need)+" 路径，prefill 不加速")
	}
	return s
}

func kvNote(m Model, ctx, batch int, t topology, o Opts, readF float64) string {
	rank := m.kvRankFactor(t.tp) / float64(t.pp*t.cp)
	base := localText(o.Lang,
		fmt.Sprintf("%.1f MB raw KV/request × %d concurrent × %.3f rank ratio", m.KVBytes(ctx)/1e6, batch, rank),
		fmt.Sprintf("%.1f MB/请求原始 KV × %d 并发 × rank比例 %.3f", m.KVBytes(ctx)/1e6, batch, rank))
	if o.HitRate > 0 {
		base += localText(o.Lang, fmt.Sprintf("; blocks in the %.0f%% shared prefix reside once", o.HitRate*100), fmt.Sprintf("；共享前缀 %.0f%% 的 block 仅驻留一份", o.HitRate*100))
	}
	if local := m.localLayers(); local > 0 {
		base += localText(o.Lang,
			fmt.Sprintf(" (%d full + %d local@%d)", m.kvLayers()-local, local, m.Window),
			fmt.Sprintf("（%d full + %d local@%d）", m.kvLayers()-local, local, m.Window))
	}
	if m.StateMB > 0 {
		base += localText(o.Lang, fmt.Sprintf(" + %.1f MB recurrent state/request", m.StateMB), fmt.Sprintf(" + %.1f MB/请求 recurrent state", m.StateMB))
	}
	if readF != 1 {
		switch o.KVQuant {
		case "fp8":
			base += localText(o.Lang, "; per-token KV capacity ×0.50", "；逐 token KV 容量 ×0.50")
		case "fp4":
			base += localText(o.Lang, "; per-token KV capacity ×0.281 (experimental SGLang block16 format)", "；逐 token KV 容量 ×0.281（SGLang block16 实验格式）")
		}
	}
	if o.KVOverhead != 1 {
		base += localText(o.Lang, fmt.Sprintf("; allocator ×%.2f", o.KVOverhead), fmt.Sprintf("；allocator ×%.2f", o.KVOverhead))
	}
	if o.KVOffload > 0 {
		base += localText(o.Lang, fmt.Sprintf("; %.0f%% remains on GPU", (1-o.KVOffload)*100), fmt.Sprintf("；GPU 保留 %.0f%%", (1-o.KVOffload)*100))
	}
	if o.KVQuant != "fp16" && readF == 1 {
		base += localText(o.Lang, "; ⚠ this hardware/engine does not support the KV format; capacity and reads use FP16", "；⚠ 当前硬件/引擎不支持该 KV 格式，容量与读取均按 FP16")
	}
	return base
}

func specNote(spec SpecMethod, batch int, lang string) string {
	s := specDescription(spec, lang)
	if batch > 1 {
		g1 := spec.Tau / (1 + spec.Ovh)
		s += localText(lang,
			fmt.Sprintf("; %.2f× gain at b=%d (%.2f× single-stream peak)", spec.gain(batch), batch, g1),
			fmt.Sprintf("；b=%d 时增益 %.2f×（单流峰值 %.2f×）", batch, spec.gain(batch), g1))
	}
	return s
}

func prefillNote(m Model, ctx int, o Opts, tLin, tAttn, tComm float64) string {
	s := ""
	if o.HitRate > 0 {
		s = localText(o.Lang,
			fmt.Sprintf("%.0f%% prefix-token hit → recompute %d tok; ", o.HitRate*100, int(float64(ctx)*(1-o.HitRate))),
			fmt.Sprintf("前缀 token 命中 %.0f%% → 重算 %d tok；", o.HitRate*100, int(float64(ctx)*(1-o.HitRate))))
	}
	s += localText(o.Lang,
		fmt.Sprintf("linear roof %.0fms + attention %.0fms + communication %.0fms", tLin, tAttn, tComm),
		fmt.Sprintf("linear roof %.0fms + attention %.0fms + 通信 %.0fms", tLin, tAttn, tComm))
	if m.Sparse > 0 {
		s += localText(o.Lang, "; sparse attention reduced by the selection budget", "；稀疏 attention 已按选择预算折减")
	}
	if m.LocalLayers > 0 && m.Window > 0 {
		s += localText(o.Lang,
			fmt.Sprintf("; %d local-attention layers use a %d-token window", m.LocalLayers, m.Window),
			fmt.Sprintf("；%d 个 local attention 层按 %d-token window", m.LocalLayers, m.Window))
	}
	return s
}

func moeNote(m Model, activeW float64, lang string) string {
	if !m.MoE {
		return localText(lang, "dense: read all weights per step", "dense：每步读全部权重")
	}
	if m.Experts > m.TopK && m.TopK > 0 {
		return localText(lang,
			fmt.Sprintf("MoE: expected unique experts under %d/%d routing; read %.1fB", m.TopK, m.Experts, activeW),
			fmt.Sprintf("MoE：按 %d/%d 路由的期望去重专家估算，读取 %.1fB", m.TopK, m.Experts, activeW))
	}
	return localText(lang,
		fmt.Sprintf("MoE: missing expert metadata; conservative min(total, active×batch) bound reads %.1fB", activeW),
		fmt.Sprintf("MoE：缺专家元数据，按 min(total, active×batch) 保守上界，读取 %.1fB", activeW))
}

func sparseReadNote(m Model, ctx int, lang string) string {
	if m.Sparse <= 0 || m.Sparse >= float64(ctx) {
		return ""
	}
	return localText(lang,
		fmt.Sprintf("; DSA sparse read %dK/%dK", int(m.Sparse/1024), ctx/1024),
		fmt.Sprintf("；DSA 稀疏读取 %dK/%dK", int(m.Sparse/1024), ctx/1024))
}

func capNote(h HW, eng Engine, o Opts) string {
	if o.MemUtil > 0 {
		return localText(o.Lang, fmt.Sprintf("%.0fG × %.2f user-configured executor budget", h.VRAM, o.MemUtil), fmt.Sprintf("%.0fG × %.2f（用户配置的执行器预算）", h.VRAM, o.MemUtil))
	}
	if h.Unified {
		return localText(o.Lang, fmt.Sprintf("%.0fG unified memory × 0.70", h.VRAM), fmt.Sprintf("统一内存 %.0fG × 0.70", h.VRAM))
	}
	if eng.ID == "llamacpp" || eng.ID == "mlx" || eng.ID == "exllama" {
		return localText(o.Lang, fmt.Sprintf("%.0fG × 0.95 local-runtime budget", h.VRAM), fmt.Sprintf("%.0fG × 0.95（本地轻量运行时预算）", h.VRAM))
	}
	return localText(o.Lang, fmt.Sprintf("%.0fG × 0.90 %s serving budget", h.VRAM, eng.Name), fmt.Sprintf("%.0fG × 0.90（%s 服务预算）", h.VRAM, eng.Name))
}

func commNote(h HW, t topology, o Opts) string {
	if t.tp*t.pp*t.ep*t.cp <= 1 {
		return localText(o.Lang, "single card; no communication", "单卡无通信")
	}
	path := fmt.Sprintf("%s %.0f GB/s", h.Link.T, o.linkBW(h))
	if h.Link.B <= 0 {
		path = fmt.Sprintf("PCIe ~%.0f GB/s", o.linkBW(h))
	}
	return t.String() + localText(o.Lang, " over ", " 走 ") + path
}

func gb(v float64) string             { return fmt.Sprintf("%.1f GB", v) }
func clamp(v, lo, hi float64) float64 { return math.Min(hi, math.Max(lo, v)) }
func round1(v float64) float64 {
	return math.Round(v*10) / 10
}
func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
func round4(v float64) float64 {
	return math.Round(v*10000) / 10000
}

// ---------- 模式 1：能装什么 ----------

type FitCell struct {
	Quant      string  `json:"quant"`
	Fit        int     `json:"fit"` // 0=❌ 1=⚠️ 2=✅
	TPS        float64 `json:"tps"`
	Accel      bool    `json:"accel"`
	Applicable bool    `json:"applicable"`
}

type FitRow struct {
	Model Model     `json:"model"`
	Cells []FitCell `json:"cells"`
}

func FitMatrix(h HW, models []Model, n, ctx, batch int, o Opts) []FitRow {
	rows := make([]FitRow, 0, len(models))
	for _, m := range models {
		row := FitRow{Model: m}
		for _, q := range MainQuants() {
			if fixed := m.FixedQuantID(); fixed != "" && fixed != q.ID {
				row.Cells = append(row.Cells, FitCell{Quant: q.ID})
				continue
			}
			mem := Memory(h, m, q, ctx, batch, n, o)
			st := 0
			if mem.Fit && mem.HeadPct > 0.10 {
				st = 2
			} else if mem.Fit {
				st = 1
			}
			tps := 0.0
			if st > 0 {
				tps = Throughput(h, m, q, ctx, batch, n, o).SingleTPS
			}
			row.Cells = append(row.Cells, FitCell{Quant: q.ID, Fit: st, TPS: tps, Accel: h.Accel(q), Applicable: true})
		}
		rows = append(rows, row)
	}
	return rows
}

// ---------- 模式 3：反向规划 ----------

// PlanOpts 规划需求：目标吞吐/时延/优化目标/排队策略。
type PlanOpts struct {
	TargetTPM float64 `json:"tpm"`        // 目标混合 tok/min（输入+输出合计）
	MinTOS    float64 `json:"tos"`        // 单流 tok/s 下限，0=不限
	Objective string  `json:"objective"`  // cost | latency | avail
	Queue     bool    `json:"queue"`      // 允许请求排队
	MaxQ      int     `json:"maxq"`       // 排队后单副本最大并发上限
	QuantOnly string  `json:"quant_only"` // 限定量化档位，空=全部
}

type Plan struct {
	HW           HW      `json:"hw"`
	N            int     `json:"n"`        // 单副本卡数
	Replicas     int     `json:"replicas"` // 副本（节点）数
	Quant        string  `json:"quant"`
	QName        string  `json:"qname"`
	EngName      string  `json:"eng_name"`
	SpecName     string  `json:"spec_name"`
	Strategy     string  `json:"strategy"`
	Single       float64 `json:"single_tps"`
	Agg          float64 `json:"agg_tps"`      // 单副本聚合 tok/s（容量场景并发口径）
	TPM          float64 `json:"tpm"`          // 集群混合 tok/min 容量
	CapacityQPS  float64 `json:"capacity_qps"` // 集群最大稳定请求/s
	ArrivalQPS   float64 `json:"arrival_qps"`  // 目标 TPM 换算的到达请求/s
	MaxConc      int     `json:"max_conc"`     // 容量场景的单副本并发
	UtilPct      float64 `json:"util_pct"`     // 目标负载 / 集群服务能力
	WaitAvgMs    float64 `json:"wait_avg_ms"`  // M/M/c 平均排队等待
	WaitP95Ms    float64 `json:"wait_p95_ms"`  // M/M/c 无条件 p95 排队等待
	QueueModel   string  `json:"queue_model"`  // none | M/M/c
	TTFTms       float64 `json:"ttft_ms"`
	TPOTms       float64 `json:"tpot_ms"`
	P95SingleTPS float64 `json:"p95_single_tps"`
	TTFTP95ms    float64 `json:"p95_ttft_ms"`
	ReqP95ms     float64 `json:"p95_req_ms"`
	ReqP99ms     float64 `json:"p99_req_ms"`
	MeanContext  float64 `json:"mean_context"`
	P99Context   int     `json:"p99_context"`
	P999Context  int     `json:"p999_context"`
	MaxContext   int     `json:"max_context"`
	MemoryP999   float64 `json:"p999_memory"`
	CostCNY      float64 `json:"cost_cny"`
	Monthly      float64 `json:"monthly"`
	PerMtok      float64 `json:"per_mtok"` // 每百万 token 成本（按集群实际容量满负载）
	Warn         string  `json:"warn,omitempty"`
}

const maxReplicas = 64 // 防止离谱目标生成几百副本的方案

func Planner(hws []HW, m Model, po PlanOpts, workload []WorkloadBucket, conc int, st Opts) []Plan {
	st = st.norm()
	st.skipTrace = true
	workload = normalizeWorkload(workload)
	if len(workload) == 0 {
		return nil
	}
	if po.TargetTPM <= 0 {
		po.TargetTPM = 6000
	}
	if po.MaxQ <= 0 {
		po.MaxQ = 256
	}
	maxContext := 0
	for _, bucket := range workload {
		maxContext = max(maxContext, bucket.Context)
	}
	quants := Quants
	if fixed := m.FixedQuantID(); fixed != "" {
		quants = []Quant{QuantByID(fixed)}
	} else if po.QuantOnly != "" {
		quants = nil
		for _, q := range Quants {
			if q.ID == po.QuantOnly {
				quants = append(quants, q)
			}
		}
		if len(quants) == 0 {
			quants = Quants
		}
	}
	var plans []Plan
	for _, h := range hws {
		if h.Svc {
			continue
		}
		maxN := 8
		if h.Unified || h.Cls == "supernode" || h.Link.Dom == 1 {
			maxN = 1
		}
		if h.Cls == "supernode" {
			maxN = 1
		}
		for _, q := range quants {
			eng := resolveEngine(st.Engine, h, q)
			if !eng.EngineOK(h) {
				continue // 框架不支持该硬件（如 TRT-LLM × Apple），不出方案
			}
			for n := 1; n <= maxN; n *= 2 {
				pf := ThroughputWorkload(h, m, q, workload, conc, n, st)
				if !pf.Fit {
					continue
				}
				minSingle := pf.SingleTPS
				if pf.Workload != nil {
					minSingle = pf.Workload.P95SingleTPS
				}
				if po.MinTOS > 0 && minSingle < po.MinTOS {
					continue
				}
				// 单副本服务能力：关闭排队时用请求并发；开启后将容量场景并发
				// 提升到驻留时间加权的 P99.9 显存上限，再受 MaxQ 限制。
				servicePerf := pf
				maxConc := conc
				if po.Queue {
					maxConc = min(pf.MaxBatch, po.MaxQ)
					maxConc = max(maxConc, conc)
					if maxConc > conc {
						pc := ThroughputWorkload(h, m, q, workload, maxConc, n, st)
						if pc.Fit {
							servicePerf = pc
						} else {
							maxConc = conc
						}
					}
				}
				if servicePerf.Workload == nil || servicePerf.reqSec <= 0 {
					continue
				}
				meanTokens := servicePerf.Workload.MeanContext + servicePerf.Workload.MeanOutput
				capReqS := 1 / servicePerf.reqSec
				capTPM := capReqS * meanTokens * 60
				if capTPM <= 0 {
					continue
				}
				replicas := int(math.Ceil(po.TargetTPM / capTPM))
				if po.Queue && float64(replicas)*capTPM <= po.TargetTPM*(1+1e-12) {
					replicas++ // ρ=1 的 M/M/c 无稳态，至少留一个副本的服务余量
				}
				if po.Objective == "avail" {
					replicas++ // 最高可用性：N+1 冗余
				}
				if replicas > maxReplicas {
					continue
				}
				totalCards := float64(n * replicas)
				clusterTPM := capTPM * float64(replicas)
				capacityQPS := capReqS * float64(replicas)
				arrivalQPS := po.TargetTPM / (meanTokens * 60)
				util, waitAvg, waitP95 := erlangC(arrivalQPS, capReqS, replicas)
				queueModel := "none"
				if po.Queue {
					queueModel = "M/M/c"
				} else {
					waitAvg, waitP95 = 0, 0
				}
				spec := SpecByID(st.Spec)
				p := Plan{
					HW: h, N: n, Replicas: replicas, Quant: q.ID, QName: q.Name,
					EngName: engineDisplay(eng, st.Lang), SpecName: specDisplay(spec, st.Lang),
					Single: servicePerf.SingleTPS, Agg: servicePerf.AggTPS, TPM: round1(clusterTPM),
					CapacityQPS: round4(capacityQPS), ArrivalQPS: round4(arrivalQPS),
					MaxConc: maxConc, UtilPct: round1(util * 100),
					WaitAvgMs: round1(waitAvg), WaitP95Ms: round1(waitP95), QueueModel: queueModel,
					TTFTms: servicePerf.TTFTms, TPOTms: servicePerf.TPOTms,
					P95SingleTPS: servicePerf.Workload.P95SingleTPS, TTFTP95ms: servicePerf.Workload.P95TTFTms,
					ReqP95ms: servicePerf.Workload.P95ReqMs, ReqP99ms: servicePerf.Workload.P99ReqMs,
					MeanContext: servicePerf.Workload.MeanContext, P99Context: servicePerf.Workload.P99Context,
					P999Context: servicePerf.Workload.P999Context, MaxContext: servicePerf.Workload.MaxContext,
					MemoryP999: servicePerf.Mem.P999Total,
					CostCNY:    h.CNY * totalCards,
				}
				p.Strategy = strategy(h, n, st.Lang)
				elec := h.TDP * totalCards * 0.6 * 24 * 30 / 1000 * 0.8 // 60% 负载，0.8 元/kWh
				if p.CostCNY > 0 {
					p.Monthly = p.CostCNY/36 + elec
					p.PerMtok = p.Monthly / (clusterTPM * 60 * 24 * 30 / 1e6)
				}
				p.Warn = warnOf(h, m, q, pf, st.Lang)
				if m.Ctx > 0 && maxContext > m.Ctx {
					p.Warn = joinWarn(p.Warn, localText(st.Lang,
						fmt.Sprintf("The workload tail exceeds the model's native context (%dK>%dK); YaRN/RoPE extension required", maxContext/1024, m.Ctx/1024),
						fmt.Sprintf("工作负载尾部超过模型原生上下文（%dK>%dK），需 YaRN/RoPE 外推", maxContext/1024, m.Ctx/1024)))
				}
				if st.KVQuant != "fp16" && !st.kvSupported(h, eng) {
					p.Warn = joinWarn(p.Warn, localText(st.Lang, "The hardware/engine does not support this KV format; capacity and reads use FP16", "所选硬件/引擎不支持该 KV 格式，容量与读取均按 FP16"))
				}
				if st.Spec == "mtp" && !m.MTP && m.MTPHeads == 0 {
					p.Warn = joinWarn(p.Warn, localText(st.Lang, "The model has no MTP-head metadata; speculative acceleration is not applied", "模型无 MTP 头元数据，未应用推测加速"))
				}
				if st.Spec != "" && st.Spec != "none" && (st.SpecTau <= 0 || st.SpecOvh <= 0) {
					p.Warn = joinWarn(p.Warn, localText(st.Lang, "Measured τ and draft/verify overhead were not supplied; speculative acceleration is not applied", "未提供实测 τ 与 draft/verify 开销，未应用推测加速"))
				}
				if maxContext >= 32768 && n > 1 {
					p.Warn = joinWarn(p.Warn, localText(st.Lang, "For long-context multi-card deployments, evaluate context parallelism or PD disaggregation; benefit depends on SLO and KV transfer", "长上下文多卡应评估 context parallel 或 PD 分离；收益取决于 SLO 与 KV 传输"))
				}
				if po.Queue {
					p.Warn = joinWarn(p.Warn, localText(st.Lang,
						fmt.Sprintf("M/M/%d queue: %.1f%% target utilization, %.0f/%.0fms mean/p95 wait", replicas, p.UtilPct, p.WaitAvgMs, p.WaitP95Ms),
						fmt.Sprintf("排队模型 M/M/%d：目标利用率 %.1f%%，平均/p95 等待 %.0f/%.0fms", replicas, p.UtilPct, p.WaitAvgMs, p.WaitP95Ms)))
				}
				if replicas > 1 && po.Objective != "avail" {
					p.Warn = joinWarn(p.Warn, localText(st.Lang,
						fmt.Sprintf("Requires a %d-replica cluster; account for load balancing and session affinity", replicas),
						fmt.Sprintf("需 %d 副本集群，注意负载均衡与会话亲和", replicas)))
				}
				plans = append(plans, p)
			}
		}
	}
	plans = dedupPlans(plans, po.Objective) // 每硬件×卡数组合只留最优量化档
	sortPlans(plans, po.Objective)
	if len(plans) > 200 { // 基本等于不限（去重后组合数上限约数百），前端再过滤/翻页
		plans = plans[:200]
	}
	return plans
}

// erlangC 按独立指数到达/服务的 M/M/c 模型计算利用率和无条件排队等待。
// P(Wq>t)=P(wait)·exp(-(cμ-λ)t)，因此 p95 在 P(wait)≤5% 时为 0。
func erlangC(lambda, mu float64, servers int) (util, avgMs, p95Ms float64) {
	if lambda <= 0 || mu <= 0 || servers < 1 {
		return 0, 0, 0
	}
	a := lambda / mu
	util = a / float64(servers)
	if util >= 1 {
		return util, math.Inf(1), math.Inf(1)
	}
	term, sum := 1.0, 1.0
	for n := 1; n < servers; n++ {
		term *= a / float64(n)
		sum += term
	}
	term *= a / float64(servers)
	tail := term / (1 - util)
	pWait := tail / (sum + tail)
	rate := float64(servers)*mu - lambda
	avgMs = pWait / rate * 1000
	if pWait > 0.05 {
		p95Ms = math.Log(pWait/0.05) / rate * 1000
	}
	return util, avgMs, p95Ms
}

// dedupPlans 按（硬件 × 单副本卡数）去重：同一组合的不同量化档只保留
// 当前优化目标下最优的一档，避免备选列表被同卡变体挤满。
func dedupPlans(plans []Plan, objective string) []Plan {
	better := planBetter(objective)
	at := map[string]int{}
	out := plans[:0]
	for _, p := range plans {
		key := fmt.Sprintf("%s|%d", p.HW.ID, p.N)
		if i, ok := at[key]; ok {
			if better(p, out[i]) {
				out[i] = p
			}
			continue
		}
		at[key] = len(out)
		out = append(out, p)
	}
	return out
}

func planCost(p Plan) float64 {
	if p.Monthly <= 0 {
		return 1e12
	}
	return p.Monthly
}

func availScoreOf(p Plan) float64 {
	s := math.Min(float64(p.Replicas), 4) * 2 // 副本冗余
	switch p.HW.Cls {
	case "datacenter", "supernode":
		s += 4 // 企业级可靠性（ECC/RAS）
	case "workstation":
		s += 2
	}
	switch p.HW.Link.T {
	case "nvlink", "xgmi", "hccs", "xelink", "ici", "blink", "mlulink", "neuronlink":
		s += 2 // 高速互联，TP 故障面小
	}
	if p.N == 1 {
		s += 1 // 单卡节点故障点最少
	}
	return s
}

// planBetter 返回「a 是否优于 b」比较器（优化目标口径）。
func planBetter(objective string) func(a, b Plan) bool {
	switch objective {
	case "latency":
		return func(a, b Plan) bool {
			la, lb := a.TTFTms+a.TPOTms, b.TTFTms+b.TPOTms
			if la != lb {
				return la < lb
			}
			return planCost(a) < planCost(b)
		}
	case "avail":
		return func(a, b Plan) bool {
			sa, sb := availScoreOf(a), availScoreOf(b)
			if sa != sb {
				return sa > sb
			}
			return planCost(a) < planCost(b)
		}
	default: // cost
		return func(a, b Plan) bool {
			ca, cb := planCost(a), planCost(b)
			if ca != cb {
				return ca < cb
			}
			return a.TTFTms+a.TPOTms < b.TTFTms+b.TPOTms
		}
	}
}

func sortPlans(plans []Plan, objective string) {
	better := planBetter(objective)
	sort.SliceStable(plans, func(i, j int) bool { return better(plans[i], plans[j]) })
}

func joinWarn(a, b string) string {
	if a == "" {
		return b
	}
	return a + "; " + b
}

func precHas(h HW, p string) bool {
	for _, x := range h.Prec {
		if x == p {
			return true
		}
	}
	return false
}

func strategy(h HW, n int, lang string) string {
	if n == 1 {
		return localText(lang, "Single card / host", "单卡 / 单机")
	}
	switch h.Link.T {
	case "nvlink":
		return fmt.Sprintf("TP%d · NVLink %.0fGB/s", n, h.Link.B)
	case "bridge":
		return localText(lang, fmt.Sprintf("TP%d · NVLink bridge", n), fmt.Sprintf("TP%d · NVLink 桥", n))
	case "xgmi", "hccs", "xelink", "ici", "blink", "mlulink", "neuronlink":
		return fmt.Sprintf("TP%d · %s %.0fGB/s", n, strings.ToUpper(h.Link.T), h.Link.B)
	case "ethernet":
		return localText(lang, fmt.Sprintf("TP%d · Ethernet RDMA (Gaudi)", n), fmt.Sprintf("TP%d · 以太网 RDMA（Gaudi 架构）", n))
	default:
		return localText(lang, fmt.Sprintf("TP%d · PCIe (no NVLink; avoid high concurrency)", n), fmt.Sprintf("TP%d · PCIe（无 NVLink，大并发慎用）", n))
	}
}

func warnOf(h HW, m Model, q Quant, pf Perf, lang string) string {
	var w []string
	if m.MoE {
		w = append(w, localText(lang, "MoE: memory uses total parameters; speed uses active parameters", "MoE：显存按总参数、速度按激活参数估算"))
	}
	if !pf.Accel {
		w = append(w, localText(lang, q.Name+" saves memory on this device but has no hardware acceleration", q.Name+" 在该卡仅省显存、不硬件加速"))
	}
	if pf.SingleTPS < 10 {
		w = append(w, localText(lang, "Low single-stream speed; consider speculative decoding only with compatible draft/prediction heads", "单流偏慢；仅在有匹配草稿/预测头时考虑推测解码"))
	}
	if pf.Mem.HeadPct < 0.15 {
		w = append(w, localText(lang, "Low memory headroom; reduce long-context concurrency or enable supported KV quantization", "显存贴边，长上下文需降并发或开 KV 量化"))
	}
	if h.Conf == "reported" {
		w = append(w, localText(lang, "Hardware specifications use publicly reported figures", "硬件参数为公开报道口径"))
	}
	return strings.Join(w, localText(lang, "; ", "；"))
}

// ---------- 速查表 ----------

type QuickRow struct {
	HW      HW      `json:"hw"`
	MaxFP16 float64 `json:"max_fp16"`
	MaxINT4 float64 `json:"max_int4"`
}

// QuickTable 每张卡能扛的最大模型参数（扣除底座/预留/8K 上下文 KV）。
func QuickTable(hws []HW) []QuickRow {
	var rows []QuickRow
	for _, h := range hws {
		if h.Svc {
			continue
		}
		budget := h.CapGB(Engines[1]) - 3.5 // 按 vLLM 0.90 服务预算
		rows = append(rows, QuickRow{HW: h, MaxFP16: budget / 2.0, MaxINT4: budget / 0.6})
	}
	return rows
}
