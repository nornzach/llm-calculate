# LLM 推理计算器 · 设计稿

> 版本 v0.2 · 2026-09-02（v0.1 基础上融合两份外部调研报告，扩到全厂商，附报告勘误见附录 A）
> 定位：一个「硬件 × 模型 × 量化 × 并行策略」的推理部署可行性 + 性能估算 + 反向规划工具。
> 形态：**Go 单二进制 + 内嵌静态网页**（`go:embed`），数据为 JSON/YAML 文件，无需数据库，可离线运行。

---

## 1. 需求拆解

| # | 模式 | 输入 | 输出 |
|---|------|------|------|
| 1 | **能装什么** | 卡/设备型号（+数量） | 可部署的模型 × 量化档位红绿灯矩阵（显存可行性 + 明细） |
| 2 | **能跑多快** | N 张 X 卡 + 某模型 + 量化 + 上下文 + 并发 | 单流/聚合 TPS、TPM、TTFT、TPOT，公式过程可见 |
| 3 | **要怎么配**（反向规划） | 模型 + 目标 TPM + 并发/上下文/SLA（+预算） | 候选部署方案清单：量化、卡型、卡数、并行方式、引擎、缓存策略、成本，按成本排序 |

你没想到、但设计中已补齐的维度：

- **单流 TPS ≠ 聚合 TPS**（batch 抬高聚合吞吐，单用户体感不变甚至更差）
- **TTFT 与 TPOT 分开算**——prefill 吃算力，decode 吃显存带宽
- **MoE**：显存按总参数、速度按激活参数（DeepSeek V3：671B 总 / 37B 激活）
- **KV cache 随上下文 × 并发线性膨胀**，长上下文场景先爆 KV 再爆权重；支持 KV cache 量化（FP8/INT4-KV）作为输入开关
- **MLA / GQA 对 KV 的压缩**（DeepSeek MLA 比 MHA 省一个数量级）
- **通信开销与 batch 挂钩**：TP 每层两次 AllReduce，通信量 ∝ batch；无 NVLink 时大 batch 多卡 TP 收益锐减——这正是 4090 无 NVLink 的痛点
- **VLM 多模态**：视觉编码器常驻显存 + 图片 token 抬高 KV
- **统一内存设备（Apple M / AMD Strix Halo）**：显存 = 内存 × 上限比例，公式不同
- **成本**：云时价 / 二手价 / 电费 → 每百万 token 成本
- **校准机制**：理论公式系统性偏乐观，每个「硬件 × 引擎」组合留校准系数，公开 benchmark 回归，UI 明示误差区间

---

## 2. 术语与单位

| 缩写 | 含义 |
|------|------|
| TPS | tokens/s，区分**单流 TPS**（体感速度）与**聚合 TPS**（整机合计） |
| TPM | tokens/min = TPS × 60（不是 RPM/QPS） |
| TTFT | 首 token 延迟 ≈ prefill 耗时 + 排队 |
| TPOT | decode 阶段单步耗时 |
| TP / PP / DP / EP | 张量并行 / 流水线并行 / 数据并行（多副本）/ 专家并行（MoE） |
| P/D 分离 | prefill 与 decode 拆到不同节点池（DeepSeek/Kimi 生产架构） |

---

## 3. 数据层设计

全部数据为仓库内 YAML/JSON，支持社区 PR 补充。每条记录带 `data_confidence: official | reported | estimated` 字段，非官方参数在 UI 上标灰提示。

### 3.1 硬件数据 schema（`data/gpus.yaml`，泛化到全厂商）

```yaml
- id: rtx4090
  name: "RTX 4090"
  vendor: nvidia            # nvidia | amd | intel | apple | huawei | mthreads | cambricon | hygon | google | groq | cerebras
  device_class: discrete_gpu  # discrete_gpu | workstation | datacenter | supernode | unified_soc | sram_asic | edge
  arch: ada                 # 决定精度支持矩阵（§3.3）
  vram_gb: 24               # unified_soc 时为可选内存档位列表
  mem_type: GDDR6X
  mem_bandwidth_gbps: 1008  # decode 速度的第一决定因素
  interconnect:
    type: pcie              # none | nvlink | nvlink_bridge | xgmi | pcie | ethernet | hccss | c2c_unified | mtlink
    bandwidth_gbps: 64      # 卡间互联总带宽（双向），TP 通信惩罚的依据
    max_domain: 1           # 全互联域内最大卡数（NVL72=72, 8卡SXM=8, 桥接=2）
  tensor_core_gen: 4        # 0=无（P100）；Apple/AMD 消费卡记矩阵加速代际说明
  precisions: [fp16, bf16, fp8, int8, int4]   # 硬件加速支持的精度
  compute: { bf16_tflops: 165, fp8_tflops: 660 }   # dense；sparse 另注
  engines: [vllm, sglang, trtllm, llamacpp, exllama]   # 可用推理栈
  tdp_w: 450
  price: { cny_used: 14500, usd_hourly: 0.45 }
  data_confidence: official
  notes: "4090D 为砍 AI TOPS 国行；48G 为第三方魔改（双面 GDDR6X 颗粒，带宽与原版一致），无质保"
```

### 3.2 硬件收录清单（首批 ~90 款，五大厂商 + 国产 + 特殊架构）

#### 3.2.1 NVIDIA 消费级（GeForce）

| 系列 | 型号 | 显存 | 带宽 GB/s | NVLink | 关键能力 |
|------|------|------|-----------|--------|----------|
| 20 系 Turing | 2060 / 2060S / 2070 / 2070S / 2080 / 2080S / 2080Ti | 6–11 G | 336–616 | 2080/Ti 桥接 2 路 ~100GB/s | FP16/INT8/INT4，**无 BF16/FP8** |
| 30 系 Ampere | 3050 / 3060 12G / 3060Ti / 3070 / 3070Ti / 3080 10G·12G / 3080Ti / 3090 / 3090Ti | 6–24 G | 272–1008 | **仅 3090/3090Ti**（桥接 2 路 ~112GB/s） | +BF16/TF32 |
| 40 系 Ada | 4060 / 4060Ti 8G·16G / 4070 / 4070S / 4070Ti / 4070TiS / 4080 / 4080S / 4090D / 4090 / **4090 48G 魔改** | 8–48 G | 272–1008 | 全系无 | +FP8（E4M3，部分栈需转换） |
| 50 系 Blackwell | 5050 / 5060 / 5060Ti 8G·16G / 5070 / 5070Ti / 5080 / 5090D V2(24G) / 5090D / 5090 | 8–32 G | 320–1792 | 全系无 | +FP4 |

> 魔改/特供备注：4090 48G 魔改 = 双面 GDDR6X 颗粒 + 涡轮散热，带宽≈原版 1008，无质保（`data_confidence: reported`）；5090D 32G 与 5090 同显存、AI TOPS 砍至 ~2375；5090D V2 24G（384-bit，带宽 ~1344，reported）。

#### 3.2.2 NVIDIA 工作站 / 数据中心

| 型号 | 显存 | 带宽 GB/s | 互联 | 备注 |
|------|------|-----------|------|------|
| RTX A5000 / A6000 | 24 / 48 G | 768 / 768 | A6000 桥接 2 路 | Ampere 专业卡 |
| RTX 4000 Ada / 5000 Ada / 5880 Ada / 6000 Ada | 20 / 32 / 48 / 48 G | 360–960 | 无 | 5880 Ada 为国行砍算力版 |
| RTX 6000 Pro Blackwell | 96 G GDDR7 | 1792 | 无 | 单机 4 卡装 70B FP16 很香 |
| P100 | 16G HBM2 | 732 | NVLink1 160（SXM） | **Pascal，无 Tensor Core** |
| V100 | 16/32G HBM2 | 900 | NVLink2 300 | 1 代 TC，仅 FP16 |
| T4 | 16G GDDR6 | 320 | 无 | INT8 ~130 TOPS 老推理卡 |
| A10 / A30 / A40 | 24 / 24 / 48 G | 600 / 933 / 696 | **仅 A30**（200GB/s） | A10 无 NVLink（报告 1 写错） |
| A100 | 40/80G HBM2e | 1555 / 2039 | NVLink3 600 | 支持 MIG 切分 |
| A800 | 80G | 2039 | **降为 400** | A100 国行 |
| L4 / L40 / L40S | 24 / 48 / 48 G | 300 / 864 / 864 | 无 | L40S 有 FP8 Transformer Engine，推理甜点 |
| H100 SXM / PCIe / NVL | 80G HBM3 | 3350 / 2000 / 3350 | NVLink4 900 | FP8 TE |
| H800 | 80G | 3350 | **降为 400** | H100 国行 |
| H20 | 96G / 141G HBM3(e) | 4000 | **NVLink 900 未砍** | 算力大砍（FP8 ~296T）→ decode 性价比、prefill 弱 |
| H200 | 141G HBM3e | 4800 | NVLink4 900 | 同 H100 算力，KV 友好 |
| B200 | 192G HBM3e | 8000 | NVLink5 1.8TB/s | +FP4 |
| B300 (Blackwell Ultra) | 288G HBM3e | ~8000 | NVLink5 | 算力 ~1.5× B200 |
| GB200 / GB300 NVL72 | 72×B200/B300 整柜 | — | 72 卡 NVSwitch 全互联域 | 按 `supernode` 单独建模（Grace CPU 内存可用于 KV offload） |
| GH200 | 96/141G HBM3 + ≤480G LPDDR5X | 4100 | C2C 统一内存 | 同 supernode 思路 |

#### 3.2.3 AMD（ROCm / llama.cpp）

| 型号 | 显存 | 带宽 | 互联 | 备注 |
|------|------|------|------|------|
| MI100 | 32G HBM2 | 1229 | XGMI 3 路 | CDNA1，老 |
| MI210 / MI250X | 64 / 128G HBM2e | 1638 / 3277 | XGMI | CDNA2，无 FP8 |
| **MI300X** | 192G HBM3 | 5300 | XGMI ~896 | CDNA3，+FP8，对标 H100 |
| MI325X | 256G HBM3e | 6000 | XGMI ~896 | MI300X 加显存版 |
| MI355X | 288G HBM3e | ~8000 | XGMI | CDNA4，+FP4（reported） |
| RX 7900 XTX / XT / GRE | 24 / 20 / 16 G | 960 / 800 / 576 | 无 | RDNA3，llama.cpp/ROCm 可玩 |
| RX 9070 XT / 9070 | 16 G | 640 | 无 | RDNA4，WMMA 增强 |
| Radeon PRO W7900 / W7800 | 48 / 32 G | 864 / 576 | 无 | 48G 专业卡性价比高 |
| **Ryzen AI Max+ 395 (Strix Halo)** | 统一内存 ≤128G，GPU 可用 ~96–110G | 256 | —（unified_soc） | 本地跑 70B INT4 的廉价方案 |

#### 3.2.4 Intel

| 型号 | 显存 | 带宽 | 互联 | 备注 |
|------|------|------|------|------|
| **Gaudi 2** | 96G HBM2e | 2450 | 片内 24×100GbE | 用标准以太网扩展，无 NVLink 类互联 |
| **Gaudi 3** | 128G HBM2e | 3670 | 24×200GbE | 同上，BF16 算力 ~1.8P（reported） |
| Data Center GPU Max 1550 | 128G HBM2e | 3277 | Xe Link | Ponte Vecchio，oneAPI |
| Arc A770 16G / B580 12G | 16 / 12 G | 560 / 456 | 无 | llama.cpp SYCL / IPEX-LLM 入门 |

> Gaudi 的扩展模型与 NVLink 系根本不同：每台机器自带大带宽网口走标准以太网，TP 通信惩罚要按 RDMA 网络建模——这是它便宜的原因之一。

#### 3.2.5 Apple M 系列（unified_soc，MLX / llama.cpp Metal）

统一内存架构，**显存 = 物理内存 × GPU 可用上限（macOS 默认 ~0.65–0.75，可调）**；带宽即内存带宽；无多卡互联（多机只能靠雷雳/以太网，exolabs 方案，带宽很低）。

| 芯片 | 内存档位 | 带宽 GB/s | 典型可跑 |
|------|----------|-----------|----------|
| M1 / M2 / M3 / M4（基础款） | 8–32 G | 68–120 | 3B–8B INT4 |
| Pro 档（M1Pro–M4Pro） | 16–64 G | 200–273 | 8B–14B |
| Max 档（M1Max–M4Max） | 32–128 G | 400–546 | 70B INT4 勉强（M4Max 128G） |
| Ultra 档（M1Ultra/M2Ultra/**M3Ultra**） | 64–512 G | 800–819 | **M3 Ultra 512G 可本地跑 DeepSeek V3/R1 Q4**（权重 ~360–400G，全站少数单设备可行方案） |

#### 3.2.6 国产卡（数值多为公开报道口径，`data_confidence: reported`）

| 厂商 | 型号 | 显存 | 带宽 | 互联 | 软件栈 | 备注 |
|------|------|------|------|------|--------|------|
| 华为昇腾 | Atlas 300I Pro / 300I Duo | 24 / 96 G LPDDR4X | 205 / 408 | 无 | CANN/MindIE | INT8 140/280 TOPS，边缘/机架推理 |
| 华为昇腾 | **910B** | 64G HBM2e | ~1600 | HCCS ~392（reported） | CANN，vLLM-Ascend | 对标 A100 |
| 华为昇腾 | **910C** | ~128G（双芯合封） | ~3200 | HCCS | 同上 | CloudMatrix 384 超节点（384 卡） |
| 摩尔线程 | MTT S80 / S3000 | 16 / 32 G GDDR6 | ~448 | 无 | MUSA（musify 移植 CUDA） | 消费/入门 |
| 摩尔线程 | **MTT S4000** | 48G GDDR6 | ~768 | 8 卡 PCIe 互联 | MUSA，KUAE 集群方案 | 主推 AI 推理 |
| 寒武纪 | MLU370-X8 | 48G LPDDR5 | 614 | MLU-Link | Neuware/MagicMind | INT8 256 TOPS |
| 海光 | DCU K100 | 64G HBM2e | ~2000 | xGMI 类 | DTK（类 ROCm） | 兼容 HIP 生态 |
| 百度昆仑芯 | P800 | —（待核实） | — | — | — | 二期收录 |
| 燧原/天数智芯等 | S60 / BI 系列 | — | — | — | — | 二期收录 |

#### 3.2.7 特殊架构（`service_only`，只记录实测性能、不套带宽公式）

| 厂商 | 型号 | 形态 | 说明 |
|------|------|------|------|
| Groq | LPU | 230MB SRAM/片，数百片集群 | 权重全驻 SRAM，单流 Llama3-8B ~800+ tok/s；仅云服务 |
| Cerebras | WSE-3 | 44GB SRAM 整晶圆 | 70B 单流 ~2000 tok/s 量级；仅云服务 |
| Google | TPU v5e/v5p/v6e | HBM，Pod 组网 | 仅 GCP 云，ICI 互联 |
| Tenstorrent | Wormhole n150/n300 | 12G GDDR6 | 开源栈，二期收录 |

这类设备的「速度由权重驻留介质决定」而非 HBM 带宽模型，计算器里用**公开实测 tok/s 直接入库**，不参与 §4 公式。

#### 3.2.8 引擎兼容性矩阵（估算效率系数的维度之一）

| 引擎 | NVIDIA | AMD | Apple | Intel | 昇腾 | 摩尔线程 |
|------|--------|-----|-------|-------|------|----------|
| vLLM/SGLang | ✅ | ROCm fork | ❌ | Gaudi fork | vLLM-Ascend | MUSA 适配版 |
| TensorRT-LLM | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ |
| llama.cpp | ✅ | ✅(HIP/Vulkan) | ✅(Metal) | ✅(SYCL/Vulkan) | ❌ | 部分 |
| MLX | ❌ | ❌ | ✅ | ❌ | ❌ | ❌ |
| MindIE / MagicMind | ❌ | ❌ | ❌ | ❌ | ✅ | ❌ |

### 3.3 精度支持矩阵（按架构继承，新厂商已并入）

| 架构 | FP16 | BF16 | INT8 | FP8 | FP4 | 说明 |
|------|------|------|------|-----|-----|------|
| Pascal (P100) | ✅无TC | ❌ | ❌ | ❌ | ❌ | |
| Volta (V100) | ✅ | ❌ | ❌ | ❌ | ❌ | |
| Turing (20系/T4) | ✅ | ❌ | ✅ | ❌ | ❌ | |
| Ampere (30系/A100) | ✅ | ✅ | ✅ | ❌ | ❌ | |
| Ada (40系/L40S) | ✅ | ✅ | ✅ | ✅E4M3 | ❌ | |
| Hopper (H100/H200) | ✅ | ✅ | ✅ | ✅TE | ❌ | |
| Blackwell (50系/B200) | ✅ | ✅ | ✅ | ✅ | ✅ | |
| CDNA2 (MI250) | ✅ | ✅ | ✅ | ❌ | ❌ | |
| CDNA3 (MI300) | ✅ | ✅ | ✅ | ✅ | ❌ | |
| CDNA4 (MI355) | ✅ | ✅ | ✅ | ✅ | ✅ | reported |
| RDNA3/4 消费卡 | ✅ | 部分 | ✅ | ❌/✅(RDNA4) | ❌ | WMMA |
| Apple M | ✅ | ✅(M2+，经 MLX) | dequant | ❌ | ❌ | 量化走反量化路径 |
| Gaudi 2/3 | ✅ | ✅主力 | ✅ | ✅ | ❌ | |
| 昇腾 910 系 | ✅ | ✅ | ✅ | ❌ | ❌ | reported |
| MUSA (摩尔线程) | ✅ | ✅ | ✅ | ❌ | ❌ | reported |

**重要规则**：硬件不支持的量化档位也能跑（反量化回 FP16/BF16 计算）——只省显存、不加速，计算器分两档提示「省显存 ✓ / 真加速 ✗」。

### 3.4 模型数据 schema（`data/models/*.json`）

```json
{
  "id": "deepseek-ai/DeepSeek-V3.1",
  "name": "DeepSeek V3.1",
  "released": "2025-08",
  "arch_type": "moe",               // dense | moe | vlm | embedding | reranker
  "params_total_b": 671,
  "params_active_b": 37,
  "layers": 61,
  "hidden": 7168,
  "attn": { "type": "mla", "kv_lora_rank": 512, "qk_rope_dim": 64 },
  "moe": { "n_experts": 256, "n_active": 8 },
  "max_context": 131072,
  "license": "MIT",
  "quant_compatible": ["fp8", "int4_awq", "gguf_q4"],
  "vlm_extra_vram_gb": 0,
  "hf_downloads": 1234567,
  "arena_elo": 1412,
  "hot_score": 98
}
```

- KV 字段按 `attn.type` 分别记录：`mha`/`gqa` 记 n_kv_heads × head_dim；`mla` 记 latent 维度；`sliding_window`（Gemma/Mistral）记窗口。
- `arch_cfg` 缺失时启发式兜底：head_dim 默认 128，层数按参数量粗估（<2B:16 / <8B:32 / <20B:40 / <50B:48 / <150B:60 / 否则 80），人工覆盖层优先级最高。
- `arch_type: embedding | reranker` 收录但**不参与吞吐计算**（非生成模型），避免误用。
- VLM 记 `vlm_extra_vram_gb`（视觉编码器常驻）+ 图片 token 换算系数（约 每图 256–1280 token，计入 KV）。

**首批收录（2023.09–2026.09，约 40 个系列）：** DeepSeek V2/V2.5/V3/V3.1/R1 + 全系蒸馏（1.5B–70B）、Qwen1.5/2/2.5/3 全系（0.5B–480B，含 Coder）、Llama-2/3/3.1/3.2/3.3/4 Scout·Maverick、GLM-4-9B/4.5/4.5-Air、Kimi K2、Mistral 7B/Mixtral 8x7B/8x22B/Small/Large、Gemma 2/3、Yi、Phi-3/4、InternLM2.5/3、Baichuan2、MiniMax M1、gpt-oss-20b/120b、Command R+、Nemotron，以及 VLM 子集（Qwen-VL、Llama-3.2-Vision、InternVL）和 embedding/reranker 子集（BGE 系列，仅展示）。

### 3.5 采集管道（`scripts/collect/`）

- **HuggingFace**：`GET /api/models?pipeline_tag=text-generation&sort=downloads` 拉列表；逐模型取 `config.json` 解析 layers/kv_heads/MLA 维度，取 `model.safetensors.index.json` 的 total 字段得真实参数量；解析失败进 `models_manual.yaml` 人工兜底（优先级最高）。
- **Arena**：LMArena 榜单快照，仅作 `hot_score` 热度权重。
- 边界说明：**HF 全量不可行也不必要**（数百万仓库、大量非 LLM）。按 ≥1B 参数 + 生成式任务 + 热度筛选，覆盖 99% 实际部署需求。
- 产物即 git 内 JSON，月度跑脚本 + 人工 review PR，运行时不实时抓取（离线可用、结果可复现）。

---

## 4. 计算公式体系（核心）

符号：`P` 总参数，`Pa` 激活参数，`q` 量化字节数，`V` 单卡显存，`BW` 显存带宽，`F` 有效算力，`B` 并发批大小，`S` 上下文长度，`N` 卡数，`TP` 张量并行度。

### 4.1 显存（模式 1 的依据）——五段式拆分

```
(a) 权重      M_w   = P × q                       # q 见下表
(b) KV cache  M_kv  = kv × S × B                  # kv 按 MHA/GQA/MLA 分别算，可开 FP8/INT4-KV 再减半/再减
(c) 框架底座  M_fw  ≈ 2.5 GB（FP16）/ 1.5 GB（量化引擎）
(d) 激活缓冲  M_act ≈ M_w × 2% × min(B, 8)         # continuous batching 下的工作区
(e) 系统预留  M_sys ≈ 0.8 GB                       # CUDA context/OS/输出缓冲
总需求 = (a)+(b)+(c)+(d)+(e)
单卡预算 V_eff = V × util                          # 离散卡 0.95（紧凑假设，贴边组合靠余量提示兜住）；统一内存 0.70
可行性：分片后 ≤ N × V_eff（TP 内权重与 KV 均分片）
```

**量化字节数表 `q`：**

| 档位 | bytes/param | 70B 权重 | 说明 |
|------|-------------|----------|------|
| FP16/BF16 | 2.0 | 140 G | |
| FP8 / GGUF Q8 | 1.0 | 70 G | Hopper/Blackwell 原生加速 |
| INT8 | ~1.05 | 73.5 G | |
| INT4 GPTQ/AWQ / NF4 | ~0.55 | 38.5 G | 生产常用（v1 实现取值 0.55） |
| INT4 EXL2 | ~0.5 | 35 G | 最紧凑 |
| GGUF Q6_K / Q5_K_M / Q4_K_M | 0.65 / 0.5 / 0.45 | 45.5 / 35 / 31.5 G | llama.cpp 系 |

**KV 每 token 字节数 `kv`：**
```
MHA/GQA: 2(K,V) × layers × n_kv_heads × head_dim × dtype_bytes
         例: Llama3-70B(GQA,80层,8kv头,128维,FP16) = 320KB/token → 128K 上下文单请求 40GB
MLA:     layers × (kv_lora_rank + rope_dim) × dtype_bytes
         例: DeepSeek V3(61层,576维) ≈ 70KB/token → 同场景仅 8.6GB（MLA 的价值）
VLM:     常规 KV + 图片 token 数 × kv
```

**统一内存设备（Apple M / Strix Halo）修正：** `V_eff = 物理内存 × 0.7`（wired limit，可调参），无 M_fw/M_sys 中的 CUDA 项但保留引擎工作区。

**模式 1 输出** = 对「模型库 × 量化档位 × 常用上下文 {4k, 32k, 128k}」枚举这张表，出 ✅/⚠️(余量<10%)/❌ 矩阵 + 五段明细。

### 4.2 吞吐与延迟（模式 2 的依据）

**Decode（显存带宽瓶颈）：**
```
单步访存  bytes_step = (Pa × q)/TP  +  B × kv × S_cur      # 大 batch 长上下文时 KV 读取不可忽略
t_step   = bytes_step / (BW × η_bw) + t_comm(B)
单流 TPS = 1 / t_step；聚合 TPS = B / t_step；TPM = ×60
η_bw ≈ 0.6~0.8（引擎系数校准）；FP16 引擎偏 0.55~0.65，高效 INT4 kernel 可到 0.75
```

**通信项（TP 才存在，与 batch 挂钩——比报告里的固定惩罚更准）：**
```
t_comm ≈ layers × 2次AllReduce × (B × hidden × 2B) / link_bw_eff
NVLink 900GB/s: 可忽略（κ≈1.05~1.15）
PCIe4 x16 ~25GB/s 有效: B 大时显著（4090 无 NVLink 的真实痛点），κ 可到 1.3~1.8
```

**Prefill（算力瓶颈）：**
```
t_prefill ≈ 2 × Pa × S_prompt / (F × η_flops)，η_flops ≈ 0.35~0.55
TTFT = t_prefill + 排队；长 prompt 用 chunked prefill 与 decode 交叠
```

**MoE 修正：** (a)(b) 显存按 `P`，decode/prefill 速度按 `Pa`——V3/K2「装得下、跑得快」的原因，也是最容易算错的地方。v1 实现中另有三个校准系数（单测锚定公开实测）：
- MoE 有效读取随 batch 过渡：b=1 只读激活专家，b≥32 读满全部专家（线性过渡）；
- MoE 单流折减：多卡 ×0.2（专家跨卡调度），单设备 ×0.35，随 batch 渐近 1；
- GGUF 多卡且无 NVLink 时带宽效率 ×0.7（llama.cpp offload 损失）。

**模式 2 输出**：单流/聚合 TPS、TPM、TTFT、TPOT + 完整 `formula_trace`（中间量全部返回，前端可展开）+ 「吞吐–并发」「显存–上下文」两条曲线的数据点数组。

### 4.3 反向规划求解器（模式 3）

输入：模型 + 目标 TPM + 平均上下文 + 并发 + SLA（TTFT/TPOT 上限，可选）+ 预算上限（可选）。

```
1. 量化枚举：{FP16, FP8, INT4-AWQ, GGUF-Q4, FP4(仅Blackwell/CDNA4)} × 过滤硬件不支持精度
2. 可行性：每种硬件 × N∈{1,2,4,8,16,…} 用 §4.1 求最小可行 N
3. 吞吐模拟：§4.2 算聚合 TPS，筛 ≥ 目标 TPM；再校验 SLA（单流 TPS / TTFT 达标）
4. 并行策略推荐规则：
   - 单卡装下 → 单卡 + DP 副本扩吞吐（最省心，优先）
   - 单机多卡：有 NVLink/XGMI → TP；无（4090/5090）→ 建议 TP≤2、降量化挤单卡、或 PP
   - 跨机：PP / DP；MoE 大模型（V3/K2）给 EP + P/D 分离生产级方案
   - Gaudi/跨机以太网：按 RDMA 链路重估 t_comm
5. 缓存与加速策略标注（按场景打勾）：
   prefix caching（共享 system prompt/多轮）、continuous batching、chunked prefill（长 prompt）、
   推测解码 EAGLE/MTP（单流 +1.5~2.5×）、KV offload（Grace/大内存机型）
6. 排序：达标方案按「每小时成本（云时价 or 二手价摊销 + 电费）」升序，输出省钱/均衡/性能三档，
   每方案附：拓扑（几机几卡、TP/PP/DP/EP 怎么切）、引擎、预计单流TPS/聚合TPM/TTFT、
   每百万 token 成本、冷启动时间量级（权重加载秒~分钟级）、部署清单 JSON/YAML 导出
```

### 4.4 校准机制（诚实性的关键）

理论公式系统性偏乐观，每张「硬件 × 引擎 × 量化」组合留 `calibration`（decode_eff / prefill_eff），用公开 benchmark 回归。**锚点（M2 单测基准，来自公开实测量级）：**

| 场景 | 公式应落在 | 实测量级 |
|------|-----------|----------|
| Qwen2.5-7B INT4 @ 1×4090 | 120–200 tok/s 单流 | llama.cpp 4090 实测 ~120–180 |
| Llama-3.1-8B FP16 @ 1×H100 | 60–100 tok/s | vLLM H100 实测 ~65–90 |
| Llama-3.1-70B INT4 @ 2×4090 | 15–30 tok/s 单流 | 社区实测 ~18–25 |
| DeepSeek-R1 INT4 @ 8×H200 (TP8) | 聚合数百 tok/s，单流 20–40 | DeepSeek 公开部署数据 |
| Llama-3.1-70B Q4_K_M @ M3 Ultra 512G | 10–20 tok/s 单流 | MLX 实测 ~12–18 |

UI 明示：**「估算值，典型误差 ±30~50%，以实测为准」**，有校准数据的组合显示「已校准」标。

---

## 5. 技术架构

```
llm_calculate/
├── cmd/server/main.go          # 入口：embed 静态资源 + JSON 数据，起 HTTP
├── internal/
│   ├── calc/                   # 纯函数计算引擎（§4 全部公式），table-driven 单测
│   │   ├── memory.go           # 五段式显存
│   │   ├── throughput.go       # decode/prefill/通信/TTFT/TPOT
│   │   ├── planner.go          # 模式3 枚举求解
│   │   └── calc_test.go        # §4.4 锚点用例
│   ├── data/                   # schema 校验、手动覆盖层合并、confidence 标记
│   └── api/                    # HTTP handler，薄层转 JSON
├── data/
│   ├── gpus.yaml               # §3.2 全厂商清单
│   ├── models/                 # 每模型一个 JSON（脚本生成）
│   ├── models_manual.yaml      # 人工兜底覆盖层
│   └── calibration.yaml        # 效率系数
├── web/
│   ├── index.html              # 三 Tab + 库浏览页 + 速查页
│   ├── app.js                  # vanilla JS，无构建步骤
│   └── echarts CDN
└── scripts/collect/            # HF/Arena 采集（Go），月度手动跑
```

**选型理由（保持 stupidly simple）**：无数据库、无前端构建链、无运行时外部依赖；`go build` 一个二进制带走；计算引擎纯 Go 可单测可复用；数据即文件，PR 友好。

**API（返回均含 `formula_trace` 与 `confidence` 字段）：**
```
GET  /api/gpus    GET /api/models
POST /api/calc/fit     # 模式1: {gpu_id, n} → 模型×量化红绿灯矩阵
POST /api/calc/perf    # 模式2: {gpu_id, n, model_id, quant, ctx, batch} → TPS/TPM/TTFT/曲线
POST /api/calc/plan    # 模式3: {model_id, target_tpm, ctx, conc, sla?, budget?} → 方案列表
POST /api/export       # 方案 → JSON/YAML 部署清单（可喂给 k8s/vLLM）
```

---

## 6. UI 草拟

单页应用，五个视图：

- **Tab1 能装什么**：选硬件（可多张）→ 「模型 × 量化」红绿灯矩阵，hover 显示五段显存明细与余量。
- **Tab2 能跑多快**：硬件×N + 模型 + 量化 + 上下文 + 并发 → 仪表盘（单流/聚合 TPS、TPM、TTFT、TPOT）+ 公式推导折叠面板 + 两条曲线。
- **Tab3 要怎么配**：模型 + 目标 TPM + 并发/上下文 + SLA/预算 → 省钱/均衡/性能三档方案卡片（拓扑、缓存建议、每百万 token 成本、导出按钮）。
- **库浏览**：硬件参数表（按厂商/显存/带宽/精度筛选排序，reported 参数标灰）、模型参数表（来源与更新日期）。
- **速查表**（来自报告 2 附录 B 的灵感，自动生成）：「每张卡 FP16/INT4 能扛的最大模型」一屏看完，例如 4090 ≈ 7B FP16 / 30B INT4，M3 Ultra 512G ≈ V3/R1 Q4。

---

## 7. 里程碑

| 阶段 | 内容 | 验收 |
|------|------|------|
| M1 | 数据 schema + NVIDIA 全系入库（§3.2.1/3.2.2）+ 校验测试 | `gpus.yaml` 过校验 |
| M2 | calc 引擎 + 单测（§4.4 五个锚点） | 单测绿，锚点落在实测量级内 |
| M3 | 模式 1/2 API + 前端两个 Tab | 网页可用 |
| M4 | HF 采集脚本 + 首批模型入库 | `data/models/` 生成 |
| M5 | 模式 3 求解器 + 成本 + 导出 + Tab3 | 方案合理 |
| M6 | AMD/Intel/Apple 数据入库 + 引擎效率系数 | 跨厂商估算可用 |
| M7 | 国产卡（reported 口径）+ 特殊架构实测库 + 校准体系 + arena 热度 | 锦上添花 |

> 国产卡/魔改卡参数多为报道口径，统一 `data_confidence: reported`，入库前逐条核对官方资料，核不到的宁可留空也不编数。

---

## 8. 明确不做（首版）

- 训练显存估算（只做推理）
- 实时比价、云库存查询；Groq/Cerebras/TPU 仅展示实测档，不套公式
- 用户系统、收藏同步（localStorage 即可）
- 端侧 NPU（手机 SoC）——二期再说

---

## 附录 A：两份外部调研报告的勘误（吸收时已修正）

| 报告原述 | 修正 |
|----------|------|
| 4090 48G 魔改 = HBM2e 4096-bit ~403GB/s | 实际为双面 GDDR6X 颗粒，带宽≈原版 1008GB/s |
| 5090D V1 = 28G | 5090D 为 32G（与 5090 同容量，砍 AI TOPS）；24G 的是 5090D V2 |
| P100 (Volta) | P100 是 **Pascal**（无 Tensor Core） |
| T4 INT8 5000 TOPS | 实为 ~130 TOPS |
| A10 有 NVLink(50GB/s) | A10 无 NVLink；有的是 A30(200GB/s) |
| H20 NVLink 降为 400GB/s | H20 的 NVLink 为 900GB/s 未砍，砍的是算力 |
| H800 显存带宽 4000GB/s | 为 3350GB/s（同 H100 SXM），砍的是 NVLink→400 |
| H10 24G / DeepSeek-V4 / Falcon-210B / Qwen3-235B-A5B | 查无实据/命名有误（235B 为 A22B），不收录 |
| Qwen2.5-32B @128K KV = 17.18GB | 公式复算为 ~34GB（64层×8kv头×128维×2×2B×128K），报告少算一倍 |
| GH200 192G LPDDR5x 版 | GH200 为 HBM3 + Grace LPDDR5X 统一内存，非「192G LPDDR5x」 |
| TP 收益上限 min(N,2)×0.85 | 过于保守；改为 §4.2 带宽聚合 − t_comm(B) 模型 |
| INT4 prefill 加速比 3.5×/5× | W4A16 反量化对 prefill 加速有限且强依赖引擎，默认取 1.0，靠校准系数修正 |
