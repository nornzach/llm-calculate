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
  ["计算路径 · COMPUTATION PIPELINE", "Computation Pipeline"],
  ["参数 → 容量 → roofline → 时延 → 吞吐", "parameters → capacity → roofline → latency → throughput"],
  ["工作负载分布 · WORKLOAD MIX", "Workload Distribution"],
  ["分桶结果 · WORKLOAD BUCKETS", "Workload Buckets"],
  ["请求到达占比 ≠ 并发驻留占比", "request-arrival share ≠ concurrent occupancy share"],
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
  ["我有模型", "I have a model"],
  ["我有硬件", "I have hardware"],
  ["处方方向 · DIRECTION", "Prescription Direction"],
  ["目标取舍 · PRIORITIES", "Objective Priorities"],
  ["最多两项", "up to two"],
  ["最低成本", "Lowest Cost"],
  ["最高 TOS", "Highest TOS"],
  ["最高 TPM", "Highest TPM"],
  ["最高可用", "Highest Availability"],
  ["工作负载模板 · WORKLOAD PRESET", "Workload Preset"],
  ["卡数 · CARDS", "Cards"],
  ["目标吞吐 · TARGET TPM", "Target Throughput"],
  ["单流下限 · MIN TOS", "Minimum Single-stream TPS"],
  ["允许排队 · QUEUE", "Allow Queueing"],
  ["排队后单副本最大并发", "Maximum Queued Concurrency"],
  ["推荐完全来自确定性规则：兼容性、显存、roofline、目标取舍与 Pareto 排序；不调用 AI。处方会给出硬件、量化、框架、并发和分桶建议，并用完整计算账本解释原因。", "Recommendations come from deterministic rules: compatibility, memory, roofline, objective tradeoffs, and Pareto ranking. No AI is called. Each prescription explains hardware, quantization, framework, concurrency, and bucketing with the full calculation ledger."],
  ["筛选方案…", "Filter prescriptions…"],
  ["校准值应用于所有候选；只在同一部署栈、模型和负载口径下填写。", "Calibration values apply to every candidate; enter them only for the same stack, model, and workload."],
  ["地图", "Map"],
  ["成本 × 单流速度", "Cost × Single-stream Speed"],
  ["模型版图", "Model Landscape"],
  ["可部署矩阵", "Deployability Matrix"],
  ["并发", "Concurrency"],
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
let workloadP, workloadPl;
let fitRun = 0, perfRun = 0, planRun = 0, recRun = 0;
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
const LONG_TAIL_WORKLOAD = [
  { context: 102400, output: 512, share: 81.55, prefix_hit: 0 },
  { context: 204800, output: 512, share: 15.29, prefix_hit: 0 },
  { context: 512000, output: 512, share: 3.06, prefix_hit: 0 },
  { context: 1048576, output: 512, share: 0.10, prefix_hit: 0 },
];
const OBJECTIVE_LABEL = { cost: tr("Lowest Cost", "最低成本"), tos: tr("Highest TOS", "最高 TOS"), tpm: tr("Highest TPM", "最高 TPM"), avail: tr("Highest Availability", "最高可用") };
const REC_PRESETS = {
  balanced: [{ context: 8192, output: 512, share: 100, prefix_hit: 0 }],
  short: [{ context: 4096, output: 256, share: 80, prefix_hit: 0 }, { context: 16384, output: 1024, share: 20, prefix_hit: 0 }],
  long: LONG_TAIL_WORKLOAD,
};
let recWorkloads = REC_PRESETS.balanced.map(x => ({ ...x }));
const presetGroups = () => [{ label: "", items: [
  { v: "balanced", n: tr("Balanced 8K/512", "均衡 8K/512"), m: tr("default deployment mix", "默认部署混合") },
  { v: "short", n: tr("Short prompts", "短提示"), m: tr("chat/agent common case", "聊天/智能体常见") },
  { v: "long", n: tr("Long tail", "长尾"), m: tr("100K–1M tail example", "100K–1M 长尾示例") },
]}];

const formatTokens = v => v >= 1048576 ? `${(v / 1048576).toFixed(v % 1048576 ? 1 : 0)}M` :
  v >= 1024 ? `${(v / 1024).toFixed(v % 1024 ? 1 : 0)}K` : `${Math.round(v)}`;

function summarizeWorkload(workload) {
  const total = workload.reduce((sum, b) => sum + b.share, 0);
  const rows = workload.map(b => ({ ...b, share: b.share / total })).sort((a, b) => a.context - b.context);
  const quantile = q => {
    let cumulative = 0;
    for (const row of rows) {
      cumulative += row.share;
      if (cumulative >= q) return row.context;
    }
    return rows.at(-1).context;
  };
  return {
    meanContext: rows.reduce((sum, b) => sum + b.share * b.context, 0),
    meanOutput: rows.reduce((sum, b) => sum + b.share * b.output, 0),
    p99Context: quantile(.99),
    p999Context: quantile(.999),
    maxContext: rows.at(-1).context,
  };
}

function workloadEditor(el, initial, onChange) {
  let rows = initial.map(x => ({ ...x }));
  let timer;
  const valid = () => rows.length > 0 && rows.length <= 8 && rows.every(r =>
    Number.isFinite(r.context) && r.context >= 512 && r.context <= 1048576 &&
    Number.isFinite(r.output) && r.output >= 1 && r.output <= 8192 &&
    Number.isFinite(r.share) && r.share > 0 && r.share <= 100 &&
    Number.isFinite(r.prefix_hit) && r.prefix_hit >= 0 && r.prefix_hit <= 90);
  const updateSummary = () => {
    const total = rows.reduce((sum, r) => sum + (Number.isFinite(r.share) ? r.share : 0), 0);
    const out = el.querySelector(".workload-total");
    out.textContent = `Σ ${total.toFixed(total < 1 ? 2 : 1)}%`;
    out.classList.toggle("invalid", !valid());
  };
  const render = () => {
    el.innerHTML = `<div class="workload-summary"><span>${tr("Arrival distribution; normalized at calculation", "请求到达分布；计算时自动归一化")}</span><strong class="workload-total"></strong></div>` +
      rows.map((r, i) => `<div class="workload-row" data-index="${i}">
        <div class="workload-row-head"><span>${tr("Bucket", "分桶")} ${String(i + 1).padStart(2, "0")}</span><button type="button" class="workload-remove" data-remove="${i}" ${rows.length === 1 ? "disabled" : ""}>${tr("Remove", "删除")}</button></div>
        <div class="workload-grid">
          <label class="workload-field">${tr("Request share (%)", "请求占比 (%)")}<input type="number" data-field="share" value="${r.share}" min="0.01" max="100" step="0.01"></label>
          <label class="workload-field">${tr("Input tokens", "输入 token")}<input type="number" data-field="context" value="${r.context}" min="512" max="1048576" step="512"></label>
          <label class="workload-field">${tr("Output tokens", "输出 token")}<input type="number" data-field="output" value="${r.output}" min="1" max="8192" step="16"></label>
          <label class="workload-field">${tr("Prefix hit (%)", "前缀命中 (%)")}<input type="number" data-field="prefix_hit" value="${r.prefix_hit}" min="0" max="90" step="1"></label>
        </div>
      </div>`).join("") +
      `<div class="workload-actions"><button type="button" data-action="add" ${rows.length >= 8 ? "disabled" : ""}>+ ${tr("Bucket", "分桶")}</button><button type="button" data-action="tail">${tr("Long-tail example", "长尾示例")}</button></div>`;
  };
  el.oninput = e => {
    const input = e.target.closest("input[data-field]");
    if (!input) return;
    rows[+input.closest(".workload-row").dataset.index][input.dataset.field] = +input.value;
    updateSummary();
    notify();
  };
  el.onclick = e => {
    const remove = e.target.closest("[data-remove]");
    const action = e.target.closest("[data-action]")?.dataset.action;
    if (remove && rows.length > 1) rows.splice(+remove.dataset.remove, 1);
    else if (action === "add" && rows.length < 8) {
      const last = rows.at(-1);
      rows.push({ context: last.context, output: last.output, share: 1, prefix_hit: last.prefix_hit });
    } else if (action === "tail") rows = LONG_TAIL_WORKLOAD.map(x => ({ ...x }));
    else return;
    render();
    notify();
  };
  render();
  return {
    get: () => valid() ? rows.map(r => ({
      context: Math.round(r.context), output: Math.round(r.output),
      share: r.share / 100, prefix_hit: r.prefix_hit / 100,
    })) : null,
    set: next => {
      rows = next.map(x => ({ ...x }));
      render();
      notify();
    },
  };
}


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
  workloadP = workloadEditor($("#p-workload"), [{ context: 4096, output: 512, share: 100, prefix_hit: 0 }], runPerf);
  CS["pl-model"] = cselect($("#pl-model"), modelGroups(), { onChange: () => { syncModelQuant("pl-model", "pl-qonly", "pl-quant-note"); runPlan(); } });
  workloadPl = workloadEditor($("#pl-workload"), [{ context: 8192, output: 512, share: 100, prefix_hit: 0 }], runPlan);
  CS["pl-qonly"] = cselect($("#pl-qonly"), quantGroups(true), { search: false, onChange: runPlan });
  CS["rec-model"] = cselect($("#rec-model"), modelGroups(), { onChange: runRecommend });
  CS["rec-hw"] = cselect($("#rec-hw"), hwGroups(), { onChange: runRecommend });
  CS["rec-qonly"] = cselect($("#rec-qonly"), quantGroups(true), { search: false, onChange: runRecommend });
  CS["rec-preset"] = cselect($("#rec-preset"), presetGroups(), { search: false, onChange: () => {
    recWorkloads = REC_PRESETS[CS["rec-preset"].get()].map(x => ({ ...x }));
    workloadP.set?.(recWorkloads); workloadPl.set?.(recWorkloads);
    runPerf(); runPlan(); runRecommend();
  }});
  CS["map-model"] = cselect($("#map-model"), modelGroups(), { onChange: runMap });
  CS["map-preset"] = cselect($("#map-preset"), presetGroups(), { search: false, onChange: runMap });
  CS["hw-vendor"] = cselect($("#hw-vendor"), [{ label: "", items: [{ v: "", n: tr("All vendors", "全部厂商") }, ...Object.keys(VENDOR).map(v => ({ v, n: vendorName(v) }))] }], { search: false, onChange: renderHWTable });
  const planFilter = () => { planPage = 0; renderPlans(); };
  CS["pl-vendor"] = cselect($("#pl-vendor"), [{ label: "", items: [{ v: "", n: tr("All vendors", "全部厂商") }, ...Object.keys(VENDOR).map(v => ({ v, n: vendorName(v) }))] }], { search: false, onChange: planFilter });
  CS["pl-cls"] = cselect($("#pl-cls"), [{ label: "", items: [{ v: "", n: tr("All classes", "全部类别") }, ...Object.keys(CLS).map(v => ({ v, n: localized(CLS, v) }))] }], { search: false, onChange: planFilter });
  CS["rec-eng"] = cselect($("#rec-eng"), [{ label: "", items: ENGINES.map(e => ({ v: e.id, n: engineName(e), m: engineNote(e) })) }], { search: false, onChange: runRecommend });
  CS["rec-spec"] = cselect($("#rec-spec"), [{ label: "", items: SPECS.map(s => ({ v: s.id, n: specName(s), m: specNote(s) })) }], { search: false, onChange: runRecommend });
  const recDir = () => $("#rec-dir button.on")?.dataset.v || "model";
  const syncRecDir = () => {
    const card = recDir() === "card";
    $("#rec-model-wrap").style.display = card ? "none" : "";
    $("#rec-hw-wrap").style.display = card ? "" : "none";
    $("#rec-cards").disabled = !card;
    $("#rec-tpm").disabled = card;
    $("#rec-tos").disabled = card;
    $("#rec-queue").disabled = card;
    $("#rec-maxq").disabled = card;
  };
  $("#rec-dir").addEventListener("click", e => {
    const b = e.target.closest("button[data-v]");
    if (!b) return;
    $$("#rec-dir button").forEach(x => x.classList.toggle("on", x === b));
    syncRecDir();
    runRecommend();
  });
  $("#rec-q").oninput = renderRecommend;
  $("#rec-queue").onchange = () => {
    $("#rec-queue-opts").style.display = $("#rec-queue").checked ? "" : "none";
    runRecommend();
  };
  $("#rec-maxq").oninput = runRecommend;
  $("#rec-obj").addEventListener("change", runRecommend);
  ["#rec-tpm", "#rec-tos", "#rec-cards"].forEach(id => { $(id).oninput = runRecommend; });
  mountAdvanced("rec", false);
  syncRecDir();
  seg("#rec-kvq", runRecommend);


  // 推理框架 / 推测解码（三页各一份实例）
  const engGroups = () => [{ label: "", items: ENGINES.map(e => ({ v: e.id, n: engineName(e), m: engineNote(e) })) }];
  const specGroups = () => [{ label: "", items: SPECS.map(s => ({ v: s.id, n: specName(s), m: specNote(s) })) }];
  CS["f-eng"] = cselect($("#f-eng"), engGroups(), { search: false, onChange: runFit });
  CS["f-spec"] = cselect($("#f-spec"), specGroups(), { search: false, onChange: runFit });
  CS["p-eng"] = cselect($("#p-eng"), engGroups(), { search: false, onChange: runPerf });
  CS["p-spec"] = cselect($("#p-spec"), specGroups(), { search: false, onChange: runPerf });
  CS["pl-eng"] = cselect($("#pl-eng"), engGroups(), { search: false, onChange: runPlan });
  CS["pl-spec"] = cselect($("#pl-spec"), specGroups(), { search: false, onChange: runPlan });

  CS["rec-eng"].set("auto", true); CS["rec-spec"].set("none", true);
  CS["map-model"].set("qwen--qwen3.8-27b", true); CS["map-preset"].set("balanced", true);
  CS["rec-qonly"].set("", true); CS["rec-preset"].set("balanced", true);
  CS["f-hw"].set("rtx4090", true); CS["f-ctx"].set("8192", true);
  CS["p-hw"].set("rtx4090", true); CS["p-model"].set("llama-3.1-70b", true);
  CS["p-quant"].set("q4km", true);
  CS["pl-model"].set("deepseek-r1", true);
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
  runMap(); runFit(); runPerf(); runPlan(); runRecommend();
  renderHWTable(); renderModelTable(); renderQuickTable(); renderGlossary();
}

function syncQueueCap() {
  const input = $("#pl-maxq");
  const concurrency = +$("#pl-c").value;
  input.min = concurrency;
  if (+input.value < concurrency) input.value = concurrency;
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
  wireMapTip();
  bind("#f-n", runFit); bind("#f-b", runFit);
  bind("#p-n", runPerf); bind("#p-b", runPerf);
  bind("#pl-tpm", runPlan);
  bind("#pl-c", () => { syncQueueCap(); runPlan(); });
  $("#map-tpm").oninput = $("#map-conc").oninput = runMap;
  $("#hw-q").oninput = renderHWTable;
  $("#f-q").oninput = () => { fitPage = 0; renderFitRows(); };
  $("#m-q").oninput = () => { modelPage = 0; renderModelTable(); };
  $("#g-q").oninput = renderGlossary;
  $$("#p-advanced input").forEach(el => { el.oninput = runPerf; });
  $$("#pl-advanced input").forEach(el => { el.oninput = runPlan; });
  $$("[data-plan-close]").forEach(el => { el.onclick = closePlanDetail; });
  document.addEventListener("keydown", e => {
    if ($("#planDetailModal").hidden) return;
    if (e.key === "Escape") closePlanDetail();
    if (e.key === "Tab") {
      e.preventDefault();
      $(".plan-modal-close").focus();
    }
  });
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
  const run = ++fitRun;
  const body = {
    hw: CS["f-hw"].get(), n: +$("#f-n").value, ctx: +CS["f-ctx"].get(), batch: +$("#f-b").value,
    eng: CS["f-eng"].get(), spec: CS["f-spec"].get(), kvq: segKvqF ? segKvqF.get() : "fp16",
    lang,
  };
  const rows = await post("/api/fit", body);
  if (run !== fitRun) return;
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
  const run = ++perfRun;
  const workload = workloadP?.get();
  if (!workload) return;
  const body = {
    hw: CS["p-hw"].get(), n: +$("#p-n").value, model: CS["p-model"].get(),
    quant: CS["p-quant"].get(), workload, batch: +$("#p-b").value,
    eng: CS["p-eng"].get(), spec: CS["p-spec"].get(), kvq: segKvqP ? segKvqP.get() : "fp16",
    advanced: advancedOpts("p"), lang,
  };
  const { perf: p, curve } = await post("/api/perf", body);
  if (run !== perfRun) return;
  const h = HW.find(x => x.id === body.hw);
  const m = MODELS.find(x => x.id === body.model);

  const stats = p.workload;
  $("#perfHero").innerHTML = [
    [tr("Decode · single stream", "decode 单流"), fmt.tps(p.single_tps), tr("tok / s · output-token weighted", "tok / s · 按输出 token 加权"), true],
    [tr("Decode · aggregate", "decode 聚合"), fmt.tps(p.agg_tps), tr(`tok / s · ${body.batch} concurrent mixed load`, `tok / s · ${body.batch} 并发混合负载`)],
    [tr("Prefill speed", "prefill 速度"), fmt.tps(p.pre_tps), tr("tok / s · workload weighted", "tok / s · 按工作负载加权")],
    [tr("Mixed TPM", "TPM 混合"), fmt.tpm(p.tpm_mixed), tr("tok / min · raw input + output", "tok / min · 原始输入+输出")],
    [tr("TTFT · mean", "TTFT · 均值"), fmt.ms(p.ttft_ms), tr(`P95 ${fmt.ms(stats.p95_ttft_ms)} · P99 ${fmt.ms(stats.p99_ttft_ms)}`, `P95 ${fmt.ms(stats.p95_ttft_ms)} · P99 ${fmt.ms(stats.p99_ttft_ms)}`)],
    [tr("TPOT · weighted", "TPOT · 加权"), fmt.ms(p.tpot_ms), tr("per-output-token interval", "逐输出 token 间隔")],
    [tr("Request latency · mean", "请求时延 · 均值"), fmt.ms(p.req_ms), `P95 ${fmt.ms(stats.p95_req_ms)} · P99 ${fmt.ms(stats.p99_req_ms)}`],
    [tr("Request capacity", "请求容量"), fmt.rate(p.req_s), tr("req / s · harmonic service demand", "req / s · 调和聚合服务预算")],
    [tr("Decode bottleneck", "decode 瓶颈"), ({ compute: tr("Compute", "算力"), memory: tr("Memory bandwidth", "显存带宽"), offload: "KV offload" })[p.bottleneck] || p.bottleneck,
      `memory ${fmt.ms(p.decode_mem_ms)} · compute ${fmt.ms(p.decode_compute_ms)} · comm ${fmt.ms(p.comm_ms)}`],
    [tr("Estimate basis", "计算口径"), !p.topology_ok ? tr("Topology fallback", "拓扑已回退") : p.accuracy === "calibrated" ? tr("Calibrated", "已校准") : tr("Analytical mix", "解析混合估算"),
      `${p.topology} · ${stats.buckets.length} ${tr("workload buckets", "个工作负载桶")} · ${p.peak_tf.toFixed(0)} TF${p.accuracy === "analytical" ? tr(" · no statistical confidence interval", " · 无统计置信区间") : ""}`],
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
  const effectiveQuant = QUANTS.find(q => q.id === p.quant);
  const bottleneckName = ({ compute: tr("compute", "算力"), memory: tr("memory bandwidth", "显存带宽"), offload: "KV offload" })[p.bottleneck] || p.bottleneck;
  const flow = [
    ["01", tr("Workload · P(L)", "负载 · P(L)"), `μL_in=${formatTokens(stats.mean_context)} · B=${body.batch}`, `P99 ${formatTokens(stats.p99_context)} · P99.9 ${formatTokens(stats.p999_context)} · max ${formatTokens(stats.max_context)}`],
    ["02", tr("Memory · M", "显存 · M"), `μ=${fmt.gb(used)} · P99.9=${fmt.gb(d.p999_total)}`, `${tr("physical", "物理")} ${fmt.gb(cap)} · ${tr("occupancy weighted", "驻留时间加权")}`],
    ["03", tr(`Roofline · ${bottleneckName}`, `Roofline · ${bottleneckName}`), `t_step=${fmt.ms(p.tpot_ms)}`, "max(t_mem, t_compute) + t_comm"],
    ["04", tr("Request latency · T", "请求时延 · T"), `μ=${fmt.ms(p.req_ms)} · P95=${fmt.ms(stats.p95_req_ms)}`, "TTFT + (L_out − 1) × TPOT"],
    ["05", tr("Service capacity · λ", "服务容量 · λ"), `λ=${fmt.rate(p.req_s)} req/s`, `TPM=${fmt.tpm(p.tpm_mixed)}`],
  ];
  $("#perfFlow").innerHTML = flow.map(([i, label, value, note]) =>
    `<div class="method-step"><span class="method-index">${i}</span><span class="method-label">${label}</span><strong class="method-value">${value}</strong><small class="method-note">${note}</small></div>`).join("");

  $("#perfBuckets").innerHTML = `<div class="tblwrap"><table class="tbl workload-table"><thead><tr>
    <th>${tr("Input", "输入")}</th><th>${tr("Output", "输出")}</th><th>${tr("Arrival", "到达占比")}</th><th>${tr("Occupancy", "驻留占比")}</th>
    <th>TTFT</th><th>TPOT</th><th>${tr("Request latency", "请求时延")}</th><th>${tr(`VRAM @ B=${body.batch}`, `显存 @ B=${body.batch}`)}</th>
  </tr></thead><tbody>${stats.buckets.map(b => `<tr>
    <td class="n">${formatTokens(b.context)}</td><td class="n">${formatTokens(b.output)}</td><td class="n">${(b.share * 100).toFixed(2)}%</td>
    <td class="n ${b.occupancy > b.share * 1.05 ? "share-shift" : ""}">${(b.occupancy * 100).toFixed(2)}%</td>
    <td class="n">${fmt.ms(b.ttft_ms)}</td><td class="n">${fmt.ms(b.tpot_ms)}</td><td class="n">${fmt.ms(b.req_ms)}</td>
    <td class="n" style="color:var(--${b.fit ? "ok" : "bad"})">${fmt.gb(b.batch_memory)}</td>
  </tr>`).join("")}</tbody></table></div>`;

  $("#vramBar").innerHTML =
    `<div class="vbar">` +
    segs.map(([k, v, c]) => `<div class="${c}" style="width:${pct(v)}%" title="${k} ${fmt.gb(v)}"></div>`).join("") +
    `<div class="vfree" style="width:${Math.max(0, 100 - pct(used))}%"></div><i class="vguard" style="left:${Math.min(100, pct(d.p999_total))}%" title="P99.9 ${fmt.gb(d.p999_total)}"></i></div>` +
    `<div class="vlegend">` +
    segs.map(([k, v, c]) => `<span><i style="background:${colors[c]}"></i>${k} <span class="mono">${fmt.gb(v)}</span></span>`).join("") +
    `<span><i class="guard"></i>P99.9 <span class="mono">${fmt.gb(d.p999_total)}</span></span>` +
    `<span><i style="background:transparent;border:1px solid var(--line2)"></i>${tr("Mean free", "均值空闲")} <span class="mono">${fmt.gb(Math.max(0, cap - used))}</span></span>` +
    (d.offloaded_kv > 0 ? `<span>${tr("External KV", "外部 KV")} <span class="mono">${fmt.gb(d.offloaded_kv)}</span></span>` : "") +
    `</div>`;

  $("#perfFitState").innerHTML = p.fit
    ? `<span style="color:var(--ok)">${tr(`P99.9 fits · ${fmt.gb(d.p999_total)} / ${fmt.gb(cap)} · ${(d.head_pct * 100).toFixed(0)}% headroom`, `P99.9 可部署 · ${fmt.gb(d.p999_total)} / ${fmt.gb(cap)} · 余量 ${(d.head_pct * 100).toFixed(0)}%`)}</span>`
    : `<span style="color:var(--bad)">${tr(`P99.9 does not fit · ${fmt.gb(d.p999_total - cap)} above physical VRAM`, `P99.9 装不下 · 超物理显存 ${fmt.gb(d.p999_total - cap)}`)}</span>`;

  $("#perfTrace").innerHTML =
    `<div class="trow thead"><span>${tr("Quantity", "量")}</span><span>${tr("Estimate", "估算值")}</span><span>${tr("Relation / assumption", "关系式 / 假设")}</span></div>` +
    `<div class="trow"><span class="tk">${tr("Deployment", "部署")}</span><span class="tv">${hardwareName(h)} × ${body.n}</span><span class="tn">${m.name} · ${effectiveQuant?.name || p.quant}${p.quant_locked ? tr(" · native checkpoint locked", " · 原生 checkpoint 锁定") : ""}${p.accel ? "" : tr(" · memory saving only; no acceleration", " · 该档仅省显存不加速")}${p.kv_supported ? "" : tr(" · selected KV format not applied", " · 所选 KV 格式未应用")}</span></div>` +
    p.trace.map(t => `<div class="trow"><span class="tk">${t.k}</span><span class="tv">${t.v}</span><span class="tn">${t.n}</span></div>`).join("") +
    `<div class="trow"><span class="tk">${tr("Maximum concurrency", "最大并发")}</span><span class="tv">${p.max_batch}</span><span class="tn">${tr("Capacity limit under the workload's occupancy-weighted P99.9 memory guard", "按工作负载驻留占比加权的 P99.9 显存保护上限")}</span></div>`;

  drawCurve(curve, body.batch);
}

function drawCurve(curve, curB) {
  const W = 520, H = 220, PL = 58, PB = 40, PT = 18, PR = 14;
  const n = curve.length;
  const x = i => PL + i * (W - PL - PR) / Math.max(1, n - 1);
  const plotBottom = H - PB;
  const selected = curve.findIndex(p => p.b === curB);
  const labelEvery = Math.max(1, Math.ceil(n / 10));
  const xTicks = curve.map((p, i) => {
    if (i % labelEvery && i !== n - 1 && i !== selected) return "";
    return `<line class="tick" x1="${x(i)}" y1="${plotBottom}" x2="${x(i)}" y2="${plotBottom + 4}"/><text class="clabel" x="${x(i)}" y="${plotBottom + 15}" text-anchor="middle">${p.b}</text>`;
  }).join("");
  const guide = selected < 0 ? "" : `<line class="current-guide" x1="${x(selected)}" y1="${PT}" x2="${x(selected)}" y2="${plotBottom}"/>`;
  const axes = (y, max, format, title) => {
    const yTicks = [0, .25, .5, .75, 1].map(r =>
      `<line class="${r ? "gridline" : "axis"}" x1="${PL}" y1="${y(r)}" x2="${W - PR}" y2="${y(r)}"/>` +
      `<text class="clabel" x="${PL - 8}" y="${y(r) + 3}" text-anchor="end">${format(max * r)}</text>`).join("");
    return `${yTicks}<line class="axis" x1="${PL}" y1="${PT}" x2="${PL}" y2="${plotBottom}"/>${xTicks}` +
      `<text class="axis-title" x="${(PL + W - PR) / 2}" y="${H - 4}" text-anchor="middle">${tr("Concurrency, B", "并发 B")}</text>` +
      `<text class="axis-title" transform="translate(12 ${(PT + plotBottom) / 2}) rotate(-90)" text-anchor="middle">${title}</text>`;
  };

  const maxTPS = Math.max(...curve.map(p => p.agg), 1);
  const yTPS = ratio => plotBottom - ratio * (plotBottom - PT);
  const tpsPath = key => curve.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${yTPS(p[key] / maxTPS).toFixed(1)}`).join("");
  const tpsDots = curve.map((p, i) => {
    const current = p.b === curB ? " current" : "";
    const infeasible = p.fit ? "" : " unfit";
    return `<circle class="cdot${current}${infeasible}" cx="${x(i)}" cy="${yTPS(p.agg / maxTPS)}" r="${current ? 4 : 2.5}"><title>${tr(`B=${p.b}: aggregate ${fmt.tps(p.agg)} tok/s${p.fit ? "" : ", over VRAM"}`, `B=${p.b}：聚合 ${fmt.tps(p.agg)} tok/s${p.fit ? "" : "，超显存"}`)}</title></circle>` +
      `<circle class="cdot secondary${current}${infeasible}" cx="${x(i)}" cy="${yTPS(p.single / maxTPS)}" r="${current ? 3.5 : 2}"><title>${tr(`B=${p.b}: single stream ${fmt.tps(p.single)} tok/s`, `B=${p.b}：单流 ${fmt.tps(p.single)} tok/s`)}</title></circle>`;
  }).join("");

  const maxMem = Math.max(...curve.map(p => p.used), ...curve.map(p => p.mean_used), ...curve.map(p => p.cap), 1);
  const yMem = ratio => plotBottom - ratio * (plotBottom - PT);
  const memPath = key => curve.map((p, i) => `${i ? "L" : "M"}${x(i).toFixed(1)},${yMem(p[key] / maxMem).toFixed(1)}`).join("");
  const memDots = curve.map((p, i) => {
    const current = p.b === curB ? " current" : "";
    return `<circle class="cdot${current}${p.fit ? "" : " unfit"}" cx="${x(i)}" cy="${yMem(p.used / maxMem)}" r="${current ? 4 : 2.5}"><title>${tr(`B=${p.b}: P99.9 ${fmt.gb(p.used)}, mean ${fmt.gb(p.mean_used)} / ${fmt.gb(p.cap)} physical`, `B=${p.b}：P99.9 ${fmt.gb(p.used)}，均值 ${fmt.gb(p.mean_used)} / 物理 ${fmt.gb(p.cap)}`)}</title></circle>`;
  }).join("");
  const capY = yMem(curve[0].cap / maxMem);
  const sweepMeta = tr(`deterministic analytical sweep · n=${n} · selected B=${curB}`, `确定性解析扫描 · n=${n} · 当前 B=${curB}`);

  $("#curveBox").innerHTML = `
    <figure class="chart">
      <h3>${tr("DECODE THROUGHPUT", "DECODE 吞吐")}</h3><p>${sweepMeta}</p>
      <svg role="img" aria-label="${tr("Aggregate and single-stream throughput by concurrency", "聚合与单流吞吐随并发变化")}" viewBox="0 0 ${W} ${H}">
        ${axes(yTPS, maxTPS, fmt.tps, tr("Throughput (tok s⁻¹)", "吞吐 (tok s⁻¹)"))}${guide}
        <path class="cline" d="${tpsPath("agg")}"/><path class="cline secondary" d="${tpsPath("single")}"/>
        ${tpsDots}
      </svg>
      <figcaption class="chart-legend"><span><i></i>${tr("Aggregate", "聚合")}</span><span><i class="secondary"></i>${tr("Single stream", "单流")}</span><span><i class="unfit"></i>${tr("Infeasible", "不可部署")}</span></figcaption>
    </figure>
    <figure class="chart">
      <h3>${tr("VRAM CAPACITY", "显存容量")}</h3><p>${sweepMeta}</p>
      <svg role="img" aria-label="${tr("VRAM use by concurrency", "显存占用随并发变化")}" viewBox="0 0 ${W} ${H}">
        ${axes(yMem, maxMem, v => v.toFixed(v >= 100 ? 0 : 1), tr("VRAM (GB)", "显存 (GB)"))}${guide}
        <path class="cline" d="${memPath("used")}"/><path class="cline secondary" d="${memPath("mean_used")}"/><line class="cline limit" x1="${PL}" y1="${capY}" x2="${W - PR}" y2="${capY}"/>
        ${memDots}
      </svg>
      <figcaption class="chart-legend"><span><i></i>P99.9 ${tr("guard", "保护值")}</span><span><i class="secondary"></i>${tr("Occupancy-weighted mean", "驻留加权均值")}</span><span><i class="limit"></i>${fmt.gb(curve[0].cap)} ${tr("physical limit", "物理上限")}</span></figcaption>
    </figure>`;
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
let lastPlans = [], planPage = 0, lastPlanBody = null, planDetailRun = 0, planDetailTrigger = null;

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
    syncQueueCap();
    runPlan();
  };
  $("#pl-tos").oninput = runPlan;
  $("#pl-maxq").oninput = () => { syncQueueCap(); runPlan(); };
  $("#pl-q").oninput = $("#pl-maxcards").oninput = () => { planPage = 0; renderPlans(); };
  segPsize = seg("#pl-psize", () => { planPage = 0; renderPlans(); });
}

async function runPlan() {
  const run = ++planRun;
  const workload = workloadPl?.get();
  if (!workload) return;
  const body = {
    model: CS["pl-model"].get(),
    tpm: +$("#pl-tpm").value, tos: +$("#pl-tos").value,
    objective: segObj ? segObj.get() : "cost",
    quant_only: CS["pl-qonly"] ? CS["pl-qonly"].get() : "",
    workload, conc: +$("#pl-c").value,
    queue: $("#pl-queue").checked, maxq: +$("#pl-maxq").value,
    eng: CS["pl-eng"].get(), spec: CS["pl-spec"].get(),
    kvq: segKvqPl ? segKvqPl.get() : "fp16",
    advanced: advancedOpts("pl"), lang,
  };
  if (customOn) body.custom = customModel();
  const plans = await post("/api/plan", body);
  if (run !== planRun) return;

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
    (body.kvq !== "fp16" ? " · KV " + body.kvq.toUpperCase() : "");
  const profile = summarizeWorkload(body.workload);
  $("#pl-line").innerHTML = line +
    `<span class="mono">${tr("Target", "目标")} <b>${fmt.tpm(body.tpm)}</b> tok/min${body.tos > 0 ? tr(` · P95 single stream ≥${body.tos}`, ` · P95 单流 ≥${body.tos}`) : ""}</span>` +
    `<span class="mono">${body.workload.length} ${tr("buckets", "个分桶")} · μ ${formatTokens(profile.meanContext)} · P99.9 ${formatTokens(profile.p999Context)} · max ${formatTokens(profile.maxContext)} · ${body.conc} ${tr("concurrent", "并发")}${body.queue ? tr(" · queue ≤", " · 排队≤") + body.maxq : ""}</span>` +
    `<span class="mono">${stack}</span>` +
    `<span class="mono" style="color:var(--acc)">${OBJ[body.objective] ?? ""}</span>`;

  lastPlanBody = body;
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
    html += `<div class="planrow ${base + i < 3 ? "top3" : ""}" data-plan-detail="${i}" tabindex="0" role="button" aria-label="${tr(`Explain ${hardwareName(p.hw)} plan`, `查看 ${hardwareName(p.hw)} 方案详解`)}">
      <span class="rank">${String(base + i + 1).padStart(2, "0")}</span>
      <span class="phw">${hardwareName(p.hw)}${repMark(p.hw.conf)}${p.n > 1 ? `<span class="x">×${p.n}</span>` : ""}<div class="msub">${scale}</div></span>
      <span class="pstrat">${p.strategy}<span class="msub">μ ${formatTokens(p.mean_context)} · P99 ${formatTokens(p.p99_context)} · P99.9 ${formatTokens(p.p999_context)} · max ${formatTokens(p.max_context)}</span></span>
      <span><span class="pk">QUANT</span><span class="pv">${p.qname}</span><div class="msub">${p.eng_name || ""}${p.spec_name && p.spec_name !== tr("Off", "关闭") ? " · " + p.spec_name : ""}</div></span>
      <span><span class="pk">${tr("SINGLE μ / P95", "单流 μ / P95")}</span><span class="pv">${fmt.tps(p.single_tps)} / ${fmt.tps(p.p95_single_tps)}</span><div class="msub">tok/s</div></span>
      <span><span class="pk">${tr("MIXED TPM", "TPM 混合")}</span><span class="pv">${fmt.tpm(p.tpm)} tok/min</span><div class="msub">${tr("P95 request", "P95 请求")} ${fmt.ms(p.p95_req_ms)} · P99 ${fmt.ms(p.p99_req_ms)} · M₉₉.₉ ${fmt.gb(p.p999_memory)}</div></span>
      <span><span class="pk">CAPACITY REQ/S</span><span class="pv">${fmt.rate(p.capacity_qps)}</span>
        <div class="msub">${tr(`Target ${fmt.rate(p.arrival_qps)} · ${p.util_pct.toFixed(1)}% utilization`, `目标 ${fmt.rate(p.arrival_qps)} · 利用率 ${p.util_pct.toFixed(1)}%`)}</div>
        ${p.queue_model === "M/M/c" ? `<div class="msub">${tr("Wait mean/p95", "等待 avg/p95")} ${fmt.ms(p.wait_avg_ms)} / ${fmt.ms(p.wait_p95_ms)}</div>` : ""}</span>
      <span><span class="pk">COST</span><span class="pv">${p.cost_cny > 0 ? fmt.cny(p.cost_cny) + tr(" · monthly ", " · 月 ") + fmt.cny(p.monthly) : tr("Contact", "面议")}</span></span>
      ${p.warn ? `<div class="warn">▸ ${p.warn}</div>` : ""}
    </div>`;
  });
  $("#planList").innerHTML = html;
  $$("#planList [data-plan-detail]").forEach((row, i) => {
    const open = () => openPlanDetail(shown[i], row);
    row.onclick = open;
    row.onkeydown = e => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        open();
      }
    };
  });
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

const escapeHTML = value => String(value ?? "").replace(/[&<>"']/g, c => ({
  "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;",
}[c]));

function detailCard(label, value, sub = "") {
  return `<div class="detail-card"><span class="detail-label">${escapeHTML(label)}</span>` +
    `<span class="detail-value">${escapeHTML(value)}</span>` +
    (sub ? `<div class="detail-sub">${escapeHTML(sub)}</div>` : "") + "</div>";
}

let recResults = [], recBody = null, recPage = 0;

function objectivesSelected() {
  return $$("#rec-obj input:checked").map(x => x.value).slice(0, 2);
}

function recommendationWorkload() {
  return recWorkloads.map(x => ({ context: Math.round(x.context), output: Math.round(x.output), share: x.share / 100, prefix_hit: x.prefix_hit / 100 }));
}

function recommendationLabel(p) {
	const h = p.plan.hw;
	const spec = p.plan.spec_name && p.plan.spec_name !== tr("Off", "关闭") ? ` · ${p.plan.spec_name}` : "";
	return `${h.name}${repMark(h.conf)} · ${p.plan.qname}${spec}`;
}

// aria-label 等属性位置必须用纯文本（repMark 含双引号会截断属性）。
function recommendationText(p) {
	return recommendationLabel(p).replace(/<[^>]*>/g, "").replace(/"/g, "'");
}

function renderRecommendLine(body, result) {
  const dir = body.direction === "card" ? tr("I have hardware", "我有硬件") : tr("I have a model", "我有模型");
  const objectives = (result?.objectives || []).map(o => OBJECTIVE_LABEL[o] || o).join(" + ");
  $("#rec-line").innerHTML =
    `<span class="dv-name">${dir}</span>` +
    `<span class="mono">${tr("Target", "目标")} ${body.direction === "model" ? fmt.tpm(body.tpm) : tr("fixed card", "固定卡数")}</span>` +
    `<span class="mono">${tr("Objectives", "取舍")} ${objectives || tr("lowest cost", "最低成本")}</span>` +
    `<span class="mono">${result?.limit || 0} ${tr("prescriptions", "个处方")}</span>`;
}

function renderRecommend() {
  const list = recResults.filter(p => !$("#rec-q").value || recommendationText(p).toLowerCase().includes($("#rec-q").value.toLowerCase()));
  if (!list.length) {
    $("#recList").innerHTML = `<div class="empty">${tr("No prescription fits the current constraints.", "当前约束下没有可推荐处方。")}</div>`;
    $("#rec-pager").innerHTML = "";
    return;
  }
  const ps = 8;
  const pages = Math.ceil(list.length / ps);
  if (recPage >= pages) recPage = pages - 1;
  const base = recPage * ps;
  const shown = list.slice(base, base + ps);
  let html = `<div class="planrow planhead">
    <span>#</span><span>${tr("Prescription", "处方")}</span><span>${tr("Model / Quant", "模型 / 量化")}</span>
    <span>${tr("Concurrency", "并发")}</span><span>${tr("TPM", "TPM")}</span><span>${tr("Latency", "时延")}</span><span>${tr("Cost", "成本")}</span><span>${tr("Why", "原因")}</span></div>`;
  shown.forEach((p, i) => {
    const plan = p.plan;
    const h = plan.hw;
    const score = (p.score * 100).toFixed(1);
    html += `<div class="planrow rec ${base + i < 3 ? "top3" : ""}" data-rec-detail="${i}" tabindex="0" role="button" aria-label="${tr(`Inspect ${recommendationText(p)}`, `查看 ${recommendationText(p)} 处方详解`)}">
      <span class="rank">${String(base + i + 1).padStart(2, "0")}</span>
      <span class="phw">${hardwareName(h)}${repMark(h.conf)}<div class="msub">${tr("Cards", "卡数")} ${plan.n}${plan.replicas > 1 ? tr(` × ${plan.replicas} replicas`, ` × ${plan.replicas} 副本`) : ""} · ${tr("TP", "TP")} ${plan.n}</div></span>
      <span class="pstrat">${escapeHTML(p.model_name)}<span class="msub">${escapeHTML(plan.qname)}${p.quant_locked ? ` · ${tr("locked", "锁定")}` : ""}</span></span>
      <span><span class="pk">${tr("CONC", "并发")}</span><span class="pv">${plan.max_conc}</span><div class="msub">${tr("engine", "引擎")} ${escapeHTML(plan.eng_name || "")}</div></span>
      <span><span class="pk">TPM</span><span class="pv">${fmt.tpm(plan.tpm)}</span><div class="msub">${tr("per replica", "每副本")} ${fmt.tpm(p.per_replica_tpm)}</div></span>
      <span><span class="pk">P95 ${tr("single", "单流")}</span><span class="pv">${fmt.tps(plan.p95_single_tps)}</span><div class="msub">TTFT ${fmt.ms(plan.ttft_ms)} · P95 ${fmt.ms(plan.p95_req_ms)}</div></span>
      <span><span class="pk">COST</span><span class="pv">${plan.monthly ? fmt.cny(plan.monthly) : tr("Contact", "面议")}</span><div class="msub">${tr("score", "得分")} ${score}</div></span>
      <span><span class="pk">${tr("WHY", "原因")}</span><span class="pv" style="font-family:var(--sans);font-size:12px;line-height:1.5">${escapeHTML(p.reason)}</span></span>
      ${p.advice ? `<div class="warn">▸ ${escapeHTML(p.advice)}</div>` : ""}
    </div>`;
  });
  $("#recList").innerHTML = html;
  $("#rec-pager").innerHTML = pages > 1
    ? `<button class="minibtn" id="rec-prev" ${recPage === 0 ? "disabled" : ""}>‹ ${tr("Previous", "上一页")}</button>
       <span class="mono">${tr(`Page ${recPage + 1} / ${pages} · ${list.length} items`, `第 ${recPage + 1} / ${pages} 页 · 共 ${list.length} 条`)}</span>
       <button class="minibtn" id="rec-next" ${recPage >= pages - 1 ? "disabled" : ""}>${tr("Next", "下一页")} ›</button>`
    : `<span class="mono dim2">${tr(`${list.length} items`, `共 ${list.length} 条`)}</span>`;
  $$("#recList [data-rec-detail]").forEach((row, i) => {
    row.onclick = () => openPlanDetail(shown[i].plan, row, {
      direction: recBody.direction === "card" ? "card" : "model",
      model: recBody.model,
      custom: recBody.custom,
      objectives: recBody.objectives,
      workload: recBody.workload,
      engine: shown[i].engine_id,
      spec: shown[i].spec,
      kvq: shown[i].kvq,
      cards: recBody.cards,
      tpm: recBody.tpm,
      advanced: recBody.advanced,
      rec: shown[i],
    });
    row.onkeydown = e => {
      if (e.key === "Enter" || e.key === " ") {
        e.preventDefault();
        row.onclick();
      }
    };
  });
  if (pages > 1) {
    $("#rec-prev").onclick = () => { recPage--; renderRecommend(); };
    $("#rec-next").onclick = () => { recPage++; renderRecommend(); };
  }
}

async function runRecommend() {
  const run = ++recRun;
  if (!CS["rec-model"] || !CS["rec-hw"]) return;
  const direction = $("#rec-dir button.on")?.dataset.v || "model";
  const body = {
    direction,
    model: CS["rec-model"].get(),
    hw: CS["rec-hw"].get(),
    objectives: objectivesSelected().join(","),
    tpm: +$("#rec-tpm").value, tos: +$("#rec-tos").value,
    quant_only: CS["rec-qonly"].get() || "",
    workload: recommendationWorkload(),
    conc: 1,
    queue: $("#rec-queue").checked, maxq: +$("#rec-maxq").value,
    eng: CS["rec-eng"].get(), spec: CS["rec-spec"].get(), kvq: $("#rec-kvq button.on")?.dataset.v || "fp16",
    advanced: advancedOpts("rec"), lang,
  };
  if (direction === "model" && customOn) body.custom = customModel();
  const data = await post("/api/recommend", body);
  if (run !== recRun) return;
  recBody = body;
  recResults = data?.picks || [];
  recPage = 0;
  renderRecommendLine(body, data);
  renderRecommend();
}



/* ---------- 首页：部署地图 ---------- */

let mapScatterRun = 0, mapHeatRun = 0;
const MAP_HEAT_HW = ["rtx5090", "rtx4090", "rtx-6000-pro", "a100-80", "h100-sxm", "h200", "b200", "mi300x", "apple-m3ultra", "ascend-910b"];
const MAP_CLS_COLOR = { consumer: "var(--weight)", workstation: "var(--active)", datacenter: "var(--acc)", supernode: "var(--adapter)", unified_soc: "var(--runtime)", edge: "var(--faint)" };
const MAP_ORG_PALETTE = ["var(--acc)", "var(--kv)", "var(--weight)", "var(--active)", "var(--adapter)", "var(--ok)", "var(--bad)", "var(--runtime)"];

function mapWorkload() {
  return (REC_PRESETS[CS["map-preset"]?.get()] || REC_PRESETS.balanced).map(x => ({ context: x.context, output: x.output, share: x.share / 100, prefix_hit: (x.prefix_hit || 0) / 100 }));
}
function mapHeatCtx() {
  const w = REC_PRESETS[CS["map-preset"]?.get()] || REC_PRESETS.balanced;
  const mean = w.reduce((s, x) => s + x.context * x.share, 0) / w.reduce((s, x) => s + x.share, 0);
  return [4096, 8192, 16384, 32768, 65536, 131072, 262144, 1048576].reduce((a, b) => Math.abs(b - mean) < Math.abs(a - mean) ? b : a);
}

// 即时悬浮提示：替代原生 title 的 1s 延迟；事件委托一次绑定整个地图视图
function wireMapTip() {
  const tip = document.createElement("div");
  tip.id = "mapTip"; tip.hidden = true;
  document.body.appendChild(tip);
  const host = $("#v-map");
  host.addEventListener("mouseover", e => {
    const t = e.target.closest("[data-tip]");
    if (!t) return;
    tip.textContent = t.dataset.tip;
    tip.hidden = false;
  });
  host.addEventListener("mousemove", e => {
    if (tip.hidden) return;
    const x = Math.min(e.clientX + 14, innerWidth - tip.offsetWidth - 10);
    const y = Math.min(e.clientY + 16, innerHeight - tip.offsetHeight - 10);
    tip.style.transform = `translate(${x}px, ${y}px)`;
  });
  host.addEventListener("mouseout", e => {
    if (e.target.closest("[data-tip]")) tip.hidden = true;
  });
}
function mapConc() { return Math.max(1, Math.min(256, +$("#map-conc").value || 16)); }

function runMap() {
  if (!CS["map-model"]) return;
  renderMapLandscape();
  runMapScatter();
  runMapHeat();
}

// 成本 × 单流速度散点：数据来自 /api/recommend 的确定性处方（picks + pareto）
async function runMapScatter() {
  const run = ++mapScatterRun;
  const body = {
    direction: "model", model: CS["map-model"].get(), objectives: "cost,tos",
    tpm: +$("#map-tpm").value || 6000, tos: 0, quant_only: "",
    workload: mapWorkload(), conc: mapConc(),
    queue: false, maxq: 0, eng: "auto", spec: "none", kvq: "fp16",
    advanced: {}, lang, limit: 30,
  };
  const data = await post("/api/recommend", body);
  if (run !== mapScatterRun) return;
  renderMapScatter(data, body);
}

function renderMapScatter(data, body) {
  const box = $("#mapScatter"), legend = $("#mapScatterLegend");
  const picks = (data?.picks || []).filter(p => p.plan.monthly > 0 && p.plan.p95_single_tps > 0);
  const m = MODELS.find(x => x.id === body.model);
  $("#mapScatterMeta").textContent = `${m ? m.name : body.model} · ${tr("conc", "并发")} ${body.conc} · ${tr("target", "目标")} ${fmt.tpm(body.tpm)} tok/min`;
  if (!picks.length) {
    box.innerHTML = `<div class="empty">${tr("No prescription fits the current constraints.", "当前约束下没有可推荐处方。")}</div>`;
    legend.innerHTML = "";
    return;
  }
  const W = 680, H = 380, PL = 66, PR = 18, PT = 16, PB = 42;
  const logs = picks.map(p => Math.log10(p.plan.monthly));
  const xMin = Math.floor(Math.min(...logs) * 2) / 2, xMax = Math.ceil(Math.max(...logs) * 2) / 2;
  const yMax = Math.max(...picks.map(p => p.plan.p95_single_tps)) * 1.1;
  const X = v => PL + (Math.log10(v) - xMin) / (xMax - xMin || 1) * (W - PL - PR);
  const Y = v => (H - PB) - v / yMax * (H - PB - PT);
  const paretoKey = p => [p.model_id, p.plan.hw.id, p.plan.quant, p.plan.n, p.plan.replicas].join("|");
  const pareto = new Set((data.pareto || []).map(paretoKey));
  const frontier = (data.pareto || []).filter(p => p.plan.monthly > 0 && p.plan.p95_single_tps > 0)
    .sort((a, b) => a.plan.monthly - b.plan.monthly);
  const maxTPM = Math.max(...picks.map(p => p.plan.tpm), 1);

  const xTicks = [];
  for (let k = Math.ceil(xMin); k <= Math.floor(xMax); k++) xTicks.push(Math.pow(10, k));
  const axes =
    [0, .25, .5, .75, 1].map(r =>
      `<line class="${r ? "gridline" : "axis"}" x1="${PL}" y1="${Y(yMax * r)}" x2="${W - PR}" y2="${Y(yMax * r)}"/>` +
      `<text class="clabel" x="${PL - 8}" y="${Y(yMax * r) + 3}" text-anchor="end">${r ? fmt.tps(yMax * r) : "0"}</text>`).join("") +
    xTicks.map(v =>
      `<line class="tick" x1="${X(v)}" y1="${H - PB}" x2="${X(v)}" y2="${H - PB + 4}"/>` +
      `<text class="clabel" x="${X(v)}" y="${H - PB + 15}" text-anchor="middle">${fmt.cny(v)}</text>`).join("") +
    `<line class="axis" x1="${PL}" y1="${H - PB}" x2="${W - PR}" y2="${H - PB}"/>` +
    `<text class="axis-title" x="${(PL + W - PR) / 2}" y="${H - 6}" text-anchor="middle">${tr("Monthly cost (CNY, log)", "月成本（元，对数）")}</text>` +
    `<text class="axis-title" transform="translate(12 ${(PT + H - PB) / 2}) rotate(-90)" text-anchor="middle">P95 TOS · tok/s</text>`;

  const line = frontier.length > 1
    ? `<path class="pline" d="${frontier.map((p, i) => `${i ? "L" : "M"}${X(p.plan.monthly).toFixed(1)},${Y(p.plan.p95_single_tps).toFixed(1)}`).join("")}"/>` : "";
  const dots = picks.map((p, i) => {
    const r = 4.5 + 6 * Math.sqrt(p.plan.tpm / maxTPM);
    const tip = `${hardwareName(p.plan.hw)} ×${p.plan.n}${p.plan.replicas > 1 ? " ×" + p.plan.replicas + "R" : ""} · ${p.plan.qname}\n` +
      `${fmt.cny(p.plan.monthly)}/${tr("mo", "月")} · P95 ${fmt.tps(p.plan.p95_single_tps)} tok/s · ${fmt.tpm(p.plan.tpm)} tok/min`;
    return `<circle class="mdot${pareto.has(paretoKey(p)) ? " pareto" : ""}" data-i="${i}" data-tip="${escapeHTML(tip)}" cx="${X(p.plan.monthly).toFixed(1)}" cy="${Y(p.plan.p95_single_tps).toFixed(1)}" r="${r.toFixed(1)}" fill="${MAP_CLS_COLOR[p.plan.hw.cls] || "var(--faint)"}"></circle>`;
  }).join("");
  box.innerHTML = `<svg viewBox="0 0 ${W} ${H}" role="img" aria-label="${tr("Cost versus speed scatter", "成本速度散点图")}">${axes}${line}${dots}</svg>`;

  const clsSeen = [...new Set(picks.map(p => p.plan.hw.cls))];
  legend.innerHTML =
    clsSeen.map(c => `<span><i style="background:${MAP_CLS_COLOR[c] || "var(--faint)"};height:8px;border-radius:50%"></i>${localized(CLS, c)}</span>`).join("") +
    `<span><i style="background:transparent;border-top:1px dashed var(--text);height:0"></i>Pareto</span>` +
    `<span class="dim2">${tr("dot size = mixed TPM", "点大小 = 混合 TPM")}</span>`;

  $$("#mapScatter .mdot").forEach(el => el.onclick = () => {
    const p = picks[+el.dataset.i];
    openPlanDetail(p.plan, el, {
      direction: "model", model: body.model, objectives: ["cost", "tos"], workload: body.workload,
      engine: p.engine_id, spec: p.spec, kvq: p.kvq, tpm: body.tpm, advanced: {}, rec: p,
    });
  });
}

// 模型版图：纯客户端，激活参数 × 上下文；点击选择模型驱动散点图
function renderMapLandscape() {
  const sel = CS["map-model"]?.get();
  const pts = MODELS.filter(m => m.params > 0 && m.ctx > 0 && (m.conf !== "fetched" || m.params >= 8));
  const counts = {};
  pts.forEach(m => { if (m.conf !== "fetched") counts[m.org] = (counts[m.org] || 0) + 1; });
  const topOrgs = Object.entries(counts).sort((a, b) => b[1] - a[1]).slice(0, MAP_ORG_PALETTE.length).map(x => x[0]);
  const orgColor = org => { const i = topOrgs.indexOf(org); return i < 0 ? "var(--faint)" : MAP_ORG_PALETTE[i]; };
  $("#mapLandscapeMeta").textContent = `${pts.length} ${tr("models · click to select", "个模型 · 点击选择")}`;

  const W = 520, H = 380, PL = 52, PR = 14, PT = 16, PB = 42;
  const act = m => m.active || m.params;
  const xMax = Math.ceil(Math.log10(Math.max(...pts.map(act))) * 10) / 10;
  const xMin = -0.1, yMin = Math.log10(2048), yMax = Math.log10(4e6);
  const X = v => PL + (Math.log10(act(v)) - xMin) / (xMax - xMin) * (W - PL - PR);
  const Y = v => (H - PB) - (Math.log10(v.ctx) - yMin) / (yMax - yMin) * (H - PB - PT);
  const axes =
    [1, 10, 100, 1000].filter(v => Math.log10(v) <= xMax).map(v =>
      `<line class="gridline" x1="${X({ params: v })}" y1="${PT}" x2="${X({ params: v })}" y2="${H - PB}"/>` +
      `<text class="clabel" x="${X({ params: v })}" y="${H - PB + 15}" text-anchor="middle">${v >= 1000 ? "1T" : v + "B"}</text>`).join("") +
    [4096, 32768, 262144, 1048576].map(v =>
      `<line class="gridline" x1="${PL}" y1="${Y({ ctx: v })}" x2="${W - PR}" y2="${Y({ ctx: v })}"/>` +
      `<text class="clabel" x="${PL - 6}" y="${Y({ ctx: v }) + 3}" text-anchor="end">${formatTokens(v)}</text>`).join("") +
    `<line class="axis" x1="${PL}" y1="${PT}" x2="${PL}" y2="${H - PB}"/>` +
    `<line class="axis" x1="${PL}" y1="${H - PB}" x2="${W - PR}" y2="${H - PB}"/>` +
    `<text class="axis-title" x="${(PL + W - PR) / 2}" y="${H - 6}" text-anchor="middle">${tr("Active params (log)", "激活参数（对数）")}</text>` +
    `<text class="axis-title" transform="translate(12 ${(PT + H - PB) / 2}) rotate(-90)" text-anchor="middle">${tr("Context (log)", "上下文（对数）")}</text>`;

  const order = { fetched: 0, reported: 1, official: 2 };
  const sorted = [...pts].sort((a, b) => (order[a.conf] ?? 0) - (order[b.conf] ?? 0) || (a.id === sel ? 1 : 0) - (b.id === sel ? 1 : 0));
  const dots = sorted.map(m => {
    const fetched = m.conf === "fetched";
    const r = fetched ? 1.7 : 3 + 2.4 * Math.log10(m.params);
    const op = m.conf === "official" ? .85 : m.conf === "reported" ? .5 : .13;
    const tip = `${m.name} · ${m.org}\n${m.params}B${m.moe ? ` / ${m.active}B act` : ""} · ${formatTokens(m.ctx)} · ${m.conf}`;
    return `<circle class="mdot${m.id === sel ? " sel" : ""}" data-id="${m.id}" data-tip="${escapeHTML(tip)}" cx="${X(m).toFixed(1)}" cy="${Y(m).toFixed(1)}" r="${r.toFixed(1)}" fill="${orgColor(m.org)}" opacity="${op}"></circle>`;
  }).join("");
  $("#mapLandscape").innerHTML = `<svg viewBox="0 0 ${W} ${H}" role="img" aria-label="${tr("Model landscape", "模型版图")}">${axes}${dots}</svg>`;
  $("#mapLandscapeLegend").innerHTML =
    topOrgs.slice(0, 5).map(o => `<span><i style="background:${orgColor(o)};height:8px;border-radius:50%"></i>${o}</span>`).join("") +
    `<span class="dim2">${tr("size = params · faded = auto-parsed", "大小 = 参数 · 淡色 = 自动解析")}</span>`;
  $$("#mapLandscape .mdot").forEach(el => el.onclick = () => {
    CS["map-model"].set(el.dataset.id, true);
    runMap();
  });
}

// 可部署热图：代表性硬件 × 旗舰模型，数据来自 /api/fit 的逐格真实计算
async function runMapHeat() {
  const run = ++mapHeatRun;
  const cols = MAP_HEAT_HW.map(id => HW.find(h => h.id === id)).filter(Boolean);
  if (!cols.length) return;
  const ctx = mapHeatCtx(), batch = mapConc();
  const resps = await Promise.all(cols.map(h => post("/api/fit", { hw: h.id, n: 1, ctx, batch, eng: "auto", spec: "none", kvq: "fp16", lang })));
  if (run !== mapHeatRun) return;
  renderMapHeat(cols, resps, ctx, batch);
}

// 每个参数档位取最新一个模型：新模型优先，官方来源优先，矩阵自然形成 FP16→4-bit 梯度。
function mapHeatRows() {
  const tiers = [[0, 12], [12, 24], [24, 45], [45, 90], [90, 160], [160, 260], [260, 360], [360, 500], [500, 800], [800, 2600]];
  const cand = MODELS.filter(m => m.params > 0 && m.ctx > 0);
  const picked = [];
  for (const [lo, hi] of tiers) {
    const best = cand.filter(m => m.params > lo && m.params <= hi && !picked.includes(m))
      .sort((a, b) => (b.year - a.year) || ((b.conf === "official") - (a.conf === "official")) || b.params - a.params)[0];
    if (best) picked.push(best);
  }
  return picked.sort((a, b) => b.params - a.params);
}

function renderMapHeat(cols, resps, ctx, batch) {
  const rows = mapHeatRows();
  const mainQ = QUANTS.filter(q => q.main);
  const byModel = resps.map(list => new Map((list || []).map(r => [r.model.id, r])));
  const head = `<thead><tr><th class="mname">${tr("Model", "模型")}</th>${cols.map(h =>
    `<th title="${h.vram}G · ${h.bw.toLocaleString()} GB/s">${hardwareName(h)}</th>`).join("")}</tr></thead>`;
  const body = rows.map(m => {
    const cells = cols.map((h, ci) => {
      const r = byModel[ci].get(m.id);
      let best = null;
      r?.cells.forEach((c, i) => {
        if (!c.applicable || !c.fit) return;
        if (!best || c.fit > best.c.fit || (c.fit === best.c.fit && mainQ[i].bytes > best.q.bytes)) best = { q: mainQ[i], c };
      });
      if (!best) return `<td><span class="hm no">—</span></td>`;
      const tier = best.q.bytes >= 1.9 ? "t16" : best.q.bytes >= 0.9 ? "t8" : "t4";
      const tip = `${m.name} × ${hardwareName(h)}\n${best.q.name} · ${fmt.tps(best.c.tps)} tok/s${best.c.accel ? "" : " · no-acc"}`;
      return `<td><span class="hm ${tier}${best.c.fit === 1 ? " edge" : ""}" data-tip="${escapeHTML(tip)}">${best.q.id.toUpperCase().slice(0, 5)}</span></td>`;
    }).join("");
    return `<tr><td class="mname">${m.name}${repMark(m.conf)}<span class="msub">${m.params}B · ${m.year}</span></td>${cells}</tr>`;
  }).join("");
  $("#mapHeat").innerHTML = head + "<tbody>" + body + "</tbody>";
  $("#mapHeatMeta").textContent = `${tr("Context", "上下文")} ${formatTokens(ctx)} · ${tr("conc", "并发")} ${batch} · KV FP16 · ${tr("single card", "单卡")}`;
  $("#mapHeatLegend").innerHTML =
    `<span><span class="hm t16">FP16</span></span><span><span class="hm t8">FP8</span></span><span><span class="hm t4">4-bit</span></span>` +
    `<span><span class="hm t4 edge">4-bit</span> ${tr("low headroom", "显存贴边")}</span><span><span class="hm no">—</span> ${tr("does not fit", "装不下")}</span>`;
}
function planAdvice(plan, perf, model, quant, body) {
  const items = [];
  const add = (level, title, text) => items.push({ level, title, text });
  const native = fixedQuantID(model);
  const required = quant?.need?.toUpperCase();

  if (!perf.eng_ok) {
    add("bad", tr("Engine and hardware do not match", "引擎与硬件不匹配"),
      tr(`${perf.eng_name} does not list ${hardwareName(plan.hw)} as a native platform. Change the engine or verify the vendor plugin before deployment.`,
        `${perf.eng_name} 未把 ${hardwareName(plan.hw)} 列为原生平台。部署前请更换引擎，或确认厂商插件和对应版本。`));
  }
  if (!perf.topology_ok) {
    add("bad", tr("Parallel topology is invalid", "并行拓扑不可用"),
      tr(`The requested topology (${perf.topology}) cannot be placed on ${plan.n} cards. Change the TP/PP/EP/CP values.`,
        `当前并行拓扑（${perf.topology}）无法放进 ${plan.n} 张卡；需要调整 TP / PP / EP / CP。`));
  }
  if (native && required && !perf.accel) {
    add("bad", tr(`${native.toUpperCase()} checkpoint lacks a matching hardware path`, `${native.toUpperCase()} 检查点缺少对应硬件能力`),
      tr(`This checkpoint is locked to ${native.toUpperCase()}, but the card's accelerated precision list does not include ${required}. Do not assume it will run natively: the runtime may reject it or dequantize/fall back. Choose hardware with ${required}, or convert/re-quantize the checkpoint first.`,
        `这个检查点被锁定为 ${native.toUpperCase()}，但该卡的硬件加速精度不含 ${required}。不要按“可原生运行”采购：运行时可能直接拒绝，也可能反量化/回退。应改选支持 ${required} 的卡，或先转换、重新量化检查点。`));
  } else if (required && !perf.accel) {
    add("warn", tr(`${required} is not accelerated on this card`, `该卡不加速 ${required}`),
      tr(`${quant.name} can reduce estimated weight memory, but this card has no native ${required} compute path. The calculator does not apply the advertised low-precision compute multiplier. Prefer supported hardware if throughput matters.`,
        `${quant.name} 仍可按低位权重估算显存，但该卡没有原生 ${required} 计算路径；计算器不会套用低精度算力倍数。吞吐重要时应换支持该精度的卡。`));
  } else if (required) {
    add("good", tr(`${required} hardware path is available`, `${required} 硬件路径可用`),
      tr(`${hardwareName(plan.hw)} lists ${required} acceleration and the estimator uses that precision's dense peak.`,
        `${hardwareName(plan.hw)} 标注支持 ${required} 加速，估算已使用对应精度的 dense 峰值。`));
  }

  if (body.kvq !== "fp16" && !perf.kv_supported) {
    add("warn", tr("Selected KV-cache precision is unsupported", "所选 KV cache 精度不受支持"),
      tr(`KV ${body.kvq.toUpperCase()} is not supported by this hardware/engine path. The estimator falls back to FP16 KV memory and read traffic; do not expect the selected KV saving.`,
        `该硬件/引擎路径不支持 KV ${body.kvq.toUpperCase()}。估算已回退到 FP16 KV 的显存和读取流量，不要期待所选 KV 精度带来的节省。`));
  }
  if (body.spec !== "none" && !perf.spec_applied) {
    add("warn", tr("Speculative decoding was not applied", "推测解码没有生效"),
      tr(`${perf.spec_name} is incompatible with this model or missing required model metadata. Current throughput excludes its speedup.`,
        `${perf.spec_name} 与该模型不兼容，或缺少模型所需元数据；当前吞吐没有计入它的加速。`));
  }
  if (!perf.fit) {
    add("bad", tr("Tail memory exceeds physical memory", "尾部显存超过物理容量"),
      tr(`P99.9 concurrent memory is ${fmt.gb(perf.mem.p999_total)} versus ${fmt.gb(perf.mem.cap)} physical memory. Reduce concurrency/context, lower KV precision on a supported path, or add cards.`,
        `P99.9 并发显存为 ${fmt.gb(perf.mem.p999_total)}，物理显存只有 ${fmt.gb(perf.mem.cap)}。应降低并发/上下文、在受支持路径下降低 KV 精度，或增加卡数。`));
  } else {
    const head = (perf.mem.cap - perf.mem.p999_total) / perf.mem.cap;
    add(head < .1 ? "warn" : "good", tr(head < .1 ? "Memory headroom is tight" : "Tail memory fits", head < .1 ? "显存余量偏紧" : "尾部显存可装下"),
      tr(`P99.9 uses ${fmt.gb(perf.mem.p999_total)} of ${fmt.gb(perf.mem.cap)} per card, leaving ${(head * 100).toFixed(1)}% physical headroom.`,
        `每卡 P99.9 使用 ${fmt.gb(perf.mem.p999_total)} / ${fmt.gb(perf.mem.cap)}，剩余物理显存 ${(head * 100).toFixed(1)}%。`));
  }
  if (model.ctx && plan.max_context > model.ctx) {
    add("bad", tr("Workload exceeds the model context window", "工作负载超过模型上下文"),
      tr(`The largest request is ${formatTokens(plan.max_context)} tokens, above the model's ${formatTokens(model.ctx)} limit. Shorten it or verify a supported context-extension method.`,
        `最大请求为 ${formatTokens(plan.max_context)} token，超过模型 ${formatTokens(model.ctx)} 的上限。请缩短输入，或确认可用的上下文扩展方案。`));
  }
  if (plan.util_pct >= 80) {
    add("warn", tr("Target load leaves little operating margin", "目标负载缺少运行余量"),
      tr(`The target consumes ${plan.util_pct.toFixed(1)}% of modeled capacity. Production bursts, fragmentation, and tail latency can erase this margin; add a replica or lower the admitted load.`,
        `目标负载占模型容量 ${plan.util_pct.toFixed(1)}%。生产流量突发、显存碎片和尾延迟会吃掉余量；建议增加副本或降低准入负载。`));
  }
  const buckets = perf.workload?.buckets || [];
  const tail = buckets.reduce((a, b) => b.context > (a?.context || 0) ? b : a, null);
  if (tail && tail.occupancy > tail.share * 3 && tail.occupancy > .1) {
    add("warn", tr("Rare long requests dominate residency", "少量长请求占据了大量在途资源"),
      tr(`${(tail.share * 100).toFixed(1)}% of arrivals at ${formatTokens(tail.context)} input tokens become ${(tail.occupancy * 100).toFixed(1)}% of in-flight occupancy because they stay much longer. Isolate or cap this traffic if short-request latency matters.`,
        `${formatTokens(tail.context)} 输入只占到达请求的 ${(tail.share * 100).toFixed(1)}%，但因驻留时间更长，占在途资源 ${(tail.occupancy * 100).toFixed(1)}%。若短请求延迟重要，建议隔离或限制这类流量。`));
  }
  if (plan.replicas === 1) {
    add("warn", tr("One replica is a single failure domain", "单副本存在单点故障"),
      tr("This plan meets capacity with one replica, but a process, host, or card failure removes all service. Use at least two replicas when availability matters.",
        "这套方案用一个副本即可达标，但进程、主机或任一卡故障都会让服务整体不可用。对可用性有要求时至少部署两个副本。"));
  }
  add("info", tr("Benchmark before purchase or SLA commitment", "采购或承诺 SLA 前必须实测"),
    tr(`This is a first-order ${perf.accuracy} roofline, not a benchmark. Re-run the exact checkpoint, engine version, quantization kernel, request distribution, and concurrency; then enter measured calibration values.`,
      `这是 ${perf.accuracy} 一阶 roofline，不是基准测试。请用完全相同的检查点、引擎版本、量化 kernel、请求分布和并发实测，再把测量值填入高级校准。`));
  return items;
}

function renderPlanDetail(plan, perf, override = null) {
  const body = override || lastPlanBody;
  const model = body.custom || MODELS.find(m => m.id === (override?.model ?? body.model)) || {};
  const quant = QUANTS.find(q => q.id === perf.quant) || { name: plan.qname };
  const advice = planAdvice(plan, perf, model, quant, body);
  const hasBad = advice.some(a => a.level === "bad");
  const hasWarn = advice.some(a => a.level === "warn");
  const verdictClass = hasBad ? "bad" : hasWarn ? "warn" : "";
  const verdict = hasBad ? tr("Do not deploy as-is", "不建议直接部署") : hasWarn ? tr("Feasible with checks", "可行，但需先核对") : tr("Feasible", "可行");
  const totalCards = plan.n * plan.replicas;
  const meanTokens = (perf.workload?.mean_context || 0) + (perf.workload?.mean_output || 0);
  const workload = perf.workload?.buckets || [];
  const d = perf.mem;

  const compatibility = [
    detailCard(tr("Checkpoint precision", "检查点精度"), fixedQuantID(model) ? `${fixedQuantID(model).toUpperCase()} · ${tr("locked", "锁定")}` : tr("Base / convertible", "基础权重 / 可转换"), fixedQuantID(model) ? tr("Planner cannot change the stored format", "规划器不能改写预量化格式") : tr("Planner may evaluate another deployment quantization", "规划器可评估其他部署量化")),
    detailCard(tr("Weight compute path", "权重计算路径"), perf.accel ? tr("Hardware accelerated", "硬件加速") : tr("No native acceleration", "无原生加速"), `${quant.name || plan.qname} · ${plan.hw.prec.map(x => x.toUpperCase()).join(" / ")}`),
    detailCard(tr("Inference engine", "推理引擎"), perf.eng_name, perf.eng_ok ? tr("Native hardware path listed", "已列出原生硬件路径") : tr("Plugin/version verification required", "需核对插件和版本")),
    detailCard(tr("KV cache", "KV cache"), body.kvq.toUpperCase(), perf.kv_supported ? tr("Selected precision applied", "所选精度已生效") : tr("Falls back to FP16 accounting", "按 FP16 回退计算")),
    detailCard(tr("Parallel topology", "并行拓扑"), perf.topology, perf.topology_ok ? tr("Fits card count", "与卡数匹配") : tr("Does not fit card count", "与卡数不匹配")),
    detailCard(tr("Speculative decoding", "推测解码"), perf.spec_name, body.spec === "none" ? tr("Disabled", "未启用") : perf.spec_applied ? tr("Speedup applied", "已计入加速") : tr("Not applied", "未生效")),
  ].join("");

  const memoryRows = [
    [tr("Weights", "模型权重"), d.weights, tr("Quantized weights after TP/PP/EP sharding", "量化后按 TP / PP / EP 分片")],
    ["KV cache", d.kv, tr("Occupancy-weighted requests, selected or fallback KV precision", "按请求驻留占比加权；使用所选或回退 KV 精度")],
    [tr("Runtime", "运行时"), d.fw, tr("Engine/framework resident memory", "引擎/框架常驻显存")],
    [tr("Activations", "激活值"), d.act, tr("Prefill/decode working tensors", "prefill / decode 工作张量")],
    [tr("Adapter / draft", "适配器 / 草稿模型"), d.adapter, tr("LoRA, speculative model, and related additions", "LoRA、推测解码草稿模型等附加占用")],
    [tr("System reserve", "系统预留"), d.sys, tr("Physical memory not assigned to the engine budget", "物理显存中未分配给引擎预算的部分")],
  ];
  const memoryHTML = memoryRows.map(([name, value, note]) =>
    `<tr><td>${escapeHTML(name)}</td><td class="n">${escapeHTML(fmt.gb(value))}</td><td>${escapeHTML(note)}</td></tr>`).join("");
  const workloadHTML = workload.map((b, i) =>
    `<tr><td class="n">${i + 1}</td><td class="n">${escapeHTML(formatTokens(b.context))}</td><td class="n">${escapeHTML(formatTokens(b.output))}</td>` +
    `<td class="n">${(b.share * 100).toFixed(2)}%</td><td class="n">${(b.occupancy * 100).toFixed(2)}%</td>` +
    `<td class="n">${escapeHTML(fmt.tps(b.single_tps))}</td><td class="n">${escapeHTML(fmt.ms(b.ttft_ms))}</td>` +
    `<td class="n">${escapeHTML(fmt.ms(b.req_ms))}</td><td class="n">${escapeHTML(fmt.gb(b.batch_memory))}</td></tr>`).join("");
  const traceHTML = (perf.trace || []).map(row =>
    `<tr><td>${escapeHTML(row.k)}</td><td class="formula">${escapeHTML(row.v)}</td><td>${escapeHTML(row.n)}</td></tr>`).join("");

  $("#planDetailBody").innerHTML = `
    ${override?.rec ? `<section class="detail-section"><h3>${tr("Prescription rationale", "处方理由")}</h3>
      <div class="detail-grid">
        ${detailCard(tr("Recommendation score", "推荐得分"), (override.rec.score * 100).toFixed(1), override.rec.objective_key || tr("pareto", "多目标"))}
        ${detailCard(tr("Objective wins", "目标胜场"), String(override.rec.objective_wins ?? 0), tr("within candidate set", "候选集内"))}
        ${detailCard(tr("Cards", "卡数"), String(plan.n), plan.replicas > 1 ? tr(`${plan.replicas} replicas`, `${plan.replicas} 副本`) : tr("single replica", "单副本"))}
        ${detailCard(tr("Max concurrency", "最大并发"), String(plan.max_conc), tr("derived from memory and latency budget", "由显存与时延预算推导"))}
      </div>
      <div class="detail-callout"><strong>${tr("Reason", "原因")}：</strong>${escapeHTML(override.rec.reason)}</div>
      ${override.rec.advice ? `<div class="detail-callout"><strong>${tr("Advice", "建议")}：</strong>${escapeHTML(override.rec.advice)}</div>` : ""}
    </section>` : ""}
    <div class="detail-hero">
      <div class="detail-verdict ${verdictClass}"><span class="detail-label">${tr("Beginner verdict", "小白结论")}</span><span class="detail-value">${verdict}</span><div class="detail-sub">${escapeHTML(model.name || body.model)} · ${escapeHTML(plan.qname)}</div></div>
      <div><span class="detail-label">${tr("Deployment shape", "部署规模")}</span><span class="detail-value">${plan.n} × ${plan.replicas} = ${totalCards}</span><div class="detail-sub">${tr("cards/replica × replicas = total cards", "每副本卡数 × 副本数 = 总卡数")}</div></div>
      <div><span class="detail-label">${tr("Single stream μ / P95", "单流 μ / P95")}</span><span class="detail-value">${fmt.tps(plan.single_tps)} / ${fmt.tps(plan.p95_single_tps)}</span><div class="detail-sub">output tok/s · TPOT ${fmt.ms(plan.tpot_ms)}</div></div>
      <div><span class="detail-label">${tr("Cluster mixed capacity", "集群混合容量")}</span><span class="detail-value">${fmt.tpm(plan.tpm)}</span><div class="detail-sub">input + output tok/min · ${tr("target", "目标")} ${fmt.tpm(override?.tpm ?? body.tpm)}</div></div>
    </div>

    <section class="detail-section"><h3>${tr("What you should do", "你应该怎么做")}</h3>
      <div class="advice-list">${advice.map(a => `<div class="advice ${a.level}"><strong>${escapeHTML(a.title)}</strong>${escapeHTML(a.text)}</div>`).join("")}</div>
    </section>

    <section class="detail-section"><h3>${tr("How the planner reached this result", "规划器如何得到这套方案")}</h3>
      <p class="detail-section-intro">${tr("The decision runs in this order. A later step cannot repair a failure in an earlier one.", "按以下顺序判断；前一步不成立，后面的高吞吐数字也不能让方案变得可部署。")}</p>
      <div class="detail-grid">
        ${detailCard("1 · " + tr("Compatibility", "兼容性"), tr("Checkpoint → engine → hardware", "检查点 → 引擎 → 硬件"), tr("Check locked quantization, compute precision, KV path, and topology first", "先检查锁定量化、计算精度、KV 路径和并行拓扑"))}
        ${detailCard("2 · " + tr("Memory", "显存"), "weights + KV + runtime + activation", tr("Both mean occupancy and P99.9 concurrent tail must fit", "平均驻留与 P99.9 并发尾部都要检查"))}
        ${detailCard("3 · " + tr("Performance", "性能"), "max(memory time, compute time) + overhead", tr("Prefill and decode are calculated separately, then combined by the workload", "prefill 与 decode 分开估算，再按工作负载合并"))}
        ${detailCard("4 · " + tr("Sizing", "容量规划"), "target demand ÷ replica capacity", tr("Round up replicas; queue estimates use M/M/c only when enabled", "副本数向上取整；启用排队时才使用 M/M/c"))}
      </div>
    </section>

    <section class="detail-section"><h3>${tr("Compatibility checklist", "兼容性清单")}</h3><div class="detail-grid">${compatibility}</div></section>

    <section class="detail-section"><h3>${tr("Memory derivation · per card", "显存推导 · 每卡")}</h3>
      <div class="tblwrap"><table class="detail-table"><thead><tr><th>${tr("Component", "组成")}</th><th class="n">${tr("Memory", "显存")}</th><th>${tr("Meaning", "含义")}</th></tr></thead><tbody>${memoryHTML}
        <tr><td><strong>${tr("Mean occupied total", "平均驻留总量")}</strong></td><td class="n"><strong>${fmt.gb(d.total)}</strong></td><td>${tr("Weighted by how long each request remains in flight", "按每类请求的在途驻留时间加权")}</td></tr>
        <tr><td><strong>P99.9 ${tr("concurrent total", "并发总量")}</strong></td><td class="n"><strong>${fmt.gb(d.p999_total)}</strong></td><td>${tr("Tail guard used for the plan's fit decision", "方案判断能否部署时使用的尾部保护值")}</td></tr>
        <tr><td>${tr("Engine budget / physical", "引擎预算 / 物理显存")}</td><td class="n">${fmt.gb(d.budget)} / ${fmt.gb(d.cap)}</td><td>${tr("The P99.9 guard must not exceed physical memory", "P99.9 保护值不能超过物理显存")}</td></tr>
      </tbody></table></div>
    </section>

    <section class="detail-section"><h3>${tr("Throughput and latency · one replica", "吞吐与时延 · 单副本")}</h3>
      <div class="detail-grid">
        ${detailCard(tr("Prefill input rate", "Prefill 输入速度"), `${fmt.tpm(perf.pre_tps * 60)} tok/min`, tr("Input-stage speed; not the same metric as mixed TPM", "输入阶段速度；与混合 TPM 不是同一个指标"))}
        ${detailCard(tr("Decode output rate", "Decode 输出速度"), `${fmt.tpm(perf.tpm)} tok/min`, `${fmt.tps(perf.agg_tps)} output tok/s`)}
        ${detailCard(tr("Mixed service rate", "混合处理速度"), `${fmt.tpm(perf.tpm_mixed)} tok/min`, tr("Completed input + output tokens under this workload", "该工作负载下完成的输入 + 输出 token"))}
        ${detailCard(tr("Request throughput", "请求吞吐"), `${fmt.rate(perf.req_s)} req/s`, `${plan.max_conc} ${tr("concurrent requests per replica", "并发请求/副本")}`)}
        ${detailCard("TTFT", fmt.ms(perf.ttft_ms), `${tr("P95", "P95")} ${fmt.ms(perf.workload?.p95_ttft_ms || 0)} · ${tr("first token", "首 token")}`)}
        ${detailCard(tr("Request latency", "请求时延"), fmt.ms(perf.req_ms), `P95 ${fmt.ms(perf.workload?.p95_req_ms || 0)} · P99 ${fmt.ms(perf.workload?.p99_req_ms || 0)}`)}
      </div>
      <div class="detail-callout">${escapeHTML(tr(
        `Metric warning: “2M TPM” is ambiguous. Prefill/input TPM, decode/output TPM, and mixed input+output TPM are different counters. This plan ranks by mixed TPM: ${fmt.tpm(perf.tpm_mixed)} per replica × ${plan.replicas} replicas = ${fmt.tpm(plan.tpm)} cluster capacity.`,
        `指标提醒：“200 万 TPM”并不唯一。Prefill/输入 TPM、Decode/输出 TPM、输入+输出混合 TPM 是三种不同口径。本方案按混合 TPM 排名：每副本 ${fmt.tpm(perf.tpm_mixed)} × ${plan.replicas} 副本 = 集群容量 ${fmt.tpm(plan.tpm)}。`
      ))}</div>
    </section>

    <section class="detail-section"><h3>${tr("Target sizing and queueing", "目标容量与排队")}</h3>
      <div class="tblwrap"><table class="detail-table"><tbody>
        <tr><td>${tr("Mean tokens/request", "平均 token/请求")}</td><td class="formula">${formatTokens(perf.workload?.mean_context || 0)} input + ${formatTokens(perf.workload?.mean_output || 0)} output = ${formatTokens(meanTokens)}</td><td>${tr("Arrival-share weighted", "按请求到达占比加权")}</td></tr>
        <tr><td>${tr("Target arrival rate", "目标到达率")}</td><td class="formula">${fmt.tpm(body.tpm)} ÷ ${formatTokens(meanTokens)} ÷ 60 = ${fmt.rate(plan.arrival_qps)} req/s</td><td>${tr("Converts target mixed TPM into requests", "把目标混合 TPM 换算为请求数")}</td></tr>
        <tr><td>${tr("Cluster service capacity", "集群服务能力")}</td><td class="formula">${fmt.rate(perf.req_s)} × ${plan.replicas} = ${fmt.rate(plan.capacity_qps)} req/s</td><td>${tr("Per-replica capacity × replicas", "每副本能力 × 副本数")}</td></tr>
        <tr><td>${tr("Modeled utilization", "模型利用率")}</td><td class="formula">${fmt.rate(plan.arrival_qps)} ÷ ${fmt.rate(plan.capacity_qps)} = ${plan.util_pct.toFixed(1)}%</td><td>${tr("Lower leaves more burst and latency margin", "越低越能承受突发并保留时延余量")}</td></tr>
        <tr><td>${tr("Queue model", "排队模型")}</td><td class="formula">${escapeHTML(plan.queue_model)} · avg ${fmt.ms(plan.wait_avg_ms)} · P95 ${fmt.ms(plan.wait_p95_ms)}</td><td>${tr("M/M/c is a simplified estimate, not a production tail guarantee", "M/M/c 只是简化估算，不是生产尾延迟保证")}</td></tr>
      </tbody></table></div>
    </section>

    <section class="detail-section"><h3>${tr("Workload buckets", "工作负载分桶")}</h3>
      <p class="detail-section-intro">${tr("Arrival share is how often requests appear. Occupancy share is how much in-flight time/resource they occupy; long requests can be rare but dominate occupancy.", "到达占比表示请求出现频率；驻留占比表示它占用在途时间/资源的比例。长请求即使很少，也可能主导驻留。")}</p>
      <div class="tblwrap"><table class="detail-table"><thead><tr><th class="n">#</th><th class="n">Input</th><th class="n">Output</th><th class="n">${tr("Arrival", "到达")}</th><th class="n">${tr("Occupancy", "驻留")}</th><th class="n">${tr("Single TPS", "单流 TPS")}</th><th class="n">TTFT</th><th class="n">${tr("Request", "请求时延")}</th><th class="n">${tr("Batch memory", "批次显存")}</th></tr></thead><tbody>${workloadHTML}</tbody></table></div>
    </section>

    <section class="detail-section"><h3>${tr("Calculation ledger", "完整计算账本")}</h3>
      <p class="detail-section-intro">${tr("These are the actual intermediate values produced by the same calculator path used by the planner.", "以下是规划器同一计算路径实际产出的中间值，不是另写的一套说明。")}</p>
      <div class="tblwrap"><table class="detail-table"><thead><tr><th>${tr("Step", "步骤")}</th><th>${tr("Value / formula", "数值 / 公式")}</th><th>${tr("Explanation", "说明")}</th></tr></thead><tbody>${traceHTML}</tbody></table></div>
      ${plan.warn ? `<div class="detail-callout"><strong>${tr("Calculator warning", "计算器原始警告")}：</strong> ${escapeHTML(plan.warn)}</div>` : ""}
      <div class="detail-callout">${tr("Not modeled: continuous-batching trajectories, kernel fusion, hierarchical network contention, cache eviction, and production tail-latency distributions.", "未建模：continuous batching 轨迹、kernel fusion、分层网络拥塞、cache 驱逐和生产尾延迟分布。")}</div>
    </section>`;
}


async function openPlanDetail(plan, trigger, override = null) {
  if (!override && !lastPlanBody) return;
  const run = ++planDetailRun;
  planDetailTrigger = trigger;
  const modal = $("#planDetailModal");
  $("#planDetailTitle").textContent = tr(`${hardwareName(plan.hw)} plan explained`, `${hardwareName(plan.hw)} 方案详解`);
  $("#planDetailBody").innerHTML = `<div class="detail-loading">${tr("Calculating the exact per-replica trace…", "正在计算该方案的逐项明细…")}</div>`;
  modal.hidden = false;
  document.body.classList.add("modal-open");
  $(".plan-modal-close").focus();
  const body = override || lastPlanBody;
  try {
    const data = await post("/api/perf", {
      hw: plan.hw.id, n: plan.n, model: override?.model ?? body.model, custom: override?.custom ?? body.custom,
      quant: plan.quant, workload: override?.workload ?? body.workload, batch: plan.max_conc,
      eng: override?.engine ?? body.eng, spec: override?.spec ?? body.spec, kvq: override?.kvq ?? body.kvq, advanced: override?.advanced ?? body.advanced, lang,
    });
    if (run !== planDetailRun) return;
    if (!data?.perf) throw new Error(data?.error || tr("No detail returned", "接口未返回明细"));
    renderPlanDetail(plan, data.perf, override);
  } catch (err) {
    if (run === planDetailRun) {
      $("#planDetailBody").innerHTML = `<div class="empty">${escapeHTML(tr(`Unable to load detail: ${err.message}`, `无法加载明细：${err.message}`))}</div>`;
    }
  }
}

function closePlanDetail() {
  const modal = $("#planDetailModal");
  if (modal.hidden) return;
  ++planDetailRun;
  modal.hidden = true;
  document.body.classList.remove("modal-open");
  if (planDetailTrigger?.isConnected) planDetailTrigger.focus();
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
