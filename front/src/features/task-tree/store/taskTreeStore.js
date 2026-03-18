import { create } from 'zustand';
import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';

// Tree data loaded from server API (replaces localStorage).

const NODE_WIDTH = 280;
const NODE_HEIGHT = 110;
const H_GAP = 80;
const V_GAP = 100;

// Empty initial state — tree loaded from server via initFromServer().
const emptyNodes = [];

// Infer node type from depth: 0=proposal, 1=milestone, 2+=task
function inferType(nodes) {
  const depthOf = (node) => {
    let d = 0, cur = node;
    while (cur && cur.parentId) {
      d++;
      cur = nodes.find((n) => n.id === cur.parentId);
    }
    return d;
  };
  return nodes.map((n) => {
    if (n.type && n.type !== 'task') return n;
    const depth = depthOf(n);
    const type = depth === 0 ? 'proposal' : depth === 1 ? 'milestone' : 'task';
    return { ...n, type };
  });
}

function parseTreeNodes(parsed) {
  if (!parsed?.nodes || parsed.nodes.length === 0) return null;
  const nodes = parsed.nodes.map((n) => ({
    id: n.id,
    uid: n.uid || n.id,
    type: n.type || 'task',
    summary: n.summary || '',
    agent: n.agent || 'Unassigned',
    status: n.status || 'pending',
    priority: n.priority || 'medium',
    parentId: n.parentId || null,
    origin: n.origin || null,
    reviewCycle: n.reviewCycle || 0,
    reviewDecision: n.reviewDecision || null,
    reason: n.reason || null,
    relationship: n.relationship || null,
    detail: n.detail || null,
  }));
  return inferType(nodes);
}

// --- Layout algorithms (unchanged) ---
function computeLayout(nodes, canvasWidth) {
  const byParent = {};
  nodes.forEach((n) => {
    const pid = n.parentId || '__root__';
    if (!byParent[pid]) byParent[pid] = [];
    byParent[pid].push(n);
  });

  const positions = {};

  function subtreeWidth(nodeId) {
    const children = byParent[nodeId] || [];
    if (children.length === 0) return NODE_WIDTH;
    const childWidths = children.map((c) => subtreeWidth(c.id));
    return childWidths.reduce((sum, w) => sum + w, 0) + (children.length - 1) * H_GAP;
  }

  function layout(nodeId, x, y) {
    positions[nodeId] = { x, y };
    const children = byParent[nodeId] || [];
    if (children.length === 0) return;
    const totalW = subtreeWidth(nodeId);
    let startX = x + NODE_WIDTH / 2 - totalW / 2;
    children.forEach((child) => {
      const sw = subtreeWidth(child.id);
      const cx = startX + sw / 2 - NODE_WIDTH / 2;
      layout(child.id, cx, y + NODE_HEIGHT + V_GAP);
      startX += sw + H_GAP;
    });
  }

  const root = nodes.find((n) => !n.parentId);
  if (root) {
    layout(root.id, canvasWidth / 2 - NODE_WIDTH / 2, 60);
  }
  return positions;
}

function buildEdges(nodes, positions) {
  const edges = [];
  nodes.forEach((n) => {
    if (n.parentId && positions[n.parentId] && positions[n.id]) {
      const parent = positions[n.parentId];
      const child = positions[n.id];
      edges.push({
        id: `edge-${n.parentId}-${n.id}`,
        from: n.parentId,
        to: n.id,
        x1: parent.x + NODE_WIDTH / 2,
        y1: parent.y + NODE_HEIGHT,
        x2: child.x + NODE_WIDTH / 2,
        y2: child.y,
        status: n.status,
      });
    }
  });
  return edges;
}

function applyNodes(nodes) {
  const positions = computeLayout(nodes, 1440);
  const edges = buildEdges(nodes, positions);
  return { nodes, positions, edges };
}

const initialLayout = applyNodes(emptyNodes);

export const useTaskTreeStore = create((set, get) => ({
  ...initialLayout,
  selectedNodeId: null,
  hoveredNodeId: null,
  isFromPlanning: false,
  isConverting: false,
  isLoading: true,

  // Load tree from server on init. Tries track tree first (live status), then plan tree.
  initFromServer: async () => {
    const planId = sessionStorage.getItem('samams_active_plan_id');
    console.log('[taskTree] initFromServer planId:', planId);
    if (!planId) {
      set({ isLoading: false });
      return;
    }

    // Try track tree first (has live milestone/task status).
    try {
      const resp = await http.get(endpoints.tracks.get(planId));
      console.log('[taskTree] Track response:', resp);
      const data = resp.data || resp;
      if (data?.nodes) {
        const nodes = parseTreeNodes(data);
        if (nodes) {
          const layout = applyNodes(nodes);
          set({ ...layout, isFromPlanning: true, isLoading: false });
          return;
        }
      }
    } catch (err) {
      console.warn('[taskTree] Track load failed:', err?.message || err);
    }

    // Fallback: plan tree (no live status).
    try {
      const { data } = await http.get(endpoints.plans.getTree(planId));
      const nodes = parseTreeNodes(data);
      if (nodes) {
        const layout = applyNodes(nodes);
        set({ ...layout, isFromPlanning: true, isLoading: false });
        return;
      }
    } catch (err) {
      console.warn('[taskTree] initFromServer failed:', err.message);
    }
    set({ isLoading: false });
  },

  zoom: 1,
  pan: { x: 0, y: 0 },
  NODE_WIDTH,
  NODE_HEIGHT,

  selectNode: (id) => set((s) => ({
    selectedNodeId: s.selectedNodeId === id ? null : id,
  })),
  setHoveredNode: (id) => set({ hoveredNodeId: id }),
  closeDetail: () => set({ selectedNodeId: null }),
  setZoom: (zoom) => set({ zoom: Math.max(0.3, Math.min(2.0, zoom)) }),
  zoomIn: () => set((s) => ({ zoom: Math.min(2.0, s.zoom + 0.1) })),
  zoomOut: () => set((s) => ({ zoom: Math.max(0.3, s.zoom - 0.1) })),
  setPan: (pan) => set({ pan }),

  fitView: () => {
    const { nodes, positions } = get();
    if (nodes.length === 0) return;
    let minX = Infinity, minY = Infinity, maxX = -Infinity, maxY = -Infinity;
    nodes.forEach((n) => {
      const p = positions[n.id];
      if (!p) return;
      minX = Math.min(minX, p.x);
      minY = Math.min(minY, p.y);
      maxX = Math.max(maxX, p.x + NODE_WIDTH);
      maxY = Math.max(maxY, p.y + NODE_HEIGHT);
    });
    const treeW = maxX - minX + 100;
    const treeH = maxY - minY + 100;
    const vw = window.innerWidth;
    const vh = window.innerHeight - 60;
    const zoom = Math.min(vw / treeW, vh / treeH, 1.5);
    set({
      zoom,
      pan: {
        x: (vw - treeW * zoom) / 2 - minX * zoom + 50,
        y: (vh - treeH * zoom) / 2 - minY * zoom + 50,
      },
    });
  },

  resetView: () => {
    const { nodes, positions } = get();
    const root = nodes.find((n) => !n.parentId);
    if (!root || !positions[root.id]) return;
    set({
      zoom: 1,
      pan: { x: window.innerWidth / 2 - positions[root.id].x - NODE_WIDTH / 2, y: 30 },
    });
  },

  _syncCounter: 0,

  // Sync tree from server — updates statuses (milestone, proposal) AND detects new nodes.
  syncTreeStructure: async () => {
    const planId = sessionStorage.getItem('samams_active_plan_id');
    if (!planId) return;
    try {
      const { data } = await http.get(endpoints.tracks.get(planId));
      if (!data?.nodes) return;
      const { nodes: currentNodes } = get();

      // Always update statuses from tracks (milestone/proposal statuses live here).
      const nodes = parseTreeNodes(data);
      if (nodes) {
        // Merge: keep proxy-synced statuses for tasks, but take milestone/proposal from tracks.
        const merged = nodes.map((trackNode) => {
          const existing = currentNodes.find((c) => c.id === trackNode.id);
          if (existing && trackNode.type === 'task') {
            // Tasks: prefer proxy-synced status (more up-to-date).
            return { ...trackNode, status: existing.status || trackNode.status };
          }
          // Milestones/proposals: always use tracks status.
          return trackNode;
        });
        const layout = applyNodes(merged);
        set({ ...layout });
        console.log(`[taskTree] Tree structure updated: ${currentNodes.length} → ${data.nodes.length} nodes`);
      }
    } catch (err) {
      // Structure sync is best-effort.
    }
  },

  // Deprecated: use initFromServer() or loadFromServer() instead.
  loadFromPlanning: () => {
    console.warn('[taskTree] loadFromPlanning is deprecated — use initFromServer()');
    return false;
  },

  // Sync node statuses from running proxy tasks.
  // Maps proxy task status → tree node status.
  syncFromProxy: async () => {
    try {
      const { data } = await http.get(endpoints.run.tasks);
      const proxyTasks = data.tasks || [];
      if (proxyTasks.length === 0) return;

      const { nodes } = get();
      const statusMap = {};
      for (const pt of proxyTasks) {
        if (!pt.nodeUid) continue;
        // Map proxy status to tree-node status
        let status = 'pending';
        switch (pt.status) {
          case 'running': case 'scaling': status = 'active'; break;
          case 'paused': status = 'paused'; break;
          case 'done': case 'stopped': status = 'complete'; break;
          case 'error': case 'cancelled': status = 'error'; break;
          default: status = 'active';
        }
        statusMap[pt.nodeUid] = { status, branchName: pt.branchName || '' };
      }

      const updated = nodes.map((n) => {
        const match = statusMap[n.uid];
        if (match) {
          return { ...n, status: match.status, branchName: match.branchName };
        }
        return n;
      });

      const layout = applyNodes(updated);
      set({ ...layout });

      // Sync tree structure every cycle (milestone/proposal statuses + new nodes).
      get().syncTreeStructure();

      // Also update tracks on server for persistence.
      const planId = sessionStorage.getItem('samams_active_plan_id');
      if (planId && Object.keys(statusMap).length > 0) {
        for (const [uid, info] of Object.entries(statusMap)) {
          http.post(endpoints.tracks.updateNodeStatus(planId, uid), {
            status: info.status,
            branchName: info.branchName,
          }).catch(() => {}); // best-effort
        }
      }
    } catch (err) {
      console.warn('[taskTree] syncFromProxy failed (proxy may not be connected):', err.message);
    }
  },

  // Load AI-generated tree from server.
  loadFromServer: async () => {
    const planId = sessionStorage.getItem('samams_active_plan_id');
    if (!planId) return false;

    set({ isConverting: true });
    try {
      // Get the plan document from server.
      const { data: doc } = await http.get(endpoints.plans.get(planId));
      if (!doc) {
        set({ isConverting: false });
        return false;
      }

      const { data } = await http.post(endpoints.ai.convertToTree, {
        plan_document: JSON.stringify(doc),
      });
      let tree = data.tree;
      tree = tree.replace(/^```json\n?/, '').replace(/\n?```$/, '');
      const parsed = JSON.parse(tree);

      // Save tree to server.
      await http.post(endpoints.plans.saveTree(planId), parsed);

      const nodes = parseTreeNodes(parsed);
      if (nodes) {
        const layout = applyNodes(nodes);
        set({ ...layout, isFromPlanning: true, isConverting: false, selectedNodeId: null });
        return true;
      }
      set({ isConverting: false });
      return false;
    } catch (err) {
      set({ isConverting: false });
      console.error('Tree conversion failed:', err);
      return false;
    }
  },

  getSelectedNode: () => {
    const { nodes, selectedNodeId } = get();
    if (!selectedNodeId) return null;
    const node = nodes.find((n) => n.id === selectedNodeId);
    if (!node) return null;
    const children = nodes.filter((n) => n.parentId === selectedNodeId);
    const depth = getDepth(nodes, selectedNodeId);
    return { ...node, childCount: children.length, depth };
  },
}));

function getDepth(nodes, nodeId) {
  let depth = 0;
  let current = nodes.find((n) => n.id === nodeId);
  while (current && current.parentId) {
    depth++;
    current = nodes.find((n) => n.id === current.parentId);
  }
  return depth;
}
