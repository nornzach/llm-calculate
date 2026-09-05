package calc

// 十模型全维度审计:27B → 1T,dense/MoE、GQA/MLA、DSA/SWA、
// 消费卡→超节点,全部量化档位 × 单/多卡 × 并发 × 规划/处方。
// 目标:任何 NaN/Inf/违反带宽物理上界/装不下却报可部署/规划不达标
// 都是算法 bug,必须修到零异常。

import (
	"fmt"
	"math"
	"os"
	"sort"
	"testing"
)

var auditModelIDs = []string{
	"gemma-3-27b",           // 27B dense, SWA 局部层
	"qwen3-30b-a3b",         // 30B MoE 3.3B 激活
	"qwen3-32b",             // 32.8B dense
	"deepseek-r1-llama-70b", // 70B dense
	"qwen3-next-80b-a3b",    // 80B MoE 3B 激活, 混合线性注意力, MTP
	"llama-4-scout",         // 109B MoE, 1M 上下文
	"gpt-oss-120b",          // 117B MoE 5.1B 激活
	"qwen3-235b-a22b",       // 235B MoE
	"glm-4.6",               // 355B MoE
	"llama-4-maverick",      // 400B MoE, 1M 上下文
	"deepseek-v3.2",         // 671B MoE MLA + DSA 稀疏 + MTP
	"kimi-k2-thinking",      // 1040B MoE MLA(T 级)
}

var auditHWIDs = []string{
	"rtx4090", "a100-80", "h100-sxm", "h200", "b200", "mi300x", "apple-m3ultra",
}

func auditLoad(t *testing.T) ([]HW, []Model) {
	t.Helper()
	hb, err := os.ReadFile("../data/hardware.json")
	if err != nil {
		t.Fatal(err)
	}
	hws, err := LoadHW(hb)
	if err != nil {
		t.Fatal(err)
	}
	mb, err := os.ReadFile("../data/models.json")
	if err != nil {
		t.Fatal(err)
	}
	models, err := LoadModels(mb)
	if err != nil {
		t.Fatal(err)
	}
	return hws, models
}

func auditPick(hws []HW, models []Model) ([]HW, []Model) {
	hm := map[string]HW{}
	for _, h := range hws {
		hm[h.ID] = h
	}
	mm := map[string]Model{}
	for _, m := range models {
		mm[m.ID] = m
	}
	var hs []HW
	for _, id := range auditHWIDs {
		h, ok := hm[id]
		if !ok {
			panic("missing audit hardware " + id)
		}
		hs = append(hs, h)
	}
	var ms []Model
	for _, id := range auditModelIDs {
		m, ok := mm[id]
		if !ok {
			panic("missing audit model " + id)
		}
		ms = append(ms, m)
	}
	return hs, ms
}

func finitePos(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v > 0 }

// 单流 decode 硬上界:每 token 至少把激活权重读一遍;n 卡 TP 聚合 n 倍带宽。
func singleTPSCeil(h HW, m Model, q Quant, n int) float64 {
	readGB := m.Active * q.Bytes
	if readGB <= 0 {
		return math.Inf(1)
	}
	return h.BW * float64(n) / readGB
}

var auditWorkload = []WorkloadBucket{
	{Context: 8192, Output: 512, Share: 0.9},
	{Context: 32768, Output: 1024, Share: 0.1},
}

// TestAudit10Matrix 显存/吞吐维度:全部组合数值健康 + 带宽上界 + fit 一致性。
func TestAudit10Matrix(t *testing.T) {
	hws, models := auditPick(auditLoad(t))
	o := Opts{Engine: "auto", Spec: "none", KVQuant: "fp16"}
	bad := 0
	for _, m := range models {
		for _, h := range hws {
			for _, q := range Quants {
				// 找最小能装下的卡数(1/2/4/8),并审计单卡不装下的结果
				pf1 := ThroughputWorkload(h, m, q, auditWorkload, 1, 1, o)
				check := func(tag string, pf Perf, batch, n int) {
					if pf.Fit && pf.Mem.P999Total > pf.Mem.Cap*1.001 {
						t.Errorf("%s %s %s b%d n%d: fit 但 P999 %.1f > 物理 %.1f", m.ID, h.ID, q.ID, batch, n, pf.Mem.P999Total, pf.Mem.Cap)
						bad++
					}
					if !pf.Fit && !math.IsNaN(pf.Mem.P999Total) && pf.Mem.P999Total > 0 && pf.Mem.P999Total <= pf.Mem.Cap*0.8 {
						t.Errorf("%s %s %s b%d n%d: 不 fit 但 P999 %.1f 远低于物理 %.1f(矛盾)", m.ID, h.ID, q.ID, batch, n, pf.Mem.P999Total, pf.Mem.Cap)
						bad++
					}
					if !pf.EstimateValid {
						if pf.Deployable || pf.SingleTPS != 0 || pf.AggTPS != 0 || pf.TTFTms != 0 || pf.TPOTms != 0 || pf.ReqS != 0 {
							t.Errorf("%s %s %s b%d n%d: 无效估算仍输出性能: %+v", m.ID, h.ID, q.ID, batch, n, pf)
							bad++
						}
						return
					}
					if pf.Fit {
						for name, v := range map[string]float64{
							"single": pf.SingleTPS, "agg": pf.AggTPS, "ttft": pf.TTFTms,
							"tpot": pf.TPOTms, "req": pf.ReqMs, "pre": pf.PreTPS,
							"tpm": pf.TPM, "tpm_mixed": pf.TPMMixed, "req_s": pf.ReqS,
						} {
							if !finitePos(v) {
								t.Errorf("%s %s %s b%d n%d: %s = %v 非法", m.ID, h.ID, q.ID, batch, n, name, v)
								bad++
							}
						}
						if ceil := singleTPSCeil(h, m, q, n) * 1.05; pf.SingleTPS > ceil {
							t.Errorf("%s %s %s b%d n%d: 单流 %.1f tok/s 超带宽上界 %.1f(BW %.0f / 每token读 %.2fG)",
								m.ID, h.ID, q.ID, batch, n, pf.SingleTPS, ceil, h.BW, m.Active*q.Bytes)
							bad++
						}
						if pf.AggTPS+1e-9 < pf.SingleTPS {
							t.Errorf("%s %s %s b%d n%d: 聚合 %.1f < 单流 %.1f", m.ID, h.ID, q.ID, batch, n, pf.AggTPS, pf.SingleTPS)
							bad++
						}
						if pf.TPMMixed+1e-9 < pf.TPM {
							t.Errorf("%s %s %s b%d n%d: 混合 TPM %.0f < decode TPM %.0f", m.ID, h.ID, q.ID, batch, n, pf.TPMMixed, pf.TPM)
							bad++
						}
						if !pf.TopologyOK {
							t.Errorf("%s %s %s b%d n%d: 有效估算的拓扑不可用 (%s)", m.ID, h.ID, q.ID, batch, n, pf.Topology)
							bad++
						}
					}
				}
				check("单卡", pf1, 1, 1)
				for _, n := range []int{2, 4, 8} {
					pf := ThroughputWorkload(h, m, q, auditWorkload, 1, n, o)
					check("多卡", pf, 1, n)
					if pf.Fit && pf.EstimateValid {
						pfb := ThroughputWorkload(h, m, q, auditWorkload, 32, n, o)
						check("并发32", pfb, 32, n)
						break // 找到最小可行 n 即可
					}
				}
			}
		}
	}
	if bad > 0 {
		t.Fatalf("矩阵审计共 %d 处异常", bad)
	}
}

// TestAudit10FitMatrix fit 矩阵维度:每个硬件 × 10 模型 × 主量化档位。
func TestAudit10FitMatrix(t *testing.T) {
	hws, models := auditPick(auditLoad(t))
	o := Opts{Engine: "auto", Spec: "none", KVQuant: "fp16"}
	for _, h := range hws {
		rows := FitMatrix(h, models, 1, singleWorkload(8192, 512), 8, o)
		if len(rows) != len(models) {
			t.Errorf("%s: fit 矩阵行数 %d ≠ %d", h.ID, len(rows), len(models))
		}
		for _, r := range rows {
			if len(r.Cells) != len(MainQuants()) {
				t.Errorf("%s/%s: 列数 %d", h.ID, r.Model.ID, len(r.Cells))
			}
			for _, c := range r.Cells {
				if c.Fit < 0 || c.Fit > 2 || math.IsNaN(c.TPS) || math.IsInf(c.TPS, 0) || c.TPS < 0 {
					t.Errorf("%s/%s/%s: 非法 cell fit=%d tps=%v", h.ID, r.Model.ID, c.Quant, c.Fit, c.TPS)
				}
				if c.Fit == 2 {
					m, q := r.Model, QuantByID(c.Quant)
					if ceil := singleTPSCeil(h, m, q, 1) * 1.05; c.TPS > ceil {
						t.Errorf("%s/%s/%s: fit 矩阵 TPS %.1f 超上界 %.1f", h.ID, m.ID, c.Quant, c.TPS, ceil)
					}
				}
			}
		}
	}
}

// TestAudit10Planner 反向规划维度:达标性、排序、副本上限、内存合法。
func TestAudit10Planner(t *testing.T) {
	hwAll, modelsAll := auditLoad(t)
	_, models := auditPick(hwAll, modelsAll)
	scenarios := []struct {
		name string
		po   PlanOpts
		wl   []WorkloadBucket
	}{
		{"6k混合", PlanOpts{TargetTPM: 6000, Objective: "cost"}, auditWorkload},
		{"高TOS", PlanOpts{TargetTPM: 6000, MinTOS: 30, Objective: "latency"}, auditWorkload},
		{"长上下文", PlanOpts{TargetTPM: 2000, Objective: "cost"}, []WorkloadBucket{{Context: 131072, Output: 1024, Share: 1}}},
		{"排队", PlanOpts{TargetTPM: 6000, Objective: "cost", Queue: true, MaxQ: 256}, auditWorkload},
	}
	for _, m := range models {
		for _, sc := range scenarios {
			plans := Planner(hwAll, m, sc.po, sc.wl, 16, Opts{Engine: "auto", Spec: "none", KVQuant: "fp16"})
			prevCost := math.Inf(-1)
			for _, p := range plans {
				if sc.po.TargetTPM > 0 && p.TPM < sc.po.TargetTPM*0.999 {
					t.Errorf("%s/%s: %s×%d TPM %.0f 不达标 %.0f", m.ID, sc.name, p.HW.ID, p.N, p.TPM, sc.po.TargetTPM)
				}
				if sc.po.MinTOS > 0 && p.P95SingleTPS < sc.po.MinTOS*0.999 {
					t.Errorf("%s/%s: %s×%d P95单流 %.1f 低于下限 %.1f", m.ID, sc.name, p.HW.ID, p.N, p.P95SingleTPS, sc.po.MinTOS)
				}
				if p.Replicas > maxReplicas {
					t.Errorf("%s/%s: %s 副本 %d 超上限", m.ID, sc.name, p.HW.ID, p.Replicas)
				}
				if math.IsNaN(p.TPM) || math.IsInf(p.TPM, 0) || p.TPM <= 0 {
					t.Errorf("%s/%s: %s TPM 非法 %v", m.ID, sc.name, p.HW.ID, p.TPM)
				}
				if p.MemoryP999 > 0 && p.MemoryP999 > p.HW.VRAM*float64(p.N)*1.001 {
					t.Errorf("%s/%s: %s×%d P999 显存 %.1f 超物理 %.1f", m.ID, sc.name, p.HW.ID, p.N, p.MemoryP999, p.HW.VRAM*float64(p.N))
				}
				if sc.po.Objective == "cost" && p.Monthly > 0 {
					if p.Monthly < prevCost-1e-6 {
						t.Errorf("%s/%s: cost 目标排序倒挂 %.0f 在 %.0f 后", m.ID, sc.name, p.Monthly, prevCost)
					}
					prevCost = p.Monthly
				}
			}
		}
	}
}

// TestAudit10Recommend 处方维度:分数域、去重、fit、卡片方向只出官方模型。
func TestAudit10Recommend(t *testing.T) {
	hwsAll, modelsAll := auditLoad(t)
	_, models := auditPick(hwsAll, modelsAll)
	wl := auditWorkload
	for _, m := range models {
		res := Recommend(hwsAll, modelsAll, m, wl, RecommendOpts{
			Direction: "model", Objectives: "cost,tos", TargetTPM: 6000, Limit: 12,
		}, Opts{Engine: "auto", Spec: "none", KVQuant: "fp16"})
		seen := map[string]bool{}
		for _, p := range res.Picks {
			if p.Score < -1e-9 || p.Score > 1.0001 || math.IsNaN(p.Score) {
				t.Errorf("%s: 处方分数越界 %.4f", m.ID, p.Score)
			}
			key := fmt.Sprintf("%s|%s|%s|%s|%s|%d|%d|%d|%s",
				p.Plan.HW.ID, p.Plan.Quant, p.EngineID, p.KVQuant, p.SpecID,
				p.Plan.N, p.Plan.Replicas, p.Plan.MaxConc, p.Topology)
			if seen[key] {
				t.Errorf("%s: 处方重复 %s", m.ID, key)
			}
			seen[key] = true
			if p.MemoryP999GB > 0 && p.MemoryP999GB > p.Plan.HW.VRAM*float64(p.Plan.N)*1.001 {
				t.Errorf("%s: 处方 %s 显存 %.1f 超物理", m.ID, p.Plan.HW.ID, p.MemoryP999GB)
			}
		}
	}
	for _, hwID := range []string{"rtx4090", "h200"} {
		res := Recommend(hwsAll, modelsAll, Model{}, wl, RecommendOpts{
			Direction: "card", HW: hwID, Cards: 1, Objectives: "cost", Limit: 12,
		}, Opts{Engine: "auto", Spec: "none", KVQuant: "fp16"})
		if len(res.Picks) == 0 {
			t.Errorf("card 模式 %s 无处方", hwID)
		}
		for _, p := range res.Picks {
			for _, m := range modelsAll {
				if m.ID == p.ModelID && m.Conf != "official" {
					t.Errorf("card 模式 %s 推荐了非官方模型 %s", hwID, m.ID)
				}
			}
		}
	}
}

// TestAudit10Special 特殊结构维度:DSA/MLA/SWA/线性注意力/MTP/超长上下文。
func TestAudit10Special(t *testing.T) {
	hws, models := auditPick(auditLoad(t))
	mm := map[string]Model{}
	for _, m := range models {
		mm[m.ID] = m
	}
	hh := map[string]HW{}
	for _, h := range hws {
		hh[h.ID] = h
	}
	o := Opts{Engine: "auto", Spec: "none", KVQuant: "fp16"}

	// DSA:deepseek-v3.2 选中 2048 token,KV 读取必须显著小于全量注意力
	dsv32, ok := mm["deepseek-v3.2"]
	if !ok || dsv32.Sparse != 2048 {
		t.Fatalf("deepseek-v3.2 稀疏补丁丢失: %+v", dsv32.Sparse)
	}
	h200 := hh["h200"]
	q := QuantByID("fp8")
	pfSparse := ThroughputWorkload(h200, dsv32, q, []WorkloadBucket{{Context: 131072, Output: 512, Share: 1}}, 1, 8, o)
	ds31 := dsv32
	ds31.Sparse = 0
	pfDense := ThroughputWorkload(h200, ds31, q, []WorkloadBucket{{Context: 131072, Output: 512, Share: 1}}, 1, 8, o)
	if pfSparse.EstimateValid && pfDense.EstimateValid && pfSparse.Fit && pfDense.Fit && pfSparse.SingleTPS <= pfDense.SingleTPS {
		t.Errorf("DSA 未提速: 稀疏 %.1f ≤ 稠密 %.1f tok/s", pfSparse.SingleTPS, pfDense.SingleTPS)
	}

	// MLA:Kimi K2 每 token KV 应远低于 GQA 同级
	k2 := mm["kimi-k2-thinking"]
	kvMLA := k2.KVTokBytes()
	glm := mm["glm-4.6"]
	kvGQA := glm.KVTokBytes()
	if kvMLA >= kvGQA {
		t.Errorf("MLA KV 未小于 GQA: k2 %.0f vs glm %.0f B/token", kvMLA, kvGQA)
	}

	// 1M 上下文:Llama-4 Scout 单卡 24G 必装不下(8×H200 FP8 也应紧张或装不下)
	scout := mm["llama-4-scout"]
	pf1m := ThroughputWorkload(hh["rtx4090"], scout, QuantByID("q4km"), []WorkloadBucket{{Context: 1048576, Output: 512, Share: 1}}, 1, 1, o)
	if pf1m.Fit {
		t.Errorf("Scout 1M 上下文在 4090 单卡报可部署,显存公式漏 KV")
	}

	// SWA:Gemma 3 27B 局部层 KV 应被 window 截断
	gemma := mm["gemma-3-27b"]
	pfG := ThroughputWorkload(hh["rtx4090"], gemma, QuantByID("int4"), []WorkloadBucket{{Context: 8192, Output: 512, Share: 1}}, 1, 1, o)
	if !pfG.Fit {
		t.Errorf("Gemma3-27B INT4 8K 在 4090 应可部署(SWA 截断 KV),实际 mem=%.1f/%.1f", pfG.Mem.P999Total, pfG.Mem.Cap)
	}

	// MTP:qwen3-next-80b 标了 mtp,推测解码应可生效
	qn := mm["qwen3-next-80b-a3b"]
	pfMTP := ThroughputWorkload(hh["h200"], qn, QuantByID("fp8"), auditWorkload, 8, 4, Opts{Engine: "auto", Spec: "mtp", KVQuant: "fp16"})
	if pfMTP.EstimateValid && pfMTP.Fit && !pfMTP.SpecApplied {
		t.Errorf("qwen3-next MTP 未生效")
	}
}

// TestAudit10Monotonic 单调性:同条件 bytes 更小 → fit 不差;batch 更大 → agg 不减(fit 时)。
func TestAudit10Monotonic(t *testing.T) {
	hws, models := auditPick(auditLoad(t))
	o := Opts{Engine: "auto", Spec: "none", KVQuant: "fp16"}
	bytesOrder := []string{"iq2", "q4km", "q8", "fp16"} // GGUF 家族内 bytes 递增
	for _, m := range models {
		for _, h := range hws {
			prevFit := true // bytes 最小的最先
			for _, qid := range bytesOrder {
				q := QuantByID(qid)
				pf := ThroughputWorkload(h, m, q, auditWorkload, 1, 1, o)
				if !prevFit && pf.Fit {
					t.Errorf("%s %s: bytes 更大的 %s 反而可部署(小档 %s 不可)", m.ID, h.ID, qid, "更小档")
				}
				prevFit = pf.Fit
			}
			q := QuantByID("fp8")
			pf1 := ThroughputWorkload(h, m, q, auditWorkload, 1, 1, o)
			pf32 := ThroughputWorkload(h, m, q, auditWorkload, 32, 1, o)
			if pf1.Fit && pf32.Fit && pf32.AggTPS < pf1.AggTPS-1e-9 {
				t.Errorf("%s %s: batch32 聚合 %.1f < batch1 %.1f", m.ID, h.ID, pf32.AggTPS, pf1.AggTPS)
			}
		}
	}
}

// 辅助:确保排序比较器引用(避免未使用告警)
var _ = sort.Float64s
