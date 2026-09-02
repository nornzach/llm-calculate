# llm-calculate

面向大语言模型推理部署的本地计算器。输入模型、硬件、量化、上下文和并发参数，即可完成显存可行性检查、吞吐/时延估算及部署方案枚举。

一个面向本地大模型部署的容量与吞吐计算器。内置 **121 款硬件**、**2,288 个模型**，覆盖显存可行性、吞吐估算、硬件方案搜索、硬件库、模型库和速查表。

> 这是容量规划和方案初筛工具，不是基准测试。未填写实测校准参数时，性能结果为无统计置信区间的 analytical roofline，不能直接作为采购承诺或生产 SLA。

## 功能

- **能装什么**：按硬件、卡数、上下文和并发生成 FP16、FP8、INT4、FP4、GGUF 等显存可行性矩阵。
- **能跑多快**：输出 decode 单流/聚合 TPS、prefill TPS、TTFT、TPOT、请求时延、req/s 和 mixed TPM。
- **怎么配**：按目标吞吐、单流下限、成本/时延/可用性目标枚举硬件、量化、卡数和副本数。
- **并行与通信**：支持 TP、PP、EP、CP；计入 TP collective、MoE TopK All-to-All、CP 和 PP 通信。
- **缓存与显存**：支持共享前缀、FP8/FP4 KV、allocator 开销、KV offload、adapter/draft、MTP 和媒体 encoder。
- **高级校准**：可填写实际权重、运行时显存、workspace、HBM/FLOPs/link 利用率及调度开销。
- **数据可追踪**：模型展示 Hugging Face / ModelScope 来源、架构、dtype、许可证、发布时间和 checkpoint payload；硬件记录逐精度 dense 峰值及厂商来源。

## 截图

### 显存可行性矩阵

![显存可行性矩阵](docs/screenshots/feasibility.png)

### 吞吐、时延与公式追踪

![吞吐和时延估算](docs/screenshots/throughput.png)

### 部署规划器

![部署规划器](docs/screenshots/planner.png)

## 快速部署

### 环境要求

- Go 1.25 或更高版本
- 可用的本地 TCP 端口；默认 `8317`

### 直接运行

```bash
git clone https://github.com/nornzach/llm-calculate.git
cd llm-calculate
go run .
```

打开：<http://localhost:8317>

指定监听地址：

```bash
go run . -addr :9000
```

服务本身不含身份认证。仅本机使用时应监听 `127.0.0.1:8317`；对外部署时应放在带 TLS 和访问控制的反向代理后。

### 编译单文件部署

```bash
go build -trimpath -o llmcalc .
./llmcalc -addr :8317
```

`web/`、`data/hardware.json`、`data/models.json`、`data/models_hf.json` 和 `data/models_modelscope.json` 均内嵌到二进制。若运行目录存在同名数据文件，程序会优先读取磁盘版本，便于不重新编译就刷新模型库；文件不存在时使用内嵌版本。

Linux 后台运行示例：

```bash
nohup ./llmcalc -addr :8317 > llmcalc.log 2>&1 &
```

停止服务：

```bash
pkill -TERM -x llmcalc
```

## 使用方式

### 1. 能装什么

1. 选择硬件和卡数。
2. 设置上下文、并发、推理引擎、推测解码和 KV 精度。
3. 查看模型矩阵：绿色表示余量充足，黄色表示可运行但显存贴边，空白表示超过当前预算。
4. 单元格数字是当前配置下的单流 decode tok/s；`no-acc` 表示该量化只节省容量/带宽，没有对应原生低精度算力路径。

### 2. 能跑多快

1. 选择硬件、模型、权重量化、上下文和 batch。
2. 查看顶部 TPS、TTFT、TPOT、请求时延、req/s 和瓶颈类型。
3. 显存条展示权重、KV、框架、workspace、adapter/draft、系统预留和空闲空间。
4. 并发曲线展示 batch 变化时的聚合 decode TPS；右侧公式追踪列出每一步假设和分项耗时。
5. 展开“高级参数”可设置 TP/PP/EP/CP、KV offload、prefill chunk 和实测校准值。

只有以下关键值均已填写时，单卡结果才标记为 `calibrated`：

- HBM 带宽利用率；
- FLOPs 利用率；
- 每步调度耗时；
- 多卡场景还需要互联带宽利用率。

### 3. 怎么配

1. 选择已有模型，或录入自定义 Dense/MoE 模型结构。
2. 输入目标 mixed TPM、单流 tok/s 下限、上下文、并发和平均输出长度。
3. 选择最低成本、最低时延或最高可用目标。
4. 开启排队后，结果会显示容量 QPS、目标 QPS、利用率以及 M/M/c 平均/p95 等待时间。
5. 使用型号、厂商、类别和总卡数过滤候选方案。

M/M/c 使用泊松到达、指数服务和独立副本假设，仅用于候选比较；生产尾延迟应使用真实请求 trace 和压测验证。

### 4. 数据库与速查

- **硬件库**：显存、带宽、互联、支持精度、逐精度峰值、TDP、参考价和来源。
- **模型库**：总/激活参数、attention/MoE 结构、KV 大小、上下文、checkpoint payload 和来源。
- **速查**：按默认服务预算快速查看单卡可承载的 FP16/INT4 模型规模。

## HTTP API

页面使用的接口也可以直接调用：

| 方法 | 路径 | 用途 |
|---|---|---|
| `GET` | `/api/hardware` | 硬件列表 |
| `GET` | `/api/models` | 模型列表 |
| `GET` | `/api/quants` | 量化档位 |
| `GET` | `/api/engines` | 推理引擎 |
| `GET` | `/api/specs` | 推测解码方法 |
| `GET` | `/api/quick` | 单卡速查表 |
| `POST` | `/api/fit` | 显存可行性矩阵 |
| `POST` | `/api/perf` | 吞吐和时延估算 |
| `POST` | `/api/plan` | 部署方案枚举 |

性能估算示例：

```bash
curl -sS http://localhost:8317/api/perf \
  -H 'Content-Type: application/json' \
  -d '{
    "hw": "h100-sxm",
    "n": 1,
    "model": "llama-3.1-8b",
    "quant": "fp16",
    "ctx": 8192,
    "batch": 4,
    "eng": "auto",
    "spec": "none",
    "kvq": "fp16",
    "hit": 0,
    "outlen": 512
  }'
```

## 全量同步模型库

采集器同步 Hugging Face 和 ModelScope 的模型详情、架构配置、MoE 路由、上下文、dtype、许可证、发布时间及 safetensors payload。默认覆盖内置主流发布机构，并用全站热门/最新列表补充其他模型：

```bash
go run ./scripts/collect -source hf -all -min-year 2023
go run ./scripts/collect -source modelscope -all -min-year 2023
```

结果分别写入 `data/models_hf.json` 和 `data/models_modelscope.json`。同步采用 **只增不删**：同 ID 使用新元数据更新，本次未返回、被限流或解析失败的旧条目原样保留。可用 `-orgs Qwen,deepseek-ai,...` 指定发布机构。

模型平台包含大量 LoRA、训练中间 checkpoint 和恶意改名衍生仓库；这些不代表独立推理架构，默认过滤。官方量化 checkpoint 会保留并记录原生权重格式。

ModelScope OpenAPI 对单个全站查询设有 3,000 条上限。因此 `-all` 会完整分页内置发布机构，并叠加全站下载量排序和默认最新排序各 3,000 条；不会把数万条社区改名镜像伪装成独立推理模型。

## 验证

```bash
go test ./...
node --check web/app.js
```

浏览器验证应至少覆盖：显存矩阵、吞吐页、并行/校准参数、KV offload、自定义 MoE 和规划器排队指标。

## 项目结构

```text
.
├── calc/                 # 显存、性能和规划算法
├── data/                 # 硬件、精选模型、HF / ModelScope 自动模型库
├── docs/screenshots/     # README 界面截图
├── scripts/collect/      # 双模型平台采集器
├── web/                  # 无框架前端
├── main.go               # HTTP 服务与嵌入资源
└── AUDIT_REPORT.md       # 公式、数据来源和精度边界审计
```

## 精度边界与资料

详细公式、回归契约、已知缺口及 NVIDIA、AMD、Hugging Face、vLLM、SGLang 等一手资料见 [AUDIT_REPORT.md](AUDIT_REPORT.md)。
