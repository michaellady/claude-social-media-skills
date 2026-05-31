/* EVC single-pane dashboard — pure read/render over the flywheel snapshot +
   per-platform drill-downs. All judgment lives upstream; this just visualizes. */

const COLORS = {
  green: "#3fb950", yellow: "#d29922", red: "#f85149", grey: "#6e7681",
  accent: "#58a6ff", muted: "#8b949e", border: "#2a313c",
  series: ["#58a6ff", "#3fb950", "#d29922", "#bc8cff", "#f778ba", "#56d4dd", "#f85149"],
};
const state = { snapshot: null, charts: {}, ytVideos: [], ytFilter: "all",
  learning: [], learnLedger: "all", learnVerdict: "all" };

Chart.defaults.color = COLORS.muted;
Chart.defaults.borderColor = COLORS.border;
Chart.defaults.font.family = "-apple-system, BlinkMacSystemFont, Segoe UI, sans-serif";

const $ = (id) => document.getElementById(id);
const fmt = (n) => (n == null || isNaN(n)) ? "—" : Number(n).toLocaleString();
const fmt1 = (n) => (n == null || isNaN(n)) ? "—" : Number(n).toFixed(1);
const trunc = (s, n) => !s ? "" : (s.length > n ? s.slice(0, n - 1) + "…" : s);
function el(tag, cls, html) { const e = document.createElement(tag); if (cls) e.className = cls; if (html != null) e.innerHTML = html; return e; }
function destroyChart(k) { if (state.charts[k]) { state.charts[k].destroy(); delete state.charts[k]; } }
function showEmpty(id, msg) { const e = $(id); if (e) { e.textContent = msg; e.classList.remove("hidden"); } }
function hideEmpty(id) { const e = $(id); if (e) e.classList.add("hidden"); }

async function getJSON(url) { const r = await fetch(url); return r.json(); }

async function load() {
  const [overview, learning, yt] = await Promise.all([
    getJSON("/api/overview"), getJSON("/api/learning"), getJSON("/api/youtube"),
  ]);
  renderMeta(overview.meta || {});
  if (!overview.ok) {
    $("banner").classList.remove("hidden");
    $("banner").textContent = overview.hint || "No snapshot. Run /flywheel first.";
    $("cards").innerHTML = "";
    return;
  }
  $("banner").classList.add("hidden");
  state.snapshot = overview.snapshot || {};
  state.voice = overview.voice || {};
  state.learnSummary = overview.learning || {};
  state.learning = (learning && learning.hypotheses) || [];
  state.ytVideos = (yt && yt.videos) || [];

  renderCards();
  renderReach();
  renderFormat();
  renderRoi();
  renderYt();
  renderAmp();
  renderLearning();
  $("foot").textContent = `snapshot ${overview.meta.snapshot_date} · schema v${(overview.meta.schema_version || "?").toString().replace(/"/g, "")} · ${state.learning.length} hypotheses across 2 ledgers`;
}

function renderMeta(meta) {
  if (!meta.has_snapshot) { $("meta").innerHTML = `<span class="stale">no snapshot</span>`; return; }
  const stale = meta.stale ? `<span class="stale">⚠ ${meta.age_days}d old</span>` : `${meta.age_days}d old`;
  $("meta").innerHTML = `snapshot <b>${meta.snapshot_date}</b> · ${stale}`;
}

function renderCards() {
  const s = state.snapshot, v = state.voice || {};
  const cards = [];
  const bh = s.beehiiv || {}, li = s.linkedin || {}, yt = s.youtube || {};
  if (bh.total_subs != null) cards.push(card("beehiiv subs", fmt(bh.total_subs), bh.new_subs_in_window, "in window"));
  if (li.newsletter_subs != null) cards.push(card("LinkedIn newsletter", fmt(li.newsletter_subs), null));
  if (li.profile_followers != null) cards.push(card("LinkedIn followers", fmt(li.profile_followers), null));
  if (yt.views != null) cards.push(card("YouTube views (wk)", fmt(yt.views), yt.subs_gained, "subs"));
  // Voice corpus card — the dual-source grounding health
  const nN = v.num_newsletter ?? 0, nY = v.num_youtube ?? 0;
  const fresh = v.youtube_transcript_fetched_at ? (v.youtube_transcript_fetched_at.slice(0, 10)) : "—";
  const vc = el("div", "card voice");
  vc.innerHTML = `<div class="label">Voice corpus</div>
    <div class="value">${nN} newsletters · ${nY} livestreams</div>
    <div class="delta">${v.available === false ? "⚠ cache absent" : "fetched " + fresh}</div>`;
  $("cards").innerHTML = "";
  cards.forEach((c) => $("cards").appendChild(c));
  $("cards").appendChild(vc);
}
function card(label, value, delta, deltaUnit) {
  const c = el("div", "card");
  let d = "";
  if (delta != null && !isNaN(delta)) {
    const dir = delta >= 0 ? "up" : "down";
    d = `<div class="delta ${dir}">${delta >= 0 ? "▲" : "▼"} ${fmt(Math.abs(delta))} ${deltaUnit || ""}</div>`;
  }
  c.innerHTML = `<div class="label">${label}</div><div class="value">${value}</div>${d}`;
  return c;
}

function renderReach() {
  destroyChart("reach"); hideEmpty("reachEmpty");
  const s = state.snapshot;
  let rows = Array.isArray(s.reconciled_reach) ? s.reconciled_reach.slice() : [];
  let labelNote = "";
  if (!rows.length) {
    // Fallback: headline numbers so the panel is never blank.
    const li = s.linkedin || {}, bh = s.beehiiv || {}, bf = s.buffer || {};
    const fb = [
      ["beehiiv", bh.total_subs], ["LinkedIn followers", li.profile_followers],
      ["LinkedIn newsletter", li.newsletter_subs], ["Buffer-tracked", bf.total_followers],
    ].filter((x) => x[1] != null).map((x) => ({ channel: x[0], followers: x[1], source: "headline" }));
    rows = fb;
    labelNote = "headline numbers (run /flywheel for full reconciled reach)";
  }
  if (!rows.length) { showEmpty("reachEmpty", "No reach data — run /flywheel."); return; }
  const ctx = $("reachChart");
  state.charts.reach = new Chart(ctx, {
    type: "bar",
    data: {
      labels: rows.map((r) => r.channel),
      datasets: [{ label: "followers / subs", data: rows.map((r) => r.followers ?? r.reach ?? 0),
        backgroundColor: COLORS.series[0] }],
    },
    options: barOpts(labelNote),
  });
}

function renderFormat() {
  destroyChart("format"); hideEmpty("formatEmpty");
  const fe = state.snapshot.format_engagement;
  const keys = fe && typeof fe === "object" ? Object.keys(fe) : [];
  if (!keys.length) { showEmpty("formatEmpty", "No format_engagement yet — tag posts + run /flywheel."); return; }
  const ctx = $("formatChart");
  state.charts.format = new Chart(ctx, {
    type: "bar",
    data: {
      labels: keys.map((k) => k.replace("format:", "")),
      datasets: [{ label: "eng rate %", data: keys.map((k) => fe[k].eng_rate_pct ?? 0),
        backgroundColor: COLORS.series[2] }],
    },
    options: barOpts(),
  });
}

function renderRoi() {
  destroyChart("roi"); hideEmpty("roiEmpty"); $("roiTable").innerHTML = "";
  const roi = Array.isArray(state.snapshot.channel_roi) ? state.snapshot.channel_roi : [];
  if (!roi.length) { showEmpty("roiEmpty", "No channel_roi yet — run /flywheel."); return; }
  const ctx = $("roiChart");
  state.charts.roi = new Chart(ctx, {
    type: "bar",
    data: {
      labels: roi.map((r) => trunc(r.channel, 14)),
      datasets: [{ label: "ROI score", data: roi.map((r) => r.roi_score ?? 0),
        backgroundColor: roi.map((r) => COLORS[r.bucket] || COLORS.grey) }],
    },
    options: barOpts(),
  });
  const t = el("div");
  roi.forEach((r) => {
    t.appendChild(el("div", "", `<span class="bucket ${r.bucket || "grey"}"></span>${r.channel} — score ${fmt1(r.roi_score)} · ${fmt(r.followers)} followers`));
  });
  $("roiTable").appendChild(t);
}

function renderYt() {
  destroyChart("yt"); hideEmpty("ytEmpty");
  renderYtFilters();
  let vids = state.ytVideos;
  if (state.ytFilter !== "all") vids = vids.filter((v) => v.video_type === state.ytFilter);
  vids = vids.filter((v) => v.views > 0);
  if (!vids.length) { showEmpty("ytEmpty", "No YouTube videos.json — run `fetch` in youtube-analytics."); return; }
  const ctx = $("ytChart");
  state.charts.yt = new Chart(ctx, {
    type: "scatter",
    data: {
      datasets: [{
        label: "videos", pointRadius: 5, pointHoverRadius: 7,
        backgroundColor: COLORS.accent,
        data: vids.map((v) => ({ x: v.views, y: v.retention, t: v.title })),
      }],
    },
    options: {
      maintainAspectRatio: false, responsive: true,
      scales: { x: { type: "logarithmic", title: { display: true, text: "views (log)" } },
        y: { title: { display: true, text: "retention %" } } },
      plugins: { legend: { display: false },
        tooltip: { callbacks: { label: (c) => `${trunc(c.raw.t, 40)} — ${fmt(c.raw.x)} views, ${fmt1(c.raw.y)}%` } } },
    },
  });
}
function renderYtFilters() {
  const types = ["all", "live", "long-form", "short"];
  const box = $("ytFilters"); box.innerHTML = "";
  types.forEach((t) => {
    const c = el("span", "chip" + (state.ytFilter === t ? " active" : ""), t);
    c.onclick = () => { state.ytFilter = t; renderYt(); };
    box.appendChild(c);
  });
}

function renderAmp() {
  hideEmpty("ampEmpty"); $("sourceDetail").classList.add("hidden");
  const body = $("ampTable").querySelector("tbody"); body.innerHTML = "";
  const ca = Array.isArray(state.snapshot.content_attribution) ? state.snapshot.content_attribution.slice() : [];
  if (!ca.length) { $("ampTable").classList.add("hidden"); showEmpty("ampEmpty", "No content_attribution yet — fan out a source via /opus-clips, then run /flywheel."); return; }
  $("ampTable").classList.remove("hidden");
  ca.sort((a, b) => (deriveReach(b) - deriveReach(a)));
  ca.forEach((rec) => {
    const src = rec.source || {};
    const tr = el("tr", "clickable");
    tr.innerHTML = `<td>${trunc(src.title || src.id || "?", 46)}</td>
      <td>${src.type || "—"}</td>
      <td class="num">${fmt(sourceReach(rec))}</td>
      <td class="num">${fmt(deriveReach(rec))}</td>
      <td class="num">${fmt1(rec.amplification_ratio)}×</td>
      <td class="num">${(rec.derivatives || []).length}</td>`;
    tr.onclick = () => showSource(src.id);
    body.appendChild(tr);
  });
}
function sourceReach(rec) { const e = rec.source_engagement || {}; return e.reach ?? e.views ?? null; }
function deriveReach(rec) { const e = rec.derived_engagement || {}; return e.reach ?? 0; }

async function showSource(id) {
  if (!id) return;
  const res = await getJSON("/api/source/" + encodeURIComponent(id));
  const box = $("sourceDetail");
  box.classList.remove("hidden");
  if (!res.ok) { box.innerHTML = `<h3>${id}</h3><p class="empty">${res.error || "not found"}</p>`; return; }
  const rec = res.record;
  const src = rec.source || {};
  const derivs = rec.derivatives || [];
  let rows = derivs.map((d) => {
    const plats = d.platforms || {};
    const platCells = Object.keys(plats).map((p) => {
      const m = plats[p];
      const val = (m.views ?? m.reactions ?? m.impressions ?? m.engagement);
      return `<span class="pill pending">${p}: ${val == null ? "—" : fmt(val)} <small>(${m.join_method || (m.pending_task ? "pending" : "?")})</small></span>`;
    }).join(" ");
    return `<tr><td>${trunc(d.title || d.clip_id || "?", 50)}</td><td>${platCells || "—"}</td></tr>`;
  }).join("");
  box.innerHTML = `<h3>${trunc(src.title || src.id, 80)} <span class="sub">${fmt1(rec.amplification_ratio)}× amplification</span></h3>
    <table class="data-table"><thead><tr><th>Derivative</th><th>Per-platform engagement (join method)</th></tr></thead>
    <tbody>${rows || `<tr><td colspan="2" class="empty">no derivatives yet</td></tr>`}</tbody></table>`;
}

function renderLearning() {
  renderLearnFilters();
  const sum = state.learnSummary || {};
  $("learnSummary").innerHTML = `${sum.total ?? 0} hypotheses — `
    + `<span class="pill confirm">${sum.confirmed ?? 0} confirmed</span> `
    + `<span class="pill refute">${sum.refuted ?? 0} refuted</span> `
    + `<span class="pill inconclusive">${sum.inconclusive ?? 0} inconclusive</span> `
    + `<span class="pill pending">${sum.pending ?? 0} pending</span>`;
  let hs = state.learning.slice();
  if (state.learnLedger !== "all") hs = hs.filter((h) => h.ledger === state.learnLedger);
  if (state.learnVerdict !== "all") {
    hs = hs.filter((h) => (state.learnVerdict === "pending" ? !h.verdict : h.verdict === state.learnVerdict));
  }
  const body = $("learnTable").querySelector("tbody"); body.innerHTML = "";
  hideEmpty("learnEmpty");
  if (!hs.length) { $("learnTable").classList.add("hidden"); showEmpty("learnEmpty", "No hypotheses match."); return; }
  $("learnTable").classList.remove("hidden");
  hs.forEach((h) => {
    const verdict = h.verdict || "pending";
    const tr = el("tr");
    tr.innerHTML = `<td>${h.date || ""}</td><td>${h.ledger}</td>
      <td>${h.surface || h.cohort || "—"}</td>
      <td title="${(h.prediction || "").replace(/"/g, "&quot;")}">${trunc(h.prediction, 70)}</td>
      <td>${h.metric || "—"}${h.direction ? " " + (h.direction === "up" ? "↑" : "↓") : ""}</td>
      <td><span class="pill ${verdict}">${verdict}</span></td>`;
    body.appendChild(tr);
  });
}
function renderLearnFilters() {
  const box = $("learnFilters"); box.innerHTML = "";
  const mk = (label, group, val, cur) => {
    const c = el("span", "chip" + (cur === val ? " active" : ""), label);
    c.onclick = () => { state[group] = val; renderLearning(); };
    return c;
  };
  ["all", "cross-surface", "youtube"].forEach((l) => box.appendChild(mk(l, "learnLedger", l, state.learnLedger)));
  const sep = el("span", "", "·"); sep.style.color = COLORS.border; box.appendChild(sep);
  ["all", "pending", "confirm", "refute", "inconclusive"].forEach((l) => box.appendChild(mk(l, "learnVerdict", l, state.learnVerdict)));
}

function barOpts(note) {
  return {
    maintainAspectRatio: false, responsive: true,
    plugins: { legend: { display: false },
      subtitle: note ? { display: true, text: note, color: COLORS.muted } : { display: false } },
    scales: { y: { beginAtZero: true } },
  };
}

$("refresh").onclick = load;
load();
