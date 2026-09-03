/* 推理计算器 — 前端逻辑（无框架，无构建） */

const $ = (s) => document.querySelector(s);
const $$ = (s) => [...document.querySelectorAll(s)];

const LANG_KEY = "llmcalc-lang";
let lang = localStorage.getItem(LANG_KEY) === "zh" ? "zh" : "en";
const tr = (en, zh) => lang === "zh" ? zh : en;

// Static copy stays next to the markup; dynamic copy uses tr() at render sites.
const STATIC_EN = new Map([
  ["推理计算器", "Inference Calculator"],
  ["能装什么", "Feasibility"],
  ["能跑多快", "Throughput"],
  ["怎么配", "Planner"],
  ["硬件库", "Hardware"],
  ["模型库", "Models"],
  ["速查", "Cheat Sheet"],
  ["术语库", "Glossary"],
  ["硬件 · HARDWARE", "Hardware"],
  ["数量 · CARDS", "Cards"],
  ["上下文 · CONTEXT", "Context"],
  ["并发 · BATCH", "Batch"],
  ["推理框架 · ENGINE", "Inference Engine"],
  ["推测解码 · SPECULATIVE", "Speculative Decoding"],
  ["KV 量化 · KV CACHE", "KV Cache Precision"],
  ["筛选模型 · FILTER", "Filter Models"],
  ["对该硬件枚举全部模型 × 量化档位。", "Evaluate every model × quantization option on this hardware."],
  ["可部署　", "Fits  "],
  ["显存贴边　", "Low headroom  "],
  ["装不下", "Does not fit"],
  ["单元格数字为一阶容量估算，不是基准测试。", "Cell values are first-order capacity estimates, not benchmarks."],
  ["= 报道/推算；", "= reported/estimated; "],
  ["= HF 自动解析。", "= parsed from HF."],
  ["模型 · MODEL", "Model"],
  ["量化 · QUANT", "Quantization"],
  ["基础权重可模拟不同部署量化；预量化检查点会自动锁定。", "Base weights can simulate deployment quantization; pre-quantized checkpoints are locked automatically."],
  ["前缀缓存命中 · PREFIX HIT", "Prefix Cache Hit"],
  ["平均输出 · OUT LEN", "Average Output"],
  ["token · 请求时延口径", "tokens · request-latency basis"],
  ["高级参数 · CALIBRATION", "Advanced Calibration"],
  ["0 = 自动/解析值", "0 = automatic / parsed"],
  ["带宽、算力、调度（多卡另含互联）均填写同口径实测值后标记 calibrated；否则是无统计置信区间的 analytical roofline。", "Marked calibrated only when bandwidth, compute, scheduling, and multi-card interconnect values share the same measured basis; otherwise this is an analytical roofline without confidence intervals."],
  ["显存分解 · MEMORY BREAKDOWN", "Memory Breakdown"],
  ["部署仿真 · CONCURRENCY SWEEP", "Concurrency Sweep"],
  ["同一公式逐点重算", "recomputed at every point"],
  ["计算过程 · FORMULA TRACE", "Formula Trace"],
  ["✎ 自定义假想模型…", "✎ Custom Model…"],
  ["总参数 B", "Total Params B"],
  ["激活 B", "Active Params B"],
  ["层数", "Layers"],
  ["隐藏维度", "Hidden Size"],
  ["KV 头数", "KV Heads"],
  ["头维度", "Head Dimension"],
  ["KV 层数", "KV Layers"],
  ["Local 层数", "Local Layers"],
  ["MLP 中间维度", "MLP Intermediate"],
  ["MoE 中间维度", "MoE Intermediate"],
  ["专家总数", "Experts"],
  ["每 token 专家", "Experts per Token"],
  ["共享专家", "Shared Experts"],
  ["MoE 层数", "MoE Layers"],
  ["MTP 预测头", "MTP Heads"],
  ["媒体 encoder 参数 B", "Media Encoder Params B"],
  ["← 回到模型库", "← Back to Model Library"],
  ["目标吞吐 · TARGET TPM", "Target Throughput"],
  ["输入+输出合计 tok/min", "input + output tok/min"],
  ["单流下限 · MIN TOS", "Minimum Single-stream TPS"],
  ["tok/s · 0=不限", "tok/s · 0 = unlimited"],
  ["优化目标 · OBJECTIVE", "Objective"],
  ["最低成本", "Lowest Cost"],
  ["最低时延", "Lowest Latency"],
  ["最高可用", "Highest Availability"],
  ["量化范围 · QUANT", "Quantization Scope"],
  ["基础权重枚举量化方案；预量化检查点只使用自身格式。", "Base weights evaluate all quantization options; pre-quantized checkpoints use their native format only."],
  ["并发 · CONCURRENCY", "Concurrency"],
  ["允许排队 · QUEUE", "Allow Queueing"],
  ["排队后单副本最大并发", "Maximum Concurrency per Queued Replica"],
  ["token · 用于 QPS 换算", "tokens · used for QPS conversion"],
  ["校准值应用于所有候选硬件；只在同一部署栈、模型和负载口径下填写。", "Calibration values apply to every candidate; enter them only for the same stack, model, and workload."],
  ["枚举硬件 × 数量 × 量化 × 副本数；结果是静态容量筛选，不替代生产压测。", "Evaluates hardware × card count × quantization × replicas; this is static capacity screening, not a production benchmark."],
  ["TPM 按原始输入+输出计数；前缀命中减少 prefill，并按共享 block 计算 KV 驻留。推测解码必须用匹配模型/负载的接受率校准。", "TPM counts original input + output. Prefix hits reduce prefill and share resident KV blocks. Speculative decoding requires acceptance calibration for the matching model and workload."],
  ["成本仅含参考采购价的 36 月摊销与假设电费（0.8 元/kWh、60% 负载），不含主机、网络、制冷、PUE、运维和实际利用率。", "Cost includes 36-month reference-price amortization and assumed power (CNY 0.8/kWh, 60% load); hosts, networking, cooling, PUE, operations, and actual utilization are excluded."],
  ["开启排队后显示 M/M/c 的目标利用率及平均/p95 等待；其泊松到达、指数服务、独立副本假设不等于生产 SLA。", "Queueing shows M/M/c target utilization and mean/p95 wait. Poisson arrivals, exponential service, and independent replicas do not represent a production SLA."],
  ["每张卡「能扛的最大模型参数量」速查 —— 已扣除框架底座与系统预留，按 8K 上下文估算，实际以模式一为准。", "Maximum model size per card after runtime and system reserves, estimated at 8K context. Use Feasibility for the actual configuration."],
  ["REFERENCE · 可搜索", "REFERENCE · SEARCHABLE"],
  ["推理系统术语库", "Inference Systems Glossary"],
  ["简写、全称、实际作用，以及它如何影响本计算器。技术名词不再只靠悬浮提示。", "Abbreviations, full names, practical meaning, and how each term affects this calculator."],
  ["一阶 roofline 与静态容量仿真；未建模 continuous batching 轨迹、kernel fusion、分层网络拥塞、cache 驱逐和生产尾延迟分布。采购或 SLA 前必须按目标 checkpoint、引擎和真实流量校准。", "First-order roofline and static capacity simulation. Continuous-batching trajectories, kernel fusion, hierarchical network congestion, cache eviction, and production tail latency are not modeled. Calibrate with the target checkpoint, engine, and real traffic before purchasing or setting an SLA."],
  ["10/页", "10/page"],
  ["20/页", "20/page"],
  ["全部", "All"],
  ["深色", "Dark"],
  ["亮色", "Light"],
  ["可部署", "Fits"],
  ["显存贴边", "Low headroom"],
  ["CATALOG · 可筛选", "CATALOG · FILTERABLE"],
  ["推理硬件库", "Inference Hardware Catalog"],
  ["对比显存、带宽、互联、低精度路径与参考价格，快速缩小部署候选范围。", "Compare memory, bandwidth, interconnects, low-precision paths, and reference prices to narrow deployment candidates."],
  ["MODEL CATALOG · 可搜索", "MODEL CATALOG · SEARCHABLE"],
  ["模型与检查点", "Models and Checkpoints"],
  ["浏览模型结构、上下文、KV 规模、原生量化格式与数据来源。", "Browse model architecture, context, KV footprint, native quantization, and data sources."],
  ["单卡容量速查", "Single-card Capacity Guide"],
  ["按相同预留口径快速比较不同硬件的大致模型容量上限。", "Compare approximate model capacity across hardware using the same reserve assumptions."],
]);

const ATTR_EN = new Map([
  ["切换深浅配色", "Toggle color theme"],
  ["名字 / 机构…", "Name / organization…"],
  ["0 = 全部层持有 KV（混合线性注意力架构用）", "0 = every layer holds KV (for hybrid linear-attention architectures)"],
  ["滑动窗口/local attention 层数；0 = 关闭", "Sliding-window/local-attention layers; 0 = off"],
  ["局部 attention 保留的 KV token 数", "KV tokens retained by local attention"],
  ["筛选型号…", "Filter hardware…"],
  ["筛选模型…", "Filter models…"],
  ["总卡数≤", "Total cards ≤"],
  ["0 = 不限", "0 = unlimited"],
  ["搜索 TP、KV cache、首 token…", "Search TP, KV cache, first token…"],
]);

function applyLanguage() {
  document.documentElement.lang = lang === "zh" ? "zh-CN" : "en";
  document.title = tr("LLM Inference Calculator", "推理计算器 · LLM Inference Calculator");
  if (lang === "en") {
    const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
    while (walker.nextNode()) {
      const node = walker.currentNode;
      const value = node.nodeValue.trim();
      const translated = STATIC_EN.get(value);
      if (translated) node.nodeValue = node.nodeValue.replace(value, translated);
    }
    document.querySelectorAll("[placeholder], [title], [aria-label]").forEach(el => {
      for (const attr of ["placeholder", "title", "aria-label"]) {
        const value = el.getAttribute(attr);
        if (ATTR_EN.has(value)) el.setAttribute(attr, ATTR_EN.get(value));
      }
    });
  }
  const button = $("#langBtn");
  if (button) {
    button.textContent = lang === "en" ? "中文" : "EN";
    button.setAttribute("aria-label", tr("Switch to Chinese", "切换到英语"));
  }
}

let HW = [], MODELS = [], QUANTS = [], ENGINES = [], SPECS = [];
let modelPage = 0;
const CS = {}; // 自定义下拉注册表
const fixedQuantID = m => m && m.native_quant && m.native_quant !== "fp16" && QUANTS.some(q => q.id === m.native_quant) ? m.native_quant : "";

const VENDOR = {
  nvidia: "NVIDIA", amd: "AMD", intel: "Intel", apple: "Apple",
  huawei: "昇腾", mthreads: "摩尔线程", cambricon: "寒武纪",
  hygon: "海光", groq: "Groq", cerebras: "Cerebras", google: "Google",
  enflame: "燧原", metax: "沐曦", biren: "壁仞", iluvatar: "天数智芯",
  kunlunxin: "昆仑芯", aws: "AWS", sambanova: "SambaNova", qualcomm: "Qualcomm",
};

const VENDOR_EN = {
  nvidia: "NVIDIA", amd: "AMD", intel: "Intel", apple: "Apple",
  huawei: "Huawei Ascend", mthreads: "Moore Threads", cambricon: "Cambricon",
  hygon: "Hygon", groq: "Groq", cerebras: "Cerebras", google: "Google",
  enflame: "Enflame", metax: "MetaX", biren: "Biren", iluvatar: "Iluvatar CoreX",
  kunlunxin: "Kunlunxin", aws: "AWS", sambanova: "SambaNova", qualcomm: "Qualcomm",
};
const vendorName = id => (lang === "zh" ? VENDOR : VENDOR_EN)[id] ?? id;
const HW_NAME_EN = {
  "rtx4090-48g": "RTX 4090 48G Mod",
  "gb300-nvl72": "GB300 NVL72 Rack",
  "gb200-nvl72": "GB200 NVL72 Rack",
  "ascend-910b": "Ascend 910B",
  "ascend-910c": "Ascend 910C",
  "mthreads-s4000": "Moore Threads MTT S4000",
  "cambricon-mlu370": "Cambricon MLU370-X8",
  "hygon-k100": "Hygon DCU K100",
  "mthreads-s3000": "Moore Threads MTT S3000",
  "enflame-s60": "Enflame S60",
  "metax-mxc500": "MetaX MXC500",
  "biren-br100": "Biren BR100",
  "iluvatar-tg100": "Iluvatar CoreX TianGai 100",
  "kunlun-p800": "Kunlunxin P800",
  "cambricon-mlu590": "Cambricon MLU590",
};
const hardwareName = h => lang === "zh" ? h.name : (HW_NAME_EN[h.id] ?? h.name);
const GLOSSARY = [
  ["LLM", "Large Language Model", "基础", "大语言模型；通过自回归或其他生成范式处理 token 序列。", "模型结构和参数量决定显存、计算与带宽需求。"],
  ["Token", "Token", "负载", "分词器处理的文本单位，不等同于汉字、英文单词或字节。", "上下文、输出长度和吞吐均以 token 计。"],
  ["Context", "Context Window", "负载", "单次请求可见的输入与已生成 token 上限。", "上下文越长，KV 显存和 attention 计算通常越高。"],
  ["Prefill", "Prompt Prefill", "阶段", "一次性处理输入提示并建立 KV cache 的阶段。", "主要影响 TTFT，长上下文可能从线性计算转为 attention 主导。"],
  ["Decode", "Autoregressive Decode", "阶段", "逐步生成输出 token 的阶段。", "通常受权重与 KV 读取带宽限制。"],
  ["Batch", "Decode Batch Size", "负载", "同一 decode step 同时处理的活跃请求数。", "提高聚合 TPS，也增加 KV、workspace 和单用户 TPOT。"],
  ["Concurrency", "Request Concurrency", "负载", "同时处于服务中的请求数量。", "规划器用它估算单副本服务能力和显存占用。"],
  ["TPS", "Tokens Per Second", "指标", "每秒生成的 token 数。", "区分单流 TPS 与所有并发合计的聚合 TPS。"],
  ["TPM", "Tokens Per Minute", "指标", "每分钟处理的 token 数。", "规划器使用输入加输出的 mixed TPM，不只统计生成 token。"],
  ["QPS", "Queries Per Second", "指标", "每秒完成的请求数，也写作 req/s。", "由 prefill 时间、输出长度和 decode 吞吐共同决定。"],
  ["TTFT", "Time To First Token", "指标", "从请求开始到首个输出 token 可用的延迟。", "这里以 prefill 路径为主，媒体 encoder 与通信也会计入。"],
  ["TPOT", "Time Per Output Token", "指标", "首 token 之后相邻输出 token 的平均间隔。", "等于有效 decode step 时间；推测解码仅在校准后改变它。"],
  ["ITL", "Inter-Token Latency", "指标", "相邻输出 token 的延迟，常与 TPOT 同口径。", "本系统以 TPOT 展示稳定生成阶段的间隔。"],
  ["Latency", "End-to-End Latency", "指标", "请求从进入到输出完成的总时间。", "按 TTFT 加后续输出 token 的 TPOT 估算，不包含网络排队。"],
  ["Throughput", "Throughput", "指标", "单位时间内完成的 token 或请求工作量。", "不要与单用户生成速度混用。"],
  ["Goodput", "SLO-compliant Throughput", "指标", "满足延迟或质量 SLO 的有效吞吐。", "未输入真实分布时不推算 goodput，需压测。"],
  ["P50 / P95 / P99", "Latency Percentiles", "指标", "延迟分布的中位数、95 和 99 分位。", "静态 roofline 不生成生产尾延迟；排队页仅给 M/M/c 假设值。"],
  ["SLO", "Service Level Objective", "服务", "团队设定的可用性、延迟或吞吐目标。", "计算结果只能筛选候选，最终 SLO 需真实流量验证。"],
  ["SLA", "Service Level Agreement", "服务", "对外承诺且可能带责任条款的服务等级协议。", "未校准 analytical 结果不能作为 SLA。"],
  ["VRAM", "Video Random Access Memory", "显存", "GPU 或加速卡可用于模型、KV 和运行时的本地内存。", "物理 VRAM 与引擎可分配预算分开显示。"],
  ["HBM", "High Bandwidth Memory", "显存", "数据中心加速卡常用的高带宽封装内存。", "标称 GB/s 乘利用率形成 decode 的有效带宽。"],
  ["KV Cache", "Key-Value Cache", "缓存", "保存 attention 历史 K/V，避免每个输出步重算全部上下文。", "按 attention 类型、层数、上下文、并发、精度和并行分片计算。"],
  ["Prefix Cache", "Prefix Caching", "缓存", "复用多个请求共同前缀的已计算 KV block。", "命中减少 prefill；共享 block 只驻留一份，但 decode 仍需读取前缀。"],
  ["Cache Hit Rate", "Prefix Cache Hit Rate", "缓存", "输入 token 中可复用前缀所占比例。", "只对前缀计算与共享驻留生效，不删除业务 token 计数。"],
  ["PagedAttention", "Paged KV Cache Attention", "缓存", "把 KV cache 切成可分页 block 以降低碎片并支持动态请求。", "KV allocator 系数用于表达 block 与碎片开销。"],
  ["Offload", "KV / Weight Offload", "缓存", "把部分数据放到 CPU、远端内存或较慢层级。", "节省 GPU 容量，但回读带宽会增加每步延迟。"],
  ["Weights", "Model Weights", "显存", "模型训练后保存的参数张量。", "优先使用匹配原生量化的 checkpoint payload，否则参数量乘 bytes/parameter。"],
  ["Activations", "Intermediate Activations", "显存", "前向计算中的中间张量。", "按 batch、prefill chunk、hidden 和并行分片给一阶 workspace 上界。"],
  ["Workspace", "Runtime Workspace", "显存", "算子、通信和执行器临时使用的显存。", "可用实测 activation GB 覆盖解析估算。"],
  ["Adapter", "Parameter-Efficient Adapter", "模型", "附加在基础模型上的小规模任务参数。", "Adapter GB 同时进入常驻显存与 decode 权重读取。"],
  ["LoRA", "Low-Rank Adaptation", "模型", "以低秩矩阵微调模型的参数高效方法。", "本系统把已加载 LoRA 视为 adapter 显存；不估计热切换成本。"],
  ["Quantization", "Model Quantization", "精度", "用更低位宽表示权重、激活或 KV，以降低容量与带宽。", "三者分别建模；不能把权重量化自动当成 KV 量化。"],
  ["Dequantization", "Dequantization", "精度", "计算前把低位权重恢复到算子使用的精度。", "W4A16 可能省带宽但不等于获得 4-bit 算力峰值。"],
  ["FP32", "32-bit Floating Point", "精度", "32 位浮点格式，推理中常用于少量敏感计算或累加。", "未作为主权重档位；本工具面向常见低精度推理。"],
  ["FP16", "16-bit Floating Point", "精度", "IEEE 半精度浮点。", "与 BF16 共用基础 2 bytes/parameter 容量档。"],
  ["BF16", "Brain Floating Point 16", "精度", "保留 FP32 指数范围的 16 位浮点。", "容量与 FP16 相同，模型 dtype 会单独显示。"],
  ["FP8", "8-bit Floating Point", "精度", "常见为 E4M3 或 E5M2 的 8 位浮点。", "W8A8 只有硬件与执行路径支持时才使用 FP8 峰值。"],
  ["FP4", "4-bit Floating Point", "精度", "4 位浮点格式族，不同缩放与打包标准并不互换。", "区分 NVFP4、MXFP4；未知路径不会套 4×算力。"],
  ["INT8", "8-bit Integer", "精度", "8 位整数量化格式。", "W8A8 可使用已录入的 INT8 dense 峰值；否则标为估算。"],
  ["INT4", "4-bit Integer", "精度", "4 位整数量化权重。", "主要按 W4A16 容量和带宽收益建模。"],
  ["W8A8", "8-bit Weights, 8-bit Activations", "精度", "权重与激活都使用 8 位的执行路径。", "可改变容量、带宽和 prefill 算力峰值。"],
  ["W4A16", "4-bit Weights, 16-bit Activations", "精度", "4 位权重、16 位激活的 weight-only 路径。", "不自动按 4-bit tensor 峰值加速。"],
  ["AWQ", "Activation-aware Weight Quantization", "精度", "利用激活统计保护重要权重通道的 weight-only 量化。", "归入 INT4/W4A16；实际 kernel 效率需校准。"],
  ["GPTQ", "Post-Training Quantization for GPT", "精度", "基于近似二阶信息的逐层训练后权重量化。", "归入 INT4/W4A16，不与 AWQ checkpoint 混用。"],
  ["QAT", "Quantization-Aware Training", "精度", "训练阶段模拟量化误差，使模型适应目标格式。", "按 checkpoint 声明的原生格式锁定，不允许重复选择另一权重格式。"],
  ["GGUF", "GPT-Generated Unified Format", "格式", "llama.cpp 生态的模型容器与多种量化布局。", "Q8/Q6/Q4_K_M/IQ2 分档，并自动优先 llama.cpp。"],
  ["bpw", "Bits Per Weight", "精度", "平均每个权重占用的 bit，包含分组 scale 等摊销。", "用于解释 GGUF/EXL 等实际体积，不等同名义 bit 数。"],
  ["MXFP4", "Microscaling FP4", "精度", "使用共享微缩放因子的 4 位浮点权重格式。", "作为独立原生 checkpoint 格式；不冒充 NVFP4。"],
  ["NVFP4", "NVIDIA FP4", "精度", "NVIDIA Blackwell 的细粒度缩放 FP4 路径。", "只有 NVIDIA FP4 硬件路径才套相应 dense 峰值。"],
  ["Dense", "Dense Model", "架构", "每个 token 都经过同一组主要模型参数。", "decode 权重读取通常接近全部文本权重。"],
  ["MoE", "Mixture of Experts", "架构", "每层包含多个专家，每个 token 只路由到其中一部分。", "容量看总参数，计算与单步读取看激活和批内唯一专家。"],
  ["Expert", "Mixture-of-Experts Subnetwork", "架构", "MoE 中可被路由选择的前馈子网络。", "专家数、Top-K 和 EP 影响权重分片、读取与通信。"],
  ["Top-K", "Top-K Expert Routing", "架构", "每个 token 选择得分最高的 K 个专家。", "用于估算激活参数、批内专家覆盖和 All-to-All 流量。"],
  ["Router Skew", "Expert Load Imbalance", "架构", "最忙 expert rank 相对平均负载的比例。", "大于 1 会降低 EP 的理想收益并增加瓶颈时间。"],
  ["MHA", "Multi-Head Attention", "注意力", "每个 query head 通常有独立 K/V head。", "KV cache 最大，按 KV head 数计算。"],
  ["MQA", "Multi-Query Attention", "注意力", "所有 query head 共享一组 K/V。", "数据中通常作为 KV heads=1 的 GQA 特例计算。"],
  ["GQA", "Grouped-Query Attention", "注意力", "多组 query head 共享较少的 K/V heads。", "减少 KV；TP 超过 KV heads 时考虑复制。"],
  ["MLA", "Multi-head Latent Attention", "注意力", "把 K/V 压缩为低维 latent cache 的注意力。", "按 latent 维度计算 KV，并保守假设 TP ranks 复制。"],
  ["SWA", "Sliding Window Attention", "注意力", "每个 token 只关注最近固定窗口。", "局部层的 KV 与 attention 计算按 window 截断。"],
  ["DSA", "DeepSeek Sparse Attention", "注意力", "从长上下文中选择有限 key token 的稀疏注意力。", "降低长上下文 attention 读取与计算，不缩小已存 KV。"],
  ["RoPE", "Rotary Position Embedding", "位置", "把位置信息编码到旋转后的 query/key。", "超过原生上下文时会提示需要外推且精度未知。"],
  ["YaRN", "Yet another RoPE extensioN", "位置", "扩展 RoPE 上下文长度的方法。", "这里只告警，不假设外推后的质量或性能不变。"],
  ["FlashAttention", "IO-Aware Exact Attention", "算子", "通过分块减少 HBM 往返，不保存完整 O(n²) attention matrix。", "workspace 公式不分配 O(n²) 矩阵，但 attention FLOPs 仍计算。"],
  ["MTP", "Multi-Token Prediction", "解码", "模型附带预测多个后续 token 的训练头或模块。", "需模型元数据且填写实测接受长度与开销后才应用收益。"],
  ["Speculative Decoding", "Speculative Decoding", "解码", "先提出候选 token，再由目标模型批量验证。", "不再套跨模型默认倍率；必须输入同 workload 的 τ 与验证开销。"],
  ["Draft Model", "Draft Model", "解码", "为目标模型快速提出候选 token 的较小模型。", "Draft 显存进入预算，速度收益需实测校准。"],
  ["EAGLE", "Extrapolation Algorithm for Greater Language-model Efficiency", "解码", "在特征层预测草稿 token 的推测解码方法。", "选择方法本身不产生收益，需匹配的头和实测参数。"],
  ["Medusa", "Multiple Decoding Heads", "解码", "在目标模型上附加多个预测头并验证候选树。", "附加头占用显存；接受率与负载相关。"],
  ["Lookahead", "Lookahead Decoding", "解码", "利用 n-gram 等结构并行提出候选，而非独立草稿模型。", "收益高度依赖重复模式，必须校准。"],
  ["Continuous Batching", "Continuous Batching", "调度", "请求完成后立即用新请求填补 batch 槽位。", "静态 batch 曲线不模拟逐请求调度轨迹，结果是容量近似。"],
  ["Chunked Prefill", "Chunked Prefill", "调度", "把长 prompt 分块，与 decode 或其他请求交错执行。", "每个 chunk 可能重复读取权重并产生调度开销。"],
  ["CUDA Graph", "CUDA Graph", "调度", "预录制 GPU 工作图以降低重复 launch 开销。", "不虚构固定加速，可通过实测 schedule ms 校准。"],
  ["Kernel Fusion", "Kernel Fusion", "算子", "把多个算子合并以减少 launch 与中间内存流量。", "未单独建模，其收益包含在实测利用率中。"],
  ["Roofline", "Roofline Performance Model", "模型", "用带宽上限和算力上限的较慢者估算执行时间。", "decode 核心为 max(memory, compute)，再加通信与调度。"],
  ["FLOPS", "Floating-Point Operations Per Second", "算力", "每秒浮点运算能力。", "必须区分 dense、稀疏、数据类型与是否使用 tensor core。"],
  ["TFLOPS", "Tera FLOPS", "算力", "每秒一万亿次浮点运算。", "硬件表保存 dense 峰值；缺逐精度规格时明确标为倍率估算。"],
  ["TOPS", "Tera Operations Per Second", "算力", "每秒一万亿次整数或低精度操作。", "INT8 等规格不能无条件与 FP16 TFLOPS 混用。"],
  ["Memory-bound", "Memory-Bandwidth Bound", "瓶颈", "执行时间主要由数据搬运而非运算数量决定。", "常见于低 batch decode；量化可能通过少读权重改善。"],
  ["Compute-bound", "Compute Bound", "瓶颈", "执行时间主要由运算峰值与利用率决定。", "长 prefill、高 batch 或长 attention 更容易进入此区间。"],
  ["TP", "Tensor Parallelism", "并行", "把单层张量计算切分到多卡。", "分片权重/部分 KV，同时加入 collective 通信。"],
  ["PP", "Pipeline Parallelism", "并行", "把不同层放在不同 pipeline stage。", "降低每卡常驻权重；端到端时延仍受 stage 与 bubble 影响。"],
  ["EP", "Expert Parallelism", "并行", "把 MoE 专家分布到不同 rank。", "分片专家权重与计算，并加入 dispatch/combine All-to-All。"],
  ["CP", "Context Parallelism", "并行", "沿 token/context 维度切分 attention 与 KV。", "降低每卡 KV/attention 负担，同时增加 context collective。"],
  ["DP", "Data Parallelism", "并行", "多副本各自处理不同请求。", "规划器用 replicas 表达服务副本，不自动搜索复杂 DP placement。"],
  ["AllReduce", "All-Reduce Collective", "通信", "把各 rank 的部分结果聚合并分发。", "TP 通信按 ring 一阶流量估算。"],
  ["All-to-All", "All-to-All Collective", "通信", "每个 rank 与多个 rank 交换不同数据。", "EP 的 token dispatch/combine 会产生该通信。"],
  ["NVLink", "NVIDIA NVLink", "互联", "NVIDIA GPU 间高带宽互联。", "按录入链路 GB/s 和可选实测利用率计算通信。"],
  ["PCIe", "Peripheral Component Interconnect Express", "互联", "GPU 与主机或无专用互联 GPU 间的通用总线。", "无已知互联时保守使用 PCIe 4.0 x16 有效 25 GB/s。"],
  ["XGMI", "AMD Infinity Fabric Link", "互联", "AMD GPU 间基于 Infinity Fabric 的互联路径。", "与 NVLink 一样按设备录入的双向口径近似。"],
  ["M/M/c", "Markovian Arrival / Service, c Servers", "排队", "泊松到达、指数服务时间、c 个独立服务台的排队模型。", "只在开启排队时估算平均与无条件 p95 等待。"],
  ["Utilization", "Server Utilization", "排队", "目标到达工作量占服务能力的比例。", "接近 100% 时排队会非线性增长；ρ≥1 无稳态。"],
  ["TDP", "Thermal Design Power", "成本", "用于散热和供电设计的功耗规格，不等于每次实测功耗。", "成本估算按 TDP、60% 负载和假设电价计算。"],
  ["PUE", "Power Usage Effectiveness", "成本", "数据中心总能耗与 IT 设备能耗之比。", "当前成本未计 PUE，生产预算必须补入。"],
  ["vLLM", "vLLM", "引擎", "面向高吞吐服务的开源 LLM 推理引擎。", "默认 NVIDIA/通用服务基线；版本与 kernel 差异需校准。"],
  ["SGLang", "SGLang Runtime", "引擎", "支持结构化生成、缓存与高性能服务的推理运行时。", "当前仅在 SGLang+FP4 硬件组合建模 FP4 KV 路径。"],
  ["TensorRT-LLM", "NVIDIA TensorRT for LLMs", "引擎", "NVIDIA 面向 LLM 的编译与运行时栈。", "只对列出的 NVIDIA 路径视为兼容；性能不设虚构默认优势。"],
  ["llama.cpp", "llama.cpp", "引擎", "跨 CPU、Metal、CUDA 等后端的本地推理运行时。", "GGUF 档位自动优先选择该引擎。"],
  ["MLX", "Apple MLX", "引擎", "Apple Silicon 统一内存机器学习框架。", "MLX checkpoint 只在 Apple 硬件标记原生路径。"],
  ["ExLlama", "ExLlama", "引擎", "面向 NVIDIA 消费卡低位权重推理的运行时。", "EXL 格式只在 NVIDIA 上标记原生路径。"],
];

const GLOSSARY_CATEGORY_EN = {
  "基础": "Basics", "负载": "Workload", "阶段": "Phase", "指标": "Metric", "服务": "Service",
  "显存": "Memory", "缓存": "Cache", "模型": "Model", "精度": "Precision", "格式": "Format",
  "架构": "Architecture", "注意力": "Attention", "位置": "Position", "算子": "Kernel", "解码": "Decoding",
  "调度": "Scheduling", "算力": "Compute", "瓶颈": "Bottleneck", "并行": "Parallelism",
  "通信": "Communication", "互联": "Interconnect", "排队": "Queueing", "成本": "Cost", "引擎": "Engine",
};
const GLOSSARY_EN = {
  "LLM": ["A large language model processes token sequences, usually with autoregressive generation.", "Architecture and parameter count determine memory, compute, and bandwidth demand."],
  "Token": ["The unit processed by a tokenizer; it is not the same as a character, word, or byte.", "Context, output length, and throughput are all counted in tokens."],
  "Context": ["The maximum input and generated-token history visible to one request.", "Longer context usually increases KV memory and attention compute."],
  "Prefill": ["The phase that processes the prompt and builds the KV cache.", "It dominates TTFT; long prompts may become attention-bound."],
  "Decode": ["The autoregressive phase that emits output tokens step by step.", "It is commonly limited by weight and KV memory bandwidth."],
  "Batch": ["Active requests processed together in one decode step.", "Larger batches raise aggregate TPS but consume more KV and workspace and can raise TPOT."],
  "Concurrency": ["The number of requests simultaneously in service.", "The planner uses it to estimate per-replica capacity and memory."],
  "TPS": ["The number of generated tokens per second.", "Single-stream TPS and aggregate TPS measure different user and system behavior."],
  "TPM": ["The number of tokens processed per minute.", "The planner counts mixed input plus output TPM, not output alone."],
  "QPS": ["Completed requests per second, also written req/s.", "It depends on prefill time, output length, and decode throughput."],
  "TTFT": ["Latency from request arrival until the first output token is available.", "The calculator derives it mainly from prefill, plus media encoding and communication when applicable."],
  "TPOT": ["Average interval between output tokens after the first token.", "It equals the effective decode-step time; speculation changes it only when calibrated."],
  "ITL": ["Inter-token latency, often measured on the same basis as TPOT.", "The calculator reports TPOT for the steady generation phase."],
  "Latency": ["Total time from request arrival to completed output.", "Estimated as TTFT plus TPOT for subsequent tokens, excluding network queueing."],
  "Throughput": ["Token or request work completed per unit time.", "Do not confuse aggregate throughput with single-user generation speed."],
  "Goodput": ["Throughput that meets a latency or quality SLO.", "It is not inferred without real workload distributions; benchmark it."],
  "P50 / P95 / P99": ["The median, 95th, and 99th percentiles of a latency distribution.", "A static roofline cannot produce production tails; queueing results use only M/M/c assumptions."],
  "SLO": ["An internal target for availability, latency, or throughput.", "Calculator results screen candidates; validate the final SLO with real traffic."],
  "SLA": ["An externally committed service level that may carry contractual obligations.", "Uncalibrated analytical results are not suitable for an SLA."],
  "VRAM": ["Local accelerator memory available to weights, KV cache, and runtime state.", "Physical capacity and the engine's allocatable budget are shown separately."],
  "HBM": ["High-bandwidth packaged memory used by many data-center accelerators.", "Rated GB/s times utilization determines effective decode bandwidth."],
  "KV Cache": ["Stored attention keys and values that avoid recomputing prior context.", "Size depends on attention type, layers, context, concurrency, precision, and sharding."],
  "Prefix Cache": ["Reusable KV blocks for prompts that share a prefix.", "Hits reduce prefill and store shared blocks once, while decode still reads the prefix."],
  "Cache Hit Rate": ["The fraction of input tokens covered by a reusable prefix.", "It affects prefix compute and shared residency, not business token accounting."],
  "PagedAttention": ["A block-paged KV-cache design that reduces fragmentation and supports dynamic requests.", "The KV allocator factor represents block and fragmentation overhead."],
  "Offload": ["Placement of weights or KV in CPU, remote, or slower memory.", "It saves accelerator capacity but adds readback latency according to offload bandwidth."],
  "Weights": ["Learned parameter tensors stored by the model.", "Native checkpoint payload is preferred; otherwise size is parameters times bytes per parameter."],
  "Activations": ["Intermediate tensors produced during forward computation.", "Batch, prefill chunk, hidden size, and sharding determine a first-order workspace bound."],
  "Workspace": ["Temporary accelerator memory used by kernels, communication, and executors.", "Measured activation memory can override the parsed estimate."],
  "Adapter": ["A small task-specific parameter set attached to a base model.", "Adapter GB contributes to resident memory and decode weight reads."],
  "LoRA": ["Low-rank matrices used for parameter-efficient fine-tuning.", "Loaded LoRA data is treated as adapter memory; hot-switch cost is not modeled."],
  "Quantization": ["Lower-precision representation of weights, activations, or KV cache.", "These three paths are modeled separately; weight quantization does not imply KV quantization."],
  "Dequantization": ["Conversion of low-bit weights to the precision used by a kernel.", "W4A16 may save bandwidth without delivering 4-bit compute throughput."],
  "FP32": ["IEEE 32-bit floating point, often retained for a small set of sensitive operations.", "It is not a primary weight option in this low-precision inference calculator."],
  "FP16": ["IEEE 16-bit floating point.", "It shares the base 2-byte-per-parameter capacity tier with BF16."],
  "BF16": ["A 16-bit float with the exponent range of FP32.", "Capacity matches FP16; the model dtype is displayed separately."],
  "FP8": ["An 8-bit floating-point family, commonly E4M3 or E5M2.", "W8A8 uses FP8 peak throughput only when hardware and execution path support it."],
  "FP4": ["A family of 4-bit floating-point formats with incompatible scaling and packing schemes.", "NVFP4 and MXFP4 are distinct; unknown paths do not receive a 4× compute multiplier."],
  "INT8": ["An 8-bit integer quantization format.", "W8A8 uses a recorded INT8 dense peak when available; otherwise it remains estimated."],
  "INT4": ["A 4-bit integer weight format.", "It is modeled mainly as W4A16 capacity and bandwidth savings."],
  "W8A8": ["An execution path with 8-bit weights and activations.", "It can change capacity, bandwidth, and prefill compute peak."],
  "W4A16": ["A weight-only path with 4-bit weights and 16-bit activations.", "It does not automatically use the hardware's 4-bit tensor peak."],
  "AWQ": ["Weight-only quantization that protects important channels using activation statistics.", "It maps to INT4/W4A16; calibrate actual kernel efficiency."],
  "GPTQ": ["Layer-wise post-training weight quantization using approximate second-order information.", "It maps to INT4/W4A16 and is not interchangeable with an AWQ checkpoint."],
  "QAT": ["Training that simulates quantization error so a model adapts to the target format.", "The checkpoint's native format is locked instead of being quantized again."],
  "GGUF": ["A llama.cpp model container supporting several quantization layouts.", "Q8, Q6, Q4_K_M, and IQ2 are separate tiers and prefer llama.cpp automatically."],
  "bpw": ["Average bits per weight, including amortized scales and group metadata.", "It explains real GGUF/EXL size and is not always the nominal bit count."],
  "MXFP4": ["A 4-bit floating format using shared microscaling factors.", "It is a distinct native checkpoint format and is not treated as NVFP4."],
  "NVFP4": ["NVIDIA's fine-grained scaled FP4 path for Blackwell.", "The FP4 dense peak applies only on a supported NVIDIA FP4 path."],
  "Dense": ["A model where each token traverses the same principal parameters.", "Decode generally reads nearly all text weights on every step."],
  "MoE": ["A mixture-of-experts model routes each token through only part of its experts.", "Capacity uses total parameters; compute and reads use active and batch-unique experts."],
  "Expert": ["A feed-forward subnetwork selected by an MoE router.", "Expert count, Top-K, and EP affect sharding, reads, and communication."],
  "Top-K": ["Routing that selects the K highest-scoring experts for each token.", "It drives active parameters, batch expert coverage, and All-to-All traffic."],
  "Router Skew": ["The busiest expert rank's load relative to the mean.", "Values above 1 reduce ideal EP gains and increase bottleneck time."],
  "MHA": ["Multi-head attention where query heads usually have independent K/V heads.", "It has the largest KV cache and is sized from the K/V-head count."],
  "MQA": ["Multi-query attention where every query head shares one K/V set.", "It is usually represented as the one-KV-head special case of GQA."],
  "GQA": ["Grouped-query attention where groups of query heads share fewer K/V heads.", "It reduces KV; TP beyond the K/V-head count may require replication."],
  "MLA": ["Attention that stores a lower-dimensional latent representation of K/V.", "KV uses latent width and conservatively assumes replication across TP ranks."],
  "SWA": ["Sliding-window attention over only the most recent tokens.", "KV and attention work for local layers are capped at the window."],
  "DSA": ["Sparse attention that selects a limited set of keys from long context.", "It reduces long-context attention reads and compute without shrinking stored KV."],
  "RoPE": ["Rotary position embeddings applied to queries and keys.", "Beyond-native context triggers an extension warning with unknown quality."],
  "YaRN": ["A method for extending RoPE context length.", "The calculator warns about it but does not assume unchanged quality or performance."],
  "FlashAttention": ["Exact IO-aware attention that tiles work to reduce HBM traffic.", "Workspace avoids an O(n²) matrix, while attention FLOPs still count."],
  "MTP": ["Model-trained modules or heads that predict multiple future tokens.", "Model metadata and measured acceptance and overhead are required before applying a gain."],
  "Speculative Decoding": ["Drafting candidate tokens and verifying them in a batch with the target model.", "No cross-model default speedup is assumed; measured τ and verify overhead are required."],
  "Draft Model": ["A smaller model that proposes tokens for a target model.", "Draft memory enters the budget and speedup must be calibrated."],
  "EAGLE": ["A speculative method that predicts draft tokens in feature space.", "Selecting it alone adds no gain; matched heads and measured parameters are required."],
  "Medusa": ["Multiple prediction heads attached to a target model to verify a token tree.", "The heads consume memory and acceptance depends on the workload."],
  "Lookahead": ["Parallel candidate generation using structures such as n-grams rather than a separate draft model.", "Benefit depends heavily on repeated patterns and requires calibration."],
  "Continuous Batching": ["Refilling batch slots as requests finish.", "The static batch curve does not simulate per-request scheduling trajectories."],
  "Chunked Prefill": ["Splitting a long prompt into chunks that can interleave with decode or other requests.", "Each chunk may reread weights and add scheduling overhead."],
  "CUDA Graph": ["A recorded GPU work graph that reduces repeated launch overhead.", "No fixed gain is invented; measured schedule time can calibrate it."],
  "Kernel Fusion": ["Combining kernels to reduce launches and intermediate memory traffic.", "It is represented only through measured utilization, not as a separate multiplier."],
  "Roofline": ["A performance model bounded by the slower of memory bandwidth and compute throughput.", "Decode uses max(memory, compute), then adds communication and scheduling."],
  "FLOPS": ["Floating-point operations performed per second.", "Dense versus sparse, dtype, and tensor-core usage must be distinguished."],
  "TFLOPS": ["One trillion floating-point operations per second.", "The hardware table stores dense peaks; missing per-precision data is marked as an estimate."],
  "TOPS": ["One trillion integer or low-precision operations per second.", "INT8 specifications cannot be substituted for FP16 TFLOPS unconditionally."],
  "Memory-bound": ["Execution limited mainly by moving data rather than arithmetic.", "Common in low-batch decode; quantization can help by reducing weight reads."],
  "Compute-bound": ["Execution limited mainly by compute peak and utilization.", "Long prefill, high batch, and long attention are more likely to reach this region."],
  "TP": ["Tensor parallelism partitions layer tensors across devices.", "It shards weights and some KV while adding collective communication."],
  "PP": ["Pipeline parallelism places different layers on different stages.", "It lowers resident weights per card; latency still includes stage time and bubbles."],
  "EP": ["Expert parallelism distributes MoE experts across ranks.", "It shards expert weights and compute and adds dispatch/combine All-to-All."],
  "CP": ["Context parallelism partitions attention and KV along the token dimension.", "It reduces per-card KV and attention work while adding context collectives."],
  "DP": ["Data parallelism uses independent replicas for different requests.", "The planner represents it with replicas and does not search complex placement."],
  "AllReduce": ["A collective that aggregates partial results and distributes the result to all ranks.", "TP communication uses a first-order ring traffic estimate."],
  "All-to-All": ["A collective where every rank exchanges distinct data with several peers.", "MoE expert dispatch and combine use this pattern."],
  "NVLink": ["NVIDIA's high-bandwidth GPU interconnect.", "Communication uses the recorded GB/s and optional measured utilization."],
  "PCIe": ["The general host and accelerator interconnect bus.", "Unknown dedicated links conservatively use 25 GB/s effective PCIe 4.0 x16."],
  "XGMI": ["AMD's GPU interconnect based on Infinity Fabric.", "Like NVLink, it uses the recorded bidirectional bandwidth as an approximation."],
  "M/M/c": ["A queue with Poisson arrivals, exponential service times, and c independent servers.", "When enabled, it estimates mean and unconditional p95 waiting time."],
  "Utilization": ["Arrival workload as a fraction of service capacity.", "Queueing rises nonlinearly near 100%; no steady state exists at ρ≥1."],
  "TDP": ["A thermal and power-design specification, not measured workload power.", "Cost uses TDP, 60% load, and the assumed electricity price."],
  "PUE": ["Total data-center energy divided by IT-equipment energy.", "Current cost excludes PUE; production budgets must add it."],
  "vLLM": ["An open-source inference engine focused on high-throughput LLM serving.", "It is the default NVIDIA/general baseline; version and kernel differences require calibration."],
  "SGLang": ["An inference runtime for structured generation, caching, and high-performance serving.", "FP4 KV is modeled only for supported SGLang plus FP4-hardware combinations."],
  "TensorRT-LLM": ["NVIDIA's compiled runtime stack for large language models.", "Only listed NVIDIA paths are compatible; no default performance advantage is invented."],
  "llama.cpp": ["A local inference runtime spanning CPU, Metal, CUDA, and other backends.", "GGUF quantization tiers automatically prefer this engine."],
  "MLX": ["Apple's machine-learning framework for unified-memory Silicon.", "MLX checkpoints are native only on Apple hardware."],
  "ExLlama": ["A runtime for low-bit inference on NVIDIA consumer GPUs.", "EXL formats are marked native only on NVIDIA hardware."],
};
const CLS = {
  consumer: ["Consumer", "消费级"], workstation: ["Workstation", "工作站"],
  datacenter: ["Data Center", "数据中心"], supernode: ["Supernode", "超节点"],
  unified_soc: ["Unified Memory", "统一内存"], edge: ["Edge", "边缘"],
  sram_asic: ["SRAM ASIC", "SRAM 专用"],
};
const LINK = {
  none: ["—", "—"], bridge: ["NVLink Bridge", "NVLink 桥"], nvlink: ["NVLink", "NVLink"],
  xgmi: ["XGMI", "XGMI"], ualoe: ["UALoE", "UALoE"], ethernet: ["Ethernet", "以太网"],
  hccs: ["HCCS", "HCCS"], unified: ["Unified Memory", "统一内存"], xelink: ["Xe Link", "Xe Link"],
  ici: ["ICI", "ICI"], blink: ["BLink", "BLink"], mlulink: ["MLU-Link", "MLU-Link"],
  neuronlink: ["NeuronLink", "NeuronLink"],
};
const localized = (map, key) => map[key]?.[lang === "zh" ? 1 : 0] ?? key;
const catalogNote = note => lang === "zh" ? note : "";
const ENGINE_EN = {
  auto: ["Automatic", "Selects a runtime from the quantization format and hardware vendor"],
  vllm: ["vLLM", "PagedAttention and continuous batching; secondary hardware paths may require vendor plugins"],
  sglang: ["SGLang", "RadixAttention and PD/EP/DP serving; verify supported combinations by version"],
  trtllm: ["TensorRT-LLM", "NVIDIA CUDA inference stack; kernels and features depend on GPU generation and version"],
  llamacpp: ["llama.cpp", "GGUF runtime across CPU, Metal, CUDA, HIP, and other backends"],
  mlx: ["MLX", "Unified-memory runtime for Apple Silicon"],
  exllama: ["ExLlamaV3", "EXL3 quantized runtime for NVIDIA GPUs"],
  lmdeploy: ["LMDeploy", "TurboMind/vLLM backends; only verified NVIDIA paths are listed here"],
  mindie: ["MindIE", "Official Huawei Ascend inference engine"],
};
const SPEC_EN = {
  none: ["Off", "Autoregressive, one token at a time"],
  mtp: ["Native MTP", "Requires model MTP metadata and measured acceptance"],
  eagle3: ["EAGLE-3", "Requires a trained draft head matched to the target model"],
  medusa: ["Medusa", "Requires Medusa heads matched to the target model"],
  draft: ["Draft Model", "Requires a compatible smaller model and measured overhead"],
  lookahead: ["Lookahead", "Zero-draft n-gram method; benefit depends on repeated patterns"],
  dflash: ["DFlash", "Requires a compatible block-diffusion draft model"],
  dflash2: ["DFlash2", "Requires a compatible draft checkpoint"],
};
const engineName = e => lang === "zh" ? e.name : (ENGINE_EN[e.id]?.[0] ?? e.name);
const engineNote = e => lang === "zh" ? e.note : (ENGINE_EN[e.id]?.[1] ?? "");
const specName = s => lang === "zh" ? s.name : (SPEC_EN[s.id]?.[0] ?? s.name);
const specNote = s => lang === "zh" ? (s.id === "none" ? "逐 token" : "需实测 τ / 开销") : (SPEC_EN[s.id]?.[1] ?? "");

const fmt = {
  tps: (v) => v >= 1000 ? (v / 1000).toFixed(1) + "k" : (+v).toFixed(1),
  rate: (v) => v >= 1000 ? (v / 1000).toFixed(1) + "k" : v >= 1 ? (+v).toFixed(1) : (+v).toFixed(3),
  tpm: (v) => v >= 1e6 ? (v / 1e6).toFixed(2) + "M" : v >= 1000 ? (v / 1000).toFixed(1) + "k" : Math.round(v),
  gb: (v) => (+v).toFixed(1) + " G",
  ms: (v) => v >= 1000 ? (v / 1000).toFixed(2) + " s" : (+v).toFixed(0) + " ms",
  cny: (v) => lang === "zh" ? (v >= 10000 ? (v / 10000).toFixed(1) + " 万" : Math.round(v).toLocaleString() + " 元") : "CNY " + (v >= 1000 ? (v / 1000).toFixed(1) + "k" : Math.round(v).toLocaleString()),
};

function repMark(conf) {
  if (conf === "reported") return ` <sup class="rep" title="${tr("Publicly reported or estimated", "公开报道或推算口径")}">R</sup>`;
  if (conf === "fetched") return ` <sup class="rep" title="${tr("Automatically parsed from a model hub; active parameters may be structurally derived or heuristic", "模型平台自动解析；请查看备注，激活参数可能为结构推导或启发式估算")}">F</sup>`;
  return "";
}

let theme = localStorage.getItem("llmcalc-theme") === "dark" ? "dark" : "light";
document.documentElement.dataset.theme = theme;

function setTheme(next) {
  theme = next;
  document.documentElement.dataset.theme = theme;
  localStorage.setItem("llmcalc-theme", theme);
  const btn = $("#themeBtn");
  if (btn) btn.textContent = theme === "light" ? tr("Dark", "深色") : tr("Light", "亮色");
}

/* ---------- 自定义下拉组件 ---------- */

function cselect(el, groups, opts = {}) {
  const search = opts.search !== false;
  el.classList.add("cs");
  el.innerHTML = `
    <button type="button" class="cs-btn"><span class="cs-v"></span><span class="cs-arrow">▾</span></button>
    <div class="cs-pop">
      ${search ? `<input class="cs-q" placeholder="${tr("Search…", "搜索…")}">` : ""}
      <div class="cs-list"></div>
    </div>`;
  const btn = el.querySelector(".cs-btn"), pop = el.querySelector(".cs-pop"),
        list = el.querySelector(".cs-list"), q = el.querySelector(".cs-q"),
        vEl = el.querySelector(".cs-v");

  let value = opts.value ?? null, flat = [], disabled = false;
  list.innerHTML = groups.map(g =>
    (g.label ? `<div class="cs-grp" data-grp>${g.label}</div>` : "") +
    g.items.map(it => `<div class="cs-opt" data-v="${it.v}" data-s="${(it.n + " " + (it.m || "")).toLowerCase()}">
      <span class="cs-n">${it.n}</span>${it.m ? `<span class="cs-m">${it.m}</span>` : ""}</div>`).join("")
  ).join("");
  flat = [...list.querySelectorAll(".cs-opt")];

  function render() {
    const cur = flat.find(o => o.dataset.v === value);
    vEl.innerHTML = cur ? `${cur.querySelector(".cs-n").textContent}` +
      (cur.querySelector(".cs-m") ? `<span class="cs-m">${cur.querySelector(".cs-m").textContent}</span>` : "") : "—";
    flat.forEach(o => o.classList.toggle("sel", o.dataset.v === value));
  }
  function close() { el.classList.remove("open"); }
  btn.onclick = (e) => {
    if (disabled) return;
    e.stopPropagation();
    $$(".cs.open").forEach(x => x !== el && x.classList.remove("open"));
    el.classList.toggle("open");
    if (el.classList.contains("open") && q) { q.value = ""; filter(""); q.focus(); }
  };
  function filter(s) {
    s = s.toLowerCase();
    flat.forEach(o => o.classList.toggle("hidden", s && !o.dataset.s.includes(s)));
    list.querySelectorAll(".cs-grp").forEach(g => {
      let vis = false, n = g.nextElementSibling;
      while (n && n.classList.contains("cs-opt")) { if (!n.classList.contains("hidden")) vis = true; n = n.nextElementSibling; }
      g.classList.toggle("hidden", !vis);
    });
    let empty = list.querySelector(".cs-empty");
    if (!vis_any()) { if (!empty) list.insertAdjacentHTML("beforeend", `<div class="cs-empty">${tr("No matches", "没有匹配项")}</div>`); }
    else if (empty) empty.remove();
  }
  const vis_any = () => flat.some(o => !o.classList.contains("hidden"));
  if (q) q.oninput = () => filter(q.value);
  flat.forEach(o => o.onclick = () => {
    value = o.dataset.v; render(); close();
    if (opts.onChange) opts.onChange(value);
  });
  document.addEventListener("click", (e) => { if (!el.contains(e.target)) close(); });
  el.addEventListener("keydown", (e) => { if (e.key === "Escape") close(); });

  render();
  return {
    get: () => value,
    set: (v, silent) => { value = v; render(); if (!silent && opts.onChange) opts.onChange(v); },
    setDisabled: (on) => {
      disabled = on; btn.disabled = on; el.classList.toggle("locked", on);
      if (on) close();
    },
  };
}

const CTX_OPTS = [
  { label: "", items: [
    { v: "4096", n: "4 K", m: "4096" },
    { v: "8192", n: "8 K", m: "8192" },
    { v: "32768", n: "32 K", m: "32768" },
    { v: "131072", n: "128 K", m: "131072" },
    { v: "262144", n: "256 K", m: "262144" },
    { v: "524288", n: "500 K", m: "524288" },
    { v: "1048576", n: "1 M", m: "1048576" },
  ]},
];

// 量化档位按家族分组；withAll 时最前加「全部档位」（规划器用）
function quantGroups(withAll) {
  const FAM = [
    ["std", tr("Standard · separate W/A precision", "标准 · W/A 精度分离")],
    ["gguf", tr("GGUF · llama.cpp ecosystem", "GGUF · llama.cpp 生态")],
    ["mlx", tr("MLX · Apple only", "MLX · Apple 专用")],
    ["exl", tr("ExLlama · consumer GPUs", "ExLlama · 消费卡")],
  ];
  const groups = FAM.map(([f, label]) => ({
    label,
    items: QUANTS.filter(q => q.fam === f)
      .map(q => ({ v: q.id, n: q.name, m: `W${q.w} A${q.a} · ${q.bytes}B` })),
  }));
  if (withAll) groups.unshift({ label: "", items: [{ v: "", n: tr("All quantization options", "全部档位") }] });
  return groups;
}

function syncModelQuant(modelKey, quantKey, noteID) {
  const m = MODELS.find(x => x.id === CS[modelKey].get());
  const fixed = fixedQuantID(m);
  const note = $("#" + noteID);
  if (fixed) {
    CS[quantKey].set(fixed, true);
    CS[quantKey].setDisabled(true);
    const q = QUANTS.find(x => x.id === fixed);
    note.textContent = tr(
      `Native ${q?.name || fixed.toUpperCase()} checkpoint: weight format is applied and locked; KV-cache precision remains independent.`,
      `原生 ${q?.name || fixed.toUpperCase()} 检查点：权重格式已自动带入并锁定；KV cache 量化仍独立选择。`);
    note.classList.add("locked");
  } else {
    CS[quantKey].setDisabled(false);
    note.textContent = modelKey === "p-model"
      ? tr("Base weights can simulate deployment quantization; KV-cache precision is an independent runtime option.", "基础权重可模拟不同部署量化；KV cache 精度是独立运行时选项。")
      : tr("Base weights evaluate quantization options; KV-cache precision does not follow the weight format.", "基础权重枚举量化方案；KV cache 精度不跟随权重格式。");
    note.classList.remove("locked");
  }
  return fixed;
}

const ADV_FIELDS = [
  { k: "tp", n: "TP ranks", v: 0, min: 0, step: 1, topo: true, tip: tr("0 = use all cards for TP", "0 = 全部卡做 TP") },
  { k: "pp", n: "PP stages", v: 0, min: 0, step: 1, topo: true },
  { k: "ep", n: "EP ranks", v: 0, min: 0, step: 1, topo: true },
  { k: "cp", n: "CP ranks", v: 0, min: 0, step: 1, topo: true },
  { k: "weight_gb", n: tr("Measured weight GB", "实际权重 GB"), v: 0, min: 0, step: .1, tip: tr("whole replica", "整个副本") },
  { k: "runtime_gb", n: tr("Runtime resident GB", "框架常驻 GB"), v: 0, min: 0, step: .1, tip: tr("measured per card", "每卡实测") },
  { k: "activation_gb", n: tr("Peak workspace GB", "峰值 workspace GB"), v: 0, min: 0, step: .1, tip: tr("measured per card", "每卡实测") },
  { k: "adapter_gb", n: "Adapter GB", v: 0, min: 0, step: .1, tip: tr("whole replica", "整个副本") },
  { k: "draft_gb", n: "Draft / MTP GB", v: 0, min: 0, step: .1, tip: tr("whole replica", "整个副本") },
  { k: "mem_util", n: tr("Memory budget ratio", "显存预算比例"), v: 0, min: 0, max: 1, step: .01 },
  { k: "bw_util", n: tr("HBM utilization", "HBM 利用率"), v: 0, min: 0, max: 1, step: .01 },
  { k: "flops_util", n: tr("Compute utilization", "算力利用率"), v: 0, min: 0, max: 1, step: .01 },
  { k: "link_util", n: tr("Interconnect utilization", "互联利用率"), v: 0, min: 0, max: 1, step: .01 },
  { k: "schedule_ms", n: tr("Scheduling ms / step", "调度 ms / step"), v: 0, min: 0, step: .1 },
  { k: "kv_overhead", n: tr("KV allocator factor", "KV allocator 系数"), v: 1, min: 1, max: 2, step: .01 },
  { k: "kv_offload", n: tr("KV offload ratio", "KV offload 比例"), v: 0, min: 0, max: 1, step: .05 },
  { k: "offload_bw", n: tr("Offload effective GB/s", "Offload 有效 GB/s"), v: 0, min: 0, step: 1 },
  { k: "prefill_chunk", n: "Prefill chunk", v: 8192, min: 1, step: 128 },
  { k: "media_tokens", n: tr("Media tokens", "媒体 token"), v: 0, min: 0, step: 1 },
  { k: "router_skew", n: tr("Router busiest / average", "Router 最忙/平均"), v: 1, min: 1, max: 16, step: .05 },
  { k: "spec_tau", n: tr("Measured accepted tokens τ", "实测接受 token τ"), v: 0, min: 0, max: 32, step: .1 },
  { k: "spec_ovh", n: tr("Draft/verify overhead", "Draft/verify 开销"), v: 0, min: 0, max: 10, step: .01 },
];

function mountAdvanced(prefix, topology) {
  const el = $(`#${prefix}-advanced`);
  el.innerHTML = ADV_FIELDS.filter(f => topology || !f.topo).map(f =>
    `<label title="${f.tip || ""}">${f.n}<input type="number" data-opt="${f.k}" value="${f.v}" min="${f.min}"` +
    `${f.max == null ? "" : ` max="${f.max}"`} step="${f.step}"></label>`).join("");
}

function advancedOpts(prefix) {
  return Object.fromEntries($$(`#${prefix}-advanced [data-opt]`).map(el => [el.dataset.opt, +el.value || 0]));
}

/* ---------- 初始化 ---------- */

async function boot() {
  applyLanguage();
  [HW, MODELS, QUANTS, ENGINES, SPECS] = await Promise.all([
    fetch("/api/hardware").then(r => r.json()),
    fetch("/api/models").then(r => r.json()),
    fetch("/api/quants").then(r => r.json()),
    fetch("/api/engines").then(r => r.json()),
    fetch("/api/specs").then(r => r.json()),
  ]);

  mountAdvanced("p", true);
  mountAdvanced("pl", false);

  $("#dbMeta").innerHTML =
    tr(`Hardware <b>${HW.length}</b> · Models <b>${MODELS.length}</b> · Data 2026-09`, `硬件 <b>${HW.length}</b> 款 · 模型 <b>${MODELS.length}</b> 个 · 数据 2026-09`);
  $("#footMeta").textContent = `v0.3 · ${HW.length} HW / ${MODELS.length} MODELS`;

  // 硬件下拉（按厂商分组，带搜索）
  const hwGroups = () => {
    const g = {};
    HW.filter(h => !h.svc).forEach(h => (g[h.vendor] ??= []).push(h));
    return Object.entries(g).map(([v, list]) => ({
      label: vendorName(v),
      items: list.map(h => ({ v: h.id, n: hardwareName(h), m: `${h.vram}G · ${h.bw.toLocaleString()} GB/s` })),
    }));
  };
  // 模型下拉（按机构分组，带搜索）
  const modelGroups = () => {
    const sorted = [...MODELS].sort((a, b) => !!fixedQuantID(a) - !!fixedQuantID(b) || a.params - b.params);
    const g = {};
    sorted.forEach(m => (g[m.org] ??= []).push(m));
    return Object.entries(g).map(([org, list]) => ({
      label: org,
      items: list.map(m => ({ v: m.id, n: m.name, m: `${m.params}B${m.moe ? " MoE" : ""} · ${fixedQuantID(m) ? tr("native ", "原生 ") + m.native_quant.toUpperCase() : m.year}` })),
    }));
  };

  CS["f-hw"] = cselect($("#f-hw"), hwGroups(), { onChange: runFit });
  CS["f-ctx"] = cselect($("#f-ctx"), CTX_OPTS, { search: false, onChange: runFit });
  CS["p-hw"] = cselect($("#p-hw"), hwGroups(), { onChange: runPerf });
  CS["p-model"] = cselect($("#p-model"), modelGroups(), { onChange: () => { syncModelQuant("p-model", "p-quant", "p-quant-note"); runPerf(); } });
  CS["p-quant"] = cselect($("#p-quant"), quantGroups(), { onChange: runPerf });
  CS["p-ctx"] = cselect($("#p-ctx"), CTX_OPTS, { search: false, onChange: runPerf });
  CS["pl-model"] = cselect($("#pl-model"), modelGroups(), { onChange: () => { syncModelQuant("pl-model", "pl-qonly", "pl-quant-note"); runPlan(); } });
  CS["pl-ctx"] = cselect($("#pl-ctx"), CTX_OPTS, { search: false, onChange: runPlan });
  CS["pl-qonly"] = cselect($("#pl-qonly"), quantGroups(true), { search: false, onChange: runPlan });
  CS["hw-vendor"] = cselect($("#hw-vendor"), [{ label: "", items: [{ v: "", n: tr("All vendors", "全部厂商") }, ...Object.keys(VENDOR).map(v => ({ v, n: vendorName(v) }))] }], { search: false, onChange: renderHWTable });
  const planFilter = () => { planPage = 0; renderPlans(); };
  CS["pl-vendor"] = cselect($("#pl-vendor"), [{ label: "", items: [{ v: "", n: tr("All vendors", "全部厂商") }, ...Object.keys(VENDOR).map(v => ({ v, n: vendorName(v) }))] }], { search: false, onChange: planFilter });
  CS["pl-cls"] = cselect($("#pl-cls"), [{ label: "", items: [{ v: "", n: tr("All classes", "全部类别") }, ...Object.keys(CLS).map(v => ({ v, n: localized(CLS, v) }))] }], { search: false, onChange: planFilter });

  // 推理框架 / 推测解码（三页各一份实例）
  const engGroups = () => [{ label: "", items: ENGINES.map(e => ({ v: e.id, n: engineName(e), m: engineNote(e) })) }];
  const specGroups = () => [{ label: "", items: SPECS.map(s => ({ v: s.id, n: specName(s), m: specNote(s) })) }];
  CS["f-eng"] = cselect($("#f-eng"), engGroups(), { search: false, onChange: runFit });
  CS["f-spec"] = cselect($("#f-spec"), specGroups(), { search: false, onChange: runFit });
  CS["p-eng"] = cselect($("#p-eng"), engGroups(), { search: false, onChange: runPerf });
  CS["p-spec"] = cselect($("#p-spec"), specGroups(), { search: false, onChange: runPerf });
  CS["pl-eng"] = cselect($("#pl-eng"), engGroups(), { search: false, onChange: runPlan });
  CS["pl-spec"] = cselect($("#pl-spec"), specGroups(), { search: false, onChange: runPlan });

  // 默认值
  CS["f-hw"].set("rtx4090", true); CS["f-ctx"].set("8192", true);
  CS["p-hw"].set("rtx4090", true); CS["p-model"].set("llama-3.1-70b", true);
  CS["p-quant"].set("q4km", true); CS["p-ctx"].set("4096", true);
  CS["pl-model"].set("deepseek-r1", true); CS["pl-ctx"].set("8192", true);
  CS["pl-qonly"].set("", true); CS["hw-vendor"].set("", true);
  CS["pl-vendor"].set("", true); CS["pl-cls"].set("", true);
  syncModelQuant("p-model", "p-quant", "p-quant-note");
  syncModelQuant("pl-model", "pl-qonly", "pl-quant-note");
  setTheme(theme);
  $("#themeBtn").onclick = () => setTheme(theme === "light" ? "dark" : "light");
  $("#langBtn").onclick = () => {
    localStorage.setItem(LANG_KEY, lang === "en" ? "zh" : "en");
    location.reload();
  };
  ["f-eng", "p-eng", "pl-eng"].forEach(id => CS[id].set("auto", true));
  ["f-spec", "p-spec", "pl-spec"].forEach(id => CS[id].set("none", true));

  // hash 深链接
  const hash = location.hash.slice(1);
  if (hash && $(`#tabs button[data-v="${hash}"]`)) {
    $$("#tabs button").forEach(x => x.classList.toggle("on", x.dataset.v === hash));
    $$(".view").forEach(v => v.classList.toggle("on", v.id === "v-" + hash));
  }

  wire();
  wireCustom();
  runFit(); runPerf(); runPlan();
  renderHWTable(); renderModelTable(); renderQuickTable(); renderGlossary();
}

function wire() {
  $$("#tabs button").forEach(b => b.onclick = () => {
    $$("#tabs button").forEach(x => x.classList.toggle("on", x === b));
    $$(".view").forEach(v => v.classList.toggle("on", v.id === "v-" + b.dataset.v));
  });
  const bind = (id, fn) => {
    const el = $(id);
    el.oninput = el.onchange = () => {
      const v = $(id + "-v"); if (v) v.textContent = el.value;
      fn();
    };
  };
  bind("#f-n", runFit); bind("#f-b", runFit);
  bind("#p-n", runPerf); bind("#p-b", runPerf);
  bind("#pl-tpm", runPlan); bind("#pl-c", runPlan);
  $("#hw-q").oninput = renderHWTable;
  $("#f-q").oninput = () => { fitPage = 0; renderFitRows(); };
  $("#m-q").oninput = () => { modelPage = 0; renderModelTable(); };
  $("#g-q").oninput = renderGlossary;
  $("#p-hit").oninput = () => { $("#p-hit-v").textContent = $("#p-hit").value + "%"; runPerf(); };
  $("#pl-hit").oninput = () => { $("#pl-hit-v").textContent = $("#pl-hit").value + "%"; runPlan(); };
  $("#p-outlen").oninput = runPerf;
  $$("#p-advanced input").forEach(el => { el.oninput = runPerf; });
  $$("#pl-advanced input").forEach(el => { el.oninput = runPlan; });
}

async function post(url, body) {
  const r = await fetch(url, {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  return r.json();
}

/* ---------- 模式一：能装什么 ---------- */

async function runFit() {
  const body = {
    hw: CS["f-hw"].get(), n: +$("#f-n").value, ctx: +CS["f-ctx"].get(), batch: +$("#f-b").value,
    eng: CS["f-eng"].get(), spec: CS["f-spec"].get(), kvq: segKvqF ? segKvqF.get() : "fp16",
    lang,
  };
  const rows = await post("/api/fit", body);
  lastFitRows = rows;
  lastFitCtx = body.ctx;
  const h = HW.find(x => x.id === body.hw);

  $("#f-devline").innerHTML =
    `<span class="dv-name">${hardwareName(h)}${repMark(h.conf)}${body.n > 1 ? ` <span class="mono" style="color:var(--acc)">× ${body.n}</span>` : ""}</span>` +
    `<span class="mono">${tr("Memory", "显存")} <b>${h.vram}G</b></span>` +
    `<span class="mono">${tr("Bandwidth", "带宽")} <b>${h.bw} GB/s</b></span>` +
    `<span class="mono">${tr("Interconnect", "互联")} <b>${localized(LINK, h.link.t)}${h.link.b ? " " + h.link.b + " GB/s" : ""}</b></span>` +
    (catalogNote(h.notes) ? `<span style="color:var(--faint)">${catalogNote(h.notes)}</span>` : "");

  fitPage = 0;
  renderFitRows();
}

let lastFitRows = [], lastFitCtx = 0, fitPage = 0;
function renderFitRows() {
  const fq = ($("#f-q")?.value || "").trim().toLowerCase();
  const rows = fq ? lastFitRows.filter(r =>
    r.model.name.toLowerCase().includes(fq) || r.model.org.toLowerCase().includes(fq)) : lastFitRows;
  const pageSize = 100;
  const pages = Math.max(1, Math.ceil(rows.length / pageSize));
  fitPage = Math.min(fitPage, pages - 1);
  const page = rows.slice(fitPage * pageSize, (fitPage + 1) * pageSize);
  const quants = QUANTS.filter(q => q.main).map(q => `<th class="n">${q.name}</th>`).join("");
  let html = `<thead><tr><th>${tr("Model", "模型")}</th><th class="n">${tr("Parameters", "参数")}</th><th>${tr("Architecture", "架构")}</th>${quants}</tr></thead><tbody>`;
  for (const r of page) {
    const m = r.model;
    const cells = r.cells.map(c => {
      if (!c.applicable) return `<td class="n"><span class="cell na" title="${tr("This is a pre-quantized repository and supports only its native weight format", "该仓库是预量化检查点，只能使用其原生权重格式")}">${tr("N/A", "不适用")}</span></td>`;
      if (c.fit === 0) return `<td class="n"><span class="cell no">—</span></td>`;
      const cls = c.fit === 2 ? "ok" : "warn";
      const flag = c.accel ? "" : `<span class="flag" title="${tr("Saves memory without hardware acceleration", "仅省显存，不硬件加速")}">no-acc</span>`;
      return `<td class="n"><span class="cell ${cls}" title="${tr("Single-stream tok/s", "单流 tok/s")}">${fmt.tps(c.tps)}</span>${flag}</td>`;
    }).join("");
    const yarn = lastFitCtx > m.ctx ? `<span class="flag" title="${tr(`Above the native ${(m.ctx / 1024).toFixed(0)}K context; requires YaRN extension`, `超原生上下文 ${(m.ctx / 1024).toFixed(0)}K，需 YaRN 外推`)}">YaRN</span>` : "";
    html += `<tr>
      <td class="mname">${m.name}${repMark(m.conf)}${yarn}<span class="msub">${m.org}</span></td>
      <td class="n">${m.params}B${m.moe ? `<span class="msub">/${m.active}B</span>` : ""}</td>
      <td class="dim">${m.moe ? "MoE" : "Dense"}${m.kvt === "mla" ? "·MLA" : ""}${m.sparse ? "·DSA" : ""}</td>
      ${cells}</tr>`;
  }
  $("#fitTable").innerHTML = html + "</tbody>";
  $("#f-pager").innerHTML =
    `<button class="minibtn" id="f-pg-prev" ${fitPage === 0 ? "disabled" : ""}>‹ ${tr("Previous", "上一页")}</button>
     <span class="mono">${tr(`Page ${fitPage + 1} / ${pages} · ${rows.length} items`, `第 ${fitPage + 1} / ${pages} 页 · 共 ${rows.length} 条`)}</span>
     <button class="minibtn" id="f-pg-next" ${fitPage >= pages - 1 ? "disabled" : ""}>${tr("Next", "下一页")} ›</button>`;
  $("#f-pg-prev").onclick = () => { fitPage--; renderFitRows(); };
  $("#f-pg-next").onclick = () => { fitPage++; renderFitRows(); };
}

/* ---------- 模式二：能跑多快 ---------- */

async function runPerf() {
  const body = {
    hw: CS["p-hw"].get(), n: +$("#p-n").value, model: CS["p-model"].get(),
    quant: CS["p-quant"].get(), ctx: +CS["p-ctx"].get(), batch: +$("#p-b").value,
    eng: CS["p-eng"].get(), spec: CS["p-spec"].get(), kvq: segKvqP ? segKvqP.get() : "fp16",
    hit: (+$("#p-hit").value) / 100, outlen: +$("#p-outlen").value,
    advanced: advancedOpts("p"),
    lang,
  };
  const { perf: p, curve } = await post("/api/perf", body);
  const h = HW.find(x => x.id === body.hw);
  const m = MODELS.find(x => x.id === body.model);

  $("#perfHero").innerHTML = [
    [tr("Decode · single stream", "decode 单流"), fmt.tps(p.single_tps), tr("tok / s · per-user experience", "tok / s · 每用户体感"), true],
    [tr("Decode · aggregate", "decode 聚合"), fmt.tps(p.agg_tps), tr(`tok / s · ${body.batch} concurrent total`, `tok / s · ${body.batch} 并发合计`)],
    [tr("Prefill speed", "prefill 速度"), fmt.tps(p.pre_tps), tr("tok / s · prompt processing", "tok / s · 提示词处理")],
    [tr("Mixed TPM", "TPM 混合"), fmt.tpm(p.tpm_mixed), tr("tok / min · input + output", "tok / min · 输入+输出")],
    ["TTFT", fmt.ms(p.ttft_ms), tr("first-token latency", "首 token 延迟")],
    ["TPOT", fmt.ms(p.tpot_ms), tr("per-token interval", "逐 token 间隔")],
    [tr("Request latency", "请求时延"), fmt.ms(p.req_ms), tr(`TTFT + ${body.outlen} output tokens`, `TTFT + ${body.outlen} tok 输出`)],
    [tr("Request rate", "请求速率"), fmt.rate(p.req_s), tr("req / s · current input/output length", "req / s · 当前输入/输出长度")],
    [tr("Decode bottleneck", "decode 瓶颈"), ({ compute: tr("Compute", "算力"), memory: tr("Memory bandwidth", "显存带宽"), offload: "KV offload" })[p.bottleneck] || p.bottleneck,
      `memory ${fmt.ms(p.decode_mem_ms)} · compute ${fmt.ms(p.decode_compute_ms)} · comm ${fmt.ms(p.comm_ms)}`],
    [tr("Estimate basis", "计算口径"), !p.topology_ok ? tr("Topology fallback", "拓扑已回退") : p.accuracy === "calibrated" ? tr("Calibrated", "已校准") : tr("Parsed estimate", "解析估算"),
      `${p.topology} · ${p.peak_tf.toFixed(0)} TF ${p.peak_exact ? tr("vendor precision-specific", "厂商逐精度") : tr("multiplier estimate", "倍率估算")}${p.accuracy === "analytical" ? tr(" · no statistical confidence interval", " · 无统计置信区间") : ""}`],
  ].map(([k, v, u, hot]) =>
    `<div class="stat ${p.fit ? "" : "bad"} ${hot ? "hot" : ""}">
      <div class="k">${k}</div><div class="v">${v}</div><div class="u">${u}</div></div>`).join("");

  const d = p.mem, cap = d.cap;
  const pct = (v) => Math.max(0, v / cap * 100);
  const segs = [
    [tr("Weights", "权重"), d.weights, "vw"], ["KV cache", d.kv, "vkv"], [tr("Runtime", "框架"), d.fw, "vfw"],
    [tr("Activations", "激活"), d.act, "vact"], ["Adapter / draft", d.adapter, "vadp"], [tr("Reserve", "预留"), d.sys, "vsys"],
  ];
  const used = segs.reduce((s, x) => s + x[1], 0);
  const colors = { vw: "var(--weight)", vkv: "var(--kv)", vfw: "var(--runtime)", vact: "var(--active)", vadp: "var(--adapter)", vsys: "var(--reserve)" };
  $("#vramBar").innerHTML =
    `<div class="vbar">` +
    segs.map(([k, v, c]) => `<div class="${c}" style="width:${pct(v)}%" title="${k} ${fmt.gb(v)}"></div>`).join("") +
    `<div class="vfree" style="width:${Math.max(0, 100 - pct(used))}%"></div></div>` +
    `<div class="vlegend">` +
    segs.map(([k, v, c]) => `<span><i style="background:${colors[c]}"></i>${k} <span class="mono">${fmt.gb(v)}</span></span>`).join("") +
    `<span><i style="background:transparent;border:1px solid var(--line2)"></i>${tr("Free", "空闲")} <span class="mono">${fmt.gb(Math.max(0, cap - used))}</span></span>` +
    (d.offloaded_kv > 0 ? `<span>${tr("External KV", "外部 KV")} <span class="mono">${fmt.gb(d.offloaded_kv)}</span></span>` : "") +
    `</div>`;

  $("#perfFitState").innerHTML = p.fit
    ? `<span style="color:var(--ok)">${tr(`Fits · engine budget ${fmt.gb(d.budget)} · ${(d.head_pct * 100).toFixed(0)}% headroom`, `可部署 · 引擎预算 ${fmt.gb(d.budget)} · 余量 ${(d.head_pct * 100).toFixed(0)}%`)}</span>`
    : `<span style="color:var(--bad)">${tr(`Does not fit · ${fmt.gb(used - cap)} above engine budget`, `装不下 · 超引擎预算 ${fmt.gb(used - cap)}`)}</span>`;

  const effectiveQuant = QUANTS.find(q => q.id === p.quant);
  $("#perfTrace").innerHTML =
    `<div class="trow"><span class="tk">${tr("Deployment", "部署")}</span><span class="tv">${hardwareName(h)} × ${body.n}</span><span class="tn">${m.name} · ${effectiveQuant?.name || p.quant}${p.quant_locked ? tr(" · native checkpoint locked", " · 原生 checkpoint 锁定") : ""}${p.accel ? "" : tr(" · memory saving only; no acceleration", " · 该档仅省显存不加速")}${p.kv_supported ? "" : tr(" · selected KV format not applied", " · 所选 KV 格式未应用")}</span></div>` +
    p.trace.map(t => `<div class="trow"><span class="tk">${t.k}</span><span class="tv">${t.v}</span><span class="tn">${t.n}</span></div>`).join("") +
    `<div class="trow"><span class="tk">${tr("Maximum concurrency", "最大并发")}</span><span class="tv">${p.max_batch}</span><span class="tn">${tr("Capacity limit using the current shared-prefix, KV, and peak-activation assumptions", "当前共享前缀、KV 与激活峰值口径的容量上限")}</span></div>`;

  drawCurve(curve, body.batch);
}

function drawCurve(curve, curB) {
  const W = 430, H = 180, PL = 42, PB = 28, PT = 16, PR = 10;
  const n = curve.length;
  const x = i => PL + i * (W - PL - PR) / Math.max(1, n - 1);
  const xLabels = curve.map((p, i) => `<text class="clabel" x="${x(i)}" y="${H - 8}" text-anchor="middle">${p.b}</text>`).join("");
  const grid = y => [0, .5, 1].map(r => `<line class="${r ? "gridline" : "axis"}" x1="${PL}" y1="${y(r)}" x2="${W - PR}" y2="${y(r)}"/>`).join("");

  const maxTPS = Math.max(...curve.map(p => p.agg), 1);
  const yTPS = ratio => H - PB - ratio * (H - PB - PT);
  const tpsPath = key => curve.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${yTPS(p[key] / maxTPS).toFixed(1)}`).join("");
  const tpsDots = curve.map((p, i) => {
    const hot = p.b === curB, cls = `cdot${p.fit ? "" : " unfit"}`;
    return `<circle class="${cls}" cx="${x(i)}" cy="${yTPS(p.agg / maxTPS)}" r="${hot ? 4 : 2.5}"><title>${tr(`Concurrency ${p.b}: aggregate ${fmt.tps(p.agg)}, single stream ${fmt.tps(p.single)} tok/s${p.fit ? "" : ", over VRAM"}`, `并发 ${p.b}：聚合 ${fmt.tps(p.agg)}，单流 ${fmt.tps(p.single)} tok/s${p.fit ? "" : "，超显存"}`)}</title></circle>`;
  }).join("");

  const maxMem = Math.max(...curve.map(p => p.used), ...curve.map(p => p.cap), 1);
  const yMem = ratio => H - PB - ratio * (H - PB - PT);
  const memPath = curve.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${yMem(p.used / maxMem).toFixed(1)}`).join("");
  const memDots = curve.map((p, i) => `<circle class="cdot${p.fit ? "" : " unfit"}" cx="${x(i)}" cy="${yMem(p.used / maxMem)}" r="${p.b === curB ? 4 : 2.5}"><title>${tr(`Concurrency ${p.b}: ${fmt.gb(p.used)} used / ${fmt.gb(p.cap)} physical`, `并发 ${p.b}：占用 ${fmt.gb(p.used)} / 物理 ${fmt.gb(p.cap)}`)}</title></circle>`).join("");
  const capY = yMem(curve[0].cap / maxMem);

  $("#curveBox").innerHTML = `
    <div class="chart">
      <h3>${tr("DECODE TPS × CONCURRENCY", "DECODE TPS × 并发")}</h3><p>${tr("Orange: aggregate; blue: single stream; red points exceed VRAM.", "橙色聚合；蓝色单流；红点表示该并发超显存。")}</p>
      <svg role="img" aria-label="${tr("Aggregate and single-stream throughput by concurrency", "聚合与单流吞吐随并发变化")}" viewBox="0 0 ${W} ${H}">
        ${grid(yTPS)}
        <text class="clabel" x="${PL - 5}" y="${PT + 4}" text-anchor="end">${fmt.tps(maxTPS)}</text>
        <path class="cline" d="${tpsPath("agg")}"/><path class="cline secondary" d="${tpsPath("single")}"/>
        ${tpsDots}${xLabels}
      </svg>
      <div class="chart-legend"><span><i></i>${tr("Aggregate TPS", "聚合 TPS")}</span><span><i class="secondary"></i>${tr("Single-stream TPS", "单流 TPS")}</span></div>
    </div>
    <div class="chart">
      <h3>${tr("VRAM × CONCURRENCY", "VRAM × 并发")}</h3><p>${tr("Total use includes system reserve; the red line is physical VRAM.", "总占用含系统预留；红线为物理显存。")}</p>
      <svg role="img" aria-label="${tr("VRAM use by concurrency", "显存占用随并发变化")}" viewBox="0 0 ${W} ${H}">
        ${grid(yMem)}
        <text class="clabel" x="${PL - 5}" y="${PT + 4}" text-anchor="end">${fmt.gb(maxMem)}</text>
        <path class="cline" d="${memPath}"/><line class="cline limit" x1="${PL}" y1="${capY}" x2="${W - PR}" y2="${capY}"/>
        ${memDots}${xLabels}
      </svg>
      <div class="chart-legend"><span><i></i>${tr("Total used", "总占用")}</span><span><i class="limit"></i>${fmt.gb(curve[0].cap)} ${tr("physical limit", "物理上限")}</span></div>
    </div>`;
}

/* ---------- 模式三：怎么配 ---------- */

// 分段选择器（Dense/MoE、优化目标等）
function seg(id, onChange) {
  const el = $(id);
  const get = () => el.querySelector("button.on")?.dataset.v || "";
  el.querySelectorAll("button").forEach(b => b.onclick = () => {
    el.querySelectorAll("button").forEach(x => x.classList.toggle("on", x === b));
    if (onChange) onChange();
  });
  return { get };
}

let customOn = false;
let segArch, segKVT, segObj, segKvqF, segKvqP, segKvqPl, segPsize;
let lastPlans = [], planPage = 0; // 「怎么配」最近一次结果，供客户端过滤/翻页

function customModel() {
  const moe = segArch.get() === "moe";
  const kvt = segKVT.get();
  const m = {
    name: tr("Custom ", "自定义 ") + (+$("#cm-params").value) + "B" + (moe ? " MoE" : ""),
    params: +$("#cm-params").value,
    moe,
    layers: +$("#cm-layers").value,
    hidden: +$("#cm-hidden").value,
    kvh: +$("#cm-kvh").value,
    dim: +$("#cm-dim").value,
    kvlayers: +$("#cm-kvlayers").value,
    local_layers: +$("#cm-local-layers").value,
    window: +$("#cm-window").value,
    kvt,
    ctx: 131072,
    intermediate: +$("#cm-intermediate").value,
    moe_intermediate: +$("#cm-moe-intermediate").value,
    experts: +$("#cm-experts").value,
    topk: +$("#cm-topk").value,
    shared_experts: +$("#cm-shared").value,
    moe_layers: +$("#cm-moelayers").value,
    mtp_heads: +$("#cm-mtpheads").value,
    encoder_params: +$("#cm-encoderparams").value,
  };
  if (moe) m.active = +$("#cm-active").value;
  if (kvt === "mla") m.mla = +$("#cm-mla").value;
  return m;
}

function wireCustom() {
  segArch = seg("#cm-arch", () => {
    $("#cm-active").disabled = segArch.get() !== "moe";
    runPlan();
  });
  segKVT = seg("#cm-kvt", () => {
    $("#cm-mla-wrap").style.display = segKVT.get() === "mla" ? "" : "none";
    runPlan();
  });
  segObj = seg("#pl-obj", () => runPlan());
  segKvqF = seg("#f-kvq", () => runFit());
  segKvqP = seg("#p-kvq", () => runPerf());
  segKvqPl = seg("#pl-kvq", () => runPlan());
  $("#pl-custom-btn").onclick = () => {
    customOn = true;
    $("#pl-model-wrap").style.display = "none";
    $("#pl-custom").style.display = "";
    CS["pl-qonly"].setDisabled(false);
    $("#pl-quant-note").textContent = tr("Custom models can evaluate every weight quantization; KV-cache precision remains independent.", "自定义模型可枚举所有权重量化；KV cache 精度仍独立选择。");
    $("#pl-quant-note").classList.remove("locked");
    runPlan();
  };
  $("#pl-custom-back").onclick = () => {
    customOn = false;
    $("#pl-model-wrap").style.display = "";
    $("#pl-custom").style.display = "none";
    syncModelQuant("pl-model", "pl-qonly", "pl-quant-note");
    runPlan();
  };
  ["cm-params", "cm-active", "cm-layers", "cm-hidden", "cm-kvh", "cm-dim", "cm-kvlayers", "cm-mla",
    "cm-local-layers", "cm-window", "cm-intermediate", "cm-moe-intermediate", "cm-experts", "cm-topk",
    "cm-shared", "cm-moelayers", "cm-mtpheads", "cm-encoderparams"]
    .forEach(id => { $("#" + id).oninput = () => runPlan(); });
  $("#pl-queue").onchange = () => {
    $("#pl-queue-opts").style.display = $("#pl-queue").checked ? "" : "none";
    runPlan();
  };
  $("#pl-tos").oninput = $("#pl-maxq").oninput = $("#pl-outlen").oninput = () => runPlan();
  $("#pl-q").oninput = $("#pl-maxcards").oninput = () => { planPage = 0; renderPlans(); };
  segPsize = seg("#pl-psize", () => { planPage = 0; renderPlans(); });
}

async function runPlan() {
  const body = {
    model: CS["pl-model"].get(),
    tpm: +$("#pl-tpm").value, tos: +$("#pl-tos").value,
    objective: segObj ? segObj.get() : "cost",
    quant_only: CS["pl-qonly"] ? CS["pl-qonly"].get() : "",
    ctx: +CS["pl-ctx"].get(), conc: +$("#pl-c").value,
    queue: $("#pl-queue").checked,
    maxq: +$("#pl-maxq").value, outlen: +$("#pl-outlen").value,
    eng: CS["pl-eng"].get(), spec: CS["pl-spec"].get(),
    kvq: segKvqPl ? segKvqPl.get() : "fp16", hit: (+$("#pl-hit").value) / 100,
    advanced: advancedOpts("pl"),
    lang,
  };
  if (customOn) body.custom = customModel();
  const plans = await post("/api/plan", body);

  const OBJ = {
    cost: tr("Lowest Cost", "最低成本"),
    latency: tr("Lowest Latency", "最低时延"),
    avail: tr("Highest Availability", "最高可用"),
  };
  let line;
  if (customOn) {
    const m = body.custom;
    line = `<span class="dv-name">${m.name}</span><span class="mono">${m.params}B${m.moe ? tr(` total / ${m.active}B active`, ` 总 / ${m.active}B 激活`) : ""} · ${m.kvt.toUpperCase()}${m.kvlayers > 0 ? tr(` · ${m.kvlayers}/${m.layers} KV layers`, ` · ${m.kvlayers}/${m.layers} 层持 KV`) : ""}</span>`;
  } else {
    const m = MODELS.find(x => x.id === body.model);
    line = `<span class="dv-name">${m.name}${repMark(m.conf)}</span>` +
      `<span class="mono">${m.params}B${m.moe ? tr(` total / ${m.active}B active`, ` 总 / ${m.active}B 激活`) : ""}</span>`;
  }
  const selectedEngine = ENGINES.find(e => e.id === body.eng);
  const selectedSpec = SPECS.find(s => s.id === body.spec);
  const stack = (selectedEngine ? engineName(selectedEngine) : body.eng) +
    (selectedSpec && selectedSpec.id !== "none" ? " · " + specName(selectedSpec) : "") +
    (body.kvq !== "fp16" ? " · KV " + body.kvq.toUpperCase() : "") +
    (body.hit > 0 ? tr(` · ${Math.round(body.hit * 100)}% prefix hit`, ` · 前缀命中 ${Math.round(body.hit * 100)}%`) : "");
  $("#pl-line").innerHTML = line +
    `<span class="mono">${tr("Target", "目标")} <b>${fmt.tpm(body.tpm)}</b> tok/min${body.tos > 0 ? tr(` · single stream ≥${body.tos}`, ` · 单流 ≥${body.tos}`) : ""}</span>` +
    `<span class="mono">${body.ctx >= 1024 ? body.ctx / 1024 + "K" : body.ctx} ${tr("context", "上下文")} · ${body.conc} ${tr("concurrent", "并发")}${body.queue ? tr(" · queue ≤", " · 排队≤") + body.maxq : ""}</span>` +
    `<span class="mono">${stack}</span>` +
    `<span class="mono" style="color:var(--acc)">${OBJ[body.objective] ?? ""}</span>`;

  lastPlans = plans || [];
  planPage = 0;
  renderPlans();
}

// 「怎么配」结果渲染：客户端模糊搜索 + 厂商/类别/总卡数过滤 + 翻页
function renderPlans() {
  if (!lastPlans.length) {
    $("#planList").innerHTML = `<div class="empty">${tr("No configuration meets the target — lower target TPM or minimum single-stream TPS, shorten context, or allow queueing.", "没有方案能达标 —— 试试降低目标 TPM 或单流下限、缩短上下文、允许排队。")}</div>`;
    $("#pl-pager").innerHTML = "";
    return;
  }
  const q = ($("#pl-q").value || "").trim().toLowerCase();
  const v = CS["pl-vendor"] ? CS["pl-vendor"].get() : "";
  const c = CS["pl-cls"] ? CS["pl-cls"].get() : "";
  const maxc = +$("#pl-maxcards").value || 0;
  const list = lastPlans.filter(p =>
    (!v || p.hw.vendor === v) &&
    (!c || p.hw.cls === c) &&
    (!maxc || p.n * p.replicas <= maxc) &&
    (!q || p.hw.name.toLowerCase().includes(q) || p.hw.id.includes(q)));
  if (!list.length) {
    $("#planList").innerHTML = `<div class="empty">${tr("No configurations remain after filtering — relax the model, vendor, class, or total-card filters.", "筛选后没有剩余方案 —— 放宽型号 / 厂商 / 类别 / 总卡数条件。")}</div>`;
    $("#pl-pager").innerHTML = "";
    return;
  }
  const ps = segPsize ? +segPsize.get() : 10;
  const pages = ps > 0 ? Math.ceil(list.length / ps) : 1;
  if (planPage >= pages) planPage = pages - 1;
  const base = ps > 0 ? planPage * ps : 0;
  const shown = ps > 0 ? list.slice(base, base + ps) : list;

  let html = `<div class="planrow planhead">
    <span>#</span><span>${tr("Hardware", "硬件方案")}</span><span>${tr("Parallel Strategy", "并行策略")}</span>
    <span>${tr("Quantization", "量化")}</span><span>${tr("Single-stream TPS", "单流 TPS")}</span><span>${tr("Mixed Cluster TPM", "集群 TPM 混合")}</span><span>${tr("QPS / Load", "QPS / 负载")}</span><span>${tr("Cost", "成本")}</span></div>`;
  shown.forEach((p, i) => {
    const scale = p.replicas > 1 ? tr(`${p.replicas} replicas · ${p.n * p.replicas} cards`, `${p.replicas} 副本 · ${p.n * p.replicas} 卡`) : tr(`${p.n} cards`, `${p.n} 卡`);
    html += `<div class="planrow ${base + i < 3 ? "top3" : ""}">
      <span class="rank">${String(base + i + 1).padStart(2, "0")}</span>
      <span class="phw">${hardwareName(p.hw)}${repMark(p.hw.conf)}${p.n > 1 ? `<span class="x">×${p.n}</span>` : ""}<div class="msub">${scale}</div></span>
      <span class="pstrat">${p.strategy}</span>
      <span><span class="pk">QUANT</span><span class="pv">${p.qname}</span><div class="msub">${p.eng_name || ""}${p.spec_name && p.spec_name !== tr("Off", "关闭") ? " · " + p.spec_name : ""}</div></span>
      <span><span class="pk">SINGLE</span><span class="pv">${fmt.tps(p.single_tps)} tok/s</span></span>
      <span><span class="pk">${tr("MIXED TPM", "TPM 混合")}</span><span class="pv">${fmt.tpm(p.tpm)} tok/min</span></span>
      <span><span class="pk">CAPACITY REQ/S</span><span class="pv">${fmt.rate(p.capacity_qps)}</span>
        <div class="msub">${tr(`Target ${fmt.rate(p.arrival_qps)} · ${p.util_pct.toFixed(1)}% utilization`, `目标 ${fmt.rate(p.arrival_qps)} · 利用率 ${p.util_pct.toFixed(1)}%`)}</div>
        ${p.queue_model === "M/M/c" ? `<div class="msub">${tr("Wait mean/p95", "等待 avg/p95")} ${fmt.ms(p.wait_avg_ms)} / ${fmt.ms(p.wait_p95_ms)}</div>` : ""}</span>
      <span><span class="pk">COST</span><span class="pv">${p.cost_cny > 0 ? fmt.cny(p.cost_cny) + tr(" · monthly ", " · 月 ") + fmt.cny(p.monthly) : tr("Contact", "面议")}</span></span>
      ${p.warn ? `<div class="warn">▸ ${p.warn}</div>` : ""}
    </div>`;
  });
  $("#planList").innerHTML = html;
  $("#pl-pager").innerHTML = pages > 1
    ? `<button class="minibtn" id="pg-prev" ${planPage === 0 ? "disabled" : ""}>‹ ${tr("Previous", "上一页")}</button>
       <span class="mono">${tr(`Page ${planPage + 1} / ${pages} · ${list.length} items`, `第 ${planPage + 1} / ${pages} 页 · 共 ${list.length} 条`)}</span>
       <button class="minibtn" id="pg-next" ${planPage >= pages - 1 ? "disabled" : ""}>${tr("Next", "下一页")} ›</button>`
    : `<span class="mono dim2">${tr(`${list.length} items`, `共 ${list.length} 条`)}</span>`;
  if (pages > 1) {
    $("#pg-prev").onclick = () => { planPage--; renderPlans(); };
    $("#pg-next").onclick = () => { planPage++; renderPlans(); };
  }
}

/* ---------- 硬件库 / 模型库 / 速查 ---------- */

function renderHWTable() {
  const q = $("#hw-q").value.trim().toLowerCase();
  const v = CS["hw-vendor"] ? CS["hw-vendor"].get() : "";
  const list = HW.filter(h =>
    (!v || h.vendor === v) &&
    (!q || h.name.toLowerCase().includes(q) || h.id.includes(q)));
  $("#hwTable").innerHTML =
    `<thead><tr><th>${tr("Hardware", "型号")}</th><th>${tr("Vendor", "厂商")}</th><th>${tr("Class", "类别")}</th><th>${tr("Architecture", "架构")}</th>
     <th class="n">${tr("Memory G", "显存 G")}</th><th class="n">${tr("Bandwidth GB/s", "带宽 GB/s")}</th><th>${tr("Interconnect", "互联")}</th>
     <th>${tr("Accelerated Precisions", "硬件加速精度")}</th><th class="n">${tr("Dense Peak TF/OPS", "dense 峰值 TF/OPS")}</th><th class="n">TDP W</th><th class="n">${tr("Reference Price", "参考价")}</th></tr></thead><tbody>` +
    list.map(h => `<tr>
      <td class="mname">${h.source_url ? `<a href="${h.source_url}" target="_blank" rel="noreferrer">${hardwareName(h)} ↗</a>` : hardwareName(h)}${repMark(h.conf)}${catalogNote(h.notes) ? `<div class="msub">${catalogNote(h.notes)}</div>` : ""}</td>
      <td>${vendorName(h.vendor)}</td>
      <td class="dim">${localized(CLS, h.cls)}</td>
      <td class="dim">${h.arch}</td>
      <td class="n">${h.vram || "—"}</td>
      <td class="n">${h.bw ? h.bw.toLocaleString() : "—"}</td>
      <td class="dim">${localized(LINK, h.link.t)}${h.link.b ? " " + h.link.b : ""}</td>
      <td>${h.prec.map(p => `<span class="tag ${["fp8", "fp4"].includes(p) ? "hot" : ""}">${p.toUpperCase()}</span>`).join("")}</td>
      <td class="n">${h.tf || "—"}${h.tf8 ? `<span class="msub">FP8 ${h.tf8}</span>` : ""}${h.tf_int8 ? `<span class="msub">INT8 ${h.tf_int8}</span>` : ""}${h.tf4 ? `<span class="msub">FP4 ${h.tf4}</span>` : ""}</td>
      <td class="n">${h.tdp || "—"}</td>
      <td class="n">${h.cny ? fmt.cny(h.cny) : "—"}</td>
    </tr>`).join("") + "</tbody>";
}

function renderModelTable() {
  const q = $("#m-q").value.trim().toLowerCase();
  const list = [...MODELS].sort((a, b) => !!fixedQuantID(a) - !!fixedQuantID(b) || a.params - b.params)
    .filter(m => !q || [m.name, m.org, m.model_type, m.architecture, m.license, m.src, m.native_quant, m.official && tr("official", "官方")].some(v => String(v || "").toLowerCase().includes(q)));
  const pageSize = 100;
  const pages = Math.max(1, Math.ceil(list.length / pageSize));
  modelPage = Math.min(modelPage, pages - 1);
  const page = list.slice(modelPage * pageSize, (modelPage + 1) * pageSize);
  $("#modelTable").innerHTML =
    `<thead><tr><th>${tr("Model", "模型")}</th><th>${tr("Organization", "机构")}</th><th class="n">${tr("Released", "发布")}</th>
     <th class="n">${tr("Total Params B", "总参数 B")}</th><th class="n">${tr("Active B", "激活 B")}</th><th>${tr("Architecture", "结构")}</th>
     <th class="n">${tr("Layers", "层数")}</th><th class="n">${tr("Mean KV KB/tok", "KV 平均 KB/tok")}</th><th class="n">${tr("Context", "上下文")}</th><th>${tr("Source / Checkpoint", "来源 / 检查点")}</th></tr></thead><tbody>` +
    page.map(m => {
      const layers = m.kvlayers || m.layers;
      const layerBytes = m.kvt === "mla" ? m.mla * 2 : 2 * m.kvh * m.dim * 2;

      const retained = m.local_layers && m.window
        ? (layers - m.local_layers) * m.ctx + m.local_layers * Math.min(m.ctx, m.window)
        : layers * m.ctx;
      const kv = retained * layerBytes / m.ctx / 1e3;
      const state = m.state_mb ? tr(` + ${m.state_mb.toFixed(0)}MB/request state`, ` + ${m.state_mb.toFixed(0)}MB/请求状态`) : "";
      const swa = m.local_layers && m.window ? `·SWA${(m.window / 1024).toFixed(0)}K` : "";
      const arch = `${m.moe ? `MoE ${m.experts || "?"}×${m.topk || "?"}` : "Dense"} · ${m.kvt.toUpperCase()}${swa}${m.sparse ? "·DSA" : ""}${m.multimodal ? "·MM" : ""}`;
      const source = ({ hf: "Hugging Face", modelscope: "ModelScope" }[m.src] || tr("Curated", "人工收录")) + (m.official ? tr(" · Official", " · 官方") : "");
      const metadata = [m.architecture || m.model_type, m.dtype, m.license].filter(Boolean).join(" · ");
      return `<tr><td class="mname">${m.source_url ? `<a href="${m.source_url}" target="_blank" rel="noreferrer">${m.name} ↗</a>` : m.name}${repMark(m.conf)}${catalogNote(m.notes) ? `<div class="msub">${catalogNote(m.notes)}</div>` : ""}</td>
      <td>${m.org}</td>
      <td class="n">${m.year}</td>
      <td class="n">${m.params}</td>
      <td class="n">${m.active || "—"}</td>
      <td class="dim">${arch}</td>
      <td class="n">${m.layers}</td>
      <td class="n">${kv.toFixed(0)}${state ? `<span class="msub">${state}</span>` : ""}</td>
      <td class="n">${(m.ctx / 1024).toFixed(0)}K</td>
      <td class="dim">${source}<span class="msub">${m.checkpoint_gb ? m.checkpoint_gb.toFixed(1) + " GB" : tr("payload unavailable", "payload 未获取")} · ${(m.native_quant || tr("unspecified", "未标注")).toUpperCase()}</span>${metadata ? `<span class="msub">${metadata}</span>` : ""}</td>
    </tr>`; }).join("") + "</tbody>";
  $("#m-pager").innerHTML =
    `<button class="minibtn" id="m-pg-prev" ${modelPage === 0 ? "disabled" : ""}>‹ ${tr("Previous", "上一页")}</button>
     <span class="mono">${tr(`Page ${modelPage + 1} / ${pages} · ${list.length} items`, `第 ${modelPage + 1} / ${pages} 页 · 共 ${list.length} 条`)}</span>
     <button class="minibtn" id="m-pg-next" ${modelPage >= pages - 1 ? "disabled" : ""}>${tr("Next", "下一页")} ›</button>`;
  $("#m-pg-prev").onclick = () => { modelPage--; renderModelTable(); };
  $("#m-pg-next").onclick = () => { modelPage++; renderModelTable(); };
}

function renderGlossary() {
  const q = ($("#g-q")?.value || "").trim().toLowerCase();
  const allRows = GLOSSARY.map(([abbr, full, cat, desc, effect]) => {
    if (lang === "zh") return [abbr, full, cat, desc, effect];
    const copy = GLOSSARY_EN[abbr] || ["Definition unavailable.", "No calculator-specific behavior."];
    return [abbr, full, GLOSSARY_CATEGORY_EN[cat] || cat, copy[0], copy[1]];
  });
  const rows = allRows.filter(row => !q || row.join(" ").toLowerCase().includes(q));
  $("#g-meta").textContent = tr(`${rows.length} / ${allRows.length} terms · weight quantization, KV cache, and acceleration paths explained separately`, `${rows.length} / ${allRows.length} 个术语 · 权重量化、KV cache 与加速路径分别解释`);
  $("#glossaryGrid").innerHTML = rows.length ? rows.map(([abbr, full, cat, desc, effect]) =>
    `<article class="term-card">
      <div class="term-top"><span class="term-abbr">${abbr}</span><span class="term-cat">${cat}</span></div>
      <div class="term-full">${full}</div>
      <p class="term-desc">${desc}</p>
      <p class="term-effect">${effect}</p>
    </article>`).join("") : `<div class="empty">${tr("No matching terms", "没有匹配术语")}</div>`;
}
function renderQuickTable() {
  fetch("/api/quick").then(r => r.json()).then(rows => {
    rows.sort((a, b) => b.hw.vram - a.hw.vram);
    $("#quickTable").innerHTML =
      `<thead><tr><th>${tr("Hardware", "硬件")}</th><th>${tr("Vendor", "厂商")}</th><th class="n">${tr("Memory G", "显存 G")}</th><th class="n">${tr("Bandwidth GB/s", "带宽 GB/s")}</th>
       <th class="n">${tr("FP16 Limit", "FP16 上限")}</th><th class="n">${tr("INT4 Limit", "INT4 上限")}</th><th>${tr("Positioning", "一句话定位")}</th></tr></thead><tbody>` +
      rows.map(r => `<tr>
        <td class="mname">${hardwareName(r.hw)}${repMark(r.hw.conf)}</td>
        <td>${vendorName(r.hw.vendor)}</td>
        <td class="n">${r.hw.vram}</td>
        <td class="n">${r.hw.bw.toLocaleString()}</td>
        <td class="n">~${Math.floor(r.max_fp16)}B</td>
        <td class="n">~${Math.floor(r.max_int4)}B</td>
        <td class="dim">${positioning(r)}</td>
      </tr>`).join("") + "</tbody>";
  });
}

function positioning(r) {
  const h = r.hw;
  if (h.cls === "supernode") return tr("Rack-scale supernode for very large MoE models", "整柜超节点，超大 MoE 专属");
  if (h.unified) return r.max_int4 >= 400 ? tr("The rare local option for R1 Q4", "本地跑 R1 Q4 的独苗") : tr("Personal local inference", "个人本地推理");
  if (r.max_int4 >= 400) return tr("Very large MoE / 70B+ FP16", "超大 MoE / 70B+ FP16");
  if (r.max_int4 >= 100) return tr("Quantized 70B-class deployment", "70B 级量化部署");
  if (r.max_int4 >= 30) return tr("Single-card 30B sweet spot", "30B 级单卡甜点");
  if (r.max_int4 >= 12) return tr("Quantized 7B–14B models", "7B–14B 量化");
  return tr("Small models / edge", "小模型 / 边缘");
}

boot();
