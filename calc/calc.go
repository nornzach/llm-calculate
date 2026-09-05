// Package calc 实现 LLM 推理计算器的显存可行性、decode/prefill
// 一阶 roofline 估算和反向部署规划。输出是容量筛选值，不是实测基准。
package calc

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
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
	PeakKind  string   `json:"peak_kind,omitempty"` // dense_matrix | vector | estimated；缺失表示来源未知
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
	Heads           int     `json:"heads,omitempty"`
	Revision        string  `json:"revision,omitempty"`
	ExtendedCtx     int     `json:"extended_ctx,omitempty"`
	ParamSource     string  `json:"param_source,omitempty"`
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

// EngineOK reports whether the runtime has at least a plausible path on this
// device. Conditional plugin/version paths are distinguished by Perf.Support.
func (e Engine) EngineOK(h HW) bool {
	status, _ := engineSupport(e, h, "")
	return status == "supported" || status == "conditional"
}

func engineSupport(e Engine, h HW, lang string) (string, string) {
	if h.Vendor == "" {
		return "unknown", localText(lang, "Hardware vendor is unknown", "硬件厂商未知")
	}
	if h.Arch == "" {
		return "unknown", localText(lang, "Hardware architecture is unknown", "硬件架构未知")
	}
	switch e.ID {
	case "trtllm":
		if h.Vendor != "nvidia" {
			return "unsupported", localText(lang, "TensorRT-LLM requires NVIDIA hardware", "TensorRT-LLM 仅支持 NVIDIA 硬件")
		}
		switch h.Arch {
		case "pascal", "volta", "turing":
			return "unsupported", localText(lang, "TensorRT-LLM does not support this legacy GPU generation", "TensorRT-LLM 不支持该旧 GPU 代际")
		case "ampere", "ada", "hopper", "blackwell":
			return "supported", ""
		case "":
			return "unknown", localText(lang, "GPU generation is unknown", "GPU 代际未知")
		default:
			return "unknown", localText(lang, "TensorRT-LLM support is not established for this GPU generation", "TensorRT-LLM 尚未确认支持该 GPU 代际")
		}
	case "llamacpp":
		if h.Vendor == "qualcomm" || h.Arch == "aic100" {
			return "unsupported", localText(lang, "llama.cpp has no Cloud AI 100 backend", "llama.cpp 没有 Cloud AI 100 后端")
		}
		if h.Vendor == "nvidia" && (h.Arch == "pascal" || h.Arch == "volta") {
			return "conditional", localText(lang, "Legacy NVIDIA GPU offload uses llama.cpp kernels and may require checkpoint conversion; verify the deployed build", "旧代 NVIDIA GPU offload 使用 llama.cpp 内核，且可能需要转换检查点；需核对部署版本")
		}
		switch h.Vendor {
		case "nvidia", "amd", "intel", "apple":
			return "supported", ""
		default:
			return "unsupported", localText(lang, "llama.cpp has no modeled backend for this accelerator", "llama.cpp 没有该加速器的已建模后端")
		}
	case "mlx":
		if h.Vendor == "apple" {
			return "supported", ""
		}
		return "unsupported", localText(lang, "MLX requires Apple Silicon", "MLX 仅支持 Apple Silicon")
	case "exllama", "lmdeploy":
		if h.Vendor != "nvidia" {
			return "unsupported", localText(lang, "The selected runtime requires NVIDIA hardware", "所选运行时仅支持 NVIDIA 硬件")
		}
		switch h.Arch {
		case "ampere", "ada", "hopper", "blackwell":
			return "supported", ""
		case "":
			return "unknown", localText(lang, "GPU generation is unknown", "GPU 代际未知")
		default:
			return "conditional", localText(lang, "Runtime support depends on generation-specific kernels", "运行时支持取决于该代际内核")
		}
	case "mindie":
		if h.Vendor == "huawei" {
			return "supported", ""
		}
		return "unsupported", localText(lang, "MindIE requires Ascend hardware", "MindIE 仅支持昇腾硬件")
	case "vllm", "sglang":
		switch h.Vendor {
		case "nvidia":
			switch h.Arch {
			case "ampere", "ada", "hopper", "blackwell":
				return "supported", ""
			case "pascal", "volta":
				return "unsupported", localText(lang, "This runtime no longer supports the legacy NVIDIA generation", "该运行时已不支持此旧代 NVIDIA GPU")
			case "":
				return "unknown", localText(lang, "GPU generation is unknown", "GPU 代际未知")
			default:
				return "conditional", localText(lang, "Runtime support for this NVIDIA generation requires version verification", "该 NVIDIA 代际需核对运行时版本")
			}
		case "amd":
			switch h.Arch {
			case "cdna2", "cdna3", "cdna4":
				return "supported", ""
			case "":
				return "unknown", localText(lang, "GPU generation is unknown", "GPU 代际未知")
			default:
				return "conditional", localText(lang, "ROCm runtime support for this generation requires version verification", "该代际 ROCm 运行时支持需核对版本")
			}
		case "intel":
			if strings.HasPrefix(h.Arch, "gaudi") {
				return "conditional", localText(lang, "Gaudi uses a vendor plugin whose model and version support must be verified", "Gaudi 依赖厂商插件，需核对模型与版本支持")
			}
			return "conditional", localText(lang, "This runtime uses a non-mainline Intel backend", "该运行时依赖非主线 Intel 后端")
		case "huawei", "hygon", "metax", "mthreads", "enflame", "biren", "iluvatar", "kunlunxin", "cambricon":
			return "conditional", localText(lang, "This hardware requires a vendor plugin or fork", "该硬件依赖厂商插件或分支")
		default:
			return "unsupported", localText(lang, "The selected runtime has no modeled backend for this vendor", "所选运行时没有该厂商的已建模后端")
		}
	default:
		return "unknown", localText(lang, "Runtime support is unknown", "运行时支持未知")
	}
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
			case "nvidia":
				if h.Arch == "pascal" || h.Arch == "volta" {
					id = "llamacpp"
				} else {
					id = "vllm"
				}
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
	return Engine{ID: id, Name: id}
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

// Opts 同时承载部署、缓存、媒体和用户场景系数。零值保持原有简单模式。
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
	BWUtil       float64 `json:"bw_util,omitempty"`       // 用户场景 HBM 带宽利用率
	FlopsUtil    float64 `json:"flops_util,omitempty"`    // 用户场景 dense 峰值利用率
	LinkUtil     float64 `json:"link_util,omitempty"`     // 用户场景互联带宽利用率
	ScheduleMS   float64 `json:"schedule_ms,omitempty"`   // 用户场景每 decode step 调度开销

	KVOverhead   float64 `json:"kv_overhead,omitempty"` // block/allocator 容量系数
	KVOffload    float64 `json:"kv_offload,omitempty"`  // 卸载到 CPU/远端的 KV 比例
	OffloadBW    float64 `json:"offload_bw,omitempty"`  // 单卡有效回读 GB/s
	PrefillChunk int     `json:"prefill_chunk,omitempty"`
	MediaTokens  int     `json:"media_tokens,omitempty"` // 视觉/音频 encoder 输入 token
	RouterSkew   float64 `json:"router_skew,omitempty"`  // EP 最忙 rank / 平均负载
	SpecTau      float64 `json:"spec_tau,omitempty"`     // 用户场景每步接受 token
	SpecOvh      float64 `json:"spec_ovh,omitempty"`     // 用户场景 draft/verify 相对开销
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
	o.MemUtil = clamp(o.MemUtil, 0, 1)
	o.BWUtil = clamp(o.BWUtil, 0, 1)
	o.FlopsUtil = clamp(o.FlopsUtil, 0, 1)
	o.LinkUtil = clamp(o.LinkUtil, 0, 1)
	if o.BWUtil > 0 {
		o.BWUtil = math.Max(o.BWUtil, 1e-6)
	}
	if o.FlopsUtil > 0 {
		o.FlopsUtil = math.Max(o.FlopsUtil, 1e-6)
	}
	if o.LinkUtil > 0 {
		o.LinkUtil = math.Max(o.LinkUtil, 1e-6)
	}
	const maxScenarioGB = 1_000_000
	o.WeightGB = clamp(o.WeightGB, 0, maxScenarioGB)
	o.RuntimeGB = clamp(o.RuntimeGB, 0, maxScenarioGB)
	o.ActivationGB = clamp(o.ActivationGB, 0, maxScenarioGB)
	o.AdapterGB = clamp(o.AdapterGB, 0, maxScenarioGB)
	o.DraftGB = clamp(o.DraftGB, 0, maxScenarioGB)
	o.ScheduleMS = clamp(o.ScheduleMS, 0, 10_000)
	o.OffloadBW = clamp(o.OffloadBW, 0, 1_000_000)
	if o.KVOffload > 0 && o.OffloadBW <= 0 {
		o.KVOffload = 0
	} else if o.KVOffload > 0 {
		o.OffloadBW = math.Max(o.OffloadBW, 1e-6)
	}
	o.RouterSkew = clamp(o.RouterSkew, 1, 16)
	o.SpecTau = clamp(o.SpecTau, 0, 32)
	o.SpecOvh = clamp(o.SpecOvh, 0, 10)
	o.MediaTokens = max(0, o.MediaTokens)
	if o.Lang != "zh" {
		o.Lang = "en"
	}
	return o
}
func (o Opts) hasScenarioInputs() bool {
	return o.WeightGB > 0 || o.RuntimeGB > 0 || o.ActivationGB > 0 ||
		o.AdapterGB > 0 || o.DraftGB > 0 || o.MemUtil > 0 ||
		o.BWUtil > 0 || o.FlopsUtil > 0 || o.LinkUtil > 0 ||
		o.ScheduleMS > 0 || o.KVOverhead != 1 || o.KVOffload > 0 ||
		o.OffloadBW > 0 || o.RouterSkew > 1 || o.MediaTokens > 0 ||
		o.HitRate > 0
}

// kvSupport separates cache storage/dequantization from native low-precision
// arithmetic. In particular, Ampere can store FP8 KV through supported
// backends even though it has no native FP8 tensor arithmetic.
func (o Opts) kvSupport(h HW, eng Engine) (string, string, bool) {
	switch o.KVQuant {
	case "", "fp16":
		return "supported", "", true
	case "fp8":
		if eng.ID != "vllm" && eng.ID != "sglang" && eng.ID != "trtllm" {
			return "unsupported", localText(o.Lang, "The selected runtime does not model FP8 KV storage", "所选运行时未建模 FP8 KV 存储"), false
		}
		if precHas(h, "fp8") {
			return "supported", "", true
		}
		if h.Vendor == "nvidia" && h.Arch == "ampere" {
			return "conditional", localText(o.Lang, "FP8 KV storage on Ampere uses backend dequantization, not native FP8 arithmetic; verify the deployed version", "Ampere 的 FP8 KV 依赖后端反量化而非原生 FP8 算术；需核对部署版本"), true
		}
		return "unsupported", localText(o.Lang, "This hardware/runtime path does not support FP8 KV storage", "该硬件/运行时路径不支持 FP8 KV 存储"), false
	case "fp4":
		if precHas(h, "fp4") && eng.ID == "sglang" {
			return "conditional", localText(o.Lang, "FP4 KV storage is experimental and version-dependent", "FP4 KV 存储仍为实验性且依赖版本"), true
		}
		return "unsupported", localText(o.Lang, "This hardware/runtime path does not support FP4 KV storage", "该硬件/运行时路径不支持 FP4 KV 存储"), false
	default:
		return "unsupported", localText(o.Lang, "Unknown KV cache format", "未知 KV cache 格式"), false
	}
}

func (o Opts) kvSupported(h HW, eng Engine) bool {
	_, _, ok := o.kvSupport(h, eng)
	return ok
}

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
		if h.PeakKind != "" && h.PeakKind != "dense_matrix" && h.PeakKind != "vector" && h.PeakKind != "estimated" {
			return nil, fmt.Errorf("hardware %q has unknown peak kind %q", h.ID, h.PeakKind)
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
		if m.Heads < 0 || m.ExtendedCtx < 0 || (m.ExtendedCtx > 0 && m.ExtendedCtx < m.Ctx) {
			return nil, fmt.Errorf("model %q has invalid head/context metadata", m.ID)
		}
		if m.Heads > 0 && (m.KVT == "mha" || m.KVT == "gqa") {
			if (m.KVT == "mha" && m.KVH != m.Heads) ||
				(m.KVT == "gqa" && (m.Heads < m.KVH || m.Heads%m.KVH != 0)) {
				return nil, fmt.Errorf("model %q has inconsistent attention heads", m.ID)
			}
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
	if h.PeakKind != "dense_matrix" || h.SourceURL == "" {
		return false
	}
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
	cards = max(1, cards)
	if o.TP == 0 && o.PP == 0 && o.EP == 0 && o.CP == 0 {
		return topology{tp: cards, pp: 1, ep: 1, cp: 1, valid: true}
	}
	t := topology{tp: max(1, o.TP), pp: max(1, o.PP), ep: max(1, o.EP), cp: max(1, o.CP)}
	if o.TP < 0 || o.PP < 0 || o.EP < 0 || o.CP < 0 ||
		t.tp > cards || t.pp > cards || t.ep > cards || t.cp > cards {
		return t
	}
	product := 1
	for _, rank := range []int{t.tp, t.pp, t.ep, t.cp} {
		if product > cards/rank {
			return t
		}
		product *= rank
	}
	t.valid = product == cards
	return t
}

func (o Opts) topologyFor(m Model, cards int) topology {
	t := o.topology(cards)
	if t.ep > 1 && !m.MoE {
		t.valid = false
	}
	return t
}

type supportAssessment struct {
	status        string
	reason        string
	estimateValid bool
}

func (a *supportAssessment) add(status, reason string, valid bool) {
	rank := func(s string) int {
		switch s {
		case "unsupported":
			return 3
		case "unknown":
			return 2
		case "conditional":
			return 1
		default:
			return 0
		}
	}
	if rank(status) > rank(a.status) {
		a.status = status
	}
	if reason != "" && !strings.Contains(a.reason, reason) {
		a.reason = joinWarn(a.reason, reason)
	}
	a.estimateValid = a.estimateValid && valid
}

func (m Model) standardHeadShape() bool {
	family := executionFamily(m)
	return strings.Contains(family, "llama") || strings.Contains(family, "qwen2") ||
		strings.Contains(family, "falcon")
}

func (m Model) queryHeads() (int, bool) {
	if m.Heads > 0 {
		return m.Heads, true
	}
	if (m.standardHeadShape() || m.ParamSource == "user-supplied") &&
		(m.KVT == "mha" || m.KVT == "gqa") && m.Dim > 0 && m.Hidden > 0 {
		heads := m.Hidden / float64(m.Dim)
		if heads == math.Trunc(heads) && heads >= 1 {
			return int(heads), true
		}
	}
	return 0, false
}

func (m Model) attentionWidth() float64 {
	if (m.KVT == "mha" || m.KVT == "gqa") && m.Dim > 0 {
		if heads, ok := m.queryHeads(); ok {
			return float64(heads * m.Dim)
		}
	}
	return m.Hidden
}

func executionFamily(m Model) string {
	return strings.ToLower(m.ModelType + " " + m.Architecture)
}

func quantSupport(h HW, q Quant, eng Engine, lang string) (string, string, bool) {
	switch q.Fam {
	case "mlx":
		if h.Vendor != "apple" || eng.ID != "mlx" {
			return "unsupported", localText(lang, "MLX weights require MLX on Apple Silicon", "MLX 权重需在 Apple Silicon 上使用 MLX"), false
		}
	case "exl":
		if h.Vendor != "nvidia" || eng.ID != "exllama" {
			return "unsupported", localText(lang, "EXL3 weights require ExLlama on NVIDIA", "EXL3 权重需在 NVIDIA 上使用 ExLlama"), false
		}
	case "gguf":
		if eng.ID != "llamacpp" {
			return "conditional", localText(lang, "GGUF support outside llama.cpp is experimental and version-dependent", "llama.cpp 之外的 GGUF 支持为实验性且依赖版本"), true
		}
	}
	switch q.ID {
	case "mxfp4":
		if eng.ID != "vllm" && eng.ID != "sglang" {
			return "unsupported", localText(lang, "MXFP4 has no modeled load path in this runtime", "该运行时没有已建模的 MXFP4 加载路径"), false
		}
		modeled := h.Vendor == "nvidia" && (h.Arch == "hopper" || h.Arch == "blackwell") ||
			h.Vendor == "amd" && (h.Arch == "cdna3" || h.Arch == "cdna4")
		if !modeled {
			return "unknown", localText(lang, "MXFP4 runtime support is not established for this accelerator generation", "该加速器代际的 MXFP4 运行时支持尚未确认"), false
		}
		if !h.Accel(q) {
			return "conditional", localText(lang, "MXFP4 uses a generation-specific runtime dequantization path without native FP4 arithmetic", "MXFP4 使用该代际特定的运行时反量化路径，但没有原生 FP4 算术"), true
		}
	case "fp8":
		if !h.Accel(q) {
			if h.Vendor == "nvidia" || h.Vendor == "amd" {
				return "conditional", localText(lang, "FP8 weights require runtime dequantization because this device lacks a native FP8 arithmetic path", "该设备缺少原生 FP8 算术路径，FP8 权重需运行时反量化"), true
			}
			return "unsupported", localText(lang, "FP8 weights have no modeled load path on this platform", "该平台没有已建模的 FP8 权重加载路径"), false
		}
	case "fp4":
		if !h.Accel(q) {
			return "unsupported", localText(lang, "NVFP4 weights require a native NVIDIA FP4 path", "NVFP4 权重需要 NVIDIA 原生 FP4 路径"), false
		}
	}
	return "supported", "", true
}

// ModelSupport checks model metadata independently of hardware or target throughput.
func ModelSupport(m Model, o Opts) (status, reason string, valid bool) {
	a := supportAssessment{status: "supported", estimateValid: true}
	family := executionFamily(m)
	if strings.Contains(family, "diffusion") || strings.Contains(family, "llada") ||
		strings.Contains(family, "dflashdraft") || strings.Contains(family, "efficientdlm") ||
		strings.Contains(family, "diffcoder") || strings.Contains(family, "sdlm") {
		a.add("unsupported", localText(o.Lang, "This diffusion/block-diffusion family has no autoregressive execution model", "该扩散/块扩散家族没有自回归执行模型"), false)
	}
	switch m.ParamSource {
	case "config", "safetensors", "model_card":
	case "name", "unknown":
		a.add("unknown", localText(o.Lang, "Logical parameter count is unverified", "逻辑参数量未经核验"), false)
	case "user-supplied":
		a.add("conditional", localText(o.Lang, "User-supplied model metadata is a what-if scenario", "用户输入的模型元数据仅作为假设场景"), true)
	case "":
		if m.Conf == "fetched" {
			a.add("unknown", localText(o.Lang, "Fetched model parameter provenance is missing", "抓取模型缺少参数来源"), false)
		} else {
			a.add("conditional", localText(o.Lang, "Model parameter provenance is not revision-scoped", "模型参数来源未绑定版本"), true)
		}
	default:
		a.add("unknown", localText(o.Lang, "Model parameter provenance is unknown", "模型参数来源未知"), false)
	}
	if m.Revision == "" {
		a.add("conditional", localText(o.Lang, "Model revision is not pinned", "模型版本未固定"), true)
	}
	if m.ModelType == "" && m.Architecture == "" && m.Conf != "" && m.ParamSource != "user-supplied" {
		a.add("unknown", localText(o.Lang, "Model architecture is unknown", "模型架构未知"), false)
	}
	if m.Multimodal && m.EncoderParams <= 0 {
		if o.MediaTokens > 0 {
			a.add("unknown", localText(o.Lang, "Multimodal encoder geometry is unknown", "多模态 encoder 几何信息未知"), false)
		} else {
			a.add("conditional", localText(o.Lang, "Only the text tower is modeled because encoder geometry is unknown", "encoder 几何信息未知，仅建模文本塔"), true)
		}
	}
	switch m.KVT {
	case "mha", "gqa":
		if m.KVH <= 0 || m.Dim <= 0 {
			a.add("unknown", localText(o.Lang, "Attention head geometry is incomplete", "attention 头几何信息不完整"), false)
		}
		if heads, ok := m.queryHeads(); ok {
			// Attention projections may have a different width from the residual
			// stream (e.g. Llama Minitron, Falcon-H1 and Qwen3).
			if m.KVT == "mha" && m.KVH != heads {
				a.add("unknown", localText(o.Lang, "MHA query and KV head counts disagree", "MHA 的 query 与 KV 头数不一致"), false)
			}
			if m.KVT == "gqa" && (heads < m.KVH || heads%m.KVH != 0) {
				a.add("unknown", localText(o.Lang, "GQA query/KV head grouping is invalid", "GQA 的 query/KV 头分组无效"), false)
			}
		} else {
			a.add("unknown", localText(o.Lang, "Query head count is unavailable for this attention family", "该 attention 家族缺少 query 头数"), false)
		}
	case "mla":
		if m.MLA <= 0 {
			a.add("unknown", localText(o.Lang, "MLA latent cache geometry is incomplete", "MLA latent cache 几何信息不完整"), false)
		}
	default:
		a.add("unknown", localText(o.Lang, "Attention execution geometry is unknown", "attention 执行几何信息未知"), false)
	}

	return a.status, a.reason, a.estimateValid
}

func assessSupport(h HW, m Model, q Quant, eng Engine, t topology, cards, ctx int, o Opts) supportAssessment {
	a := supportAssessment{status: "supported", estimateValid: true}
	if h.Svc || h.Cls == "supernode" {
		a.add("unsupported", localText(o.Lang, "Aggregate service or supernode rows are not single-device roofline inputs", "聚合服务或超节点条目不能作为单设备 roofline 输入"), false)
	}
	if h.VRAM <= 0 || h.BW <= 0 || h.TF <= 0 {
		a.add("unknown", localText(o.Lang, "Hardware lacks per-device memory, bandwidth, or dense compute inputs", "硬件缺少单设备显存、带宽或 dense 算力输入"), false)
	}
	switch h.PeakKind {
	case "dense_matrix":
		if h.SourceURL == "" {
			a.add("conditional", localText(o.Lang, "Dense matrix peak is declared without a source", "dense 矩阵峰值已声明但缺少来源"), true)
		}
	case "vector":
		a.add("conditional", localText(o.Lang, "Vector peak is used as a compute ceiling; matrix-kernel efficiency is not certified", "使用向量峰值作为计算上限；矩阵内核效率未经核验"), true)
	case "estimated":
		a.add("conditional", localText(o.Lang, "Dense compute peak is estimated rather than a sourced specification", "dense 算力峰值为估计值而非有来源的规格"), true)
	default:
		a.add("conditional", localText(o.Lang, "Dense compute peak provenance is unknown; TF is treated as a scenario input", "dense 算力峰值来源未知；TF 仅作为场景输入"), true)
	}
	status, reason := engineSupport(eng, h, o.Lang)
	a.add(status, reason, status == "supported" || status == "conditional")
	status, reason, valid := quantSupport(h, q, eng, o.Lang)
	a.add(status, reason, valid)

	modelStatus, modelReason, modelValid := ModelSupport(m, o)
	a.add(modelStatus, modelReason, modelValid)

	if !t.valid {
		a.add("unsupported", localText(o.Lang, "TP×PP×EP×CP must exactly match the requested card count", "TP×PP×EP×CP 必须精确匹配请求卡数"), false)
	}
	if cards > 1 {
		if h.Link.Dom <= 0 || h.Link.B <= 0 || h.Link.T == "" || h.Link.T == "none" {
			a.add("unknown", localText(o.Lang, "Cross-card topology is not described", "未描述跨卡拓扑"), false)
		} else if cards > h.Link.Dom {
			a.add("unknown", localText(o.Lang, "Requested cards exceed the described full-bandwidth link domain", "请求卡数超过已描述的全带宽互联域"), false)
		} else if h.Link.T == "bridge" && cards > 2 {
			a.add("unsupported", localText(o.Lang, "Point-to-point bridges do not form a multi-card full mesh", "点对点桥接不能构成多卡全互联"), false)
		}
	}
	if t.tp > 1 {
		heads, ok := m.queryHeads()
		if !ok {
			a.add("unknown", localText(o.Lang, "Query head count is required to validate tensor parallelism", "验证 tensor parallel 需要 query 头数"), false)
		} else if heads%t.tp != 0 {
			a.add("unsupported", localText(o.Lang, "Query head count is not divisible by tensor parallel degree", "query 头数不能被 tensor parallel 度整除"), false)
		}
		if (m.KVT == "mha" || m.KVT == "gqa") && m.KVH > 0 && m.KVH%t.tp != 0 && t.tp%m.KVH != 0 {
			a.add("unsupported", localText(o.Lang, "KV heads can be neither evenly divided nor evenly replicated across tensor-parallel ranks", "KV 头无法在 tensor-parallel ranks 间均匀切分或复制"), false)
		}
	}
	if t.cp > 1 {
		if eng.ID == "vllm" {
			a.add("unsupported", localText(o.Lang, "Context parallelism is not vLLM decode-context parallelism and is not modeled on this path", "context parallel 并非 vLLM decode-context parallel，本路径未建模"), false)
		} else {
			a.add("conditional", localText(o.Lang, "Context-parallel execution depends on runtime-specific sequence partitioning", "context parallel 执行取决于运行时的序列切分实现"), true)
		}
	}
	status, reason, valid = o.kvSupport(h, eng)
	a.add(status, reason, valid)
	if m.Ctx <= 0 {
		a.add("unknown", localText(o.Lang, "Native context limit is unknown", "原生上下文上限未知"), false)
	} else if ctx > m.Ctx {
		switch {
		case m.ExtendedCtx <= 0:
			a.add("unknown", localText(o.Lang, "Input plus output exceeds the native limit and no verified extended limit is available", "输入加输出超过原生上限，且没有已核验的扩展上限"), false)
		case ctx > m.ExtendedCtx:
			a.add("unsupported", localText(o.Lang, "Input plus output exceeds the verified extended limit", "输入加输出超过已核验的扩展上限"), false)
		default:
			a.add("conditional", localText(o.Lang, "Input plus output requires the extended context limit", "输入加输出需要扩展上下文上限"), true)
		}
	}
	return a
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
	tokens := float64(batch) * float64(min(ctx, o.PrefillChunk))
	// FlashAttention 不保存 O(n²) attention matrix；保留 residual、QKV/MLP workspace 的一阶上界。
	return tokens * m.Hidden * 2 * 4 / 1e9 / (float64(t.tp) * float64(t.cp))
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
	return m.kvBatchBytes(ctx, batch, int(float64(ctx)*clamp(hit, 0, 1)))
}

func (m Model) kvBatchBytes(ctx, batch, shared int) float64 {
	if ctx <= 0 || batch <= 0 {
		return 0
	}
	shared = min(ctx, max(0, shared))
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

// kvRankFactor is defined only for divisible head sharding or whole-head
// replication; assessSupport rejects every other TP geometry.
func (m Model) kvRankFactor(tp int) float64 {
	if tp <= 1 || m.KVT == "mla" || m.KVH <= 0 {
		return 1
	}
	if m.KVH%tp == 0 {
		return 1 / float64(tp)
	}
	if tp%m.KVH == 0 {
		return 1 / float64(m.KVH)
	}
	return 1
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
		textWeight*(parts.baseTotal/math.Max(textParams, 1e-9)/(float64(t.tp)*float64(t.pp))+
			parts.expertTotal/math.Max(textParams, 1e-9)/(float64(t.tp)*float64(t.ep)*float64(t.pp)))

	// The first output token comes from prefill; subsequent tokens append KV.
	// Only input-prefix blocks may be shared across requests.
	kvRaw := m.kvBatchBytes(ctx+max(0, o.OutLen-1), batch, int(float64(ctx)*o.HitRate)) / 1e9 * o.kvMemF(h, eng) * o.KVOverhead *
		m.kvRankFactor(t.tp) / (float64(t.pp) * float64(t.cp))
	offloadedKV := kvRaw * o.KVOffload
	state := m.StateMB / 1000 * float64(batch) / (float64(t.tp) * float64(t.pp))
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
	adapter := (o.AdapterGB + draft) / (float64(t.tp) * float64(t.pp))
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
	P95SingleTPS float64              `json:"p95_single_tps"` // 95% 请求可达到的单流 TPS 下限（TPS P05）
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
	EngOK           bool           `json:"eng_ok"` // 框架是否有 supported/conditional 硬件路径
	SpecName        string         `json:"spec_name"`
	Bottleneck      string         `json:"bottleneck"`        // memory | compute
	DecodeMemMs     float64        `json:"decode_mem_ms"`     // 每 decode step 的显存 roof
	DecodeComputeMs float64        `json:"decode_compute_ms"` // 每 decode step 的算力 roof
	CommMs          float64        `json:"comm_ms"`           // 每 decode step 的 TP collective
	ScheduleMs      float64        `json:"schedule_ms"`       // 每 decode step 的调度场景值
	LayerMs         float64        `json:"layer_ms"`          // 每 decode step 的多卡层固定开销
	OffloadMs       float64        `json:"offload_ms"`
	EncoderMs       float64        `json:"encoder_ms"`
	PeakTF          float64        `json:"peak_tf"`
	PeakExact       bool           `json:"peak_exact"`
	Accuracy        string         `json:"accuracy"` // analytical | scenario
	Topology        string         `json:"topology"`
	TopologyOK      bool           `json:"topology_ok"`
	Support         string         `json:"support"` // supported | conditional | unsupported | unknown
	SupportReason   string         `json:"support_reason"`
	EstimateValid   bool           `json:"estimate_valid"`
	Deployable      bool           `json:"deployable"`
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
		return "Requires model metadata for MTP heads; acceptance and gain are user-supplied scenario inputs"
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

// linkLatencyMs 每个 collective 的小消息延迟下限(ms)。
// 依据:NCCL 小消息 allreduce 实测延迟 NVLink ~10μs、PCIe ~25μs、
// IB/RoCE ~20μs、以太网 ~40μs;batch=1 时通信量趋零,延迟主导步时。
func linkLatencyMs(h HW) float64 {
	switch h.Link.T {
	case "nvlink", "xgmi":
		return 0.010
	case "hccs":
		return 0.012
	case "bridge":
		return 0.015
	case "ib", "neuronlink":
		return 0.020
	case "unified", "pcie":
		return 0.025
	case "ethernet":
		return 0.040
	}
	return 0.030
}

// collectiveMs n 个 collective 的总耗时:每个取 max(流量时间, latMul×延迟下限)。
func collectiveMs(h HW, trafficGB float64, n int, latMul float64, o Opts) float64 {
	if n <= 0 {
		return 0
	}
	per := math.Max(trafficGB/float64(n)/o.linkBW(h)*1000, latMul*linkLatencyMs(h))
	return per * float64(n)
}

// layerFixedMs decode 每层 kernel 序列下限(GEMM/attention/路由/同步),
// 仅多卡:单卡 launch 开销已被 CUDA graph 与引擎 StepMs 吸收。
// 校准:SGLang R1 TP8 batch=1 实测 21ms/step(47.67 tok/s, verda);
// Qwen3-235B 8×B200 实测 10.4ms/step(96.65 tok/s, SGLang 官方)。
// 残差为引擎代际差异,档位目标 ±2×。
func layerFixedMs(m Model, batch, cards int) float64 {
	if cards <= 1 {
		return 0
	}
	per := 0.02
	if m.MoE {
		per = 0.20 // MoE 分组 GEMM + 路由的 kernel 序列远长于 dense
	}
	return float64(m.Layers) * per * 8 / (8 + float64(batch))
}

func tpCommMs(h HW, m Model, tokens float64, t topology, o Opts) float64 {
	if t.tp <= 1 {
		return 0
	}
	// 每层 attention/MLP 各一次 AllReduce;ring 每次传 2*(TP-1)/TP 份 payload。
	ring := 2 * float64(t.tp-1) / float64(t.tp)
	traffic := 2 * ring * float64(m.Layers) * tokens * m.Hidden * 2 / 1e9
	return collectiveMs(h, traffic, 2*m.Layers, 1, o)
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
	// 每个 MoE 层按 TopK 路由做 dispatch + combine All-to-All;
	// all2all 小消息延迟约 2× allreduce(DeepEP 低延迟模式实测)。
	routes := float64(max(1, m.TopK))
	traffic := 2 * float64(t.ep-1) / float64(t.ep) * float64(layers) * tokens * routes * m.Hidden * 2 / 1e9
	return collectiveMs(h, traffic*routerSkew(m, tokens, t.ep, o), 2*layers, 2, o)
}

func cpCommMs(h HW, m Model, tokens float64, t topology, o Opts) float64 {
	if t.cp <= 1 {
		return 0
	}
	traffic := 2 * float64(t.cp-1) / float64(t.cp) * float64(m.kvLayers()) * tokens * m.Hidden * 2 / 1e9
	return collectiveMs(h, traffic, 2*m.kvLayers(), 1, o)
}

func ppCommMs(h HW, m Model, tokens float64, t topology, o Opts) float64 {
	if t.pp <= 1 {
		return 0
	}
	traffic := float64(t.pp-1) * tokens * m.Hidden * 2 / 1e9
	return collectiveMs(h, traffic, 2*(t.pp-1), 1, o)
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
	engineStatus, _ := engineSupport(eng, h, o.Lang)
	assessment := assessSupport(h, m, q, eng, t, max(1, cards), ctx+o.OutLen, o)
	p := Perf{
		Fit: mem.Fit, Mem: mem, QuantID: q.ID, QuantLocked: quantLocked,
		KVSupported: o.kvSupported(h, eng), Accel: h.Accel(q),
		EngName: engineDisplay(eng, o.Lang), EngOK: engineStatus == "supported" || engineStatus == "conditional", SpecName: specDisplay(spec, o.Lang),
		PeakTF: h.PeakTF(q), PeakExact: h.peakExact(q),
		Accuracy: "analytical", Topology: t.String(), TopologyOK: t.valid,
		Support: assessment.status, SupportReason: assessment.reason, EstimateValid: assessment.estimateValid,
	}
	p.Deployable = p.EstimateValid && p.Support == "supported" && p.Fit
	if h.PeakKind != "dense_matrix" || o.hasScenarioInputs() || m.Src == "custom" || m.ParamSource == "user-supplied" {
		p.Accuracy = "scenario"
	}
	if !p.EstimateValid {
		if !o.skipTrace {
			p.Trace = []TraceRow{
				tr(localText(o.Lang, "Estimate status", "估算级别"), p.Accuracy, localText(o.Lang, "Performance is withheld because the requested execution path is not modeled", "请求执行路径未建模，因此不输出性能")),
				tr(localText(o.Lang, "Support", "支持状态"), p.Support, p.SupportReason),
				tr(localText(o.Lang, "Parallel topology", "并行拓扑"), p.Topology, localText(o.Lang, "The requested topology is preserved; no fallback estimate is substituted", "保留请求拓扑，不替换为回退估算")),
				tr(localText(o.Lang, "Inference engine", "推理框架"), engineDisplay(eng, o.Lang), p.SupportReason),
			}
		}
		return p
	}

	parts := m.weights(batch)
	weightTotal := o.weightGB(m, q)
	bytesPerParam := weightTotal / math.Max(m.Params, 1e-9)
	skew := routerSkew(m, float64(batch), t.ep, o)
	activeW := activeWeightRead(m, batch)
	wGB := activeW * bytesPerParam / float64(t.tp)
	linearActive := math.Max(0, math.Min(m.Active, m.Params-m.EncoderParams))
	if parts.expertTotal > 0 {
		wGB = (parts.baseTotal + parts.expertRead*skew/float64(t.ep)) * bytesPerParam / float64(t.tp)
		linearActive = parts.baseTotal + parts.expertActive*skew/float64(t.ep)
	}
	wGB += o.AdapterGB / float64(t.tp)

	// ponytail: mean decode length approximates a growing cache; per-step
	// simulation is needed for exact long-generation timing.
	decodeCtx := ctx + max(0, o.OutLen-1)/2
	kvReadCtx := decodeCtx
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
	fullKeys := float64(decodeCtx)
	if m.Sparse > 0 {
		fullKeys = math.Min(fullKeys, m.Sparse)
	}
	localKeys := float64(decodeCtx)
	if localL > 0 {
		localKeys = math.Min(localKeys, float64(m.Window))
	}
	if m.Sparse > 0 {
		localKeys = math.Min(localKeys, m.Sparse)
	}
	decodeF := 2*linearActive*1e9*float64(batch) +
		4*float64(batch)*m.attentionWidth()*(float64(fullL)*fullKeys+float64(localL)*localKeys)/float64(t.cp)
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
	tLayer := layerFixedMs(m, batch, cards)
	tStep := math.Max(tMem, tCompute) + tComm + tFixed + tLayer
	p.Bottleneck = "memory"
	if tCompute > tMem {
		p.Bottleneck = "compute"
	} else if tOffload > tHBM && tOffload > tCompute {
		p.Bottleneck = "offload"
	}
	p.DecodeMemMs = round2(tMem)
	p.DecodeComputeMs = round2(tCompute)
	p.CommMs = round2(tComm)
	p.LayerMs = round2(tLayer)
	p.OffloadMs = round2(tOffload)
	p.ScheduleMs = round2(tFixed)

	specScenario := spec
	if o.SpecTau > 0 {
		specScenario.Tau = o.SpecTau
	}
	if o.SpecOvh > 0 {
		specScenario.Ovh = o.SpecOvh
	}
	g := specScenario.gain(batch)
	modelSpecOK := spec.ID != "mtp" || m.MTP || m.MTPHeads > 0
	// 内置 τ/Ovh 与用户覆盖值都是场景输入，不代表同条件实测基准。
	specOK := spec.ID == "none" || modelSpecOK
	if !specOK {
		g = 1
	}
	decodeTokens := math.Max(0, float64(o.OutLen-1))
	p.SpecApplied = decodeTokens > 0 && spec.ID != "none" && specOK
	if p.SpecApplied {
		p.Accuracy = "scenario"
	}
	single := 1000 / tStep * g
	agg := float64(batch) * 1000 / tStep * g

	// Prefix 命中 P、待算 token N 时，dense causal QK+AV =
	// 2*L*D*N*(2*(P+N)-N)。CP 在 token 维度分摊 prefill。
	inEff := float64(ctx) * (1 - o.HitRate)
	preTokens := inEff / float64(t.cp)
	preSkew := routerSkew(m, math.Max(1, preTokens), t.ep, o)
	preActive := math.Max(0, math.Min(m.Active, m.Params-m.EncoderParams))
	preParts := m.weights(max(1, int(math.Min(inEff, 1e6))))
	preRead := activeWeightRead(m, max(1, int(math.Min(inEff, 1e6))))
	if preParts.expertTotal > 0 {
		preActive = preParts.baseTotal + preParts.expertActive*preSkew/float64(t.ep)
		preRead = preParts.baseTotal + preParts.expertRead*preSkew/float64(t.ep)
	}
	tLinCompute := 2 * preActive * preTokens / (peakTF * float64(t.tp) * flopsUtil)
	preWeightGB := preRead*bytesPerParam/float64(t.tp) + o.AdapterGB/float64(t.tp)
	tWeight := preWeightGB / (h.BW * eta) * 1000
	chunks := math.Max(1, math.Ceil(preTokens/float64(o.PrefillChunk)))
	tLin := math.Max(tLinCompute, tWeight*chunks)

	var attnF float64
	if fullL > 0 {
		if m.Sparse > 0 {
			attnF += 4 * float64(fullL) * inEff * math.Min(float64(ctx), m.Sparse) * m.attentionWidth()
		} else {
			attnF += 2 * float64(fullL) * inEff * (2*float64(ctx) - inEff) * m.attentionWidth()
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
		attnF += 4 * float64(localL) * keyPairs * m.attentionWidth()
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
	kvScale := m.kvRankFactor(t.tp) / (float64(t.pp) * float64(t.cp)) * kvmf * o.KVOverhead * (1 - o.KVOffload) / 1e9
	kvCtx, shared := ctx+max(0, o.OutLen-1), int(float64(ctx)*o.HitRate)
	kvOne := m.kvBatchBytes(kvCtx, 1, shared) * kvScale
	kvPerReq := (m.kvBatchBytes(kvCtx, 2, shared)-m.kvBatchBytes(kvCtx, 1, shared))*kvScale +
		m.StateMB/1000/(float64(t.tp)*float64(t.pp))
	actPerReq := mem.Act / float64(max(1, batch))
	fixed := mem.Weights + mem.Fw + mem.Adapter + math.Max(0, kvOne-kvPerReq)
	if perRequest := kvPerReq + actPerReq; perRequest > 0 {
		p.MaxBatch = max(0, int((mem.Budget-fixed)/perRequest))
	}
	if decodeTokens == 0 {
		p.SingleTPS, p.AggTPS, p.TPOTms, p.TPM = 0, 0, 0, 0
		p.DecodeMemMs, p.DecodeComputeMs, p.CommMs = 0, 0, 0
		p.ScheduleMs, p.LayerMs, p.OffloadMs = 0, 0, 0
		p.Bottleneck, p.SpecApplied = "prefill", false
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
			localText(o.Lang, "analytical is an uncalibrated roofline; scenario uses unverified peak inputs or custom/advanced assumptions", "analytical 为未校准 roofline；scenario 使用未核验峰值或自定义/高级假设")),
		tr(localText(o.Lang, "Support", "支持状态"), p.Support, p.SupportReason),
		tr(localText(o.Lang, "Parallel topology", "并行拓扑"), p.Topology,
			localText(o.Lang, fmt.Sprintf("%d cards; the product must match", cards), fmt.Sprintf("%d cards；乘积必须相等", cards))),
		tr(localText(o.Lang, "Inference engine", "推理框架"), engineDisplay(eng, o.Lang), engNote(eng, h, p.EngOK, p.Accuracy == "scenario", o.Lang)),
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
			localText(o.Lang, fmt.Sprintf("roofline + communication + %.2fms per-layer kernels + %.2fms scheduling (%s)", tLayer, tFixed, eng.Name), fmt.Sprintf("roofline + 通信 + 层固定 %.2fms + 调度 %.2fms（%s）", tLayer, tFixed, eng.Name))),
	}
	if o.KVOffload > 0 {
		p.Trace = append(p.Trace, tr("KV offload", fmt.Sprintf("%.2f ms/step", tOffload),
			localText(o.Lang,
				fmt.Sprintf("%.0f%% of KV reread through a %.0f GB/s tier; %.1f GB external capacity", o.KVOffload*100, o.OffloadBW, mem.OffloadedKV),
				fmt.Sprintf("%.0f%% KV 经 %.0f GB/s 层级回读；外部容量 %.1f GB", o.KVOffload*100, o.OffloadBW, mem.OffloadedKV))))
	}
	if decodeTokens > 0 && spec.ID != "none" {
		note := specNote(specScenario, batch, o.Lang)
		if !modelSpecOK {
			note = localText(o.Lang, "⚠ The model has no MTP-head metadata; acceleration is not applied", "⚠ 模型没有 MTP 头元数据，本次不应用加速")
		} else if o.SpecTau <= 0 || o.SpecOvh <= 0 {
			note = localText(o.Lang, "Using the method's built-in scenario τ/overhead; validate it on the target workload", "使用档位内置的场景 τ/开销；需在目标 workload 上验证")
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
	if !t.valid {
		p.Trace = append(p.Trace, tr(localText(o.Lang, "⚠ Invalid topology", "⚠ 拓扑无效"), p.Topology,
			localText(o.Lang, "TP×PP×EP×CP must equal cards, and EP only applies to MoE; no performance fallback is emitted", "TP×PP×EP×CP 必须等于 cards，且 EP 仅适用于 MoE；不输出性能回退")))
	}
	if m.Multimodal && m.EncoderParams == 0 {
		p.Trace = append(p.Trace, tr(localText(o.Lang, "⚠ Multimodal encoder", "⚠ 多模态 encoder"), localText(o.Lang, "Unknown parameter count", "参数量未知"),
			localText(o.Lang, "The text tower is modeled; media encoder TTFT is omitted", "文本塔可计算；媒体 encoder TTFT 未计入")))
	}
	if m.Ctx > 0 && ctx+o.OutLen > m.Ctx {
		p.Trace = append(p.Trace, tr(localText(o.Lang, "⚠ Context extension", "⚠ 上下文外推"),
			localText(o.Lang, fmt.Sprintf("%dK > %dK native", (ctx+o.OutLen)/1024, m.Ctx/1024), fmt.Sprintf("%dK > 原生 %dK", (ctx+o.OutLen)/1024, m.Ctx/1024)),
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
	combined := supportAssessment{status: p.Support, reason: p.SupportReason, estimateValid: p.EstimateValid}
	p.Workload = &WorkloadStats{}
	if !o.skipTrace {
		p.Workload.Buckets = make([]WorkloadBucketPerf, len(runs))
	}
	var meanCtx, meanOut, effectiveInput, preSeconds, reqSeconds, reqLatency, ttft float64
	var decodeTokens, singleSeconds, aggregateSeconds, decodeMem, decodeCompute, comm, schedule, layer, offload, encoder float64
	maxProfileContext := 0
	p.SpecApplied = false
	for _, run := range runs {
		share := run.bucket.Share
		outTokens := math.Max(0, float64(run.bucket.Output-1))
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
		layer += share * outTokens * run.perf.LayerMs
		offload += share * outTokens * run.perf.OffloadMs
		encoder += share * run.perf.EncoderMs
		p.SpecApplied = p.SpecApplied || outTokens > 0 && run.perf.SpecApplied
		combined.add(run.perf.Support, run.perf.SupportReason, run.perf.EstimateValid)
	}
	p.Support, p.SupportReason, p.EstimateValid = combined.status, combined.reason, combined.estimateValid
	if p.SpecApplied {
		p.Accuracy = "scenario"
	}
	if !p.EstimateValid {
		for i := range runs {
			runs[i].occupancy = runs[i].bucket.Share
		}
	}
	if p.EstimateValid && decodeTokens > 0 {
		p.SingleTPS = round1(decodeTokens / singleSeconds)
		p.AggTPS = round1(decodeTokens / aggregateSeconds)
		p.TPOTms = round1(singleSeconds / decodeTokens * 1000)
		p.DecodeMemMs = round2(decodeMem / decodeTokens)
		p.DecodeComputeMs = round2(decodeCompute / decodeTokens)
		p.CommMs = round2(comm / decodeTokens)
		p.ScheduleMs = round2(schedule / decodeTokens)
		p.LayerMs = round2(layer / decodeTokens)
		p.OffloadMs = round2(offload / decodeTokens)
	} else {
		p.SingleTPS, p.AggTPS, p.TPOTms = 0, 0, 0
		p.DecodeMemMs, p.DecodeComputeMs, p.CommMs = 0, 0, 0
		p.ScheduleMs, p.LayerMs, p.OffloadMs = 0, 0, 0
	}
	if p.EstimateValid && preSeconds > 0 {
		p.PreTPS = round1(effectiveInput / preSeconds)
	} else {
		p.PreTPS = 0
	}
	p.reqSec = 0
	reqRate := 0.0
	if p.EstimateValid && reqSeconds > 0 {
		p.reqSec = reqSeconds
		reqRate = 1 / reqSeconds
		p.TTFTms = round1(ttft)
		p.ReqMs = round1(reqLatency)
		p.tPre = preSeconds * 1000
		p.EncoderMs = round2(encoder)
	} else {
		p.TTFTms, p.ReqMs, p.tPre, p.EncoderMs = 0, 0, 0, 0
	}
	p.ReqS = round4(reqRate)
	p.TPM = round1(p.AggTPS * 60)
	p.TPMMixed = round1(reqRate * (meanCtx + meanOut) * 60)
	p.Bottleneck = "unmodeled"
	if p.EstimateValid {
		p.Bottleneck = "prefill"
		if decodeTokens > 0 {
			p.Bottleneck = "memory"
			if p.DecodeComputeMs > p.DecodeMemMs {
				p.Bottleneck = "compute"
			} else if p.OffloadMs > p.DecodeMemMs-p.OffloadMs && p.OffloadMs > p.DecodeComputeMs {
				p.Bottleneck = "offload"
			}
		}
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
	if mix.Cap > 0 {
		mix.HeadPct = (mix.Cap - mix.P999Total) / mix.Cap
	}
	mix.Fit = allOneFit && mix.P999Total <= mix.Cap
	p.Mem, p.Fit = mix, mix.Fit
	p.Deployable = p.EstimateValid && p.Support == "supported" && p.Fit
	if allOneFit && p.EstimateValid {
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

	decodeRuns := make([]workloadRun, 0, len(runs))
	decodeShare := 0.0
	for _, run := range runs {
		if run.bucket.Output > 1 {
			decodeRuns = append(decodeRuns, run)
			decodeShare += run.bucket.Share
		}
	}
	for i := range decodeRuns {
		decodeRuns[i].bucket.Share /= decodeShare
	}
	ctx95 := workloadQuantile(runs, .95, func(r workloadRun) float64 { return float64(r.bucket.Context) })
	ctx99 := workloadQuantile(runs, .99, func(r workloadRun) float64 { return float64(r.bucket.Context) })
	ctx999 := workloadQuantile(runs, .999, func(r workloadRun) float64 { return float64(r.bucket.Context) })
	ttft95 := workloadQuantile(runs, .95, func(r workloadRun) float64 { return r.perf.TTFTms })
	ttft99 := workloadQuantile(runs, .99, func(r workloadRun) float64 { return r.perf.TTFTms })
	ttft999 := workloadQuantile(runs, .999, func(r workloadRun) float64 { return r.perf.TTFTms })
	req95 := workloadQuantile(runs, .95, func(r workloadRun) float64 { return r.perf.ReqMs })
	req99 := workloadQuantile(runs, .99, func(r workloadRun) float64 { return r.perf.ReqMs })
	req999 := workloadQuantile(runs, .999, func(r workloadRun) float64 { return r.perf.ReqMs })
	var tpsFloor95 workloadRun
	if len(decodeRuns) > 0 {
		tpsFloor95 = workloadQuantile(decodeRuns, .05, func(r workloadRun) float64 { return r.perf.SingleTPS })
	}
	stats := p.Workload
	stats.MeanContext, stats.MeanOutput = round1(meanCtx), round1(meanOut)
	stats.P95Context, stats.P99Context, stats.P999Context, stats.MaxContext = ctx95.bucket.Context, ctx99.bucket.Context, ctx999.bucket.Context, maxProfileContext
	if p.EstimateValid {
		stats.P95TTFTms, stats.P99TTFTms, stats.P999TTFTms = ttft95.perf.TTFTms, ttft99.perf.TTFTms, ttft999.perf.TTFTms
		stats.P95ReqMs, stats.P99ReqMs, stats.P999ReqMs = req95.perf.ReqMs, req99.perf.ReqMs, req999.perf.ReqMs
		if len(decodeRuns) > 0 {
			stats.P95SingleTPS = tpsFloor95.perf.SingleTPS
		}
	}
	if !o.skipTrace {
		for i, run := range runs {
			stats.Buckets[i] = WorkloadBucketPerf{
				Context: run.bucket.Context, Output: run.bucket.Output, Share: run.bucket.Share,
				Occupancy: run.occupancy, PrefixHit: run.bucket.PrefixHit,
				BatchMemory: run.perf.Mem.Total, Fit: run.perf.Fit,
			}
			if p.EstimateValid {
				stats.Buckets[i].TTFTms = run.perf.TTFTms
				stats.Buckets[i].ReqMs = run.perf.ReqMs
				if run.bucket.Output > 1 {
					stats.Buckets[i].SingleTPS = run.perf.SingleTPS
					stats.Buckets[i].TPOTms = run.perf.TPOTms
				}
			}
		}
	}
	if o.skipTrace {
		return p
	}

	metadata := min(4, len(p.Trace))
	if len(p.Trace) > 0 {
		p.Trace[0].V = p.Accuracy
		if !p.EstimateValid {
			p.Trace[0].N = localText(o.Lang, "Performance is withheld because at least one workload bucket is not modeled", "至少一个 workload 桶未建模，因此不输出性能")
		}
	}
	if len(p.Trace) > 1 {
		p.Trace[1].V, p.Trace[1].N = p.Support, p.SupportReason
	}
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
			localText(o.Lang, "TTFT + subsequent decode intervals × TPOT", "TTFT + 后续 decode 间隔 × TPOT")),
		tr(localText(o.Lang, "Steady-state mixed rate", "混合稳态速率"), fmt.Sprintf("%.2f req/s · %.0f tok/min", p.ReqS, p.TPMMixed),
			localText(o.Lang, "harmonic aggregation of per-bucket serial service demand", "按各桶串行服务预算做调和聚合")),
	)
	return p
}

func engNote(eng Engine, h HW, ok, scenario bool, lang string) string {
	if !ok {
		return localText(lang,
			fmt.Sprintf("⚠ %s does not list native %s support; no performance estimate is emitted", eng.Name, h.Vendor),
			fmt.Sprintf("⚠ %s 未列出 %s 原生支持；不输出性能估算", eng.Name, h.Vendor))
	}
	if scenario {
		return engineDescription(eng, lang) + localText(lang, "; using user-supplied scenario coefficients", "；使用用户输入的场景系数")
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
	rank := m.kvRankFactor(t.tp) / (float64(t.pp) * float64(t.cp))
	base := localText(o.Lang,
		fmt.Sprintf("%.1f MB raw KV/request × %d concurrent × %.3f rank ratio; %d input + %d cached output tokens", m.KVBytes(ctx+max(0, o.OutLen-1))/1e6, batch, rank, ctx, max(0, o.OutLen-1)),
		fmt.Sprintf("%.1f MB/请求原始 KV × %d 并发 × rank比例 %.3f；%d 输入 + %d 已缓存输出 token", m.KVBytes(ctx+max(0, o.OutLen-1))/1e6, batch, rank, ctx, max(0, o.OutLen-1)))
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
	if t.tp <= 1 && t.pp <= 1 && t.ep <= 1 && t.cp <= 1 {
		return localText(o.Lang, "single card; no communication", "单卡无通信")
	}
	path := fmt.Sprintf("%s %.0f GB/s", h.Link.T, o.linkBW(h))
	if h.Link.B <= 0 {
		path = fmt.Sprintf("PCIe ~%.0f GB/s", o.linkBW(h))
	}
	return t.String() + localText(o.Lang, " over ", " 走 ") + path
}

func gb(v float64) string { return fmt.Sprintf("%.1f GB", v) }
func clamp(v, lo, hi float64) float64 {
	if math.IsNaN(v) {
		return lo
	}
	return math.Min(hi, math.Max(lo, v))
}
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
	Quant         string  `json:"quant"`
	Fit           int     `json:"fit"` // Memory only: 0=over capacity, 1=<=10% headroom, 2=>10% headroom.
	TPS           float64 `json:"tps"`
	Accel         bool    `json:"accel"`
	Applicable    bool    `json:"applicable"`
	Support       string  `json:"support"`
	SupportReason string  `json:"support_reason"`
}

type FitRow struct {
	Model Model     `json:"model"`
	Cells []FitCell `json:"cells"`
}

func FitMatrix(h HW, models []Model, n int, workload []WorkloadBucket, batch int, o Opts) []FitRow {
	o.skipTrace = true
	quants := MainQuants()
	rows := make([]FitRow, 0, len(models))
	for _, m := range models {
		row := FitRow{Model: m}
		for _, q := range quants {
			if fixed := m.FixedQuantID(); fixed != "" && fixed != q.ID {
				row.Cells = append(row.Cells, FitCell{Quant: q.ID})
				continue
			}
			p := ThroughputWorkload(h, m, q, workload, batch, n, o)
			st, reason := 0, p.SupportReason
			switch {
			case !p.Fit:
				reason = joinWarn(reason, localText(o.Lang, "Workload exceeds the memory budget", "工作负载超过显存预算"))
			case p.Mem.HeadPct > 0.10:
				st = 2
			default:
				st = 1
			}
			tps := 0.0
			if st > 0 && p.EstimateValid {
				tps = p.SingleTPS
			}
			row.Cells = append(row.Cells, FitCell{
				Quant: q.ID, Fit: st, TPS: tps, Accel: p.Accel, Applicable: true,
				Support: p.Support, SupportReason: reason,
			})
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
	Accuracy      string  `json:"accuracy"`
	HW            HW      `json:"hw"`
	N             int     `json:"n"`        // 单副本卡数
	Replicas      int     `json:"replicas"` // 副本（节点）数
	Quant         string  `json:"quant"`
	Topology      string  `json:"topology"`
	QName         string  `json:"qname"`
	EngName       string  `json:"eng_name"`
	SpecName      string  `json:"spec_name"`
	Strategy      string  `json:"strategy"`
	Single        float64 `json:"single_tps"`
	Agg           float64 `json:"agg_tps"`      // 单副本聚合 tok/s（容量场景并发口径）
	TPM           float64 `json:"tpm"`          // 集群混合 tok/min 容量
	CapacityQPS   float64 `json:"capacity_qps"` // 集群最大稳定请求/s
	ArrivalQPS    float64 `json:"arrival_qps"`  // 目标 TPM 换算的到达请求/s
	MaxConc       int     `json:"max_conc"`     // 容量场景的单副本并发
	UtilPct       float64 `json:"util_pct"`     // 目标负载 / 集群服务能力
	WaitAvgMs     float64 `json:"wait_avg_ms"`  // M/M/c 平均排队等待
	WaitP95Ms     float64 `json:"wait_p95_ms"`  // M/M/c 无条件 p95 排队等待
	QueueModel    string  `json:"queue_model"`  // none | M/M/c
	TTFTms        float64 `json:"ttft_ms"`
	TPOTms        float64 `json:"tpot_ms"`
	P95SingleTPS  float64 `json:"p95_single_tps"` // 95% 请求可达到的单流 TPS 下限
	TTFTP95ms     float64 `json:"p95_ttft_ms"`
	ReqP95ms      float64 `json:"p95_req_ms"`
	ReqP99ms      float64 `json:"p99_req_ms"`
	MeanContext   float64 `json:"mean_context"`
	P99Context    int     `json:"p99_context"`
	P999Context   int     `json:"p999_context"`
	MaxContext    int     `json:"max_context"`
	MemoryP999    float64 `json:"p999_memory"`
	CostCNY       float64 `json:"cost_cny"`
	Monthly       float64 `json:"monthly"`
	PerMtok       float64 `json:"per_mtok"` // 每百万 token 成本（按集群实际容量满负载）
	Warn          string  `json:"warn,omitempty"`
	Support       string  `json:"support"`
	SupportReason string  `json:"support_reason"`
	Deployable    bool    `json:"deployable"`
}

// ---------- 确定性处方推荐 ----------

// RecommendOpts 描述部署处方搜索的两个方向与目标组合。
// 只使用同一计算器内部的确定性公式与剪枝规则，不调用外部 LLM。
type RecommendOpts struct {
	Direction  string  `json:"direction"`  // model | card
	HW         string  `json:"hw"`         // direction=card 时的固定硬件
	Cards      int     `json:"cards"`      // 已持有卡数（模型模式也可作为最大单副本卡数）
	Objectives string  `json:"objectives"` // cost | tos | tpm | avail，逗号分隔最多两个
	TargetTPM  float64 `json:"tpm"`
	MinTOS     float64 `json:"tos"`
	QuantOnly  string  `json:"quant_only"`
	Queue      bool    `json:"queue"`
	MaxQ       int     `json:"maxq"`
	Conc       int     `json:"conc"`
	Limit      int     `json:"limit"`
}

// Prescription 是一个可执行配置：某个量化×推理栈×拓扑×并发的最终结果。
type Prescription struct {
	Plan           Plan    `json:"plan"`
	ModelID        string  `json:"model_id"`
	ModelName      string  `json:"model_name"`
	EngineID       string  `json:"engine_id"`
	KVQuant        string  `json:"kvq"`
	SpecID         string  `json:"spec"`
	QuantLocked    bool    `json:"quant_locked"`
	HWAccel        bool    `json:"hw_accel"`
	EngineOK       bool    `json:"engine_ok"`
	TopologyOK     bool    `json:"topology_ok"`
	WorkPerReplica float64 `json:"per_replica_tpm"` // 单副本混合 TPM
	Topology       string  `json:"topology"`
	PeakTF         float64 `json:"peak_tf"`
	Accuracy       string  `json:"accuracy"`
	MemoryP999GB   float64 `json:"p999_gb"`
	HeadroomPct    float64 `json:"headroom_pct"`
	Score          float64 `json:"score"`
	ObjectiveWins  int     `json:"objective_wins"`
	Reason         string  `json:"reason"`
	Advice         string  `json:"advice"`
	Explain        string  `json:"explain"`
}

// RecommendResult 按用户选中的 1–2 个目标返回 Pareto/处方排序。
type RecommendResult struct {
	Objectives []string       `json:"objectives"`
	Limit      int            `json:"limit"`
	Pareto     []Prescription `json:"pareto"`
	Picks      []Prescription `json:"picks"`
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
	if po.Queue {
		po.MaxQ = max(po.MaxQ, conc)
	}
	maxContext, maxSequence := 0, 0
	for _, bucket := range workload {
		maxContext = max(maxContext, bucket.Context)
		maxSequence = max(maxSequence, bucket.Context+bucket.Output)
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
	plans := make([]Plan, 0)
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
				if !pf.EstimateValid || pf.Support == "unsupported" || pf.Support == "unknown" || !pf.Fit {
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
						if pc.EstimateValid && pc.Support != "unsupported" && pc.Support != "unknown" &&
							pc.Fit && pc.Workload.P95SingleTPS >= po.MinTOS {
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
				requiredReplicas := math.Ceil(po.TargetTPM / capTPM)
				if requiredReplicas < 1 || requiredReplicas > maxReplicas || math.IsNaN(requiredReplicas) {
					continue
				}
				replicas := int(requiredReplicas)
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
					CostCNY:    h.CNY * totalCards, Support: servicePerf.Support,
					Topology: servicePerf.Topology,
					Accuracy: servicePerf.Accuracy, SupportReason: servicePerf.SupportReason, Deployable: servicePerf.Deployable,
				}
				p.Strategy = strategy(h, n, st.Lang)
				elec := h.TDP * totalCards * 0.6 * 24 * 30 / 1000 * 0.8 // 60% 负载，0.8 元/kWh
				if p.CostCNY > 0 {
					p.Monthly = p.CostCNY/36 + elec
					p.PerMtok = p.Monthly / (clusterTPM * 60 * 24 * 30 / 1e6)
				}
				p.Warn = warnOf(h, m, q, pf, st.Lang)
				p.Warn = joinWarn(p.Warn, p.SupportReason)
				if m.Ctx > 0 && maxSequence > m.Ctx {
					p.Warn = joinWarn(p.Warn, localText(st.Lang,
						fmt.Sprintf("The workload tail exceeds the model's native context (%dK>%dK); YaRN/RoPE extension required", maxSequence/1024, m.Ctx/1024),
						fmt.Sprintf("工作负载尾部超过模型原生上下文（%dK>%dK），需 YaRN/RoPE 外推", maxSequence/1024, m.Ctx/1024)))
				}
				if st.KVQuant != "fp16" && !st.kvSupported(h, eng) {
					p.Warn = joinWarn(p.Warn, localText(st.Lang, "The hardware/engine does not support this KV format; capacity and reads use FP16", "所选硬件/引擎不支持该 KV 格式，容量与读取均按 FP16"))
				}
				if st.Spec == "mtp" && !m.MTP && m.MTPHeads == 0 {
					p.Warn = joinWarn(p.Warn, localText(st.Lang, "The model has no MTP-head metadata; speculative acceleration is not applied", "模型无 MTP 头元数据，未应用推测加速"))
				}
				if st.Spec != "" && st.Spec != "none" && (st.SpecTau <= 0 || st.SpecOvh <= 0) {
					p.Warn = joinWarn(p.Warn, localText(st.Lang, "Using built-in speculative-decoding scenario coefficients; validate them on the target workload", "使用内置推测解码场景系数；需在目标 workload 上验证"))
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

// dedupPlans 只合并完全相同的部署形状；卡数、副本、并发、拓扑或运行栈
// 不同都代表用户可执行的不同配置，不能互相覆盖。
func dedupPlans(plans []Plan, objective string) []Plan {
	better := planBetter(objective)
	at := map[string]int{}
	out := plans[:0]
	for _, p := range plans {
		key := fmt.Sprintf("%s|%s|%s|%s|%d|%d|%d|%s",
			p.HW.ID, p.Quant, p.EngName, p.SpecName, p.N, p.Replicas, p.MaxConc, p.Topology)
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
func planP95(p Plan) float64 {
	return p.ReqP95ms + p.WaitP95Ms
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
			if planP95(a) != planP95(b) {
				return planP95(a) < planP95(b)
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
			return planP95(a) < planP95(b)
		}
	}
}

func sortPlans(plans []Plan, objective string) {
	better := planBetter(objective)
	sort.SliceStable(plans, func(i, j int) bool { return better(plans[i], plans[j]) })
}

func parseRecommendObjectives(s string) []string {
	var out []string
	seen := map[string]bool{}
	for _, part := range strings.Split(strings.ToLower(s), ",") {
		part = strings.TrimSpace(part)
		if part == "" || seen[part] {
			continue
		}
		switch part {
		case "cost", "tos", "tpm", "avail":
			out = append(out, part)
			seen[part] = true
		}
	}
	if len(out) > 2 {
		out = out[:2]
	}
	if len(out) == 0 {
		out = []string{"cost"}
	}
	return out
}

func recommendPlanningObjective(objectives []string) string {
	if len(objectives) == 0 {
		return "cost"
	}
	switch objectives[0] {
	case "tos", "tpm":
		return "latency"
	case "avail":
		return "avail"
	default:
		return "cost"
	}
}

func planMonthly(p Plan) float64 {
	if p.Monthly <= 0 {
		return 1e12
	}
	return p.Monthly
}

func rankPrescriptions(items []Prescription, objectives []string) {
	// 每个目标先按候选集合做 0..1 归一，再平均；目标越多，分数越稳健。
	if len(items) == 0 {
		return
	}
	costVals := make([]float64, 0, len(items))
	tosVals := make([]float64, len(items))
	tpmVals := make([]float64, len(items))
	availVals := make([]float64, len(items))
	for i := range items {
		if cost := items[i].Plan.Monthly; cost > 0 {
			costVals = append(costVals, cost)
		}
		tosVals[i] = items[i].Plan.P95SingleTPS
		tpmVals[i] = items[i].Plan.TPM
		availVals[i] = availScoreOf(items[i].Plan)
	}
	for i := range items {
		score := 0.0
		for _, obj := range objectives {
			switch obj {
			case "cost":
				if cost := items[i].Plan.Monthly; cost > 0 {
					score += normalizeLower(costVals, cost)
				}
			case "tos":
				score += normalizeHigher(tosVals, items[i].Plan.P95SingleTPS)
			case "tpm":
				score += normalizeHigher(tpmVals, items[i].Plan.TPM)
			case "avail":
				score += normalizeHigher(availVals, availScoreOf(items[i].Plan))
			}
		}
		if len(objectives) > 0 {
			score /= float64(len(objectives))
		}
		items[i].Score = round4(score)
		wins := 0
		for _, obj := range objectives {
			if obj == "cost" && items[i].Plan.Monthly <= 0 {
				continue
			}
			best := true
			for j := range items {
				if j == i {
					continue
				}
				if prescriptionMetric(items[j], obj) > prescriptionMetric(items[i], obj) {
					best = false
					break
				}
			}
			if best {
				wins++
			}
		}
		items[i].ObjectiveWins = wins
	}
}

func normalizeLower(vals []float64, v float64) float64 {
	minv := math.Inf(1)
	for _, x := range vals {
		if x < minv {
			minv = x
		}
	}
	if math.IsInf(minv, 0) || math.IsNaN(minv) || minv <= 0 {
		return 0
	}
	return minv / v
}

func normalizeHigher(vals []float64, v float64) float64 {
	maxv := 0.0
	for _, x := range vals {
		if x > maxv {
			maxv = x
		}
	}
	if maxv <= 0 {
		return 0
	}
	return v / maxv
}

func prescriptionAdvice(p Prescription, st Opts, lang string) string {
	var a []string
	if !p.EngineOK {
		a = append(a, localText(lang, "Engine compatibility is weak", "引擎兼容性较弱"))
	}
	if !p.HWAccel && !p.QuantLocked {
		a = append(a, localText(lang, "This card does not accelerate the selected weight precision; use it for memory savings, not compute speed", "该卡不加速所选权重精度；它主要省显存，不提速"))
	}
	if st.KVQuant != "fp16" && !st.kvSupported(p.Plan.HW, resolveEngine(st.Engine, p.Plan.HW, QuantByID(p.Plan.Quant))) {
		a = append(a, localText(lang, "Selected KV format falls back to FP16 accounting; re-check support before buying", "所选 KV 格式按 FP16 回退；采购前务必确认支持"))
	}
	if p.Plan.Replicas == 1 {
		a = append(a, localText(lang, "Single replica means one failure takes the service down", "单副本意味着一个故障就会让服务不可用"))
	}
	if p.Plan.UtilPct >= 80 {
		a = append(a, localText(lang, "Target load is close to capacity; leave burst and tail latency headroom", "目标负载已接近容量，需为突发和尾延迟留余量"))
	}
	if p.Plan.MemoryP999 > 0 && p.HeadroomPct < 0.1 {
		a = append(a, localText(lang, "Memory margin is tight; lower concurrency or KV precision on a supported path", "显存余量偏紧；建议降并发，或在受支持路径下降 KV 精度"))
	}
	if len(a) == 0 {
		a = append(a, localText(lang, "This preset is internally consistent; still validate with a matching benchmark before SLA or procurement", "该处方在规则内自洽；采购或承诺 SLA 前仍需用同口径基准压测"))
	}
	a = append(a, localText(lang,
		fmt.Sprintf("KV %s is included in the estimate when supported", strings.ToUpper(p.KVQuant)),
		fmt.Sprintf("KV %s 在支持时已计入估算", strings.ToUpper(p.KVQuant))))
	return strings.Join(a, "；")
}

func prescriptionFromPlan(p Plan, st Opts, lang string) Prescription {
	eng := resolveEngine(st.Engine, p.HW, QuantByID(p.Quant))
	kvQuant, specID := st.KVQuant, st.Spec
	if kvQuant == "" {
		kvQuant = "fp16"
	}
	if specID == "" {
		specID = "none"
	}
	return Prescription{
		Plan:           p,
		EngineID:       eng.ID,
		KVQuant:        kvQuant,
		SpecID:         specID,
		QuantLocked:    false,
		HWAccel:        false,
		EngineOK:       true,
		TopologyOK:     true,
		WorkPerReplica: p.TPM / float64(max(1, p.Replicas)),
		Accuracy:       "analytical",
		MemoryP999GB:   round1(p.MemoryP999),
	}
}

func prescriptionReason(p Prescription, objectives []string, lang string) string {
	parts := []string{
		localText(lang,
			fmt.Sprintf("Pick %s: %s + %s, %s, TP%d × %d replicas", p.Plan.HW.Name, p.Plan.QName, p.Plan.EngName, p.Plan.Strategy, p.Plan.N, p.Plan.Replicas),
			fmt.Sprintf("推荐 %s：%s + %s，%s，TP%d × %d 副本", p.Plan.HW.Name, p.Plan.QName, p.Plan.EngName, p.Plan.Strategy, p.Plan.N, p.Plan.Replicas)),
		localText(lang,
			fmt.Sprintf("total cards %d, per-replica concurrency %d, mixed capacity %.1f tok/min", p.Plan.N*p.Plan.Replicas, p.Plan.MaxConc, p.Plan.TPM),
			fmt.Sprintf("总卡 %d 张、单副本并发 %d、混合容量 %.1f tok/min", p.Plan.N*p.Plan.Replicas, p.Plan.MaxConc, p.Plan.TPM)),
	}
	for _, obj := range objectives {
		switch obj {
		case "cost":
			if p.Plan.Monthly > 0 {
				parts = append(parts, localText(lang, fmt.Sprintf("cost-first (estimated monthly %.1f CNY)", p.Plan.Monthly), fmt.Sprintf("成本优先（估算月成本 %.1f 元）", p.Plan.Monthly)))
			} else {
				parts = append(parts, localText(lang, "price unknown; cost cannot be ranked", "价格未知，无法比较成本"))
			}
		case "tos":
			parts = append(parts, localText(lang, fmt.Sprintf("stream-speed-first (P95 single-stream %.1f tok/s)", p.Plan.P95SingleTPS), fmt.Sprintf("单流速度优先（P95 单流 %.1f tok/s）", p.Plan.P95SingleTPS)))
		case "tpm":
			parts = append(parts, localText(lang, fmt.Sprintf("throughput-first (%.1f tok/min)", p.Plan.TPM), fmt.Sprintf("吞吐优先（%.1f tok/min）", p.Plan.TPM)))
		case "avail":
			parts = append(parts, localText(lang, "availability-first (replica redundancy + enterprise class)", "可用性优先（副本冗余 + 企业级硬件）"))
		}
	}
	if p.HWAccel {
		parts = append(parts, localText(lang, fmt.Sprintf("%s is hardware-accelerated on this card", p.Plan.QName), fmt.Sprintf("%s 在该卡有原生加速", p.Plan.QName)))
	} else if p.QuantLocked {
		parts = append(parts, localText(lang, fmt.Sprintf("%s is a locked format; this card is treated as compatible/fallback", p.Plan.QName), fmt.Sprintf("%s 是锁定格式，该卡按兼容/回退口径计算", p.Plan.QName)))
	}
	return strings.Join(parts, "；")
}

func fillPrescriptionPerf(pres *Prescription, pf Perf) {
	pres.QuantLocked = pres.QuantLocked || pf.QuantLocked
	pres.HWAccel = pf.Accel
	pres.EngineOK = pf.EngOK
	pres.TopologyOK = pf.TopologyOK
	pres.Topology = pf.Topology
	pres.PeakTF = round1(pf.PeakTF)
	pres.Accuracy = pf.Accuracy
	pres.MemoryP999GB = round1(pf.Mem.P999Total)
	if pf.Mem.Cap > 0 {
		pres.HeadroomPct = round4(math.Max(0, (pf.Mem.Cap-pf.Mem.P999Total)/pf.Mem.Cap))
	}
}

// recommendForModel 从模型出发，枚举硬件×量化×副本，返回处方而非“模型可装哪里”。
func recommendForModel(hws []HW, m Model, workload []WorkloadBucket, opts RecommendOpts, st Opts) []Prescription {
	objectives := parseRecommendObjectives(opts.Objectives)
	po := PlanOpts{
		TargetTPM: opts.TargetTPM,
		MinTOS:    opts.MinTOS,
		Objective: recommendPlanningObjective(objectives),
		Queue:     opts.Queue,
		MaxQ:      opts.MaxQ,
		QuantOnly: opts.QuantOnly,
	}
	if po.TargetTPM <= 0 {
		po.TargetTPM = 6000
	}
	if po.MaxQ <= 0 {
		po.MaxQ = 256
	}
	if opts.Cards <= 0 {
		opts.Cards = 1
	}

	var out []Prescription
	for _, h := range hws {
		if h.Svc {
			continue
		}
		// 先按现有 Planner 求一个合规集合，再补处方所需的 Perf 细节。
		plans := Planner([]HW{h}, m, po, workload, max(1, opts.Conc), st)
		// Include both ordinary and N+1 configurations for a paired availability goal.
		if len(objectives) == 2 && slices.Contains(objectives, "avail") {
			other := po
			if po.Objective == "avail" {
				other.Objective = "cost"
			} else {
				other.Objective = "avail"
			}
			plans = append(plans, Planner([]HW{h}, m, other, workload, max(1, opts.Conc), st)...)
		}
		for _, p := range plans {
			pf := ThroughputWorkload(h, m, QuantByID(p.Quant), workload, p.MaxConc, p.N, st)
			if !pf.EstimateValid || pf.Support == "unsupported" || pf.Support == "unknown" || !pf.Fit || pf.Workload == nil {
				continue
			}
			pres := prescriptionFromPlan(p, st, st.Lang)
			pres.ModelID = m.ID
			pres.ModelName = m.Name
			pres.QuantLocked = m.FixedQuantID() != ""
			fillPrescriptionPerf(&pres, pf)
			pres.Reason = prescriptionReason(pres, objectives, st.Lang)
			pres.Advice = prescriptionAdvice(pres, st, st.Lang)
			out = append(out, pres)
		}
	}
	return out
}

// recommendForCard 从硬件出发，枚举模型×量化×栈，返回可部署的处方集合。
func recommendForCard(models []Model, h HW, workload []WorkloadBucket, opts RecommendOpts, st Opts) []Prescription {
	objectives := parseRecommendObjectives(opts.Objectives)
	if opts.Cards <= 0 {
		opts.Cards = 1
	}
	var out []Prescription
	for _, m := range models {
		if m.Conf != "official" {
			continue // 卡片模式只推荐人工收录的官方模型，避免采集仓库噪声
		}
		quants := Quants
		if fixed := m.FixedQuantID(); fixed != "" {
			quants = []Quant{QuantByID(fixed)}
		} else if opts.QuantOnly != "" {
			quants = nil
			for _, q := range Quants {
				if q.ID == opts.QuantOnly {
					quants = append(quants, q)
				}
			}
			if len(quants) == 0 {
				quants = Quants
			}
		}
		for _, q := range quants {
			eng := resolveEngine(st.Engine, h, q)
			if !eng.EngineOK(h) {
				continue
			}
			pf := ThroughputWorkload(h, m, q, workload, max(1, opts.Conc), opts.Cards, st)
			if !pf.EstimateValid || pf.Support == "unsupported" || pf.Support == "unknown" || !pf.Fit || pf.Workload == nil {
				continue
			}
			p := Plan{
				HW: h, N: opts.Cards, Replicas: 1,
				Strategy: strategy(h, opts.Cards, st.Lang),
				Quant:    q.ID, QName: q.Name,
				EngName: engineDisplay(eng, st.Lang), SpecName: specDisplay(SpecByID(st.Spec), st.Lang),
				Single: pf.SingleTPS, Agg: pf.AggTPS, TPM: round1(pf.TPMMixed),
				CapacityQPS: round4(1 / math.Max(pf.reqSec, 1e-9)), ArrivalQPS: 0,
				Topology: pf.Topology,
				MaxConc:  max(1, opts.Conc), UtilPct: 0,
				QueueModel: "none",
				TTFTms:     pf.TTFTms, TPOTms: pf.TPOTms,
				P95SingleTPS: pf.Workload.P95SingleTPS, TTFTP95ms: pf.Workload.P95TTFTms,
				ReqP95ms: pf.Workload.P95ReqMs, ReqP99ms: pf.Workload.P99ReqMs,
				MeanContext: pf.Workload.MeanContext, P99Context: pf.Workload.P99Context,
				P999Context: pf.Workload.P999Context, MaxContext: pf.Workload.MaxContext,
				MemoryP999: pf.Mem.P999Total,
				CostCNY:    h.CNY * float64(opts.Cards), Support: pf.Support,
				Accuracy: pf.Accuracy, SupportReason: pf.SupportReason, Deployable: pf.Deployable,
			}
			elec := h.TDP * float64(opts.Cards) * 0.6 * 24 * 30 / 1000 * 0.8
			if p.CostCNY > 0 {
				p.Monthly = p.CostCNY/36 + elec
				if pf.TPMMixed > 0 {
					p.PerMtok = p.Monthly / (pf.TPMMixed * 60 * 24 * 30 / 1e6)
				}
			}
			p.Warn = warnOf(h, m, q, pf, st.Lang)
			p.Warn = joinWarn(p.Warn, p.SupportReason)
			pres := prescriptionFromPlan(p, st, st.Lang)
			pres.ModelID = m.ID
			pres.ModelName = m.Name
			pres.QuantLocked = m.FixedQuantID() != ""
			fillPrescriptionPerf(&pres, pf)
			pres.Reason = prescriptionReason(pres, objectives, st.Lang)
			pres.Advice = prescriptionAdvice(pres, st, st.Lang)
			out = append(out, pres)
		}
	}
	return out
}

func dedupePrescriptions(items []Prescription) []Prescription {
	seen := map[string]int{}
	out := items[:0]
	for _, p := range items {
		key := fmt.Sprintf("%s|%s|%s|%s|%s|%s|%d|%d|%d|%s",
			p.ModelID, p.Plan.HW.ID, p.Plan.Quant, p.EngineID, p.KVQuant, p.SpecID,
			p.Plan.N, p.Plan.Replicas, p.Plan.MaxConc, p.Topology)
		if i, ok := seen[key]; ok {
			if p.Plan.TPM > out[i].Plan.TPM || (p.Plan.TPM == out[i].Plan.TPM && planCost(p.Plan) < planCost(out[i].Plan)) {
				out[i] = p
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, p)
	}
	return out
}

// Recommend 生成确定性处方。direction=model 表示“这个模型怎么配”，direction=card 表示“这张卡能部署什么”。
func Recommend(hws []HW, models []Model, m Model, workload []WorkloadBucket, opts RecommendOpts, st Opts) RecommendResult {
	objectives := parseRecommendObjectives(opts.Objectives)
	workload = normalizeWorkload(workload)
	if len(workload) == 0 {
		return RecommendResult{Objectives: objectives, Limit: opts.Limit}
	}

	var items []Prescription
	if opts.Direction == "card" {
		if opts.Cards <= 0 {
			opts.Cards = 1
		}
		for _, h := range hws {
			if h.ID == opts.HW {
				items = recommendForCard(models, h, workload, opts, st)
				break
			}
		}
	} else {
		items = recommendForModel(hws, m, workload, opts, st)
	}
	if len(items) == 0 {
		return RecommendResult{Objectives: objectives, Limit: opts.Limit}
	}

	items = dedupePrescriptions(items)
	rankPrescriptions(items, objectives)
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].Score != items[j].Score {
			return items[i].Score > items[j].Score
		}
		if items[i].Plan.TPM != items[j].Plan.TPM {
			return items[i].Plan.TPM > items[j].Plan.TPM
		}
		if planCost(items[i].Plan) != planCost(items[j].Plan) {
			return planCost(items[i].Plan) < planCost(items[j].Plan)
		}
		if planP95(items[i].Plan) != planP95(items[j].Plan) {
			return planP95(items[i].Plan) < planP95(items[j].Plan)
		}
		if items[i].Plan.HW.ID != items[j].Plan.HW.ID {
			return items[i].Plan.HW.ID < items[j].Plan.HW.ID
		}
		return items[i].Plan.Quant < items[j].Plan.Quant
	})

	// Pareto：保留在任一目标下不可支配的方案，避免“单一答案”误导。
	pareto := make([]Prescription, 0, len(items))
	for i, a := range items {
		dominated := false
		for j, b := range items {
			if i == j {
				continue
			}
			betterOrEqual := true
			strict := false
			for _, obj := range objectives {
				av, bv := prescriptionMetric(a, obj), prescriptionMetric(b, obj)
				if bv > av+1e-12 {
					strict = true
				}
				if av > bv+1e-12 {
					betterOrEqual = false
					break
				}
			}
			if betterOrEqual && strict {
				dominated = true
				break
			}
		}
		if !dominated {
			pareto = append(pareto, a)
		}
	}
	if len(pareto) == 0 {
		pareto = items
	}

	// 输出上限只影响展示，不影响内部搜索。
	limit := opts.Limit
	if limit <= 0 {
		limit = 12
	}
	if limit > 50 {
		limit = 50
	}
	if len(pareto) > limit {
		if len(objectives) > 1 {
			// Preserve an endpoint for each objective before filling the display
			// limit; equal-cost quant variants must not hide the other tradeoff.
			endpoints := make([]Prescription, 0, len(objectives)+len(pareto))
			for _, objective := range objectives {
				best := pareto[0]
				for _, candidate := range pareto[1:] {
					if prescriptionMetric(candidate, objective) > prescriptionMetric(best, objective) {
						best = candidate
					}
				}
				endpoints = append(endpoints, best)
			}
			pareto = dedupePrescriptions(append(endpoints, pareto...))
		}
		pareto = pareto[:limit]
	}
	if len(items) > limit {
		items = items[:limit]
	}
	return RecommendResult{Objectives: objectives, Limit: limit, Pareto: pareto, Picks: items}
}

func prescriptionMetric(p Prescription, objective string) float64 {
	switch objective {
	case "cost":
		return -planCost(p.Plan)
	case "tos":
		return p.Plan.P95SingleTPS
	case "tpm":
		return p.Plan.TPM
	case "avail":
		return availScoreOf(p.Plan)
	default:
		return 0
	}
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

// QuickTable 给出扣除服务预留与固定 3.5 GB 后的权重预算，不含模型相关 KV/激活。
func QuickTable(hws []HW) []QuickRow {
	var rows []QuickRow
	for _, h := range hws {
		if h.Svc {
			continue
		}
		budget := h.CapGB(Engines[1]) - 3.5 // 按 vLLM 0.90 服务预算
		rows = append(rows, QuickRow{HW: h, MaxFP16: math.Max(0, budget) / 2.0, MaxINT4: math.Max(0, budget) / 0.6})
	}
	return rows
}
