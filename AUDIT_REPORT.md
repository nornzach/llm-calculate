# LLM 推理计算器修复与可信度边界

**更新日期：2026-09-05**

本报告取代旧版“全量修正”“零启发式 active”及不带同条件证据的速度锚点结论。结构校验通过不等于数据已经逐条核验；解析估算也不是压测。

## 已修复

### 计算与推荐

- 规划、推荐与前端去重保留量化、引擎、KV、推测方式、卡数、副本、并发及拓扑差异，不再先按最高 TPM 吞掉便宜部署。
- latency 目标使用完整请求 P95（服务 P95 加估算排队 P95），不是 TTFT + 一个 TPOT。两种 P95 相加仍是保守场景分数，不是联合分布的实测分位数。
- Output=1 不再被虚构为一次后续 decode；decode 平均值、分位数及桶指标只计实际后续 token。全部输出为一个 token 时，decode 指标为零，TTFT、req/s、mixed TPM 仍有意义。
- 调度开销明细使用实际纳入 decode step 的数值。
- 显式 query heads 用于 TP 约束和 attention FLOPs。Qwen3 等模型的 query 宽度可能不同于 hidden size，不能通用地用 hidden/head_dim 推断 query heads；残差通信仍使用 hidden。
- 无效 TP/PP/EP/CP 不再给出正常回退速度。聚合整柜、diffusion/block diffusion、未建模 CP/DCP 或不完整关键几何不会产生正常性能结论。

### 支持状态

Perf 和规划结果区分：

- `fit`：仅显存条件。
- `support`：supported、conditional、unsupported、unknown。
- `support_reason`：约束或缺失前提。
- `estimate_valid`：该输入是否适用于当前性能模型。
- `deployable`：静态已支持且显存满足；不是现场部署认证。

无效估算的速度为零并由界面隐藏。未知/不支持或无效估算不进入自动推荐；conditional 可作为待验证候选，但不标绿或声称已确认部署。

兼容性不再以厂商匹配作为唯一条件：旧 NVIDIA 架构、厂商插件、量化加载、query/KV 分片、互联域和执行家族有独立约束。auto 可为旧 NVIDIA 选择 llama.cpp 条件路径，显式选择不会被偷偷替换。FP8 KV 存储与原生 FP8 算术能力分开，A100 仍需要核对 CUDA、attention backend 和运行时版本。MXFP4 使用统一加载路径规则，不再由 Accel 充当可加载性。

`analytical` 与 `scenario` 都不是实测校准。填写覆盖参数不会获得 `calibrated` 认证。`peak_exact` 只对有来源的对应 dense 矩阵规格成立；向量、营销换算及未核验 TF 只能作为有条件的场景输入。

### 数据

硬件保留 121 个 ID。确认修改包括 A100 PCIe/SXM、P100 PCIe、V100 SXM2 的规格组合，RX 9070 XT 的 dense FP16/FP8/INT8 峰值和 TPU7x 的官方 pod 口径。DGX Spark 的 140W 芯片 TDP、240W PSU 额定值不能当作实测整机功耗，整机功耗保留未知。

历史 H100 PCIe 600GB/s NVLink 和 B200 2250/4500/9000 dense 峰值未被误改。GB200/GB300 标称/可用容量与 MI355X 链路聚合口径的争议不作为确认错误。

模型采集器：

- 按 SHA 固定 config、分片 index 和单文件 blob 元数据读取。
- 逻辑参数与 packed tensor 元素、payload 字节分开；处理共享 embedding 和明确的嵌套量化配置。
- 不再用 payload/参数量比值猜存储格式；缺少 KV heads 时按配置中的 MQA/MHA 语义处理，不再猜 GQA-8。
- 保存 heads、revision、param_source、extended_ctx 和 encoder_params；不能取得的值不会虚构核验。
- Qwen3-0.6B 为 0.6B；NVIDIA Qwen3-8B-NVFP4 为 8.2B/FP4；Falcon-7B 为 MQA、一个 KV head。
- **复核更正：Qwen2.5-7B Base 原生配置为 131072；Instruct 为 32768，扩展 131072 需对应运行条件。不能因为名字相近就统一成 32K。**
- 精选 Qwen 条目同步相同 SHA 来源中的架构元数据；Llama 3.1 8B/70B 使用 Meta 发布规模作取整场景，未假装绑定具体权重 revision。

当前目录为 62 个精选 + 2195 个 HF + 0 个 ModelScope，共 2257 个 ID。12 条记录固定了 revision；14 条记录具有已核对的参数来源（含取整模型卡规模）；296 条多模态记录仍缺独立 encoder 参数。其余旧记录没有完成逐字段外部核验。官方发布机构标签不代表参数、格式或性能已经验证。

### 页面

显存条、性能页、矩阵、地图、方案与处方统一使用支持状态。无效配置只显示状态与容量诊断，不展示虚构速度。语言切换通过一次性 sessionStorage 保留页面、原生输入、选择器、负载、高级参数、自定义模型和展开状态。首页默认使用已补齐元数据的 Qwen3-8B，而不是未经核验的新条目。

## 验证证据

- `go test ./...` 通过；移除了缺乏同条件来源、用社区范围或“同构估计”硬断言速度的旧锚点测试，没有重钉伪精度。
- `node --check web/app.js` 与 `go build -trimpath -o llmcalc .` 通过。
- 真实采集命令定向刷新 `nvidia/Qwen3-8B-NVFP4`，输出 8.2B、FP4、固定 SHA `ccd10a893cbca613259517c3efe08e151ddf2b8e` 和 6.396932352GB payload。
- HTTP 反例确认：P100/TRT、TP3/4卡、Qwen2.5 TP8、整柜和 diffusion 不再输出正常性能；A100 FP8 KV 不因缺原生 FP8 算术被一概否决。
- Llama 70B INT4/6000 TPM 场景中，规划器保留 H200 的 1/2/4/8 卡成本。推荐返回与规划相同的最低月成本方案；推荐输出上限不意味着包含每个硬件候选。
- 混合 Output=1/101 的请求不再污染 decode 统计；长输出 latency 方案按完整请求 P95 排序。
- 实际 Chromium 页面验证：地图 9000 TPM/并发3、性能页 H200/TP3/输出777和高级面板展开状态在语言切换后保留；TP3 被拒绝并隐藏速度，恢复 TP4 后显示条件估算与完整 E2E 指标。

上述是计算器行为验证，不是 GPU 实测 benchmark。

## 仍需的生产前提

1. 固定 checkpoint、引擎/驱动/OS、kernel、量化方法、KV backend 和实际拓扑。
2. 测量加载显存、workspace、同负载 TTFT/TPOT/吞吐、功耗和误差范围。
3. 真实连续批处理、cache 驱逐、通信拥塞与 overlap 未被静态模型完整模拟。
4. P99.9 显存是负载分桶的矩近似，排队是 M/M/c；不保证生产尾延迟或 SLO。
5. 所有 CNY 为未核验参考假设，非当前报价；36 月摊销、电价和功耗比例不是完整 TCO。
6. SLO/goodput、质量约束、完整 TCO、MIG/多模型共卡、场景导出和完整非自回归模拟仍属功能扩展，本轮未实现。

## 一手来源

- [A100 官方数据表](https://www.nvidia.com/content/dam/en-zz/Solutions/Data-Center/a100/pdf/nvidia-a100-datasheet-us-nvidia-1758950-r4-web.pdf)
- [P100 PCIe](https://images.nvidia.com/content/tesla/pdf/nvidia-tesla-p100-PCIe-datasheet.pdf)
- [V100](https://images.nvidia.com/content/technologies/volta/pdf/tesla-volta-v100-datasheet-letter-fnl-web.pdf)
- [RX 9070 XT](https://www.amd.com/en/products/graphics/desktops/radeon/9000-series/amd-radeon-rx-9070xt.html)
- [DGX B200](https://www.nvidia.com/en-us/data-center/dgx-b200/)
- [DGX Spark](https://www.nvidia.com/en-us/products/workstations/dgx-spark/)
- [TPU7x](https://docs.cloud.google.com/tpu/docs/tpu7x)
- [Qwen3-0.6B](https://huggingface.co/Qwen/Qwen3-0.6B)
- [Qwen3-8B-NVFP4](https://huggingface.co/nvidia/Qwen3-8B-NVFP4)
- [Qwen2.5 Base config](https://huggingface.co/Qwen/Qwen2.5-7B/raw/main/config.json)
- [Qwen2.5 Instruct config](https://huggingface.co/Qwen/Qwen2.5-7B-Instruct/raw/main/config.json)
- [Falcon-7B config](https://huggingface.co/tiiuae/falcon-7b/raw/main/config.json)
- [DiffusionGemma](https://huggingface.co/google/diffusiongemma-26B-A4B-it)
- [Meta Llama 3.1](https://github.com/meta-llama/llama-models/blob/main/models/llama3_1/MODEL_CARD.md)
- [TensorRT-LLM support matrix](https://nvidia.github.io/TensorRT-LLM/reference/support-matrix.html)
- [vLLM FP8 KV](https://docs.vllm.ai/en/latest/features/quantization/quantized_kvcache/)
- [vLLM context parallelism](https://docs.vllm.ai/en/latest/serving/context_parallel_deployment/)
