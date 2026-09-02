# LLM 推理计算器审计报告

**审计日期：** 2026-09-02  
**审计范围：** 模型元数据、加速卡、量化、推理引擎、推测解码、缓存、并行部署、显存与性能公式  
**结论：** 未校准结果适合显存可行性和一阶容量筛选；完整填写同条件实测利用率后可作为部署 what-if。任何口径都不应脱离压测直接用于采购承诺、生产 SLA 或成本预算。

本报告替代此前同名报告。此前报告含多项可证伪结论，例如把 B200 带宽写成 14 TB/s、把 B300 写成 22 TB/s、把 RTX 5090 写成 24 GB/672 GB/s、声称 B300/NVFP4/EXL3/推测解码尚未实现，并在无同条件基准的情况下给出“典型误差 ±30~50%”。这些结论均不再有效。

## 1. 可信度结论

| 部分 | 当前用途 | 可信度边界 |
|---|---|---|
| 权重显存 | 初筛/校准 | 优先使用用户实测加载 GB；原生量化匹配时使用 safetensors payload；否则按参数量×格式 bytes/parameter。混合精度、padding 和运行时重排仍需实测 |
| KV/固定状态显存 | 初筛 | 已区分 MHA/GQA/MLA、全局/滑窗/线性 attention、共享前缀、KV allocator、量化、CP 与 offload；未模拟驱逐、远端容量上限和逐层混合 dtype |
| 可部署性 | 静态容量判断 | 引擎预算、运行时、workspace、adapter/draft 均可覆盖；默认预算仍是场景值，不是设备探测 |
| Decode/Prefill | 一阶 roofline | 已建模 HBM、算力、TP/PP/EP/CP 通信、chunked prefill、媒体 encoder 与 offload；analytical 无统计置信区间 |
| 校准模式 | 同部署 what-if | 只有 HBM/算力/调度及多卡互联利用率均有实测输入时标为 `calibrated`；校准值不可跨模型、版本和负载迁移 |
| 规划器 | 候选枚举 | 显示容量、目标到达率、利用率及 M/M/c 平均/p95 排队等待；其泊松到达、指数服务和独立副本假设不等于生产 SLA |
| 自动采集模型 | 发现与粗筛 | 结构和 payload 覆盖大幅补齐；`conf=fetched` 仍不是官方背书，模型更新后必须重新采集 |
| 硬件与价格 | 规格检索 | 关键旗舰逐精度 dense 峰值附一手来源；缺逐精度峰值时明确使用倍率估算，`reported` 和价格不能当采购报价 |

## 2. 本次全量修正

### 2.1 模型参数与检查点

采集器现在优先使用 Hugging Face `safetensors.total` 作为与存储 dtype 无关的参数量，并独立保存 safetensors payload。分片仓库读取 index `metadata.total_size`；单文件仓库从 `siblings[].size` 汇总。

存储精度不再盲信 `quantization_config`：payload bytes/parameter ≥1.45 判为 FP16/BF16 族，0.72–1.45 判为 8-bit 族，更低判为 4-bit 族并保留已知 MXFP4/NVFP4 格式。因此 DeepSeek-V3-0324 的 1,369 GB 源仓库被正确识别为 FP16/BF16 payload，而不是因配置能力误标 FP8。

MoE active 参数优先级：

1. 官方模型卡覆盖；
2. `MoE layers × experts × 3 × hidden × expert intermediate` 的结构推导；
3. 模型名 `A…B`；
4. 仅在结构缺失时使用启发式，并在备注中明确标识。

重点抽查：

| 模型 | 总参数 | 激活参数 | 路由 | Payload / 原生格式 |
|---|---:|---:|---|---|
| OpenAI GPT-OSS-120B | 116.8B | 5.1B 官方 | 128×Top4，36 MoE 层 | 65.25 GB / MXFP4 |
| Qwen3-30B-A3B-Instruct-2507 | 30.5B | 3.3B 结构推导 | 128×Top8，48 MoE 层 | 61.06 GB / FP16 |
| DeepSeek-V3-0324 | 684.5B | 37B 官方 | 256×Top8，58 MoE 层 | 1,369.06 GB / FP16 |
| Kimi-K2-Instruct | 1,026.4B | 32B 官方 | 384×Top8，60 MoE 层 | 1,029.19 GB / FP8 |

模型字段新增 `model_type`、dense/MoE intermediate、shared experts、MoE layers、MTP heads、multimodal、encoder params、checkpoint GB、native quant 和 source URL。完整刷新结果：

- 445 条 HF 自动记录；与 62 条精选记录合并后运行时共 507 个模型；
- 190 条 MoE，`experts/topk` 缺失为 0；
- 153 条 active 由结构推导，8 条仍为明确标识的启发式；
- 418 条取得 checkpoint payload，445 条均有源仓库 URL；
- 97 条 local/sliding attention、18 条 hybrid recurrent state、57 条 MTP、12 条多模态包装模型。

### 2.2 显存与 KV

单卡可分配量与物理显存分开：

`Budget = VRAM × mem_util`，`System reserve = VRAM - Budget`

`Fit` 只比较权重、KV、运行时、workspace、adapter/draft 与 `Budget`，不再把系统预留重复扣减。70B INT4 在 2×24 GB、4K、低并发下因此是“贴边可行”而非错误的“装不下”；FitMatrix 仍按 <10% 余量显示警告。

权重按拓扑拆分：

- 文本基础权重：`base / (TP×PP)`；
- routed expert：`experts / (TP×PP×EP)`；
- encoder：保守按 `encoder / TP`；
- 用户实测 `weight_gb` 优先于所有推导。

KV 处理：

- GQA 每 rank 比例 `ceil(KV_heads/TP)/KV_heads`，TP 超过 KV heads 时复制；
- MLA latent cache 默认在 TP ranks 复制；
- PP/CP 分摊 KV，linear recurrent state 单独按请求计；
- prefix hit 对 full-attention 共享 block 只驻留一份；local attention 只共享仍位于滑窗尾部的 block；
- `kv_overhead` 表达 allocator/block 开销；
- `kv_offload` 从 GPU 容量移出，并按 `offload_bw` 把回读和写入时延计回；
- max batch 同时计入每请求 KV、state 和 prefill workspace，不再只除 KV。

### 2.3 Decode、Prefill 与并行

当前 decode 单步：

`T_step = max(T_HBM + T_offload, T_compute) + T_TP + T_EP + T_CP + T_PP + T_schedule`

- HBM 读取包括当前 batch 期望触达的唯一 MoE 专家、完整 K+V、线性 state 与 adapter；
- MoE 有结构时分离基础/专家权重；EP 对 expert 计算和存储分片，并按 TopK 计 dispatch+combine All-to-All；
- `router_skew` 表达最忙 rank / 平均负载，低 token 数还会自动加入离散路由下界；
- TP 用 ring `2×(TP-1)/TP`，CP 和 PP 分别计 attention collective 与 stage activation；
- TP×PP×EP×CP 必须等于卡数；Dense+EP 或乘积错误会标记无效并回退全 TP；
- 硬件有逐精度 dense 峰值时直接使用，否则输出 `peak_exact=false` 并标记倍率估算。

当前 prefill：

- 线性项为总计算时间与每 chunk 权重读取时间之较大值；
- `prefill_chunk` 明确计重复权重读取和每 chunk 调度；
- causal、sparse、sliding/local attention 分别使用对应 key 数；
- 计入 TP/EP/CP/PP 通信、KV 写入、offload 写入；
- `encoder_params × media_tokens` 单独形成多模态 encoder 计算/权重读取时间；
- TTFT、TPOT、请求总时延、req/s、decode TPM 和输入+输出 mixed TPM 使用同一输出长度口径。

### 2.4 硬件逐精度峰值

| 加速卡 | 显存 | 带宽 | Dense FP16/BF16 | Dense FP8/INT8 | Dense FP4 | 互联 |
|---|---:|---:|---:|---:|---:|---:|
| NVIDIA H100 SXM | 80 GB | 3.35 TB/s | 989 TF | 1,979 TF / 1,979 TOPS | — | NVLink 900 GB/s |
| NVIDIA H200 | 141 GB | 4.8 TB/s | 989 TF | 1,979 TF / 1,979 TOPS | — | NVLink 900 GB/s |
| NVIDIA B200 | 192 GB 物理规格 | 8 TB/s | 2.25 PF | 4.5 PF / 4.5 POPS | 9 PF | NVLink 1.8 TB/s |
| NVIDIA B300 | 288 GB | 8 TB/s | 2.5 PF | 5 PF / 5 POPS | 15 PF NVFP4 | NVLink 1.8 TB/s |
| NVIDIA Rubin | 288 GB HBM4 | 22 TB/s | 4 PF | 17.5 PF | 50 PF NVFP4 | NVLink 3.6 TB/s |
| AMD MI300X | 192 GB | 5.3 TB/s | 1.307 PF | 2.615 PF / 2.615 POPS | — | Infinity Fabric |
| AMD MI325X | 256 GB | 6 TB/s | 1.307 PF | 2.615 PF / 2.615 POPS | — | Infinity Fabric |
| AMD MI355X | 288 GB | 8 TB/s | 2.5 PF | 5 PF / 5 POPS | 10.1 PF MXFP4 | Infinity Fabric |
| AMD MI455X | 432 GB HBM4 | 23.3 TB/s | 5 PF | 20.1 PF / 20.1 POPS | 40.3 PF MXFP4 | UALoE 3.6 TB/s |

这些字段均使用 dense 口径；NVIDIA 页面带 structured sparsity 的数值不再直接当 dense 峰值。Rubin/MI455X 的初步规格及未公布功耗/价格保持明确状态，不补猜测值。

### 2.5 量化、adapter 与推测解码

| 选择项 | 当前解释 | 边界 |
|---|---|---|
| FP8/INT8 W8A8 | 容量约 1 byte/parameter；优先用硬件逐精度峰值 | kernel、scale、未量化层需按 checkpoint/引擎核对 |
| INT4/AWQ、GGUF、MLX、EXL3 | W4A16/格式级容量与带宽收益 | 不自动套 4×算力 |
| NVFP4 | NVIDIA W4A4 路径 | 只有硬件和引擎支持时使用 FP4 峰值 |
| MXFP4 | MXFP4 权重路径 | 不等于通用 W4A4；GPT-OSS 仍按其实际执行栈校准 |
| FP8/FP4 KV | 分别按 0.5 / 0.281 容量 | FP4 读取加速仅在 SGLang+FP4 GPU 路径计入 |
| Adapter/draft/MTP | 显式 GB 加入权重与访存；MTP 需模型元数据 | 推测增益由 `spec_tau/spec_ovh` 实测覆盖，不保证默认倍率 |

### 2.6 服务负载

规划器不再用“队列深度÷吞吐”冒充等待时间。目标 mixed TPM 先换算 arrival req/s，单副本通过 prefill+decode 串行资源预算得到 service req/s，再使用 Erlang-C：

- 利用率 `ρ = λ/(cμ)`；
- 平均等待 `P(wait)/(cμ-λ)`；
- `P(Wq>t)=P(wait)·exp(-(cμ-λ)t)` 得到无条件 p95；
- `ρ=1` 不存在稳态，开启排队时会增加副本保留服务余量。

UI 同时显示容量 QPS、目标 QPS、利用率、平均/p95 等待，并明确标注 M/M/c 假设。

## 3. 当前覆盖与剩余边界

### 3.1 已可表达的输入

- 模型：总/激活/encoder 参数、layers、hidden、intermediate、MoE experts/top-k/shared experts/MoE layers、MTP heads、MHA/GQA/MLA、KV/local layers、window、linear state、sparse budget、原生 context；
- 检查点：实际加载权重、源 payload、原生量化；
- 部署：TP/PP/EP/CP、运行时常驻、workspace、adapter/draft、显存预算、HBM/算力/互联利用率、调度开销；
- KV/缓存：KV dtype、allocator 系数、prefix hit、offload 比例与有效带宽；
- 负载：固定 context、batch、output length、prefill chunk、media tokens、router skew、spec acceptance/overhead；
- 输出：容量分解、headroom、max batch、decode/prefill 分项、拓扑/峰值来源、req/s、mixed TPM、规划容量与 M/M/c 队列指标。

### 3.2 数据完整性

- 自动模型 445 条中，MoE 路由字段已全覆盖；仍有 8 条 active 只能启发式，27 条未取得 payload；
- 12 条自动多模态记录能识别包装模型，但缺独立 encoder 参数时只计算文本塔并告警；用户可用 `encoder_params`/`media_tokens` 补齐；
- 121 条硬件中 39 条为 `reported`，7 条缺 TDP，13 条缺参考价；仅 11 条附逐项一手 URL；
- 37 条宣称 FP8 能力但缺逐卡 FP8 dense 峰值，11 条 FP4、97 条 INT8 记录缺对应逐精度峰值；计算器会标成倍率估算而非伪装成精确规格。

### 3.3 尚未自动覆盖

以下项目没有足够输入时不会被压缩成无来源固定倍率：

1. Data Parallel、DCP/DPA、wide-EP、PD/EPD 的自动联合搜索与跨节点 placement；
2. 真实 expert histogram、拓扑分层、collective 并发/拥塞和通信-计算 overlap；
3. continuous batching 调度轨迹、CUDA Graph、kernel fusion、算子特定效率；
4. prefix cache 生命周期、驱逐、跨请求不同前缀、分层缓存容量与远端命中；
5. 自动模型的精确视觉/音频塔、encoder cache、逐 adapter 热切换；
6. safetensors 内逐 tensor 混合 dtype、scale、padding 和运行时重排；
7. 到达/长度/采样参数分布及生产 P50/P95/P99、goodput、SLO 违约率；
8. 主机 CPU/NUMA/PCIe/NIC、存储、PUE、机柜功率、云折扣和运维成本；
9. diffusion language model 等非自回归执行范式。

这些是输入缺口，不是再加几个全局系数能可靠修复的问题。

## 4. 精度解释

`analytical` 表示：

- 使用厂商理论峰值或明确标识的倍率估算；
- 使用默认 HBM、FLOPs、通信和调度利用率；
- 输出没有统计置信区间。

`calibrated` 表示：

- HBM、FLOPs、调度均有同部署实测输入；
- 多卡时互联利用率也已输入；
- 结果仍只对该模型、checkpoint、引擎版本、拓扑和负载口径有效。

因此不存在可辩护的全局“±30%”误差。静态模型不能从均值可靠推导生产尾延迟。

## 5. 生产使用门槛

用于实际 sizing 前至少需要：

1. 精确 checkpoint 清单、原生量化和实际加载 `weight_gb`；
2. 引擎 commit/version、kernel、KV dtype、运行时常驻与峰值 workspace；
3. TP/PP/EP/CP placement 和每条链路实测有效带宽；
4. 真实请求 trace：到达率、prefix 重用、输入/输出长度、media token、采样参数；
5. 同条件 TTFT、ITL、TPS、显存、功耗及 P50/P95/P99；
6. 用实测反推 HBM/FLOPs/link 利用率、调度开销、router skew、spec τ/overhead；
7. 使用真实 trace 或压测验证 M/M/c 假设失配后的尾延迟。

未完成这些步骤前，只使用容量余量、瓶颈方向和候选数量级，不使用绝对性能值做承诺。

## 6. 验证

回归契约覆盖：

- checkpoint payload 只在原生量化匹配时使用，用户实测权重优先；
- 系统预留不重复扣减，workspace 不再按权重百分比伪造；
- GQA/MLA TP 复制、PP/CP 分片、EP expert 分片与 TopK All-to-All；
- 无效拓扑回退、逐精度峰值 exact/estimated 状态；
- prefix 共享驻留、KV allocator、量化与 offload 的容量/时延交换；
- chunked prefill 重复读权重、多模态 encoder、adapter/draft；
- decode HBM/compute/communication/schedule roofline 与 long-context attention；
- 校准状态、TTFT/TPOT/E2E、req/s、mixed TPM；
- Erlang-C 解析值、规划容量/目标 QPS/利用率/平均和 p95 等待；
- collector 的结构 active 推导、payload 位宽判定、单文件 safetensors 汇总。

最终验证：

- `go test ./...` 全部通过；
- `node --check web/app.js` 通过，`data/hardware.json` 解析为 121 条；
- 最终服务加载 121 个硬件、507 个去重模型；
- 浏览器实测高级面板 22 个字段；输入 TP4、HBM/FLOPs/link/调度校准和 50% KV offload 后，API 返回 `calibrated`、`TP4 · PP1 · EP1 · CP1`、26.84 ms offload、0.671 GB 外部 KV；
- 自定义 30.5B/3.3B MoE（128×Top8、48 MoE 层、MTP、encoder）规划返回 133 个候选，并显示 M/M/c 容量、目标利用率和平均/p95 等待；
- 模型库展示 HF 一手链接与 checkpoint GB/native quant，硬件库展示逐精度峰值与已录入的一手链接；全页无横向溢出。

## 7. 一手资料

### 硬件

1. [NVIDIA H100 Tensor Core GPU](https://www.nvidia.com/en-us/data-center/h100/)
2. [NVIDIA H200 Tensor Core GPU](https://www.nvidia.com/en-us/data-center/h200/)
3. [NVIDIA Blackwell architecture / B200](https://www.nvidia.com/en-us/data-center/technologies/blackwell-architecture/)
4. [NVIDIA Blackwell Ultra / B300](https://developer.nvidia.com/blog/inside-nvidia-blackwell-ultra-the-chip-powering-the-ai-factory-era)
5. [NVIDIA Vera Rubin NVL72 specifications](https://www.nvidia.com/en-us/data-center/vera-rubin-nvl72/)
6. [NVIDIA GeForce RTX 5090](https://www.nvidia.com/en-us/geforce/graphics-cards/50-series/rtx-5090/)
7. [AMD Instinct MI300X](https://www.amd.com/en/products/accelerators/instinct/mi300/mi300x.html)
8. [AMD Instinct MI325X](https://www.amd.com/en/products/accelerators/instinct/mi300/mi325x.html)
9. [AMD Instinct MI355X](https://www.amd.com/en/products/accelerators/instinct/mi350/mi355x.html)
10. [AMD Instinct MI455X](https://www.amd.com/en/products/accelerators/instinct/mi400/mi455x.html)

### 模型

11. [Qwen3-30B-A3B-Instruct-2507 model card](https://huggingface.co/Qwen/Qwen3-30B-A3B-Instruct-2507)
12. [DeepSeek-V3-0324 model card](https://huggingface.co/deepseek-ai/DeepSeek-V3-0324)
13. [OpenAI GPT-OSS-120B model card](https://huggingface.co/openai/gpt-oss-120b)
14. [Kimi-K2-Instruct model card](https://huggingface.co/moonshotai/Kimi-K2-Instruct)
15. [Kimi K3 model card](https://huggingface.co/moonshotai/Kimi-K3)
16. [Qwen3.8-2.4T-A95B model card](https://huggingface.co/Qwen/Qwen3.8-2.4T-A95B)
17. [GLM-5 repository](https://github.com/zai-org/GLM-5)
18. [Mistral Small 4 model card](https://huggingface.co/mistralai/Mistral-Small-4-119B-2603)

### 量化、缓存与服务

19. [vLLM quantization](https://docs.vllm.ai/en/latest/features/quantization/)
20. [vLLM quantized KV cache](https://docs.vllm.ai/en/latest/features/quantization/quantized_kvcache/)
21. [vLLM speculative decoding](https://docs.vllm.ai/en/latest/features/speculative_decoding/)
22. [vLLM disaggregated prefill](https://docs.vllm.ai/en/latest/features/disagg_prefill/)
23. [vLLM hybrid attention support](https://vllm.ai/blog/2025-09-11-qwen3-next)
24. [SGLang documentation index](https://docs.sglang.io/llms.txt)
25. [SGLang quantized KV cache](https://docs.sglang.ai/advanced_features/quantized_kv_cache.html)
26. [SGLang production metrics](https://docs.sglang.io/docs/references/production_metrics.md)
27. [TensorRT-LLM](https://github.com/NVIDIA/TensorRT-LLM)
28. [llama.cpp quantization](https://github.com/ggml-org/llama.cpp/blob/master/tools/quantize/README.md)
29. [MLX-LM](https://github.com/ml-explore/mlx-lm)
30. [ExLlamaV3](https://github.com/turboderp-org/exllamav3)
31. [LLM inference performance modeling survey](https://arxiv.org/html/2402.16363)
