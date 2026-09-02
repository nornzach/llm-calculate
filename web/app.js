/* 推理计算器 — 前端逻辑（无框架，无构建） */

const $ = (s) => document.querySelector(s);
const $$ = (s) => [...document.querySelectorAll(s)];

let HW = [], MODELS = [], QUANTS = [], ENGINES = [], SPECS = [];
const CS = {}; // 自定义下拉注册表

const VENDOR = {
  nvidia: "NVIDIA", amd: "AMD", intel: "Intel", apple: "Apple",
  huawei: "昇腾", mthreads: "摩尔线程", cambricon: "寒武纪",
  hygon: "海光", groq: "Groq", cerebras: "Cerebras", google: "Google",
  enflame: "燧原", metax: "沐曦", biren: "壁仞", iluvatar: "天数智芯",
  kunlunxin: "昆仑芯", aws: "AWS", sambanova: "SambaNova", qualcomm: "Qualcomm",
};
const CLS = {
  consumer: "消费级", workstation: "工作站", datacenter: "数据中心",
  supernode: "超节点", unified_soc: "统一内存", edge: "边缘", sram_asic: "SRAM 专用",
};
const LINK = {
  none: "—", bridge: "NVLink 桥", nvlink: "NVLink", xgmi: "XGMI", ualoe: "UALoE",
  ethernet: "以太网", hccs: "HCCS", unified: "统一内存", xelink: "Xe Link", ici: "ICI",
  blink: "BLink", mlulink: "MLU-Link", neuronlink: "NeuronLink",
};

const fmt = {
  tps: (v) => v >= 1000 ? (v / 1000).toFixed(1) + "k" : (+v).toFixed(1),
  rate: (v) => v >= 1000 ? (v / 1000).toFixed(1) + "k" : v >= 1 ? (+v).toFixed(1) : (+v).toFixed(3),
  tpm: (v) => v >= 1e6 ? (v / 1e6).toFixed(2) + "M" : v >= 1000 ? (v / 1000).toFixed(1) + "k" : Math.round(v),
  gb: (v) => (+v).toFixed(1) + " G",
  ms: (v) => v >= 1000 ? (v / 1000).toFixed(2) + " s" : (+v).toFixed(0) + " ms",
  cny: (v) => v >= 10000 ? (v / 10000).toFixed(1) + " 万" : Math.round(v).toLocaleString() + " 元",
};

function repMark(conf) {
  if (conf === "reported") return ' <sup class="rep" title="公开报道或推算口径">R</sup>';
  if (conf === "fetched") return ' <sup class="rep" title="HF 自动解析；请查看备注，激活参数可能为启发式估算">F</sup>';
  return "";
}

/* ---------- 自定义下拉组件 ---------- */

function cselect(el, groups, opts = {}) {
  const search = opts.search !== false;
  el.classList.add("cs");
  el.innerHTML = `
    <button type="button" class="cs-btn"><span class="cs-v"></span><span class="cs-arrow">▾</span></button>
    <div class="cs-pop">
      ${search ? '<input class="cs-q" placeholder="搜索…">' : ""}
      <div class="cs-list"></div>
    </div>`;
  const btn = el.querySelector(".cs-btn"), pop = el.querySelector(".cs-pop"),
        list = el.querySelector(".cs-list"), q = el.querySelector(".cs-q"),
        vEl = el.querySelector(".cs-v");

  let value = opts.value ?? null, flat = [];
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
    if (!vis_any()) { if (!empty) list.insertAdjacentHTML("beforeend", '<div class="cs-empty">没有匹配项</div>'); }
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
    ["std", "标准 · W/A 精度分离"],
    ["gguf", "GGUF · llama.cpp 生态"],
    ["mlx", "MLX · Apple 专用"],
    ["exl", "ExLlama · 消费卡"],
  ];
  const groups = FAM.map(([f, label]) => ({
    label,
    items: QUANTS.filter(q => q.fam === f)
      .map(q => ({ v: q.id, n: q.name, m: `W${q.w} A${q.a} · ${q.bytes}B` })),
  }));
  if (withAll) groups.unshift({ label: "", items: [{ v: "", n: "全部档位" }] });
  return groups;
}

const ADV_FIELDS = [
  { k: "tp", n: "TP ranks", v: 0, min: 0, step: 1, topo: true, tip: "0 = 全部卡做 TP" },
  { k: "pp", n: "PP stages", v: 0, min: 0, step: 1, topo: true },
  { k: "ep", n: "EP ranks", v: 0, min: 0, step: 1, topo: true },
  { k: "cp", n: "CP ranks", v: 0, min: 0, step: 1, topo: true },
  { k: "weight_gb", n: "实际权重 GB", v: 0, min: 0, step: .1, tip: "整个副本" },
  { k: "runtime_gb", n: "框架常驻 GB", v: 0, min: 0, step: .1, tip: "每卡实测" },
  { k: "activation_gb", n: "峰值 workspace GB", v: 0, min: 0, step: .1, tip: "每卡实测" },
  { k: "adapter_gb", n: "Adapter GB", v: 0, min: 0, step: .1, tip: "整个副本" },
  { k: "draft_gb", n: "Draft / MTP GB", v: 0, min: 0, step: .1, tip: "整个副本" },
  { k: "mem_util", n: "显存预算比例", v: 0, min: 0, max: 1, step: .01 },
  { k: "bw_util", n: "HBM 利用率", v: 0, min: 0, max: 1, step: .01 },
  { k: "flops_util", n: "算力利用率", v: 0, min: 0, max: 1, step: .01 },
  { k: "link_util", n: "互联利用率", v: 0, min: 0, max: 1, step: .01 },
  { k: "schedule_ms", n: "调度 ms / step", v: 0, min: 0, step: .1 },
  { k: "kv_overhead", n: "KV allocator 系数", v: 1, min: 1, max: 2, step: .01 },
  { k: "kv_offload", n: "KV offload 比例", v: 0, min: 0, max: 1, step: .05 },
  { k: "offload_bw", n: "Offload 有效 GB/s", v: 0, min: 0, step: 1 },
  { k: "prefill_chunk", n: "Prefill chunk", v: 8192, min: 1, step: 128 },
  { k: "media_tokens", n: "媒体 token", v: 0, min: 0, step: 1 },
  { k: "router_skew", n: "Router 最忙/平均", v: 1, min: 1, max: 16, step: .05 },
  { k: "spec_tau", n: "实测接受 token τ", v: 0, min: 0, max: 32, step: .1 },
  { k: "spec_ovh", n: "Draft/verify 开销", v: 0, min: 0, max: 10, step: .01 },
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
    `硬件 <b>${HW.length}</b> 款 · 模型 <b>${MODELS.length}</b> 个 · 数据 2026-09`;
  $("#footMeta").textContent = `v0.3 · ${HW.length} HW / ${MODELS.length} MODELS`;

  // 硬件下拉（按厂商分组，带搜索）
  const hwGroups = () => {
    const g = {};
    HW.filter(h => !h.svc).forEach(h => (g[h.vendor] ??= []).push(h));
    return Object.entries(g).map(([v, list]) => ({
      label: VENDOR[v] ?? v,
      items: list.map(h => ({ v: h.id, n: h.name, m: `${h.vram}G · ${h.bw.toLocaleString()} GB/s` })),
    }));
  };
  // 模型下拉（按机构分组，带搜索）
  const modelGroups = () => {
    const sorted = [...MODELS].sort((a, b) => a.params - b.params);
    const g = {};
    sorted.forEach(m => (g[m.org] ??= []).push(m));
    return Object.entries(g).map(([org, list]) => ({
      label: org,
      items: list.map(m => ({ v: m.id, n: m.name, m: `${m.params}B${m.moe ? " MoE" : ""} · ${m.year}` })),
    }));
  };

  CS["f-hw"] = cselect($("#f-hw"), hwGroups(), { onChange: runFit });
  CS["f-ctx"] = cselect($("#f-ctx"), CTX_OPTS, { search: false, onChange: runFit });
  CS["p-hw"] = cselect($("#p-hw"), hwGroups(), { onChange: runPerf });
  CS["p-model"] = cselect($("#p-model"), modelGroups(), { onChange: runPerf });
  CS["p-quant"] = cselect($("#p-quant"), quantGroups(), { onChange: runPerf });
  CS["p-ctx"] = cselect($("#p-ctx"), CTX_OPTS, { search: false, onChange: runPerf });
  CS["pl-model"] = cselect($("#pl-model"), modelGroups(), { onChange: runPlan });
  CS["pl-ctx"] = cselect($("#pl-ctx"), CTX_OPTS, { search: false, onChange: runPlan });
  CS["pl-qonly"] = cselect($("#pl-qonly"), quantGroups(true), { search: false, onChange: runPlan });
  CS["hw-vendor"] = cselect($("#hw-vendor"), [{ label: "", items: [{ v: "", n: "全部厂商" }, ...Object.entries(VENDOR).map(([v, n]) => ({ v, n }))] }], { search: false, onChange: renderHWTable });
  const planFilter = () => { planPage = 0; renderPlans(); };
  CS["pl-vendor"] = cselect($("#pl-vendor"), [{ label: "", items: [{ v: "", n: "全部厂商" }, ...Object.entries(VENDOR).map(([v, n]) => ({ v, n }))] }], { search: false, onChange: planFilter });
  CS["pl-cls"] = cselect($("#pl-cls"), [{ label: "", items: [{ v: "", n: "全部类别" }, ...Object.entries(CLS).map(([v, n]) => ({ v, n }))] }], { search: false, onChange: planFilter });

  // 推理框架 / 推测解码（三页各一份实例）
  const engGroups = () => [{ label: "", items: ENGINES.map(e => ({ v: e.id, n: e.name, m: e.note })) }];
  const specGroups = () => [{ label: "", items: SPECS.map(s => ({ v: s.id, n: s.name, m: s.tau > 1 ? `τ≈${s.tau}` : "逐 token" })) }];
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
  renderHWTable(); renderModelTable(); renderQuickTable();
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
  $("#f-q").oninput = renderFitRows;
  $("#m-q").oninput = renderModelTable;
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
  };
  const rows = await post("/api/fit", body);
  lastFitRows = rows;
  lastFitCtx = body.ctx;
  const h = HW.find(x => x.id === body.hw);

  $("#f-devline").innerHTML =
    `<span class="dv-name">${h.name}${repMark(h.conf)}${body.n > 1 ? ` <span class="mono" style="color:var(--acc)">× ${body.n}</span>` : ""}</span>` +
    `<span class="mono">显存 <b>${h.vram}G</b></span>` +
    `<span class="mono">带宽 <b>${h.bw} GB/s</b></span>` +
    `<span class="mono">互联 <b>${LINK[h.link.t] ?? h.link.t}${h.link.b ? " " + h.link.b + " GB/s" : ""}</b></span>` +
    (h.notes ? `<span style="color:var(--faint)">${h.notes}</span>` : "");

  renderFitRows();
}

let lastFitRows = [], lastFitCtx = 0;
function renderFitRows() {
  const fq = ($("#f-q")?.value || "").trim().toLowerCase();
  const rows = fq ? lastFitRows.filter(r =>
    r.model.name.toLowerCase().includes(fq) || r.model.org.toLowerCase().includes(fq)) : lastFitRows;
  const quants = QUANTS.filter(q => q.main).map(q => `<th class="n">${q.name}</th>`).join("");
  let html = `<thead><tr><th>模型</th><th class="n">参数</th><th>架构</th>${quants}</tr></thead><tbody>`;
  for (const r of rows) {
    const m = r.model;
    const cells = r.cells.map(c => {
      if (c.fit === 0) return `<td class="n"><span class="cell no">—</span></td>`;
      const cls = c.fit === 2 ? "ok" : "warn";
      const flag = c.accel ? "" : `<span class="flag" title="仅省显存，不硬件加速">no-acc</span>`;
      return `<td class="n"><span class="cell ${cls}" title="单流 tok/s">${fmt.tps(c.tps)}</span>${flag}</td>`;
    }).join("");
    const yarn = lastFitCtx > m.ctx ? `<span class="flag" title="超原生上下文 ${(m.ctx / 1024).toFixed(0)}K，需 YaRN 外推">YaRN</span>` : "";
    html += `<tr>
      <td class="mname">${m.name}${repMark(m.conf)}${yarn}<span class="msub">${m.org}</span></td>
      <td class="n">${m.params}B${m.moe ? `<span class="msub">/${m.active}B</span>` : ""}</td>
      <td class="dim">${m.moe ? "MoE" : "Dense"}${m.kvt === "mla" ? "·MLA" : ""}${m.sparse ? "·DSA" : ""}</td>
      ${cells}</tr>`;
  }
  $("#fitTable").innerHTML = html + "</tbody>";
}

/* ---------- 模式二：能跑多快 ---------- */

async function runPerf() {
  const body = {
    hw: CS["p-hw"].get(), n: +$("#p-n").value, model: CS["p-model"].get(),
    quant: CS["p-quant"].get(), ctx: +CS["p-ctx"].get(), batch: +$("#p-b").value,
    eng: CS["p-eng"].get(), spec: CS["p-spec"].get(), kvq: segKvqP ? segKvqP.get() : "fp16",
    hit: (+$("#p-hit").value) / 100, outlen: +$("#p-outlen").value,
    advanced: advancedOpts("p"),
  };
  const { perf: p, curve } = await post("/api/perf", body);
  const h = HW.find(x => x.id === body.hw);
  const m = MODELS.find(x => x.id === body.model);

  $("#perfHero").innerHTML = [
    ["decode 单流", fmt.tps(p.single_tps), "tok / s · 每用户体感", true],
    ["decode 聚合", fmt.tps(p.agg_tps), `tok / s · ${body.batch} 并发合计`],
    ["prefill 速度", fmt.tps(p.pre_tps), "tok / s · 提示词处理"],
    ["TPM 混合", fmt.tpm(p.tpm_mixed), "tok / min · 输入+输出"],
    ["TTFT", fmt.ms(p.ttft_ms), "首 token 延迟"],
    ["TPOT", fmt.ms(p.tpot_ms), "逐 token 间隔"],
    ["请求时延", fmt.ms(p.req_ms), `TTFT + ${body.outlen} tok 输出`],
    ["请求速率", fmt.rate(p.req_s), "req / s · 当前输入/输出长度"],
    ["decode 瓶颈", ({ compute: "算力", memory: "显存带宽", offload: "KV offload" })[p.bottleneck] || p.bottleneck,
      `memory ${fmt.ms(p.decode_mem_ms)} · compute ${fmt.ms(p.decode_compute_ms)} · comm ${fmt.ms(p.comm_ms)}`],
    ["计算口径", !p.topology_ok ? "拓扑已回退" : p.accuracy === "calibrated" ? "已校准" : "解析估算",
      `${p.topology} · ${p.peak_tf.toFixed(0)} TF ${p.peak_exact ? "厂商逐精度" : "倍率估算"}${p.accuracy === "analytical" ? " · 无统计置信区间" : ""}`],
  ].map(([k, v, u, hot]) =>
    `<div class="stat ${p.fit ? "" : "bad"} ${hot ? "hot" : ""}">
      <div class="k">${k}</div><div class="v">${v}</div><div class="u">${u}</div></div>`).join("");

  const d = p.mem, cap = d.cap;
  const pct = (v) => Math.max(0, v / cap * 100);
  const segs = [
    ["权重", d.weights, "vw"], ["KV cache", d.kv, "vkv"], ["框架", d.fw, "vfw"],
    ["激活", d.act, "vact"], ["Adapter / draft", d.adapter, "vadp"], ["预留", d.sys, "vsys"],
  ];
  const used = segs.reduce((s, x) => s + x[1], 0);
  const colors = { vw: "#b85a2b", vkv: "#ff5a1f", vfw: "#54573f", vact: "#3c4034", vadp: "#7b6942", vsys: "#2b2d26" };
  $("#vramBar").innerHTML =
    `<div class="vbar">` +
    segs.map(([k, v, c]) => `<div class="${c}" style="width:${pct(v)}%" title="${k} ${fmt.gb(v)}"></div>`).join("") +
    `<div class="vfree" style="width:${Math.max(0, 100 - pct(used))}%"></div></div>` +
    `<div class="vlegend">` +
    segs.map(([k, v, c]) => `<span><i style="background:${colors[c]}"></i>${k} <span class="mono">${fmt.gb(v)}</span></span>`).join("") +
    `<span><i style="background:transparent;border:1px solid var(--line2)"></i>空闲 <span class="mono">${fmt.gb(Math.max(0, cap - used))}</span></span>` +
    (d.offloaded_kv > 0 ? `<span>外部 KV <span class="mono">${fmt.gb(d.offloaded_kv)}</span></span>` : "") +
    `</div>`;

  $("#perfFitState").innerHTML = p.fit
    ? `<span style="color:var(--ok)">可部署 · 引擎预算 ${fmt.gb(d.budget)} · 余量 ${(d.head_pct * 100).toFixed(0)}%</span>`
    : `<span style="color:var(--bad)">装不下 · 超引擎预算 ${fmt.gb(used - cap)}</span>`;

  $("#perfTrace").innerHTML =
    `<div class="trow"><span class="tk">部署</span><span class="tv">${h.name} × ${body.n}</span><span class="tn">${m.name} · ${QUANTS.find(q => q.id === body.quant).name}${p.accel ? "" : " · 该档仅省显存不加速"}</span></div>` +
    p.trace.map(t => `<div class="trow"><span class="tk">${t.k}</span><span class="tv">${t.v}</span><span class="tn">${t.n}</span></div>`).join("") +
    `<div class="trow"><span class="tk">最大并发</span><span class="tv">${p.max_batch}</span><span class="tn">当前共享前缀、KV 与激活峰值口径的容量上限</span></div>`;

  drawCurve(curve, body.batch);
}

function drawCurve(curve, curB) {
  const W = 560, H = 170, PL = 44, PB = 26, PT = 24, PR = 12;
  const maxY = Math.max(...curve.map(p => p.agg), 1);
  const n = curve.length;
  const x = (i) => PL + i * (W - PL - PR) / (n - 1);
  const y = (v) => H - PB - (v / maxY) * (H - PB - PT);
  let path = "", dots = "", labels = "";
  curve.forEach((p, i) => {
    path += (i ? "L" : "M") + x(i).toFixed(1) + "," + y(p.agg).toFixed(1);
    const hot = p.b === curB;
    dots += `<circle class="cdot" cx="${x(i)}" cy="${y(p.agg)}" r="${hot ? 3.5 : 2}" ${hot ? "" : 'opacity=".55"'}/>`;
    labels += `<text class="clabel" x="${x(i)}" y="${H - 8}" text-anchor="middle">${p.b}</text>`;
    if (hot || i === n - 1 || i === 0)
      labels += i === n - 1
        ? `<text class="clabelv" x="${x(i) - 6}" y="${y(p.agg) - 7}" text-anchor="end">${fmt.tps(p.agg)}</text>`
        : `<text class="clabelv" x="${x(i)}" y="${y(p.agg) - 7}" text-anchor="middle">${fmt.tps(p.agg)}</text>`;
  });
  $("#curveBox").innerHTML =
    `<svg viewBox="0 0 ${W} ${H}">
      <line class="axis" x1="${PL}" y1="${H - PB}" x2="${W - PR}" y2="${H - PB}"/>
      <line class="axis" x1="${PL}" y1="${PT}" x2="${PL}" y2="${H - PB}"/>
      <text class="clabel" x="${PL - 6}" y="${PT + 6}" text-anchor="end">${fmt.tps(maxY)}</text>
      <text class="clabel" x="${PL - 6}" y="${H - PB + 3}" text-anchor="end">0</text>
      <path class="cline" d="${path}"/>${dots}${labels}
    </svg>`;
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
    name: "自定义 " + (+$("#cm-params").value) + "B" + (moe ? " MoE" : ""),
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
    runPlan();
  };
  $("#pl-custom-back").onclick = () => {
    customOn = false;
    $("#pl-model-wrap").style.display = "";
    $("#pl-custom").style.display = "none";
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
  };
  if (customOn) body.custom = customModel();
  const plans = await post("/api/plan", body);

  const OBJ = { cost: "最低成本", latency: "最低时延", avail: "最高可用" };
  let line;
  if (customOn) {
    const m = body.custom;
    line = `<span class="dv-name">${m.name}</span><span class="mono">${m.params}B${m.moe ? ` 总 / ${m.active}B 激活` : ""} · ${m.kvt.toUpperCase()}${m.kvlayers > 0 ? ` · ${m.kvlayers}/${m.layers} 层持 KV` : ""}</span>`;
  } else {
    const m = MODELS.find(x => x.id === body.model);
    line = `<span class="dv-name">${m.name}${repMark(m.conf)}</span>` +
      `<span class="mono">${m.params}B${m.moe ? ` 总 / ${m.active}B 激活` : ""}</span>`;
  }
  const engName = (ENGINES.find(e => e.id === body.eng) || {}).name || body.eng;
  const specName = (SPECS.find(s => s.id === body.spec) || {}).name || "";
  const stack = engName +
    (specName && specName !== "关闭" ? " · " + specName : "") +
    (body.kvq !== "fp16" ? " · KV " + body.kvq.toUpperCase() : "") +
    (body.hit > 0 ? ` · 前缀命中 ${Math.round(body.hit * 100)}%` : "");
  $("#pl-line").innerHTML = line +
    `<span class="mono">目标 <b>${fmt.tpm(body.tpm)}</b> tok/min${body.tos > 0 ? ` · 单流 ≥${body.tos}` : ""}</span>` +
    `<span class="mono">${body.ctx >= 1024 ? body.ctx / 1024 + "K" : body.ctx} 上下文 · ${body.conc} 并发${body.queue ? " · 排队≤" + body.maxq : ""}</span>` +
    `<span class="mono">${stack}</span>` +
    `<span class="mono" style="color:var(--acc)">${OBJ[body.objective] ?? ""}</span>`;

  lastPlans = plans || [];
  planPage = 0;
  renderPlans();
}

// 「怎么配」结果渲染：客户端模糊搜索 + 厂商/类别/总卡数过滤 + 翻页
function renderPlans() {
  if (!lastPlans.length) {
    $("#planList").innerHTML = `<div class="empty">没有方案能达标 —— 试试降低目标 TPM 或单流下限、缩短上下文、允许排队。</div>`;
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
    $("#planList").innerHTML = `<div class="empty">筛选后没有剩余方案 —— 放宽型号 / 厂商 / 类别 / 总卡数条件。</div>`;
    $("#pl-pager").innerHTML = "";
    return;
  }
  const ps = segPsize ? +segPsize.get() : 10;
  const pages = ps > 0 ? Math.ceil(list.length / ps) : 1;
  if (planPage >= pages) planPage = pages - 1;
  const base = ps > 0 ? planPage * ps : 0;
  const shown = ps > 0 ? list.slice(base, base + ps) : list;

  let html = `<div class="planrow planhead">
    <span>#</span><span>硬件方案</span><span>并行策略</span>
    <span>量化</span><span>单流 TPS</span><span>集群 TPM 混合</span><span>QPS / 负载</span><span>成本</span></div>`;
  shown.forEach((p, i) => {
    const scale = p.replicas > 1 ? `${p.replicas} 副本 · ${p.n * p.replicas} 卡` : `${p.n} 卡`;
    html += `<div class="planrow ${base + i < 3 ? "top3" : ""}">
      <span class="rank">${String(base + i + 1).padStart(2, "0")}</span>
      <span class="phw">${p.hw.name}${repMark(p.hw.conf)}${p.n > 1 ? `<span class="x">×${p.n}</span>` : ""}<div class="msub">${scale}</div></span>
      <span class="pstrat">${p.strategy}</span>
      <span><span class="pk">QUANT</span><span class="pv">${p.qname}</span><div class="msub">${p.eng_name || ""}${p.spec_name && p.spec_name !== "关闭" ? " · " + p.spec_name : ""}</div></span>
      <span><span class="pk">SINGLE</span><span class="pv">${fmt.tps(p.single_tps)} tok/s</span></span>
      <span><span class="pk">TPM 混合</span><span class="pv">${fmt.tpm(p.tpm)} tok/min</span></span>
      <span><span class="pk">CAPACITY REQ/S</span><span class="pv">${fmt.rate(p.capacity_qps)}</span>
        <div class="msub">目标 ${fmt.rate(p.arrival_qps)} · 利用率 ${p.util_pct.toFixed(1)}%</div>
        ${p.queue_model === "M/M/c" ? `<div class="msub">等待 avg/p95 ${fmt.ms(p.wait_avg_ms)} / ${fmt.ms(p.wait_p95_ms)}</div>` : ""}</span>
      <span><span class="pk">COST</span><span class="pv">${p.cost_cny > 0 ? fmt.cny(p.cost_cny) + " · 月 " + fmt.cny(p.monthly) : "面议"}</span></span>
      ${p.warn ? `<div class="warn">▸ ${p.warn}</div>` : ""}
    </div>`;
  });
  $("#planList").innerHTML = html;
  $("#pl-pager").innerHTML = pages > 1
    ? `<button class="minibtn" id="pg-prev" ${planPage === 0 ? "disabled" : ""}>‹ 上一页</button>
       <span class="mono">第 ${planPage + 1} / ${pages} 页 · 共 ${list.length} 条</span>
       <button class="minibtn" id="pg-next" ${planPage >= pages - 1 ? "disabled" : ""}>下一页 ›</button>`
    : `<span class="mono dim2">共 ${list.length} 条</span>`;
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
    `<thead><tr><th>型号</th><th>厂商</th><th>类别</th><th>架构</th>
     <th class="n">显存 G</th><th class="n">带宽 GB/s</th><th>互联</th>
     <th>硬件加速精度</th><th class="n">dense 峰值 TF/OPS</th><th class="n">TDP W</th><th class="n">参考价</th></tr></thead><tbody>` +
    list.map(h => `<tr>
      <td class="mname">${h.source_url ? `<a href="${h.source_url}" target="_blank" rel="noreferrer">${h.name} ↗</a>` : h.name}${repMark(h.conf)}${h.notes ? `<div class="msub">${h.notes}</div>` : ""}</td>
      <td>${VENDOR[h.vendor] ?? h.vendor}</td>
      <td class="dim">${CLS[h.cls] ?? h.cls}</td>
      <td class="dim">${h.arch}</td>
      <td class="n">${h.vram || "—"}</td>
      <td class="n">${h.bw ? h.bw.toLocaleString() : "—"}</td>
      <td class="dim">${LINK[h.link.t] ?? h.link.t}${h.link.b ? " " + h.link.b : ""}</td>
      <td>${h.prec.map(p => `<span class="tag ${["fp8", "fp4"].includes(p) ? "hot" : ""}">${p.toUpperCase()}</span>`).join("")}</td>
      <td class="n">${h.tf || "—"}${h.tf8 ? `<span class="msub">FP8 ${h.tf8}</span>` : ""}${h.tf_int8 ? `<span class="msub">INT8 ${h.tf_int8}</span>` : ""}${h.tf4 ? `<span class="msub">FP4 ${h.tf4}</span>` : ""}</td>
      <td class="n">${h.tdp || "—"}</td>
      <td class="n">${h.cny ? fmt.cny(h.cny) : "—"}</td>
    </tr>`).join("") + "</tbody>";
}

function renderModelTable() {
  const q = $("#m-q").value.trim().toLowerCase();
  const list = [...MODELS].sort((a, b) => a.params - b.params)
    .filter(m => !q || m.name.toLowerCase().includes(q) || m.org.toLowerCase().includes(q));
  $("#modelTable").innerHTML =
    `<thead><tr><th>模型</th><th>机构</th><th class="n">发布</th>
     <th class="n">总参数 B</th><th class="n">激活 B</th><th>结构</th>
     <th class="n">层数</th><th class="n">KV 平均 KB/tok</th><th class="n">上下文</th><th>来源 / 检查点</th></tr></thead><tbody>` +
    list.map(m => {
      const layers = m.kvlayers || m.layers;
      const layerBytes = m.kvt === "mla" ? m.mla * 2 : 2 * m.kvh * m.dim * 2;
      const retained = m.local_layers && m.window
        ? (layers - m.local_layers) * m.ctx + m.local_layers * Math.min(m.ctx, m.window)
        : layers * m.ctx;
      const kv = retained * layerBytes / m.ctx / 1e3;
      const state = m.state_mb ? ` + ${m.state_mb.toFixed(0)}MB/请求状态` : "";
      const swa = m.local_layers && m.window ? `·SWA${(m.window / 1024).toFixed(0)}K` : "";
      const arch = `${m.moe ? `MoE ${m.experts || "?"}×${m.topk || "?"}` : "Dense"} · ${m.kvt.toUpperCase()}${swa}${m.sparse ? "·DSA" : ""}${m.multimodal ? "·MM" : ""}`;
      return `<tr><td class="mname">${m.source_url ? `<a href="${m.source_url}" target="_blank" rel="noreferrer">${m.name} ↗</a>` : m.name}${repMark(m.conf)}${m.notes ? `<div class="msub">${m.notes}</div>` : ""}</td>
      <td>${m.org}</td>
      <td class="n">${m.year}</td>
      <td class="n">${m.params}</td>
      <td class="n">${m.active || "—"}</td>
      <td class="dim">${arch}</td>
      <td class="n">${m.layers}</td>
      <td class="n">${kv.toFixed(0)}${state ? `<span class="msub">${state}</span>` : ""}</td>
      <td class="n">${(m.ctx / 1024).toFixed(0)}K</td>
      <td class="dim">${m.src === "hf" ? "HF 采集" : "人工收录"}${m.checkpoint_gb ? `<span class="msub">${m.checkpoint_gb.toFixed(1)} GB · ${(m.native_quant || "?").toUpperCase()}</span>` : `<span class="msub">payload 未获取</span>`}</td>
    </tr>`; }).join("") + "</tbody>";
}

function renderQuickTable() {
  fetch("/api/quick").then(r => r.json()).then(rows => {
    rows.sort((a, b) => b.hw.vram - a.hw.vram);
    $("#quickTable").innerHTML =
      `<thead><tr><th>硬件</th><th>厂商</th><th class="n">显存 G</th><th class="n">带宽 GB/s</th>
       <th class="n">FP16 上限</th><th class="n">INT4 上限</th><th>一句话定位</th></tr></thead><tbody>` +
      rows.map(r => `<tr>
        <td class="mname">${r.hw.name}${repMark(r.hw.conf)}</td>
        <td>${VENDOR[r.hw.vendor] ?? r.hw.vendor}</td>
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
  if (h.cls === "supernode") return "整柜超节点，超大 MoE 专属";
  if (h.unified) return r.max_int4 >= 400 ? "本地跑 R1 Q4 的独苗" : "个人本地推理";
  if (r.max_int4 >= 400) return "超大 MoE / 70B+ FP16";
  if (r.max_int4 >= 100) return "70B 级量化部署";
  if (r.max_int4 >= 30) return "30B 级单卡甜点";
  if (r.max_int4 >= 12) return "7B–14B 量化";
  return "小模型 / 边缘";
}

boot();
