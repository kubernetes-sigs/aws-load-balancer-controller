/* =========================================================================
   STATE + ROUTER
   ========================================================================= */
const state = {
  namespace: null,
  gateways: [],
  currentGateway: null,
  diff: null,
  filter: 'changed',
  detailKey: null,
  hideKnownInDetail: true,
  hideKnownAll: true,
  currentView: 'map',
};

const $ = (id) => document.getElementById(id);
const escapeHTML = (s) => String(s).replace(/[&<>"']/g, c => (
  { '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' }[c]
));

function formatMigratedFrom(tag) {
  if (!tag) return '';
  if (tag.startsWith('ingress-group/')) {
    const group = tag.slice('ingress-group/'.length);
    return `<span class="migrated-type">IngressGroup</span><span class="migrated-detail">IngressGroupName: ${escapeHTML(group)}</span>`;
  }
  if (tag.startsWith('ingress/')) {
    const parts = tag.slice('ingress/'.length).split('/');
    if (parts.length === 2) {
      return `<span class="migrated-type">Ingress</span><span class="migrated-detail">Namespace: ${escapeHTML(parts[0])}</span><span class="migrated-detail">IngressName: ${escapeHTML(parts[1])}</span>`;
    }
  }
  return escapeHTML(tag);
}

function getUrlParams() {
  const params = new URLSearchParams(window.location.search);
  return {
    namespace: params.get('namespace') || null,
    gateway: params.get('gateway') || null,
  };
}

function pushState(ns, gw) {
  const url = new URL(window.location.href);
  if (ns) url.searchParams.set('namespace', ns);
  else url.searchParams.delete('namespace');
  if (gw) url.searchParams.set('gateway', gw);
  else url.searchParams.delete('gateway');
  window.history.pushState({}, '', url.toString());
}

async function init() {
  const { namespace, gateway } = getUrlParams();
  if (namespace && gateway) showComparison(namespace, gateway);
  else if (namespace) showGatewayList(namespace);
  else showLanding();
}

/* =========================================================================
   LANDING — namespace list
   ========================================================================= */
async function showLanding() {
  state.namespace = null;
  state.currentGateway = null;
  $('landing').style.display = 'block';
  $('gateway-list').style.display = 'none';
  $('comparison').style.display = 'none';
  $('back-btn').style.display = 'none';
  $('crumbs').style.display = 'none';
  $('drawer').classList.remove('visible');

  const table = $('ns-table');
  table.innerHTML = '<div class="loading">Loading namespaces…</div>';

  const resp = await fetch('/api/namespaces');
  if (!resp.ok) {
    table.innerHTML = '<div class="empty">Failed to load namespaces.</div>';
    return;
  }
  const namespaces = await resp.json();
  const arr = Array.isArray(namespaces) ? namespaces : [];
  $('ns-meta').textContent = arr.length + ' total';

  if (arr.length === 0) {
    table.innerHTML = '<div class="empty">No gateways with dry-run plans in this cluster.</div>';
    return;
  }

  table.innerHTML =
    '<div class="t-header"><div>Namespace</div><div>Gateways</div><div></div></div>' +
    arr.map(ns => `
      <div class="t-row" data-ns="${escapeHTML(ns.namespace)}" role="row" tabindex="0">
        <div class="t-name">${escapeHTML(ns.namespace)}</div>
        <div class="t-count"><span class="num">${ns.gatewayCount}</span>with dry-run plans</div>
        <div class="t-arrow" aria-hidden="true">→</div>
      </div>`).join('');

  table.querySelectorAll('.t-row').forEach(row => {
    const go = () => { pushState(row.dataset.ns, null); showGatewayList(row.dataset.ns); };
    row.addEventListener('click', go);
    row.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); go(); }
    });
  });
}

/* =========================================================================
   GATEWAY LIST — gateways in a namespace
   ========================================================================= */
async function showGatewayList(namespace) {
  state.namespace = namespace;
  state.currentGateway = null;
  $('landing').style.display = 'none';
  $('gateway-list').style.display = 'block';
  $('comparison').style.display = 'none';
  $('back-btn').style.display = 'inline-flex';
  $('crumbs').style.display = 'flex';
  $('crumb-ns').textContent = namespace;
  $('crumb-gw-sep').style.display = 'none';
  $('crumb-gw').style.display = 'none';
  $('drawer').classList.remove('visible');

  $('gw-list-title').textContent = namespace;

  const table = $('gw-table');
  table.innerHTML = '<div class="loading">Loading gateways…</div>';

  const resp = await fetch(`/api/gateways?namespace=${encodeURIComponent(namespace)}`);
  if (!resp.ok) {
    table.innerHTML = '<div class="error-msg">Failed to load gateways.</div>';
    return;
  }
  const data = await resp.json();
  state.gateways = Array.isArray(data) ? data : [];
  $('gw-meta').textContent = state.gateways.length + ' total';

  if (state.gateways.length === 0) {
    table.innerHTML = '<div class="empty">No gateways with dry-run plans in this namespace.</div>';
    return;
  }

  table.innerHTML =
    '<div class="t-header"><div>Gateway</div><div>Migrated From Ingress/IngressGroup</div><div>Summary</div><div></div></div>' +
    state.gateways.map(gw => {
      let summaryHTML = '';
      if (gw.error) {
        summaryHTML = `<span class="gw-error-text">Error</span>`;
      } else if (gw.summary) {
        const pills = [];
        if (gw.summary.changed) pills.push(`<span class="count-pill changed">${gw.summary.changed} changed</span>`);
        if (gw.summary.removed) pills.push(`<span class="count-pill removed">${gw.summary.removed} removed</span>`);
        if (gw.summary.added) pills.push(`<span class="count-pill added">${gw.summary.added} added</span>`);
        if (gw.summary.same) pills.push(`<span class="count-pill same">${gw.summary.same} same</span>`);
        summaryHTML = pills.join(' ');
      }
      const sourceHTML = formatMigratedFrom(gw.migratedFrom || '');
      return `
      <div class="t-row" data-gw="${escapeHTML(gw.name)}" role="row" tabindex="0">
        <div class="t-name">${escapeHTML(gw.name)}</div>
        <div class="t-source">${sourceHTML}</div>
        <div class="t-count">${summaryHTML}</div>
        <div class="t-arrow" aria-hidden="true">→</div>
      </div>`;
    }).join('');

  table.querySelectorAll('.t-row').forEach(row => {
    const go = () => { pushState(namespace, row.dataset.gw); showComparison(namespace, row.dataset.gw); };
    row.addEventListener('click', go);
    row.addEventListener('keydown', (e) => {
      if (e.key === 'Enter' || e.key === ' ') { e.preventDefault(); go(); }
    });
  });
}

/* =========================================================================
   COMPARISON — diff detail for a single gateway
   ========================================================================= */
async function showComparison(namespace, gatewayName) {
  state.namespace = namespace;
  state.currentGateway = gatewayName;
  state.filter = 'changed';
  state.hideKnownAll = true;
  state.hideKnownInDetail = true;
  $('landing').style.display = 'none';
  $('gateway-list').style.display = 'none';
  $('comparison').style.display = 'grid';
  $('back-btn').style.display = 'inline-flex';
  $('crumbs').style.display = 'flex';
  $('crumb-ns').textContent = namespace;
  $('crumb-gw-sep').style.display = 'inline';
  $('crumb-gw').style.display = 'inline';
  $('crumb-gw').textContent = gatewayName;
  $('drawer').classList.remove('visible');

  const gw = state.gateways.find(g => g.name === gatewayName);
  if (gw && gw.error) {
    $('strip').style.display = 'none';
    $('split').style.display = 'none';
    document.querySelectorAll('.error-msg').forEach(e => e.remove());
    $('comparison').insertAdjacentHTML('beforeend',
      `<div class="error-msg">${escapeHTML(gw.error)}</div>`);
    return;
  }
  document.querySelectorAll('.error-msg').forEach(e => e.remove());

  const url = `/api/diff?namespace=${encodeURIComponent(namespace)}&gateway=${encodeURIComponent(gatewayName)}`;
  const resp = await fetch(url);
  if (!resp.ok) {
    $('strip').style.display = 'none';
    $('split').style.display = 'none';
    $('comparison').insertAdjacentHTML('beforeend',
      '<div class="error-msg">Failed to load diff.</div>');
    return;
  }
  state.diff = await resp.json();
  $('comparison-header').style.display = 'block';
  $('toolbar').style.display = 'flex';
  $('hide-known-all').checked = state.hideKnownAll;
  $('hide-known-toggle').checked = state.hideKnownInDetail;

  state.currentView = 'map';
  syncView();
}

function renderDiff() {
  const diff = state.diff;
  if (!diff) return;

  const summary = diff.summary || { same: 0, changed: 0, removed: 0, added: 0 };
  const total = summary.same + summary.changed + summary.removed + summary.added;
  $('metrics').innerHTML = `
    <button class="seg-btn all" data-filter="all" type="button" title="Show all resources regardless of status">
      <span class="seg-dot"></span><span class="seg-label">All</span><span class="seg-count">${total}</span>
    </button>
    <button class="seg-btn same" data-filter="same" type="button" title="Fields identical between Ingress and Gateway models">
      <span class="seg-dot"></span><span class="seg-label">Same</span><span class="seg-count">${summary.same}</span>
    </button>
    <button class="seg-btn changed" data-filter="changed" type="button" title="Fields whose values differ between the two models">
      <span class="seg-dot"></span><span class="seg-label">Changed</span><span class="seg-count">${summary.changed}</span>
    </button>
    <button class="seg-btn removed" data-filter="removed" type="button" title="Fields present in Ingress but absent from Gateway">
      <span class="seg-dot"></span><span class="seg-label">Removed</span><span class="seg-count">${summary.removed}</span>
    </button>
    <button class="seg-btn added" data-filter="added" type="button" title="Fields present in Gateway but absent from Ingress">
      <span class="seg-dot"></span><span class="seg-label">Added</span><span class="seg-count">${summary.added}</span>
    </button>
  `;
  $('metrics').querySelectorAll('.seg-btn').forEach(btn => {
    const toggle = () => {
      if (btn.dataset.filter === 'all') {
        setFilter('all');
      } else {
        setFilter(state.filter === btn.dataset.filter ? 'all' : btn.dataset.filter);
      }
    };
    btn.addEventListener('click', toggle);
  });
  syncFilterUI();

  const resources = {};
  diff.entries.forEach(e => {
    const key = e.resourceType + '|' + e.correlationId;
    if (!resources[key]) {
      resources[key] = {
        type: e.resourceType,
        correlationId: e.correlationId,
        ingressId: e.ingressResourceId || '',
        gatewayId: e.gatewayResourceId || '',
        entries: [],
        status: 'same',
      };
    }
    if (e.ingressResourceId && !resources[key].ingressId) resources[key].ingressId = e.ingressResourceId;
    if (e.gatewayResourceId && !resources[key].gatewayId) resources[key].gatewayId = e.gatewayResourceId;
    resources[key].entries.push(e);
  });
  Object.values(resources).forEach(r => {
    const statuses = new Set(r.entries.map(e => e.status));
    r.status = statuses.size === 1 ? [...statuses][0] : 'changed';
  });

  const byType = {};
  Object.values(resources).forEach(r => {
    (byType[r.type] = byType[r.type] || []).push(r);
  });

  const typeOrder = [
    'AWS::ElasticLoadBalancingV2::LoadBalancer',
    'AWS::ElasticLoadBalancingV2::Listener',
    'AWS::ElasticLoadBalancingV2::ListenerRule',
    'AWS::ElasticLoadBalancingV2::TargetGroup',
  ];
  const allTypes = [...new Set([...typeOrder, ...Object.keys(byType)])];

  renderColumn('ingress-resources', byType, allTypes, 'ingress');
  renderColumn('gateway-resources', byType, allTypes, 'gateway');

  $('drawer').classList.remove('visible');
}

function syncView() {
  const isMap = state.currentView === 'map';
  $('resource-map').style.display = isMap ? 'block' : 'none';
  $('split').style.display = isMap ? 'none' : 'grid';
  $('strip').style.display = isMap ? 'none' : 'flex';
  document.querySelectorAll('.view-tab').forEach(tab => {
    tab.classList.toggle('active', tab.dataset.view === state.currentView);
  });

  if (isMap) {
    $('drawer').classList.remove('visible');
    renderResourceMap();
  } else {
    renderDiff();
    const firstCard = document.querySelector('.res-card');
    if (firstCard) firstCard.click();
  }
}

function setFilter(next) {
  state.filter = next;
  syncFilterUI();
  renderDiff();
  if ($('drawer').classList.contains('visible')) {
    renderDetail();
  }
}
function syncFilterUI() {
  document.querySelectorAll('.seg-btn').forEach(btn => {
    btn.classList.toggle('active', btn.dataset.filter === state.filter);
  });
}

function renderColumn(containerId, byType, allTypes, side) {
  const container = $(containerId);
  let html = '';

  allTypes.forEach(type => {
    const resources = byType[type];
    if (!resources) return;
    let filtered = resources.filter(r => {
      if (state.filter === 'all') return true;
      return r.entries.some(e => e.status === state.filter);
    });
    if (state.hideKnownAll) {
      filtered = filtered.filter(r => {
        const scoped = state.filter === 'all'
          ? r.entries
          : r.entries.filter(e => e.status === state.filter);
        return scoped.some(e => !e.known);
      });
    }
    if (filtered.length === 0) return;

    const onSide = filtered.filter(r => (side === 'ingress' ? r.ingressId : r.gatewayId));
    if (onSide.length === 0) return;

    const shortType = type.split('::').pop();
    html += `<div class="res-group">
      <div class="res-group-head">
        <span class="type">${escapeHTML(shortType)}</span>
        <span class="type-count">${onSide.length}</span>
        <span class="hr" aria-hidden="true"></span>
      </div>`;
    onSide.forEach(r => {
      const rawID = side === 'ingress' ? r.ingressId : r.gatewayId;
      const shortId = rawID.length > 64 ? '…' + rawID.slice(-62) : rawID;
      const dataKey = r.type + '|' + r.correlationId;
      if (state.filter === 'all') {
        html += `<button class="res-card" data-key="${escapeHTML(dataKey)}" type="button">
          <span class="rid">${escapeHTML(shortId)}</span>
        </button>`;
      } else {
        html += `<button class="res-card status-${state.filter}" data-key="${escapeHTML(dataKey)}" type="button">
          <span class="rid">${escapeHTML(shortId)}</span>
          <span class="rstatus"><span class="dot"></span>${state.filter}</span>
        </button>`;
      }
    });
    html += `</div>`;
  });

  container.innerHTML = html;
  container.querySelectorAll('.res-card').forEach(card => {
    card.addEventListener('click', () => showDetail(card.dataset.key));
  });
}

/* =========================================================================
   DETAIL DRAWER
   ========================================================================= */
function showDetail(key) {
  state.detailKey = key;
  state.hideKnownInDetail = state.hideKnownAll;
  $('hide-known-toggle').checked = state.hideKnownInDetail;
  document.querySelectorAll('.res-card.active').forEach(c => c.classList.remove('active'));
  document.querySelectorAll(`.res-card[data-key="${CSS.escape(key)}"]`).forEach(c => c.classList.add('active'));
  renderDetail();
  $('drawer').classList.add('visible');
}

function renderDetail() {
  const key = state.detailKey;
  if (!key) return;
  const entries = state.diff.entries.filter(e => (e.resourceType + '|' + e.correlationId) === key);
  if (entries.length === 0) return;

  const [resType, corrID] = key.split('|');
  const shortType = resType.split('::').pop();
  const ingressID = entries.map(e => e.ingressResourceId).find(Boolean) || '';
  const gatewayID = entries.map(e => e.gatewayResourceId).find(Boolean) || '';
  let title = `${shortType} · ${corrID}`;
  if (ingressID && gatewayID && ingressID !== gatewayID) {
    title += `\n↳ ingress: ${ingressID}\n↳ gateway: ${gatewayID}`;
  }
  $('drawer-title').textContent = title;

  const skipStatusFilter = state.currentView === 'map';
  const filtered = (skipStatusFilter || state.filter === 'all')
    ? entries
    : entries.filter(e => e.status === state.filter);

  const hasKnown = filtered.some(e => e.known);
  $('drawer-toggle-wrap').style.display = hasKnown ? 'inline-flex' : 'none';
  const showKnown = hasKnown && !state.hideKnownInDetail;

  const headCells = [
    '<th style="width: 30%;">Field</th>',
    '<th>Ingress</th>',
    '<th>Gateway</th>',
    '<th style="width: 110px;">Status</th>',
  ];
  if (showKnown) headCells.push('<th style="width: 28%;">Known Cause</th>');
  $('detail-thead').innerHTML = `<tr>${headCells.join('')}</tr>`;

  const visible = state.hideKnownInDetail
    ? filtered.filter(e => !e.known)
    : filtered;

  $('detail-body').innerHTML = visible.map(e => {
    const ing = e.ingress != null ? JSON.stringify(e.ingress) : '';
    const gw  = e.gateway != null ? JSON.stringify(e.gateway) : '';
    const cells = [
      `<td class="field">${escapeHTML(e.field)}</td>`,
      `<td class="val${ing ? '' : ' empty'}">${ing ? escapeHTML(ing) : '—'}</td>`,
      `<td class="val${gw ? '' : ' empty'}">${gw ? escapeHTML(gw) : '—'}</td>`,
      `<td><span class="status-tag ${e.status}"><span class="dot"></span>${e.status}</span></td>`,
    ];
    if (showKnown) {
      cells.push(e.known
        ? `<td><div class="known-cause"><span class="known-cause-mark"></span><span class="known-cause-text">${escapeHTML(e.knownCause || 'Migration artifact')}</span></div></td>`
        : '<td></td>');
    }
    return `<tr class="row-${e.status}">${cells.join('')}</tr>`;
  }).join('');
}

/* =========================================================================
   NAVIGATION HANDLERS
   ========================================================================= */
function navigateBack() {
  if (state.currentGateway) {
    pushState(state.namespace, null);
    showGatewayList(state.namespace);
  } else if (state.namespace) {
    pushState(null, null);
    showLanding();
  }
}

$('drawer-close').addEventListener('click', () => {
  $('drawer').classList.remove('visible');
  document.querySelectorAll('.res-card.active').forEach(c => c.classList.remove('active'));
});
$('hide-known-toggle').addEventListener('change', (e) => {
  state.hideKnownInDetail = e.target.checked;
  renderDetail();
});
$('hide-known-all').addEventListener('change', (e) => {
  state.hideKnownAll = e.target.checked;
  state.hideKnownInDetail = e.target.checked;
  $('hide-known-toggle').checked = e.target.checked;
  if ($('drawer').classList.contains('visible')) renderDetail();
  if (state.currentView === 'map') {
    renderResourceMap();
  } else {
    renderDiff();
  }
});
$('back-btn').addEventListener('click', (e) => { e.preventDefault(); navigateBack(); });
$('brand').addEventListener('click', (e) => { e.preventDefault(); pushState(null, null); showLanding(); });
$('crumb-home').addEventListener('click', () => { pushState(null, null); showLanding(); });
$('crumb-ns').addEventListener('click', () => {
  if (state.namespace) { pushState(state.namespace, null); showGatewayList(state.namespace); }
});

window.addEventListener('popstate', () => {
  const { namespace, gateway } = getUrlParams();
  if (namespace && gateway) showComparison(namespace, gateway);
  else if (namespace) showGatewayList(namespace);
  else showLanding();
});

document.addEventListener('keydown', (e) => {
  if (e.key === 'Escape' && $('drawer').classList.contains('visible')) {
    $('drawer').classList.remove('visible');
    document.querySelectorAll('.res-card.active').forEach(c => c.classList.remove('active'));
  }
});

// Drawer resize
(function() {
  const grip = $('drawer-grip');
  const panel = $('drawer');
  let startY, startHeight, pointerId = null;

  grip.addEventListener('pointerdown', (e) => {
    pointerId = e.pointerId;
    grip.setPointerCapture(pointerId);
    startY = e.clientY;
    startHeight = panel.offsetHeight;
    e.preventDefault();
  });
  grip.addEventListener('pointermove', (e) => {
    if (pointerId === null) return;
    const newHeight = startHeight + (startY - e.clientY);
    const maxHeight = Math.max(200, window.innerHeight - 140);
    panel.style.height = Math.max(200, Math.min(newHeight, maxHeight)) + 'px';
  });
  const release = () => {
    if (pointerId !== null) {
      grip.releasePointerCapture(pointerId);
      pointerId = null;
    }
  };
  grip.addEventListener('pointerup', release);
  grip.addEventListener('pointercancel', release);
})();

/* =========================================================================
   VIEW TABS + RESOURCE MAP
   ========================================================================= */
document.querySelectorAll('.view-tab').forEach(tab => {
  tab.addEventListener('click', () => {
    state.currentView = tab.dataset.view;
    syncView();
  });
});

const TYPE_ORDER = [
  'AWS::ElasticLoadBalancingV2::LoadBalancer',
  'AWS::ElasticLoadBalancingV2::Listener',
  'AWS::ElasticLoadBalancingV2::ListenerRule',
  'AWS::ElasticLoadBalancingV2::TargetGroup',
  'K8S::ElasticLoadBalancingV2::TargetGroupBinding',
];
const SG_TYPE = 'AWS::EC2::SecurityGroup';
const TYPE_LABELS = {
  'AWS::ElasticLoadBalancingV2::LoadBalancer': 'Load Balancer',
  'AWS::EC2::SecurityGroup': 'Security Groups',
  'AWS::ElasticLoadBalancingV2::Listener': 'Listeners',
  'AWS::ElasticLoadBalancingV2::ListenerRule': 'Listener Rules',
  'AWS::ElasticLoadBalancingV2::TargetGroup': 'Target Groups',
  'K8S::ElasticLoadBalancingV2::TargetGroupBinding': 'Target Group Bindings',
};

// Compute effective node status considering "hide known" toggle.
// When known changes are hidden, a node whose diffs are all known shows as "same".
function computeEffectiveStatus(node) {
  if (!state.hideKnownAll) return node.status;

  // Find all diff entries for this node.
  const entries = state.diff.entries.filter(e => {
    const nodeId = e.resourceType + '|' + (e.gatewayResourceId || e.correlationId);
    return nodeId === node.id;
  });
  if (entries.length === 0) return node.status;

  // Exclude known entries and recompute status.
  const unknownEntries = entries.filter(e => !e.known);
  if (unknownEntries.length === 0) return 'same';

  const priority = { same: 0, added: 1, removed: 2, changed: 3 };
  let maxStatus = 'same';
  for (const e of unknownEntries) {
    if ((priority[e.status] || 0) > (priority[maxStatus] || 0)) {
      maxStatus = e.status;
    }
  }
  return maxStatus;
}

function renderTopoNode(n) {
  const effectiveStatus = computeEffectiveStatus(n);
  const badge = effectiveStatus !== 'same' ? `<span class="topo-node-badge ${effectiveStatus}">${effectiveStatus}</span>` : '';
  return `<div class="topo-node status-${effectiveStatus}" data-node-id="${escapeHTML(n.id)}" tabindex="0">
    <span class="topo-node-dot"></span>
    <span class="topo-node-label">${escapeHTML(n.label)}</span>${badge}
  </div>`;
}

function renderResourceMap() {
  const topo = state.diff.topology;
  if (!topo || !topo.nodes || topo.nodes.length === 0) {
    $('resource-map-inner').innerHTML = '<div class="empty">No topology data available.</div>';
    return;
  }

  const nodesByType = {};
  topo.nodes.forEach(n => {
    (nodesByType[n.resourceType] = nodesByType[n.resourceType] || []).push(n);
  });

  // Render columns (lanes) left to right — SecurityGroup rendered separately below LB.
  const columns = TYPE_ORDER.filter(t => nodesByType[t] && nodesByType[t].length > 0);
  const sgNodes = nodesByType[SG_TYPE] || [];

  let html = '<div class="topo-legend">';
  html += '<span class="topo-legend-item"><span class="topo-legend-dot status-same"></span>Same</span>';
  html += '<span class="topo-legend-item"><span class="topo-legend-dot status-changed"></span>Changed</span>';
  html += '<span class="topo-legend-item"><span class="topo-legend-dot status-added"></span>Added</span>';
  html += '<span class="topo-legend-item"><span class="topo-legend-dot status-removed"></span>Removed</span>';
  html += '</div>';

  html += '<div class="topo-lanes">';
  columns.forEach((type, colIdx) => {
    const nodes = nodesByType[type];
    const typeLabel = TYPE_LABELS[type] || type.split('::').pop();
    html += `<div class="topo-lane" data-col="${colIdx}">`;
    html += `<div class="topo-lane-head">${escapeHTML(typeLabel)}</div>`;
    html += `<div class="topo-lane-body">`;
    nodes.forEach(n => { html += renderTopoNode(n); });
    // Render SecurityGroups below LoadBalancer in the same lane.
    if (type === 'AWS::ElasticLoadBalancingV2::LoadBalancer' && sgNodes.length > 0) {
      html += `<div class="topo-sg-section">`;
      html += `<div class="topo-lane-head">${escapeHTML(TYPE_LABELS[SG_TYPE])}</div>`;
      sgNodes.forEach(n => { html += renderTopoNode(n); });
      html += `</div>`;
    }
    html += `</div></div>`;
  });
  html += '</div>';

  // SVG overlay for edges.
  html += '<svg class="topo-edges" id="topo-edges"></svg>';

  $('resource-map-inner').innerHTML = html;

  // Draw edges after DOM is ready.
  requestAnimationFrame(() => drawTopoEdges(topo.edges));

  // Node click → open drawer for that resource.
  document.querySelectorAll('.topo-node').forEach(node => {
    node.addEventListener('click', () => {
      const nodeId = node.dataset.nodeId;
      document.querySelectorAll('.topo-node.active').forEach(n => n.classList.remove('active'));
      node.classList.add('active');
      highlightTopoEdges(nodeId);

      // Resolve topology node ID to diff key (resourceType|correlationId).
      const detailKey = resolveDiffKey(nodeId);
      if (detailKey) {
        showDetail(detailKey);
        $('drawer').classList.add('visible');
      }
    });
  });
}

// resolveDiffKey maps a topology node ID (resourceType|rawResourceId) to the
// diff entry key (resourceType|correlationId). For most resources these are the
// same; for TargetGroup/TGB the correlationId is derived from serviceRef.
function resolveDiffKey(nodeId) {
  const [resType, rawId] = nodeId.split('|');
  // Try direct match first (works for LB, Listener, ListenerRule).
  const directKey = resType + '|' + rawId;
  const directMatch = state.diff.entries.find(e => (e.resourceType + '|' + e.correlationId) === directKey);
  if (directMatch) return directKey;

  // Fallback: find by gatewayResourceId.
  const byGwId = state.diff.entries.find(e => e.resourceType === resType && e.gatewayResourceId === rawId);
  if (byGwId) return resType + '|' + byGwId.correlationId;

  return null;
}

function drawTopoEdges(edges) {
  const svg = document.getElementById('topo-edges');
  if (!svg) return;
  const container = $('resource-map-inner');
  const rect = container.getBoundingClientRect();
  svg.setAttribute('width', rect.width);
  svg.setAttribute('height', rect.height);
  svg.innerHTML = '';

  edges.forEach(edge => {
    const fromEl = container.querySelector(`[data-node-id="${CSS.escape(edge.from)}"]`);
    const toEl = container.querySelector(`[data-node-id="${CSS.escape(edge.to)}"]`);
    if (!fromEl || !toEl) return;

    const fromRect = fromEl.getBoundingClientRect();
    const toRect = toEl.getBoundingClientRect();

    // Detect if nodes are in the same column (vertically stacked).
    const sameColumn = Math.abs(fromRect.left - toRect.left) < fromRect.width;

    let d;
    if (sameColumn) {
      // Vertical edge: bottom of "from" to top of "to".
      const x1 = fromRect.left + fromRect.width / 2 - rect.left;
      const y1 = fromRect.bottom - rect.top;
      const x2 = toRect.left + toRect.width / 2 - rect.left;
      const y2 = toRect.top - rect.top;
      const midY = (y1 + y2) / 2;
      d = `M${x1},${y1} C${x1},${midY} ${x2},${midY} ${x2},${y2}`;
    } else {
      // Horizontal edge: right of "from" to left of "to".
      const x1 = fromRect.right - rect.left;
      const y1 = fromRect.top + fromRect.height / 2 - rect.top;
      const x2 = toRect.left - rect.left;
      const y2 = toRect.top + toRect.height / 2 - rect.top;
      const midX = (x1 + x2) / 2;
      d = `M${x1},${y1} C${midX},${y1} ${midX},${y2} ${x2},${y2}`;
    }

    const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
    path.setAttribute('d', d);
    path.setAttribute('class', 'topo-edge');
    path.dataset.from = edge.from;
    path.dataset.to = edge.to;
    svg.appendChild(path);
  });
}

function highlightTopoEdges(nodeId) {
  document.querySelectorAll('.topo-edge').forEach(edge => {
    const connected = edge.dataset.from === nodeId || edge.dataset.to === nodeId;
    edge.classList.toggle('highlighted', connected);
    edge.classList.toggle('dimmed', !connected);
  });
}

window.addEventListener('resize', () => {
  if (state.currentView === 'map' && state.diff.topology) {
    requestAnimationFrame(() => drawTopoEdges(state.diff.topology.edges));
  }
});

/* =========================================================================
   EXPORT
   ========================================================================= */
let pendingExport = null;

function showExportModal(exportFn) {
  pendingExport = exportFn;
  $('export-modal').style.display = 'flex';
}
function hideExportModal() {
  pendingExport = null;
  $('export-modal').style.display = 'none';
}
$('export-modal-cancel').addEventListener('click', hideExportModal);
$('export-modal-close').addEventListener('click', hideExportModal);
$('export-modal').addEventListener('click', (e) => {
  if (e.target === $('export-modal')) hideExportModal();
});
$('export-modal-confirm').addEventListener('click', () => {
  if (pendingExport) pendingExport();
  hideExportModal();
});

$('export-json-btn').addEventListener('click', () => {
  if (!state.diff) return;
  showExportModal(() => {
    const payload = {
      namespace: state.namespace,
      gateway: state.currentGateway,
      exportedAt: new Date().toISOString(),
      diff: state.diff,
    };
    downloadFile(
      JSON.stringify(payload, null, 2),
      `migration-diff-${state.namespace}-${state.currentGateway}.json`,
      'application/json'
    );
  });
});

$('export-html-btn').addEventListener('click', () => {
  if (!state.diff) return;
  showExportModal(() => {
    const diff = state.diff;
    const summary = diff.summary || { same: 0, changed: 0, removed: 0, added: 0 };

    const rows = diff.entries.map(e => {
      const ing = e.ingress != null ? JSON.stringify(e.ingress) : '';
      const gw = e.gateway != null ? JSON.stringify(e.gateway) : '';
      return `<tr class="row-${esc(e.status)}">
        <td>${esc(e.resourceType.split('::').pop())}</td>
        <td class="mono">${esc(e.correlationId)}</td>
        <td class="mono">${esc(e.field)}</td>
        <td class="mono">${ing ? esc(ing) : '<em>—</em>'}</td>
        <td class="mono">${gw ? esc(gw) : '<em>—</em>'}</td>
        <td><span class="tag ${esc(e.status)}">${esc(e.status)}</span></td>
        <td>${e.known ? esc(e.knownCause || 'Migration artifact') : ''}</td>
      </tr>`;
    }).join('\n');

    const html = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<title>Migration Report — ${esc(state.namespace)}/${esc(state.currentGateway)}</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif; font-size: 14px; color: #0f141a; margin: 0; padding: 32px; background: #f6f6f9; }
h1 { font-size: 20px; margin-bottom: 4px; }
.meta { color: #656871; font-size: 13px; margin-bottom: 24px; }
.summary { display: flex; gap: 16px; margin-bottom: 24px; flex-wrap: wrap; }
.summary-item { padding: 8px 16px; background: #fff; border: 1px solid #c6c6cd; border-radius: 8px; }
.summary-item .label { font-size: 12px; color: #656871; }
.summary-item .value { font-size: 20px; font-weight: 700; }
table { width: 100%; border-collapse: collapse; background: #fff; border: 1px solid #c6c6cd; border-radius: 8px; overflow: hidden; }
th { background: #f9f9fa; text-align: left; padding: 8px 12px; font-size: 12px; font-weight: 700; border-bottom: 1px solid #c6c6cd; }
td { padding: 8px 12px; border-bottom: 1px solid #ebebf0; vertical-align: top; word-break: break-all; }
.mono { font-family: Monaco, Menlo, Consolas, monospace; font-size: 12px; }
.tag { display: inline-block; padding: 2px 8px; border-radius: 4px; font-size: 11px; font-weight: 700; text-transform: uppercase; }
.tag.same { background: #effff1; color: #00802f; }
.tag.changed { background: #fffef0; color: #855900; }
.tag.removed { background: #fff5f5; color: #db0000; }
.tag.added { background: #f0fbff; color: #006ce0; }
.row-changed td:first-child { box-shadow: inset 3px 0 0 #855900; }
.row-removed td:first-child { box-shadow: inset 3px 0 0 #db0000; }
.row-added td:first-child { box-shadow: inset 3px 0 0 #006ce0; }
.row-same td:first-child { box-shadow: inset 3px 0 0 #00802f; }
</style>
</head>
<body>
<h1>Migration Diff Report</h1>
<div class="meta">
  Namespace: <strong>${esc(state.namespace)}</strong> · Gateway: <strong>${esc(state.currentGateway)}</strong><br>
  Exported: ${new Date().toLocaleString()}
</div>
<div class="summary">
  <div class="summary-item"><div class="label">Same</div><div class="value">${summary.same}</div></div>
  <div class="summary-item"><div class="label">Changed</div><div class="value">${summary.changed}</div></div>
  <div class="summary-item"><div class="label">Removed</div><div class="value">${summary.removed}</div></div>
  <div class="summary-item"><div class="label">Added</div><div class="value">${summary.added}</div></div>
</div>
<table>
<thead><tr><th>Resource Type</th><th>Resource</th><th>Field</th><th>Ingress</th><th>Gateway</th><th>Status</th><th>Known Cause</th></tr></thead>
<tbody>
${rows}
</tbody>
</table>
</body>
</html>`;

    downloadFile(
      html,
      `migration-report-${state.namespace}-${state.currentGateway}.html`,
      'text/html'
    );
  });
});

function esc(s) { return escapeHTML(s); }

function downloadFile(content, filename, mimeType) {
  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

init();
