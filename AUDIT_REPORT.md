# LLM 推理计算器全面功能审计

**更新日期：2026-09-05**

本报告取代旧版“全量修正”“零启发式 active”及不带同条件证据的速度锚点结论。结构校验通过不等于数据已经逐条核验；解析估算也不是压测。

## 本轮结果与覆盖范围

本轮沿真实页面 → HTTP handler → 共享计算器 → 目录解析链路检查了全部九个页面及现有计算选项。确认错误已修复，未把数据缺失、物理 OOM 和算法异常合并为一个“计算失败”。以下“通过”指本地程序行为及回归检查，不表示所有模型/硬件组合都能运行，也不表示 GPU 性能已获实测认证。

| 功能点 | 检查内容 | 本轮结果 |
| --- | --- | --- |
| 地图 | 模型/负载/TPM/并发、成本速度候选、Pareto、显存热图、详情与性能跳转 | Qwen3.8-27B 的 8K/512、16 并发、6000 TPM 返回候选；H200/B200 显示 FP16 容量。未知执行用 `?`，未列原生格式用 N/A，OOM 用 `—` |
| 能装什么 | 硬件/卡数/上下文/并发、六个主量化列、KV 精度、模型搜索、容量过滤与跳转 | 容量不再被执行支持状态覆盖；conditional 不再一律被标为显存贴边；原生量化和容量条件有回归 |
| 能跑多快 | Prefill、decode、TTFT/TPOT、完整 E2E、混合 TPM、分桶、显存组成、曲线与账本 | 输出增长计入 KV；完整序列校验上下文。浏览器将输出 512 改成 8192 后，示例显存由 27.1 GB 增至 35.2 GB，超出 V100 32 GB 后隐藏速度 |
| 怎么配 | 模型/硬件双方向、四个单目标及双目标、量化/负载/卡数/并发、排队、分页和详情 | TOS 使用单流 TPS；可用性作为第二目标也枚举 N+1；展示截断保留各目标端点；固定硬件不会携带禁用的目标/排队约束 |
| 方案库 | TPM/TOS 硬约束、成本/完整响应 P95/可用性、排队、厂商/类别/名称/总卡数过滤、分页和详情 | 截图 Qwen3.8-27B、NVFP4、1M TPM 有合规候选；H200 搜索与 20 条分页浏览器通过；规划结果保留 analytical/scenario 口径 |
| 自定义模型 | Dense/MoE、GQA/MHA/MLA、独立 query heads、KV/local 层、专家/MTP/encoder 参数、退出恢复 | MHA 的 KV heads 与 query heads 同步；默认头维度与 active 输入不再违反 HTML step；隐藏/禁用字段不拦截其他模式；语言切换保留 MHA 和 48/48 头数 |
| 硬件库 | 121 条目录、厂商/文本过滤、规格/来源/价格提示、带入显存页 | H200 筛选唯一记录并正确带入显存页；整柜和服务型设备保留不可本地估算边界 |
| 模型库 | 2257 条目录、来源/架构/版本/精度、搜索/分页、带入性能页 | Qwen3.8-27B-FP8 显示 FP8、30.9 GB payload；进入性能页锁定 FP8，4090 24 GB 明确提示仅权重就超显存 |
| 速查 | 固定权重预算公式、硬件行、显存页跳转 | 权重预算不冒充含 KV/激活的模型适配结论；API 与浏览器通过 |
| 术语库与全局交互 | 99 个术语、中文/英文搜索、主题、语言状态恢复、键盘及鼠标操作、详情关闭 | 英文 KV 搜索返回 19 条；无匹配搜索给出空态；深浅主题与中英切换正常，浏览器控制台无 JS 错误 |
| HTTP 接口 | 六个 GET 数据接口、四个 POST 计算接口、真实路由响应、错误码/JSON | 新增实际 handler 回归；拒绝非法 JSON、尾随内容、未知字段、超过 1 MiB 的请求、未知目标/方向/ID/运行栈、不一致排队约束 |
| 高级计算与采集 | TP/PP/EP/CP、互联、KV offload/精度/前缀、滑窗/状态、推测解码、校准参数、checkpoint payload、HF/ModelScope 解析与合并 | 由共享计算及采集回归覆盖；这些选项的物理实现边界见下文，ModelScope 仅有本地解析测试，没有在线目录验证 |

## 本轮新增修复

1. **生成 KV 漏算**：峰值显存计入输入及已生成的输出 token；共享前缀只共享输入；decode attention/读取使用生成期间的平均上下文近似。8B 模型、4 并发、4096 输入，输出 1→8192 增加 4.294443008 GB KV 的独立公式断言通过。
2. **上下文超限误判**：模型窗口校验使用输入+输出；1M 长尾模板为输出预留 512 token，不再默认越界。
3. **目标排序错误**：最高 TOS 不再按完整响应延迟排序；删除会反转成本/TOS 目标的任意警告乘数。完整响应 P95 仍用于 latency 目标。
4. **双目标候选缺失**：包含可用性时同时考虑普通与 N+1 方案；返回条数截断不会只剩同价量化变体而丢掉另一目标的最佳候选。
5. **多模态重复扣参**：文本计算不再对已经表示文本塔 active 的参数再次扣减 encoder；已知 8B 文本塔与 10B 总参数/2B encoder 的 decode FLOPs 一致。
6. **自定义与方向切换**：增加独立 query heads，修正 MHA 和 HTML step，禁用非活动字段；固定硬件详情不展示无效的目标到达率公式；排队最大并发随基础并发保持合法。
7. **结果口径丢失**：Plan 带回 accuracy；自定义模型明确为 scenario；支持状态正常但无额外限制时不再显示“未返回执行依据”。选择器文本统一 HTML 转义。
8. **FP8 识别错误**：识别分块 FP8、8-bit float compressed-tensors 和明确的 F8 tensor 类型；FP8 非 packed 参数沿用未打包元素计数。重新读取 106 个相关官方仓库，纠正 100 条格式：63 条 FP16→FP8、35 条 INT8→FP8、2 条 INT4→FP8。例如 Nemotron-3-Super-120B FP8 不再被通用 packed 参数公式放大成 1495.3B，当前目录使用来源元素计数 123.6B。没有根据文件名或 payload 比例猜精度。

FP8 对照来源：[Qwen3.8-27B-FP8 固定版本配置](https://huggingface.co/Qwen/Qwen3.8-27B-FP8/raw/017b9c7af6b5689d5dd426a76e0bc077eb5ca20a/config.json)，其中包含 quant_method=fp8 和 weight_block_size=[128,128]。

## 本轮验证记录

- `go test -race -json ./...`：**111 个测试通过**，三个 Go package 全部通过。
- 整库检查：**22,980 个配置**，覆盖每个模型 × 六个主量化（H200 基线），以及每款硬件 × 六个代表模型 × 全部量化；其中 16,111 个估算有效、6,869 个受限制。受限制结果必须隐藏速度并给出原因。该数量不是全量笛卡尔积，也不是硬件实测。
- 模型元数据检查：2,075 个可进入对应估算路径，182 个缺少必要数据或执行家族未建模；是否装得下还取决于具体配置。
- `go vet ./...`、`node --check web/app.js`、`git diff --check`、`go build -trimpath -o llmcalc .` 通过。
- 真实浏览器验证目标为 **http://localhost:8317/**。九个页面、详情和跨页动作均使用当前本地服务；未取得用户截图对应的线上地址，因此不把本地结果称为线上验收。

## 既有修复与设计边界

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

当前目录为 62 个精选 + 2195 个 HF + 0 个 ModelScope，共 2257 个 ID。1737 条记录有固定 revision；参数来源为 safetensors 1039、config 1011、model_card 14、name 68、未注明 125。296 条多模态记录仍缺独立 encoder 参数。这里只统计保存的来源字段，并不声称已经逐条外部核验所有字段。官方发布机构标签不代表参数、格式或性能已经验证。

### 页面

显存条、性能页、矩阵、地图、方案与处方统一使用支持状态。无效配置只显示状态与容量诊断，不展示虚构速度。语言切换通过一次性 sessionStorage 保留页面、原生输入、选择器、负载、高级参数、自定义模型和展开状态。首页默认使用已补齐元数据的 Qwen3-8B，而不是未经核验的新条目。

## 既有回归说明

- `go test ./...` 通过；移除了缺乏同条件来源、用社区范围或“同构估计”硬断言速度的旧锚点测试，没有重钉伪精度。
- `node --check web/app.js` 与 `go build -trimpath -o llmcalc .` 通过。
- 真实采集命令定向刷新 `nvidia/Qwen3-8B-NVFP4`，输出 8.2B、FP4、固定 SHA `ccd10a893cbca613259517c3efe08e151ddf2b8e` 和 6.396932352GB payload。
- HTTP 反例确认：P100/TRT、TP3/4卡、Qwen2.5 TP8、整柜和 diffusion 不再输出正常性能；A100 FP8 KV 不因缺原生 FP8 算术被一概否决。
- Llama 70B INT4/6000 TPM 场景中，规划器保留 H200 的 1/2/4/8 卡成本。推荐返回与规划相同的最低月成本方案；推荐输出上限不意味着包含每个硬件候选。
- 混合 Output=1/101 的请求不再污染 decode 统计；长输出 latency 方案按完整请求 P95 排序。
- 实际 Chromium 页面验证：地图 9000 TPM/并发3、性能页 H200/TP3/输出777和高级面板展开状态在语言切换后保留；TP3 被拒绝并隐藏速度，恢复 TP4 后显示条件估算与完整 E2E 指标。

上述是计算器行为验证，不是 GPU 实测 benchmark。

## 仍需的生产前提与未验证项

1. 固定 checkpoint、引擎/驱动/OS、kernel、量化方法、KV backend 和实际拓扑。
2. 测量加载显存、workspace、同负载 TTFT/TPOT/吞吐、功耗和误差范围。
3. 真实连续批处理、cache 驱逐、通信拥塞与 overlap 未被静态模型完整模拟。
4. P99.9 显存是负载分桶的矩近似，排队是 M/M/c；不保证生产尾延迟或 SLO。
5. 所有 CNY 为未核验参考假设，非当前报价；36 月摊销、电价和功耗比例不是完整 TCO。
6. 采集器的 config 参数公式仍是标准 attention/MLP 的结构估计，特殊 MLA、纯状态空间及混合结构需要更具体的 tensor/模型卡依据；非标准 MLX 位宽的映射也未做本轮逐仓库验证。目录保留 0.1B 级取整，尤其不应用于小模型的高精度性能比较。
7. SLO/goodput、质量约束、完整 TCO、MIG/多模型共卡、场景导出和完整非自回归模拟仍属功能扩展，本轮未实现。

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
