package calc

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func hw(id string, vram, bw float64) HW {
	return HW{ID: id, Name: id, VRAM: vram, BW: bw, TF: 300, Prec: []string{"fp16", "bf16", "fp8", "int8"}}
}
func singleWorkload(ctx, output int) []WorkloadBucket {
	return []WorkloadBucket{{Context: ctx, Output: output, Share: 1}}
}

var (
	hw4090  = HW{ID: "rtx4090", Vendor: "nvidia", VRAM: 24, BW: 1008, TF: 330, Prec: []string{"fp16", "bf16", "fp8", "int8"}}
	h100    = HW{ID: "h100", Vendor: "nvidia", Arch: "hopper", VRAM: 80, BW: 3350, TF: 989, Prec: []string{"fp16", "bf16", "fp8", "int8"}}
	h200    = HW{ID: "h200", Vendor: "nvidia", Arch: "hopper", VRAM: 141, BW: 4800, TF: 989, Prec: []string{"fp16", "bf16", "fp8", "int8"}, Link: Link{T: "nvlink", B: 900, Dom: 8}}
	m3ultra = HW{ID: "m3u", Vendor: "apple", Arch: "apple-m3", VRAM: 512, BW: 819, TF: 68, Prec: []string{"fp16", "bf16"}, Unified: true}
)

var (
	qwen7b  = Model{ID: "qwen2.5-7b", Params: 7.6, Active: 7.6, Layers: 28, Hidden: 3584, KVT: "gqa", KVH: 4, Dim: 128}
	llama8b = Model{ID: "llama-3.1-8b", Params: 8, Active: 8, Layers: 32, Hidden: 4096, KVT: "gqa", KVH: 8, Dim: 128, Ctx: 131072}
	llama70 = Model{ID: "llama-3.1-70b", Params: 70, Active: 70, Layers: 80, Hidden: 8192, KVT: "gqa", KVH: 8, Dim: 128}
	r1      = Model{ID: "deepseek-r1", Params: 671, Active: 37, Layers: 61, Hidden: 7168, KVT: "mla", MLA: 576, MoE: true, Experts: 256, TopK: 8, MTP: true}
)

func inRange(t *testing.T, name string, v, lo, hi float64) {
	t.Helper()
	if v < lo || v > hi {
		t.Errorf("%s = %.1f，期望落在 [%.0f, %.0f]", name, v, lo, hi)
	}
}

// 代表性部署只验证容量与有限输出。吞吐锚点必须来自同模型、检查点、
// 引擎版本、并发和提示分布的实测，不能把社区异构数字写成回归真值。
func TestRepresentativeDeployments(t *testing.T) {
	cases := []struct {
		name       string
		h          HW
		m          Model
		q          string
		ctx, b, tp int
		o          Opts
	}{
		{"7B INT4 / 4090", hw4090, qwen7b, "int4", 4096, 1, 1, Opts{}},
		{"8B FP16 / H100", h100, llama8b, "fp16", 4096, 1, 1, Opts{}},
		{"70B EXL3 / 2x4090", hw4090, llama70, "exl3", 4096, 1, 2, Opts{Engine: "exllama"}},
		{"R1 FP8 / 8xH200", h200, r1, "fp8", 4096, 1, 8, Opts{}},
		{"70B MLX4 / M3 Ultra", m3ultra, llama70, "mlx4", 4096, 1, 1, Opts{Engine: "mlx"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := Throughput(tc.h, tc.m, QuantByID(tc.q), tc.ctx, tc.b, tc.tp, tc.o)
			if !p.Fit || p.SingleTPS <= 0 || math.IsNaN(p.SingleTPS) || math.IsInf(p.SingleTPS, 0) {
				t.Fatalf("代表配置应可估算且有限: %+v", p)
			}
		})
	}
}

// 显存：70B FP16 单卡 4090 必须装不下；INT4 必须装下
func TestMemoryFeasibility(t *testing.T) {
	if Memory(hw4090, llama70, QuantByID("fp16"), 8192, 8, 1, Opts{}).Fit {
		t.Error("70B FP16 不应能单卡 4090")
	}
	if Memory(hw4090, llama70, QuantByID("int4"), 8192, 8, 1, Opts{}).Fit {
		t.Error("70B INT4 不应能单卡 4090")
	}
	// 修正系统预留重复扣减后，2×24G 的 70B INT4 可装入，但余量不足 10%，
	// FitMatrix 必须标为贴边而不是稳妥。
	edge := Memory(hw4090, llama70, QuantByID("int4"), 4096, 1, 2, Opts{})
	if !edge.Fit || edge.HeadPct > 0.10 {
		t.Errorf("70B INT4 / 2×4090 应为贴边可行，得 fit=%v head=%.1f%%", edge.Fit, edge.HeadPct*100)
	}
	if got := FitMatrix(hw4090, []Model{llama70}, 2, 4096, 1, Opts{})[0].Cells[2].Fit; got != 1 {
		t.Errorf("70B INT4 / 2×4090 应显示警告态，得 %d", got)
	}
	if !Memory(hw4090, llama70, QuantByID("exl3"), 4096, 1, 2, Opts{Engine: "exllama"}).Fit {
		t.Error("70B EXL3 应能以 ExLlamaV3 在 2×4090 低并发贴边部署")
	}
	if Memory(hw4090, llama70, QuantByID("exl3"), 8192, 8, 2, Opts{Engine: "exllama"}).Fit {
		t.Error("70B EXL3 @ 8K×8 并发不应仍报告可行")
	}
	// R1 FP16 任何单卡都不行，8×H200 FP8 才行
	if Memory(h200, r1, QuantByID("fp16"), 8192, 8, 1, Opts{}).Fit {
		t.Error("R1 FP16 不应能单卡")
	}
	if !Memory(h200, r1, QuantByID("fp8"), 8192, 8, 8, Opts{}).Fit {
		t.Error("R1 FP8 应能 8×H200")
	}
}

// KV：Llama3-70B 应约 320KB/token，DeepSeek MLA 应约 70KB/token
func TestKVTok(t *testing.T) {
	kv70 := llama70.KVTokBytes()
	inRange(t, "70B KV/tok KB", kv70/1e3, 300, 350)
	kvR1 := r1.KVTokBytes()
	inRange(t, "R1 MLA KV/tok KB", kvR1/1e3, 60, 80)
}

func TestSlidingWindowKV(t *testing.T) {
	m := Model{Params: 0.001, Active: 0.001, Layers: 6, Hidden: 2048, KVT: "gqa", KVH: 8, Dim: 128, LocalLayers: 5, Window: 1024}
	wantTokens := 8192 + 5*1024
	want := float64(wantTokens) * m.kvLayerBytes()
	if got := m.KVBytes(8192); got != want {
		t.Errorf("SWA KV 应只保留 local window: %.0f vs %.0f", got, want)
	}
	if m.KVBytes(8192) >= m.KVTokBytes()*8192 {
		t.Error("SWA 长上下文 KV 必须小于全局 attention")
	}
	local := Throughput(h100, m, QuantByID("fp16"), 8192, 1, 1, Opts{})
	m.LocalLayers = 0
	global := Throughput(h100, m, QuantByID("fp16"), 8192, 1, 1, Opts{})
	if local.TTFTms >= global.TTFTms || local.DecodeMemMs >= global.DecodeMemMs {
		t.Errorf("SWA 应降低长上下文 prefill 与 decode KV 读取: local=%+v global=%+v", local, global)
	}
}

// 反向规划：R1 目标 6000 TPM 应给出方案且默认按成本升序
func TestPlanner(t *testing.T) {
	hws := []HW{hw4090, h200, hw("a100", 80, 2039)}
	plans := Planner(hws, r1, PlanOpts{TargetTPM: 6000}, singleWorkload(8192, 512), 16, Opts{})
	if len(plans) == 0 {
		t.Fatal("R1 6000 TPM 应有方案")
	}
	for _, p := range plans {
		if p.Replicas < 1 || p.TPM < 6000 {
			t.Errorf("方案 %s×%d r%d 集群 TPM %.0f 未达标", p.HW.ID, p.N, p.Replicas, p.TPM)
		}
	}
	for i := 1; i < len(plans); i++ {
		if plans[i].Monthly > 0 && plans[i-1].Monthly > 0 && plans[i].Monthly < plans[i-1].Monthly {
			t.Error("cost 目标下方案应按月成本升序")
		}
	}
	// 排队模式应抬高单副本承载并发
	q := Planner(hws, r1, PlanOpts{TargetTPM: 6000, Queue: true, MaxQ: 128}, singleWorkload(8192, 512), 16, Opts{})
	if len(q) == 0 {
		t.Fatal("排队模式应有方案")
	}
	// 时延目标应按 TTFT+TPOT 升序
	l := Planner(hws, r1, PlanOpts{TargetTPM: 6000, Objective: "latency"}, singleWorkload(8192, 512), 16, Opts{})
	for i := 1; i < len(l); i++ {
		li, lj := l[i].TTFTms+l[i].TPOTms, l[i-1].TTFTms+l[i-1].TPOTms
		if li < lj {
			t.Error("latency 目标下方案应按时延升序")
		}
	}
}

// 备选列表按（硬件×卡数）去重，不出现同卡变体重复行，条数上限 200
func TestPlannerDedup(t *testing.T) {
	hws := []HW{hw4090, h200, b200, hw("a100", 80, 2039), hw("mi300x", 192, 5300)}
	for _, obj := range []string{"", "cost", "latency", "avail"} {
		plans := Planner(hws, r1, PlanOpts{TargetTPM: 6000, Objective: obj}, singleWorkload(8192, 512), 16, Opts{})
		if len(plans) == 0 {
			t.Fatalf("objective=%q 应有方案", obj)
		}
		if len(plans) > 200 {
			t.Errorf("objective=%q 方案数 %d 超上限 200", obj, len(plans))
		}
		seen := map[string]bool{}
		for _, p := range plans {
			key := fmt.Sprintf("%s|%d", p.HW.ID, p.N)
			if seen[key] {
				t.Errorf("objective=%q 出现重复组合 %s（量化 %s）", obj, key, p.Quant)
			}
			seen[key] = true
		}
	}
}

// ---------- 推理栈干涉因子 ----------

var b200 = HW{ID: "b200", Vendor: "nvidia", Arch: "blackwell", VRAM: 192, BW: 8000, TF: 2250, TF8: 4500, TF4: 9000,
	Prec: []string{"fp16", "bf16", "fp8", "fp4"}, Link: Link{T: "nvlink", B: 1800, Dom: 8}}

// 零值 Opts 必须与显式 vLLM+关闭推测 完全一致（默认等价性）
func TestDefaultEquivalence(t *testing.T) {
	a := Throughput(hw4090, llama70, QuantByID("int4"), 4096, 4, 2, Opts{})
	b := Throughput(hw4090, llama70, QuantByID("int4"), 4096, 4, 2, Opts{Engine: "vllm", Spec: "none", KVQuant: "fp16"})
	if a.SingleTPS != b.SingleTPS || a.TTFTms != b.TTFTms || a.Mem.Total != b.Mem.Total {
		t.Errorf("零值 Opts 与显式默认不一致: %+v vs %+v", a, b)
	}
}

// 推测方法的场景公式可计算，但没有同模型、同 workload 的实测 τ/开销时不得改变结果。
func TestSpecGain(t *testing.T) {
	if g := SpecByID("none").gain(1); g != 1 {
		t.Errorf("none gain 应为 1，得 %.2f", g)
	}
	if g := SpecByID("dflash").gain(1); g < 4 || g > 6 {
		t.Errorf("dflash 场景公式 b=1 得 %.2f", g)
	}
	e32 := SpecByID("eagle3").gain(32)
	if e32 >= 1.0 {
		t.Errorf("eagle3 高并发场景应允许反噬，得 %.2f", e32)
	}
	base := Throughput(hw4090, qwen7b, QuantByID("int4"), 4096, 1, 1, Opts{})
	uncalibrated := Throughput(hw4090, qwen7b, QuantByID("int4"), 4096, 1, 1, Opts{Spec: "eagle3"})
	if uncalibrated.SingleTPS != base.SingleTPS || uncalibrated.SpecApplied {
		t.Errorf("未校准推测解码不得套默认倍率: %.1f vs %.1f", uncalibrated.SingleTPS, base.SingleTPS)
	}
	calibrated := Throughput(hw4090, qwen7b, QuantByID("int4"), 4096, 1, 1, Opts{Spec: "eagle3", SpecTau: 2.8, SpecOvh: 0.15})
	if calibrated.SingleTPS <= base.SingleTPS || !calibrated.SpecApplied {
		t.Errorf("提供实测 τ/开销后应应用场景增益: %.1f vs %.1f", calibrated.SingleTPS, base.SingleTPS)
	}
}

// KV 量化只有硬件与引擎路径同时受支持时才改变容量和读取量。
func TestKVQuant(t *testing.T) {
	m0 := Memory(hw4090, llama70, QuantByID("int4"), 32768, 8, 2, Opts{})
	m8 := Memory(hw4090, llama70, QuantByID("int4"), 32768, 8, 2, Opts{KVQuant: "fp8"})
	m4 := Memory(hw4090, llama70, QuantByID("int4"), 32768, 8, 2, Opts{KVQuant: "fp4"})
	if m8.KV > m0.KV*0.51 {
		t.Errorf("4090+vLLM FP8 KV 应压缩容量: fp16 %.2f / fp8 %.2f", m0.KV, m8.KV)
	}
	if m4.KV != m0.KV {
		t.Errorf("4090+vLLM 不支持 FP4 KV，不得只套容量倍率: fp16 %.2f / fp4 %.2f", m0.KV, m4.KV)
	}
	p0 := Throughput(hw4090, llama70, QuantByID("int4"), 32768, 8, 2, Opts{})
	p8 := Throughput(hw4090, llama70, QuantByID("int4"), 32768, 8, 2, Opts{KVQuant: "fp8"})
	p4 := Throughput(hw4090, llama70, QuantByID("int4"), 32768, 8, 2, Opts{KVQuant: "fp4"})
	if p8.AggTPS <= p0.AggTPS || !p8.KVSupported {
		t.Errorf("受支持 FP8 KV 应降低读取量: %.1f vs %.1f", p8.AggTPS, p0.AggTPS)
	}
	if p4.AggTPS != p0.AggTPS || p4.KVSupported {
		t.Errorf("不支持的 FP4 KV 不得改变结果: %.1f vs %.1f", p4.AggTPS, p0.AggTPS)
	}
	s0 := Throughput(b200, llama70, QuantByID("int4"), 32768, 8, 1, Opts{Engine: "sglang"})
	s4 := Throughput(b200, llama70, QuantByID("int4"), 32768, 8, 1, Opts{Engine: "sglang", KVQuant: "fp4"})
	if s4.AggTPS <= s0.AggTPS || s4.Mem.KV >= s0.Mem.KV || !s4.KVSupported {
		t.Errorf("B200+SGLang FP4 KV 应同时降低容量与读取量")
	}
}

// 前缀缓存仍须让新 query 读取并关注缓存 prefix，attention 不能按未命中比例线性缩放。
func TestPrefixHit(t *testing.T) {
	attn := Model{Params: 0.001, Active: 0.001, Layers: 80, Hidden: 8192, KVT: "gqa", KVH: 8, Dim: 128}
	p0 := Throughput(h100, attn, QuantByID("fp16"), 8192, 1, 1, Opts{})
	p8 := Throughput(h100, attn, QuantByID("fp16"), 8192, 1, 1, Opts{HitRate: 0.8})
	ratio := p8.TTFTms / p0.TTFTms
	if ratio < 0.34 || ratio > 0.38 {
		t.Errorf("80%% prefix 命中的 attention 比例应约 0.36，得 %.2f", ratio)
	}
	if p8.SingleTPS != p0.SingleTPS {
		t.Errorf("命中率不应影响 decode: %.1f vs %.1f", p8.SingleTPS, p0.SingleTPS)
	}
}

// 未校准同条件基准前，各框架共享 roofline 系数；只强制已知的平台兼容性。
func TestEngineCompatibilityAndNeutralBaseline(t *testing.T) {
	vl := Throughput(h100, llama8b, QuantByID("fp16"), 8192, 1, 1, Opts{Engine: "vllm"})
	trt := Throughput(h100, llama8b, QuantByID("fp16"), 8192, 1, 1, Opts{Engine: "trtllm"})
	if trt.TTFTms != vl.TTFTms || trt.SingleTPS != vl.SingleTPS {
		t.Errorf("未校准时不应虚构 TRT-LLM/vLLM 性能排名: trt=%+v vllm=%+v", trt, vl)
	}
	if (Engine{ID: "x", Vendors: []string{"nvidia"}}).EngineOK(m3ultra) {
		t.Error("nvidia-only 引擎不应兼容 Apple")
	}
	plans := Planner([]HW{m3ultra}, llama70, PlanOpts{TargetTPM: 600}, singleWorkload(4096, 512), 4, Opts{Engine: "trtllm"})
	if len(plans) != 0 {
		t.Error("TRT-LLM × M3 Ultra 不应出方案")
	}
	if len(Planner([]HW{m3ultra}, llama70, PlanOpts{TargetTPM: 600}, singleWorkload(4096, 512), 4, Opts{})) == 0 {
		t.Error("auto 应回退 MLX 并出方案")
	}
}

// FP4：Blackwell 原生加速，4090 仅省显存
func TestFP4Accel(t *testing.T) {
	if !b200.Accel(QuantByID("fp4")) {
		t.Error("B200 应原生加速 FP4")
	}
	if hw4090.Accel(QuantByID("fp4")) {
		t.Error("4090 无 FP4 单元，不应加速")
	}
}

func TestFP4FormatCompatibility(t *testing.T) {
	mi := HW{Vendor: "amd", TF: 2500, TF4: 10100, Prec: []string{"fp16", "fp8", "fp4"}}
	if mi.Accel(QuantByID("fp4")) {
		t.Error("AMD MXFP4 硬件不应被标成 NVIDIA NVFP4 路径")
	}
	if !mi.Accel(QuantByID("mxfp4")) {
		t.Error("MI355X 类硬件应识别 MXFP4 权重路径")
	}
	if got := mi.PeakTF(QuantByID("mxfp4")); got != mi.TF {
		t.Errorf("MXFP4 权重不能直接套 W4A4 峰值，得 %.0f TF", got)
	}
}

// 逐精度 dense 峰值优先于通用倍率；旧卡无低精度路径时回落 FP16 峰值。
func TestPrecisionSpecificPeak(t *testing.T) {
	if got := b200.PeakTF(QuantByID("fp8")); got != 4500 {
		t.Errorf("B200 FP8 dense 峰值 = %.0f，期望 4500", got)
	}
	if got := b200.PeakTF(QuantByID("fp4")); got != 9000 {
		t.Errorf("B200 FP4 dense 峰值 = %.0f，期望 9000", got)
	}
	v100 := HW{TF: 125, Prec: []string{"fp16"}}
	if got := v100.PeakTF(QuantByID("fp8")); got != 125 {
		t.Errorf("V100 无 FP8 路径，应回落 125 TF，得 %.0f", got)
	}
}

// ---------- 长上下文 ----------

// 注意力二次项：128K prefill 应远超线性外推（8K 的 16 倍），1M 时主导
func TestLongCtxQuadratic(t *testing.T) {
	p8 := Throughput(h100, llama70, QuantByID("fp16"), 8192, 1, 1, Opts{})
	p128 := Throughput(h100, llama70, QuantByID("fp16"), 131072, 1, 1, Opts{})
	ratio := p128.TTFTms / p8.TTFTms
	if ratio < 20 {
		t.Errorf("128K TTFT 应显著超线性（8K×16=%.0fms 线性口径），实测比 %.1f×", p8.TTFTms*16, ratio)
	}
	// 4K→1M 注意力项应压倒一切：1M TTFT 在单卡 H100 上应以小时/十分钟级计
	p1m := Throughput(h100, llama70, QuantByID("fp16"), 1048576, 1, 1, Opts{})
	if p1m.TTFTms < 1e6 {
		t.Errorf("70B FP16 单卡 1M prefill 应 >1000s 量级，得 %.0fms", p1m.TTFTms)
	}
}

// DSA 稀疏注意力：长上下文 decode/prefill 双受益，KV 存储不变
func TestSparseAttention(t *testing.T) {
	v32 := Model{ID: "ds-v32", Params: 671, Active: 37, Layers: 61, Hidden: 7168, KVT: "mla", MLA: 576, MoE: true, Sparse: 2048, Ctx: 163840}
	v31 := Model{ID: "ds-v31", Params: 671, Active: 37, Layers: 61, Hidden: 7168, KVT: "mla", MLA: 576, MoE: true, Ctx: 163840}
	d := Throughput(h200, v32, QuantByID("fp8"), 131072, 8, 8, Opts{})
	n := Throughput(h200, v31, QuantByID("fp8"), 131072, 8, 8, Opts{})
	if d.TTFTms >= n.TTFTms {
		t.Errorf("DSA prefill 应更便宜: %.0f vs %.0f ms", d.TTFTms, n.TTFTms)
	}
	if d.AggTPS <= n.AggTPS {
		t.Errorf("DSA decode 应更快: %.1f vs %.1f", d.AggTPS, n.AggTPS)
	}
	if d.Mem.KV != n.Mem.KV {
		t.Errorf("DSA 不应改变 KV 存储: %.1f vs %.1f", d.Mem.KV, n.Mem.KV)
	}
}

// YaRN 警告：超原生上下文时 trace 应有外推提示
func TestYarnWarn(t *testing.T) {
	p := Throughput(h100, llama8b, QuantByID("fp16"), 262144, 1, 1, Opts{Lang: "zh"})
	found := false
	for _, r := range p.Trace {
		if r.K == "⚠ 上下文外推" {
			found = true
		}
	}
	if !found {
		t.Error("256K 超原生 128K 应有 YaRN 警告")
	}
}

func TestLocalizedPresentation(t *testing.T) {
	en := Throughput(h100, llama8b, QuantByID("fp16"), 4096, 1, 1, Opts{})
	zh := Throughput(h100, llama8b, QuantByID("fp16"), 4096, 1, 1, Opts{Lang: "zh"})
	if en.Trace[0].K != "Estimate status" {
		t.Fatalf("default locale should be English, got %q", en.Trace[0].K)
	}
	if zh.Trace[0].K != "估算级别" {
		t.Fatalf("Chinese locale not applied, got %q", zh.Trace[0].K)
	}
	if got := strategy(h100, 1, ""); got != "Single card / host" {
		t.Fatalf("default strategy locale should be English, got %q", got)
	}
	if got := strategy(h100, 1, "zh"); got != "单卡 / 单机" {
		t.Fatalf("Chinese strategy locale not applied, got %q", got)
	}
}

// ---------- 量化体系 ----------

// W8A8/W4A4 的 prefill 算力使用硬件逐精度峰值；无硬件路径不加速。
func TestQuantComputePath(t *testing.T) {
	f16 := Throughput(h100, llama70, QuantByID("fp16"), 32768, 1, 1, Opts{})
	f8 := Throughput(h100, llama70, QuantByID("fp8"), 32768, 1, 1, Opts{})
	if f8.TTFTms > f16.TTFTms*0.6 {
		t.Errorf("FP8 W8A8 在 H100 上 prefill 应接近 2×: %.0f vs %.0f ms", f8.TTFTms, f16.TTFTms)
	}
	n4 := Throughput(b200, llama70, QuantByID("fp4"), 32768, 1, 1, Opts{})
	b16 := Throughput(b200, llama70, QuantByID("fp16"), 32768, 1, 1, Opts{})
	if n4.TTFTms > b16.TTFTms*0.35 {
		t.Errorf("NVFP4 在 B200 上 prefill 应接近 4×: %.0f vs %.0f ms", n4.TTFTms, b16.TTFTms)
	}
	v100 := HW{ID: "v100", Vendor: "nvidia", VRAM: 32, BW: 900, TF: 125, Prec: []string{"fp16"}}
	if v100.PeakTF(QuantByID("fp8")) != v100.TF {
		t.Error("V100 无 FP8 路径，不应套低精度算力倍率")
	}
}

// GGUF/MLX 量化与非匹配框架组合时 trace 应有警告
func TestQuantEngineMismatch(t *testing.T) {
	p := Throughput(h100, llama8b, QuantByID("q4km"), 4096, 1, 1, Opts{Engine: "vllm", Lang: "zh"})
	found := false
	for _, r := range p.Trace {
		if r.K == "量化路径" && strings.HasPrefix(r.N, "⚠ GGUF") {
			found = true
		}
	}
	if !found {
		t.Error("GGUF×vLLM 应有错配警告")
	}
	p2 := Throughput(m3ultra, llama8b, QuantByID("mlx4"), 4096, 1, 1, Opts{Engine: "mlx", Lang: "zh"})
	for _, r := range p2.Trace {
		if r.K == "量化路径" && strings.HasPrefix(r.N, "⚠") {
			t.Error("MLX×MLX 不应有警告")
		}
	}
}

// fit 矩阵展示所有主档位，包括官方 MXFP4 checkpoint。
func TestMainQuants(t *testing.T) {
	if n := len(MainQuants()); n != 6 {
		t.Errorf("fit 矩阵应为 6 列，得 %d", n)
	}
	rows := FitMatrix(h100, []Model{llama8b}, 1, 4096, 1, Opts{})
	if len(rows[0].Cells) != 6 {
		t.Errorf("fit 行应有 6 个单元格，得 %d", len(rows[0].Cells))
	}
}

// MLX/EXL 的 Accel 语义：平台内加速，平台外仅省显存
func TestQuantAccelFam(t *testing.T) {
	if !m3ultra.Accel(QuantByID("mlx4")) {
		t.Error("MLX 4bit 在 Apple 上应有快路径")
	}
	if h100.Accel(QuantByID("mlx4")) {
		t.Error("MLX 量化在 NVIDIA 上不应加速")
	}
	if hw4090.Accel(QuantByID("q4km")) {
		t.Error("GGUF 任何卡都不应标记加速")
	}
}

// ---------- 混合 TPM 口径 ----------

// 混合 TPM = req/s × (原始输入+输出) × 60；prefix hit 节省计算但不删除业务 token。
func TestMixedTPM(t *testing.T) {
	p := Throughput(h100, llama8b, QuantByID("fp16"), 8192, 4, 1, Opts{OutLen: 512})
	if p.PreTPS <= 0 {
		t.Fatal("应有 prefill 速度指标")
	}
	if p.TPMMixed <= p.TPM {
		t.Errorf("混合 TPM（%.0f）应大于纯 decode TPM（%.0f）", p.TPMMixed, p.TPM)
	}
	ph := Throughput(h100, llama8b, QuantByID("fp16"), 8192, 4, 1, Opts{OutLen: 512, HitRate: 0.8})
	if ph.ReqS <= p.ReqS || ph.TPMMixed <= p.TPMMixed {
		t.Errorf("prefix hit 应提高请求率和同口径 TPM: req %.2f/%.2f, TPM %.0f/%.0f", ph.ReqS, p.ReqS, ph.TPMMixed, p.TPMMixed)
	}
	want := ph.ReqS * (8192 + 512) * 60
	if math.Abs(ph.TPMMixed-want)/want > 0.02 {
		t.Errorf("混合 TPM 与 req/s 不自洽: %.0f vs %.0f", ph.TPMMixed, want)
	}
}

func TestKVTPReplication(t *testing.T) {
	if got := llama70.kvRankFactor(8); got != 0.125 {
		t.Errorf("8 KV heads / TP8 应每 rank 一头，得 %.3f", got)
	}
	if got := llama70.kvRankFactor(16); got != 0.125 {
		t.Errorf("TP 超过 KV heads 时应复制，不得继续减半，得 %.3f", got)
	}
	if got := r1.kvRankFactor(8); got != 1 {
		t.Errorf("默认 MLA latent cache 在 TP ranks 复制，得 %.3f", got)
	}
}

func TestHybridStateMemory(t *testing.T) {
	m := Model{Params: 35, Active: 3, Layers: 40, Hidden: 2048, KVT: "gqa", KVH: 2, Dim: 256, KVLayers: 10, StateMB: 32}
	with := Memory(h100, m, QuantByID("fp8"), 8192, 2, 2, Opts{})
	m.StateMB = 0
	without := Memory(h100, m, QuantByID("fp8"), 8192, 2, 2, Opts{})
	if math.Abs((with.KV-without.KV)-0.032) > 1e-9 {
		t.Errorf("hybrid recurrent state 应按 batch/TP 计入 0.032GB，得 %.6f", with.KV-without.KV)
	}
}

func TestMTPRequiresMetadataAndCalibration(t *testing.T) {
	base := Throughput(h100, qwen7b, QuantByID("fp16"), 4096, 1, 1, Opts{})
	noMetadata := Throughput(h100, qwen7b, QuantByID("fp16"), 4096, 1, 1, Opts{Spec: "mtp", SpecTau: 1.9, SpecOvh: 0.06})
	if noMetadata.SingleTPS != base.SingleTPS {
		t.Errorf("无 MTP 元数据的模型不应获得加速: %.1f vs %.1f", noMetadata.SingleTPS, base.SingleTPS)
	}
	m := qwen7b
	m.MTP = true
	uncalibrated := Throughput(h100, m, QuantByID("fp16"), 4096, 1, 1, Opts{Spec: "mtp"})
	if uncalibrated.SingleTPS != base.SingleTPS {
		t.Errorf("有 MTP 但无实测 τ/开销时不应套倍率")
	}
	on := Throughput(h100, m, QuantByID("fp16"), 4096, 1, 1, Opts{Spec: "mtp", SpecTau: 1.9, SpecOvh: 0.06})
	if on.SingleTPS <= base.SingleTPS || !on.SpecApplied {
		t.Errorf("元数据和校准齐全时应应用 MTP 场景增益: %.1f vs %.1f", on.SingleTPS, base.SingleTPS)
	}
}

func TestDecodeComputeRoof(t *testing.T) {
	m := Model{Params: 70, Active: 70, Layers: 80, Hidden: 8192, KVT: "gqa", KVH: 8, Dim: 128}
	slow := HW{VRAM: 1e6, BW: 1e9, TF: 1, Prec: []string{"fp16"}}
	fast := slow
	fast.TF = 100
	a := Throughput(slow, m, QuantByID("fp16"), 4096, 64, 1, Opts{})
	b := Throughput(fast, m, QuantByID("fp16"), 4096, 64, 1, Opts{})
	if a.AggTPS >= b.AggTPS {
		t.Errorf("计算受限 decode 应受 TFLOPS 屋顶约束: %.1f vs %.1f", a.AggTPS, b.AggTPS)
	}
	if a.Bottleneck != "compute" || a.DecodeComputeMs <= a.DecodeMemMs {
		t.Errorf("应暴露 compute 瓶颈及分项时延: %+v", a)
	}
}

func TestRequestLatencyUsesDecodeIntervals(t *testing.T) {
	p := Throughput(h100, llama8b, QuantByID("fp16"), 4096, 1, 1, Opts{OutLen: 8})
	want := p.TTFTms + 7*p.TPOTms
	if math.Abs(p.ReqMs-want) > 0.8 {
		t.Errorf("8 token 请求应为 TTFT + 7 个 decode 间隔: %.1f vs %.1f", p.ReqMs, want)
	}
	one := Throughput(h100, llama8b, QuantByID("fp16"), 4096, 1, 1, Opts{OutLen: 1})
	if one.ReqMs != one.TTFTms {
		t.Errorf("首 token 已包含在 TTFT，outlen=1 不应再加 decode: %.1f vs %.1f", one.ReqMs, one.TTFTms)
	}
}

func TestMoEExpertOccupancy(t *testing.T) {
	if got := activeWeightRead(r1, 1); math.Abs(got-r1.Active) > 1e-9 {
		t.Errorf("MoE b1 应只触达 active 权重，得 %.1fB", got)
	}
	got := activeWeightRead(r1, 32)
	if got <= r1.Active || got >= r1.Params {
		t.Errorf("MoE b32 期望去重专家读取应在 active 与 total 之间，得 %.1fB", got)
	}
}

func TestCheckpointPayloadOnlyMatchesNativeQuant(t *testing.T) {
	m := llama8b
	m.CheckpointGB, m.NativeQuant = 9.5, "fp8"
	if got := (Opts{}).weightGB(m, QuantByID("fp8")); got != 9.5 {
		t.Errorf("原生 FP8 应使用 safetensors payload，得 %.1fGB", got)
	}
	if got := (Opts{}).weightGB(m, QuantByID("fp16")); got != 16 {
		t.Errorf("转为 FP16 不应复用 FP8 payload，得 %.1fGB", got)
	}
	if got := (Opts{WeightGB: 7.25}).weightGB(m, QuantByID("fp16")); got != 7.25 {
		t.Errorf("用户实测权重应优先，得 %.2fGB", got)
	}
}

func TestNativeCheckpointLocksWeightQuantization(t *testing.T) {
	m := llama8b
	m.NativeQuant, m.CheckpointGB = "fp8", 9.5
	p := Throughput(h100, m, QuantByID("int4"), 4096, 1, 1, Opts{})
	if p.QuantID != "fp8" || !p.QuantLocked || p.Mem.Weights != 9.5 {
		t.Fatalf("预量化 checkpoint 未锁定原生格式: %+v", p)
	}
	rows := FitMatrix(h100, []Model{m}, 1, 4096, 1, Opts{})
	applicable := 0
	for _, cell := range rows[0].Cells {
		if cell.Applicable {
			applicable++
			if cell.Quant != "fp8" {
				t.Fatalf("fit 矩阵放行了非原生格式: %+v", cell)
			}
		}
	}
	if applicable != 1 {
		t.Fatalf("预量化模型应仅有一个可用权重格式，得 %d", applicable)
	}
	plans := Planner([]HW{h100}, m, PlanOpts{TargetTPM: 60, QuantOnly: "int4"}, singleWorkload(4096, 512), 1, Opts{})
	for _, plan := range plans {
		if plan.Quant != "fp8" {
			t.Fatalf("规划器绕过了原生格式锁定: %+v", plan)
		}
	}
}

func TestMemoryBudgetAndActivationContract(t *testing.T) {
	d := Memory(h100, llama8b, QuantByID("fp16"), 8192, 4, 1, Opts{})
	allocated := d.Weights + d.KV + d.Fw + d.Act + d.Adapter
	if math.Abs(d.Total-(allocated+d.Sys)) > 1e-9 || math.Abs(d.Budget-72) > 1e-9 {
		t.Errorf("系统预留不得重复扣减: %+v", d)
	}
	if d.Fit != (allocated <= d.Budget) {
		t.Errorf("Fit 必须比较已分配与引擎预算: allocated %.2f budget %.2f", allocated, d.Budget)
	}
	small, large := llama8b, llama8b
	small.Params, small.Active = 8, 8
	large.Params, large.Active = 80, 80
	a := Memory(h100, small, QuantByID("fp16"), 8192, 4, 1, Opts{})
	b := Memory(h100, large, QuantByID("fp16"), 8192, 4, 1, Opts{})
	if a.Act != b.Act {
		t.Errorf("workspace 应由 token×hidden 推导，不得按权重百分比增长: %.3f vs %.3f", a.Act, b.Act)
	}
}

func TestPrefixCacheSharesResidentBlocks(t *testing.T) {
	base := Memory(h100, llama8b, QuantByID("fp16"), 8192, 8, 1, Opts{})
	hit := Memory(h100, llama8b, QuantByID("fp16"), 8192, 8, 1, Opts{HitRate: 0.8})
	if hit.KV >= base.KV {
		t.Errorf("并发请求共享前缀 block 后 KV 应减少: %.3f vs %.3f", hit.KV, base.KV)
	}
	oneA := Memory(h100, llama8b, QuantByID("fp16"), 8192, 1, 1, Opts{})
	oneB := Memory(h100, llama8b, QuantByID("fp16"), 8192, 1, 1, Opts{HitRate: 0.8})
	if oneA.KV != oneB.KV {
		t.Errorf("单请求没有跨请求共享收益: %.3f vs %.3f", oneA.KV, oneB.KV)
	}
}

func TestParallelTopologyContracts(t *testing.T) {
	ep := Throughput(h200, r1, QuantByID("fp8"), 8192, 8, 8, Opts{TP: 1, EP: 8})
	one := Memory(h200, r1, QuantByID("fp8"), 8192, 8, 1, Opts{TP: 1})
	if !ep.TopologyOK || ep.CommMs <= 0 || ep.Mem.Weights >= one.Weights {
		t.Errorf("EP 应分片专家权重并产生 All-to-All: ep=%+v one=%+v", ep, one)
	}
	cp := Throughput(h200, llama70, QuantByID("int4"), 32768, 4, 4, Opts{TP: 1, CP: 4})
	single := Memory(h200, llama70, QuantByID("int4"), 32768, 4, 1, Opts{TP: 1})
	if !cp.TopologyOK || cp.CommMs <= 0 || cp.Mem.KV >= single.KV/3 {
		t.Errorf("CP4 应把 KV 约分成四份并产生通信: cp=%+v singleKV=%.2f", cp, single.KV)
	}
	pp := Memory(h200, llama70, QuantByID("int4"), 8192, 1, 4, Opts{TP: 1, PP: 4})
	if pp.Weights >= single.Weights/3 {
		t.Errorf("PP4 应按 stage 分片文本权重: %.2f vs %.2f", pp.Weights, single.Weights)
	}
	bad := Throughput(h200, llama70, QuantByID("int4"), 8192, 1, 8, Opts{EP: 8})
	if bad.TopologyOK || bad.Topology != "TP8 · PP1 · EP1 · CP1" {
		t.Errorf("Dense EP 必须标记无效并回退全 TP: %+v", bad)
	}
}

func TestOffloadChunkCalibrationAndEncoder(t *testing.T) {
	base := Throughput(h200, llama70, QuantByID("int4"), 32768, 8, 1, Opts{})
	off := Throughput(h200, llama70, QuantByID("int4"), 32768, 8, 1, Opts{KVOffload: 0.75, OffloadBW: 25})
	if off.Mem.KV >= base.Mem.KV || off.Mem.OffloadedKV <= 0 || off.OffloadMs <= 0 || off.SingleTPS >= base.SingleTPS {
		t.Errorf("KV offload 应以时延换显存: base=%+v off=%+v", base, off)
	}
	chunkModel := Model{Params: 100, Active: 1, MoE: true, Layers: 24, Hidden: 2048, KVT: "gqa", KVH: 4, Dim: 128}
	whole := Throughput(h100, chunkModel, QuantByID("fp16"), 8192, 1, 1, Opts{PrefillChunk: 8192})
	chunk := Throughput(h100, chunkModel, QuantByID("fp16"), 8192, 1, 1, Opts{PrefillChunk: 512})
	if chunk.TTFTms <= whole.TTFTms {
		t.Errorf("更小 prefill chunk 重复读取权重，不应降低 TTFT: %.0f vs %.0f", chunk.TTFTms, whole.TTFTms)
	}
	uncal := Throughput(h100, llama8b, QuantByID("fp16"), 8192, 1, 1, Opts{})
	cal := Throughput(h100, llama8b, QuantByID("fp16"), 8192, 1, 1,
		Opts{BWUtil: 0.3, FlopsUtil: 0.2, ScheduleMS: 2})
	if cal.Accuracy != "calibrated" || cal.SingleTPS == uncal.SingleTPS {
		t.Errorf("完整单卡校准输入应改变输出并标记 calibrated: %+v", cal)
	}
	mm := llama8b
	mm.Params, mm.Active, mm.EncoderParams, mm.Multimodal = 10, 8, 2, true
	text := Throughput(h100, mm, QuantByID("fp16"), 8192, 1, 1, Opts{})
	media := Throughput(h100, mm, QuantByID("fp16"), 8192, 1, 1, Opts{MediaTokens: 576})
	if media.EncoderMs <= 0 || media.TTFTms <= text.TTFTms {
		t.Errorf("媒体 encoder 应增加 TTFT: text=%+v media=%+v", text, media)
	}
}

func TestErlangCAndPlannerLoadMetrics(t *testing.T) {
	util, avg, p95 := erlangC(0.5, 1, 1)
	if math.Abs(util-0.5) > 1e-9 || math.Abs(avg-1000) > 1e-9 || math.Abs(p95-4605.170) > 0.01 {
		t.Errorf("M/M/1 解析值错误: util %.3f avg %.3f p95 %.3f", util, avg, p95)
	}
	plans := Planner([]HW{h200}, llama8b,
		PlanOpts{TargetTPM: 6000, Queue: true, MaxQ: 64, QuantOnly: "fp16"},
		singleWorkload(8192, 256), 4, Opts{})
	if len(plans) == 0 {
		t.Fatal("排队规划应有方案")
	}
	for _, p := range plans {
		if p.QueueModel != "M/M/c" || p.CapacityQPS < p.ArrivalQPS || p.UtilPct <= 0 || p.UtilPct >= 100 ||
			math.IsInf(p.WaitAvgMs, 0) || math.IsInf(p.WaitP95Ms, 0) {
			t.Errorf("规划负载指标必须稳定且自洽: %+v", p)
		}
	}
}
func TestThroughputWorkloadSingleBucketParity(t *testing.T) {
	o := Opts{OutLen: 256, HitRate: 0.2}
	base := Throughput(h100, llama8b, QuantByID("fp16"), 8192, 4, 1, o)
	mixed := ThroughputWorkload(h100, llama8b, QuantByID("fp16"),
		[]WorkloadBucket{{Context: 8192, Output: 256, Share: 1, PrefixHit: 0.2}}, 4, 1, Opts{})
	if mixed.Workload == nil || len(mixed.Workload.Buckets) != 1 {
		t.Fatalf("单桶工作负载必须返回分布明细: %+v", mixed.Workload)
	}
	if math.Abs(mixed.ReqS-base.ReqS) > 0.01 || math.Abs(mixed.TPMMixed-base.TPMMixed) > 1 ||
		math.Abs(mixed.Mem.Total-base.Mem.Total) > 1e-9 || mixed.Mem.P999Total != mixed.Mem.Total {
		t.Errorf("单桶聚合应退化为原单点计算: base=%+v mixed=%+v", base, mixed)
	}
	if mixed.Workload.MeanContext != 8192 || mixed.Workload.P999Context != 8192 ||
		mixed.Workload.MaxContext != 8192 || mixed.Workload.Buckets[0].Occupancy != 1 {
		t.Errorf("单桶统计错误: %+v", mixed.Workload)
	}
}

func TestThroughputWorkloadLongTailReweightsOccupancy(t *testing.T) {
	workload := []WorkloadBucket{
		{Context: 102400, Output: 512, Share: 0.80},
		{Context: 204800, Output: 512, Share: 0.15},
		{Context: 512000, Output: 512, Share: 0.03},
		{Context: 1048576, Output: 512, Share: 0.001},
	}
	p := ThroughputWorkload(h200, qwen7b, QuantByID("fp16"), workload, 4, 1, Opts{})
	if p.Workload == nil {
		t.Fatal("长尾工作负载缺少统计")
	}
	if p.Workload.MeanContext < 131000 || p.Workload.MeanContext > 132000 {
		t.Errorf("归一化平均上下文应约 131.5K，得 %.1f", p.Workload.MeanContext)
	}
	if p.Workload.P99Context != 512000 || p.Workload.P999Context != 1048576 ||
		p.Workload.MaxContext != 1048576 {
		t.Errorf("上下文分位错误: P99=%d P99.9=%d max=%d", p.Workload.P99Context, p.Workload.P999Context, p.Workload.MaxContext)
	}
	tail := p.Workload.Buckets[len(p.Workload.Buckets)-1]
	if tail.Occupancy <= tail.Share {
		t.Errorf("长请求驻留占比应高于到达占比: arrival %.5f occupancy %.5f", tail.Share, tail.Occupancy)
	}
	if p.Mem.P999Total <= p.Mem.Total || p.Workload.P99ReqMs <= p.ReqMs {
		t.Errorf("长尾必须抬高显存保护值和尾延迟: mean/p999 mem %.1f/%.1f mean/p99 req %.1f/%.1f",
			p.Mem.Total, p.Mem.P999Total, p.ReqMs, p.Workload.P99ReqMs)
	}
}

func TestPlannerUsesWorkloadDistribution(t *testing.T) {
	workload := []WorkloadBucket{
		{Context: 4096, Output: 128, Share: 0.96, PrefixHit: 0.5},
		{Context: 65536, Output: 1024, Share: 0.04},
	}
	plans := Planner([]HW{h200}, llama8b,
		PlanOpts{TargetTPM: 100, QuantOnly: "fp16"}, workload, 4, Opts{})
	if len(plans) == 0 {
		t.Fatal("分布式工作负载应生成规划方案")
	}
	p := plans[0]
	if p.MeanContext < 6000 || p.P99Context != 65536 || p.MaxContext != 65536 ||
		p.P95SingleTPS <= 0 || p.ReqP99ms <= p.ReqP95ms {
		t.Errorf("规划结果未携带真实工作负载分位: %+v", p)
	}
	wantArrival := 100 / ((p.MeanContext + 0.96*128 + 0.04*1024) * 60)
	if p.ArrivalQPS == 0 || math.Abs(p.ArrivalQPS-wantArrival) > 0.0001 {
		t.Errorf("目标 TPM 应按平均输入输出换算 QPS 且保留低流量精度: got %.4f want %.4f", p.ArrivalQPS, wantArrival)
	}
}

func TestWorkloadPercentilesUseTheirOwnMetric(t *testing.T) {
	workload := []WorkloadBucket{
		{Context: 32768, Output: 1, Share: 0.06},
		{Context: 4096, Output: 512, Share: 0.88},
		{Context: 512, Output: 8192, Share: 0.06},
	}
	mixed := ThroughputWorkload(h100, llama8b, QuantByID("fp16"), workload, 4, 1, Opts{})
	longContext := Throughput(h100, llama8b, QuantByID("fp16"), 32768, 4, 1, Opts{OutLen: 1})
	longOutput := Throughput(h100, llama8b, QuantByID("fp16"), 512, 4, 1, Opts{OutLen: 8192})
	if mixed.Workload.P95SingleTPS != longContext.SingleTPS {
		t.Errorf("95%% 请求吞吐下限应使用 TPS 的 P05: got %.1f want %.1f", mixed.Workload.P95SingleTPS, longContext.SingleTPS)
	}
	if mixed.Workload.P95TTFTms != longContext.TTFTms {
		t.Errorf("TTFT P95 必须按 TTFT 自身排序: got %.1f want %.1f", mixed.Workload.P95TTFTms, longContext.TTFTms)
	}
	if mixed.Workload.P95ReqMs != longOutput.ReqMs {
		t.Errorf("请求时延 P95 必须按请求时延自身排序: got %.1f want %.1f", mixed.Workload.P95ReqMs, longOutput.ReqMs)
	}
}

func TestExtremeCalibrationAndTopologyRemainFinite(t *testing.T) {
	p := ThroughputWorkload(h200, r1, QuantByID("fp8"), singleWorkload(1048576, 8192), 256, 8, Opts{
		WeightGB: math.MaxFloat64, RuntimeGB: math.MaxFloat64, ActivationGB: math.MaxFloat64,
		AdapterGB: math.MaxFloat64, DraftGB: math.MaxFloat64,
		BWUtil: math.SmallestNonzeroFloat64, FlopsUtil: math.SmallestNonzeroFloat64,
		LinkUtil: math.SmallestNonzeroFloat64, KVOffload: 1, OffloadBW: math.SmallestNonzeroFloat64,
	})
	for name, value := range map[string]float64{
		"memory": p.Mem.Total, "memory guard": p.Mem.P999Total, "TPS": p.SingleTPS,
		"request rate": p.ReqS, "request latency": p.ReqMs,
	} {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			t.Errorf("%s must remain finite, got %v", name, value)
		}
	}

	overflow := Throughput(h200, r1, QuantByID("fp8"), 4096, 4, 1, Opts{
		TP: math.MaxInt, PP: math.MaxInt, EP: math.MaxInt, CP: math.MaxInt,
	})
	if overflow.TopologyOK || overflow.Topology != "TP1 · PP1 · EP1 · CP1" {
		t.Errorf("overflowed topology must fall back safely: %+v", overflow)
	}
}

func TestPlannerExtremeTargetReturnsEmptyArray(t *testing.T) {
	plans := Planner([]HW{h200}, llama8b, PlanOpts{TargetTPM: math.MaxFloat64, QuantOnly: "fp16"},
		singleWorkload(4096, 128), 4, Opts{})
	if plans == nil || len(plans) != 0 {
		t.Fatalf("unreachable target must return a non-nil empty result: %+v", plans)
	}
}

func TestCatalogLoadersRejectConflictingMetadata(t *testing.T) {
	_, err := LoadModels([]byte(`[{"id":"bad","name":"Bad","org":"x","params":2,"active":3,"layers":1,"hidden":1,"kvt":"gqa","kvh":1,"dim":1,"ctx":1}]`))
	if err == nil {
		t.Fatal("active > total parameters must fail catalog loading")
	}
	_, err = LoadModels([]byte(`[{"id":"bad","name":"Bad","org":"x","params":3,"active":2,"layers":1,"hidden":1,"kvt":"gqa","kvh":1,"dim":1,"ctx":1,"native_quant":"compressed"}]`))
	if err == nil {
		t.Fatal("unknown native quant must fail catalog loading")
	}
	_, err = LoadHW([]byte(`[{"id":"bad","name":"Bad","vendor":"x","vram":1,"bw":0,"tf":1,"prec":["fp16"]}]`))
	if err == nil {
		t.Fatal("local hardware without bandwidth must fail catalog loading")
	}
	if _, err = LoadHW([]byte(`[{"id":"service","name":"Service","vendor":"x","svc":true}]`)); err != nil {
		t.Fatalf("API-only hardware is exempt from roofline inputs: %v", err)
	}
}
