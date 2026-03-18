import { create } from 'zustand';
import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';

// Server-backed storage via /plans API (replaces localStorage).

async function loadDocumentFromServer() {
  try {
    // Load the last active plan ID from sessionStorage (lightweight pointer).
    const activeId = sessionStorage.getItem('samams_active_plan_id');
    if (!activeId) return null;
    const { data } = await http.get(endpoints.plans.get(activeId));
    return data;
  } catch {
    return null;
  }
}

async function loadPlanListFromServer() {
  try {
    const { data } = await http.get(endpoints.plans.list);
    return Array.isArray(data) ? data : [];
  } catch {
    return [];
  }
}

async function savePlanToServer(doc) {
  try {
    await http.post(endpoints.plans.save, doc);
  } catch (err) {
    console.error('[planning] Save to server failed:', err);
  }
}

async function deletePlanFromServer(id) {
  try {
    await http.delete(endpoints.plans.remove(id));
  } catch (err) {
    console.error('[planning] Delete from server failed:', err);
  }
}

function createEmptyDocument() {
  const now = new Date().toISOString();
  return {
    id: Date.now().toString(),
    title: '',
    goal: '',
    description: '',
    features: [],
    techSpec: {
      techStack: '',
      architecture: '',
      folderStructure: '',
      framework: '',
      codingConventions: '',
      boundedContexts: '',
    },
    abstractSpec: {
      domainOverview: '',
      aggregates: '',
      events: '',
      workflows: '',
    },
    createdAt: now,
    updatedAt: now,
  };
}

const initialMessage = {
  id: 'init-1',
  sender: 'sentinel',
  content: `SENTINEL AI Planning Mode Activated\nI'll help you plan your project. Provide the following:\n1. Project Title\n2. Main Goal\n3. Feature List\n4. Priority\n\nChat freely and it will be reflected in the document automatically.\n\nUse quick commands to generate each section with AI.`,
  timestamp: new Date().toISOString(),
};

export const usePlanningStore = create((set, get) => ({
  document: createEmptyDocument(),
  isSaved: false,
  showConvertButton: false,
  isLoading: true,

  planList: [],
  showPlanList: false,

  // Load plans from server on init.
  initFromServer: async () => {
    const [doc, list] = await Promise.all([
      loadDocumentFromServer(),
      loadPlanListFromServer(),
    ]);
    set({
      document: doc || createEmptyDocument(),
      isSaved: !!doc,
      showConvertButton: !!doc,
      planList: list,
      isLoading: false,
    });
  },

  togglePlanList: () => set((s) => ({ showPlanList: !s.showPlanList })),

  loadPlan: (id) => {
    const list = get().planList;
    const plan = list.find((p) => p.id === id);
    if (plan) {
      sessionStorage.setItem('samams_active_plan_id', plan.id);
      set({ document: plan, isSaved: true, showConvertButton: true, showPlanList: false });
    }
  },

  deletePlan: async (id) => {
    const list = get().planList.filter((p) => p.id !== id);
    await deletePlanFromServer(id);
    set({ planList: list });
  },

  newPlan: () => {
    const doc = createEmptyDocument();
    sessionStorage.removeItem('samams_active_plan_id');
    set({ document: doc, isSaved: false, showConvertButton: false, showPlanList: false });
  },

  messages: [initialMessage],
  chatInput: '',
  isTyping: false,

  isGeneratingPlan: false,
  isConvertingTree: false,

  setTitle: (title) => set((s) => ({
    document: { ...s.document, title, updatedAt: new Date().toISOString() },
    isSaved: false,
  })),

  setGoal: (goal) => set((s) => ({
    document: { ...s.document, goal, updatedAt: new Date().toISOString() },
    isSaved: false,
  })),

  setDescription: (description) => set((s) => ({
    document: { ...s.document, description, updatedAt: new Date().toISOString() },
    isSaved: false,
  })),

  updateTechSpec: (field, value) => set((s) => ({
    document: {
      ...s.document,
      techSpec: { ...(s.document.techSpec || {}), [field]: value },
      updatedAt: new Date().toISOString(),
    },
    isSaved: false,
  })),

  updateAbstractSpec: (field, value) => set((s) => ({
    document: {
      ...s.document,
      abstractSpec: { ...(s.document.abstractSpec || {}), [field]: value },
      updatedAt: new Date().toISOString(),
    },
    isSaved: false,
  })),

  addFeature: () => set((s) => ({
    document: {
      ...s.document,
      features: [
        ...s.document.features,
        { id: `feature-${Date.now()}`, name: '', description: '', priority: 'medium', details: [] },
      ],
      updatedAt: new Date().toISOString(),
    },
    isSaved: false,
  })),

  updateFeature: (id, updates) => set((s) => ({
    document: {
      ...s.document,
      features: s.document.features.map((f) => f.id === id ? { ...f, ...updates } : f),
      updatedAt: new Date().toISOString(),
    },
    isSaved: false,
  })),

  removeFeature: (id) => set((s) => ({
    document: {
      ...s.document,
      features: s.document.features.filter((f) => f.id !== id),
      updatedAt: new Date().toISOString(),
    },
    isSaved: false,
  })),

  addDetail: (featureId) => set((s) => ({
    document: {
      ...s.document,
      features: s.document.features.map((f) =>
        f.id === featureId ? { ...f, details: [...f.details, ''] } : f
      ),
      updatedAt: new Date().toISOString(),
    },
    isSaved: false,
  })),

  updateDetail: (featureId, index, value) => set((s) => ({
    document: {
      ...s.document,
      features: s.document.features.map((f) =>
        f.id === featureId
          ? { ...f, details: f.details.map((d, i) => (i === index ? value : d)) }
          : f
      ),
      updatedAt: new Date().toISOString(),
    },
    isSaved: false,
  })),

  removeDetail: (featureId, index) => set((s) => ({
    document: {
      ...s.document,
      features: s.document.features.map((f) =>
        f.id === featureId
          ? { ...f, details: f.details.filter((_, i) => i !== index) }
          : f
      ),
      updatedAt: new Date().toISOString(),
    },
    isSaved: false,
  })),

  saveDocument: async () => {
    const { document, planList } = get();
    const updated = { ...document, updatedAt: new Date().toISOString() };

    // Save to server.
    await savePlanToServer(updated);
    sessionStorage.setItem('samams_active_plan_id', updated.id);

    const idx = planList.findIndex((p) => p.id === updated.id);
    let newList;
    if (idx >= 0) {
      newList = planList.map((p) => (p.id === updated.id ? updated : p));
    } else {
      newList = [updated, ...planList];
    }
    set({ document: updated, isSaved: true, showConvertButton: true, planList: newList });
  },

  setChatInput: (chatInput) => set({ chatInput }),

  sendChat: async (text) => {
    if (!text.trim()) return;
    const trimmed = text.trim();
    const userMsg = {
      id: `msg-${Date.now()}`,
      sender: 'user',
      content: trimmed,
      timestamp: new Date().toISOString(),
    };
    set((s) => ({ messages: [...s.messages, userMsg], chatInput: '', isTyping: true }));

    const lower = trimmed.toLowerCase();

    if (lower.includes('ai plan') || lower.includes('ai generate') || lower.includes('auto plan') || lower.includes('generate plan')) {
      await generatePlanFromServer(trimmed, get, set);
      return;
    }

    if (lower.startsWith('goal:') || lower.startsWith('goal :')) {
      await generateFieldFromServer('goal', get, set);
      return;
    }

    if (lower.startsWith('description:') || lower.startsWith('description :') || lower.includes('description')) {
      await generateDescriptionFromServer(get, set);
      return;
    }

    if (lower.startsWith('features:') || lower.startsWith('features :') || lower.startsWith('feature list:')) {
      await generateFieldFromServer('features', get, set);
      return;
    }

    if (lower.includes('tech spec')) {
      await generateSpecFromServer('tech', get, set);
      return;
    }

    if (lower.includes('abstract')) {
      await generateSpecFromServer('abstract', get, set);
      return;
    }

    if (lower.includes('tree') || lower.includes('node tree') || lower.includes('convert')) {
      await convertToTreeFromServer(get, set);
      return;
    }

    // Title: local only
    parseAndApply(trimmed, get, set);

    // Fallback AI chat (conversational via Claude)
    try {
      const doc = get().document;
      const ctx = `title="${doc.title}", goal="${doc.goal}", ${doc.features.length} features`;
      const { data } = await http.post(endpoints.ai.chat, {
        message: trimmed,
        context: ctx,
      });
      addAIMessage(data.reply, set);
    } catch {
      const response = generateLocalResponse(trimmed, get().document);
      addAIMessage(response, set);
    }
  },

  clearChat: () => set({ messages: [initialMessage] }),

  generatePlanWithAI: async (prompt) => {
    const userMsg = {
      id: `msg-${Date.now()}`,
      sender: 'user',
      content: `AI Plan Generation: ${prompt}`,
      timestamp: new Date().toISOString(),
    };
    set((s) => ({ messages: [...s.messages, userMsg], isTyping: true }));
    await generatePlanFromServer(prompt, get, set);
  },

  convertToTreeWithAI: async () => {
    const userMsg = {
      id: `msg-${Date.now()}`,
      sender: 'user',
      content: 'Convert to Node Tree',
      timestamp: new Date().toISOString(),
    };
    set((s) => ({ messages: [...s.messages, userMsg], isTyping: true }));
    await convertToTreeFromServer(get, set);
  },
}));

// --- Build context excluding a specific field ---
function buildContext(doc, exclude) {
  const fields = [];
  if (exclude !== 'title' && doc.title) fields.push(`Project Title: ${doc.title}`);
  if (exclude !== 'goal' && doc.goal) fields.push(`Main Goal: ${doc.goal}`);
  if (exclude !== 'description' && doc.description) fields.push(`Description: ${doc.description}`);
  if (exclude !== 'features' && doc.features.length) fields.push(`Features: ${doc.features.map((f) => `${f.name} (${f.priority})`).join(', ')}`);
  if (exclude !== 'techSpec') {
    if (doc.techSpec?.techStack) fields.push(`Tech Stack: ${doc.techSpec.techStack}`);
    if (doc.techSpec?.architecture) fields.push(`Architecture: ${doc.techSpec.architecture}`);
    if (doc.techSpec?.folderStructure) fields.push(`Folder Structure: ${doc.techSpec.folderStructure}`);
    if (doc.techSpec?.framework) fields.push(`Framework: ${doc.techSpec.framework}`);
    if (doc.techSpec?.codingConventions) fields.push(`Coding Conventions: ${doc.techSpec.codingConventions}`);
    if (doc.techSpec?.boundedContexts) fields.push(`Bounded Contexts: ${doc.techSpec.boundedContexts}`);
  }
  if (exclude !== 'abstractSpec') {
    if (doc.abstractSpec?.domainOverview) fields.push(`Domain Overview: ${doc.abstractSpec.domainOverview}`);
    if (doc.abstractSpec?.aggregates) fields.push(`Aggregates: ${doc.abstractSpec.aggregates}`);
    if (doc.abstractSpec?.events) fields.push(`Events: ${doc.abstractSpec.events}`);
    if (doc.abstractSpec?.workflows) fields.push(`Workflows: ${doc.abstractSpec.workflows}`);
  }
  return fields.length > 0 ? fields.join('\n') : '(no information available yet)';
}

// --- Server AI: Generate Plan ---
async function generatePlanFromServer(prompt, get, set) {
  set({ isGeneratingPlan: true });
  try {
    addAIMessage('Generating plan... (Claude AI processing)', set);

    const { data } = await http.post(endpoints.ai.generatePlan, { prompt });
    let plan = data.plan;
    plan = plan.replace(/^```json\n?/, '').replace(/\n?```$/, '');

    const parsed = JSON.parse(plan);
    const now = new Date().toISOString();

    const features = (parsed.features || []).map((f, i) => ({
      id: `feature-${Date.now()}-${i}`,
      name: f.name || '',
      description: f.description || '',
      priority: f.priority || 'medium',
      details: f.details || [],
    }));

    const stringify = (v) => {
      if (v == null) return '';
      if (typeof v === 'string') {
        if (v.includes('/, ') || v.includes(') ,') || /\w+\/\s*\([^)]+\),/.test(v)) {
          const parts = v.split(/,\s*/).map((p) => p.trim()).filter(Boolean);
          if (parts.length > 2) {
            return parts.map((p) => {
              const m = p.match(/^(.+?\/)\s*\((.+)\)$/);
              if (m) return `├── ${m[1].padEnd(20)} — ${m[2]}`;
              return `├── ${p}`;
            }).join('\n');
          }
        }
        return v;
      }
      if (Array.isArray(v)) {
        return v.map((item) => {
          if (typeof item === 'string') return `- ${item}`;
          if (typeof item === 'object') {
            return Object.entries(item).map(([k, val]) => `  ${k}: ${typeof val === 'object' ? JSON.stringify(val) : val}`).join('\n');
          }
          return String(item);
        }).join('\n\n');
      }
      if (typeof v === 'object') {
        return Object.entries(v).map(([k, val]) => `${k}: ${typeof val === 'object' ? JSON.stringify(val) : val}`).join('\n');
      }
      return String(v);
    };

    const techSpec = parsed.techSpec ? {
      techStack: stringify(parsed.techSpec.techStack),
      architecture: stringify(parsed.techSpec.architecture),
      folderStructure: stringify(parsed.techSpec.folderStructure),
      framework: stringify(parsed.techSpec.framework),
      codingConventions: stringify(parsed.techSpec.codingConventions),
      boundedContexts: stringify(parsed.techSpec.boundedContexts),
    } : get().document.techSpec;

    const abstractSpec = parsed.abstractSpec ? {
      domainOverview: stringify(parsed.abstractSpec.domainOverview),
      aggregates: stringify(parsed.abstractSpec.aggregates),
      events: stringify(parsed.abstractSpec.events),
      workflows: stringify(parsed.abstractSpec.workflows),
    } : get().document.abstractSpec;

    const document = {
      ...get().document,
      title: parsed.title || get().document.title,
      goal: parsed.goal || get().document.goal,
      description: parsed.description || get().document.description || '',
      features,
      techSpec,
      abstractSpec,
      ...(parsed.structuredSkeleton ? { structuredSkeleton: parsed.structuredSkeleton } : {}),
      updatedAt: now,
    };

    set({ document, isSaved: false, isGeneratingPlan: false });

    addAIMessage(
      `Plan generated by AI!\n\n` +
      `Title: ${document.title}\n` +
      `Goal: ${document.goal}\n` +
      `Features: ${features.length} created\n\n` +
      `Review and edit in the editor panel.\nSave when ready.`,
      set
    );
  } catch (err) {
    set({ isGeneratingPlan: false });
    addAIMessage(`Plan generation failed: ${err.message || 'Server connection error'}`, set);
  }
}

// --- Server AI: Generate Field (goal / features) ---
async function generateFieldFromServer(field, get, set) {
  const doc = get().document;
  const labels = { goal: 'Goal', features: 'Feature List' };
  const label = labels[field];

  set({ isGeneratingPlan: true });
  try {
    addAIMessage(`Generating ${label}... (Claude AI processing)`, set);

    const ctx = buildContext(doc, field);
    let promptText;
    if (field === 'goal') {
      promptText = `Based on the following project information, generate the main goal.\n\n${ctx}\n\nOutput JSON: {"goal": "the goal text"}\nOutput valid JSON only.`;
    } else {
      promptText = `Based on the following project information, generate a comprehensive feature list.\n\n${ctx}\n\nOutput JSON:\n{"features": [{"name": "feature name", "description": "what it does", "priority": "high|medium|low", "details": ["sub-task 1", "sub-task 2"]}]}\nGenerate 5-10 key features with sub-tasks. Output valid JSON only.`;
    }

    const { data } = await http.post(endpoints.ai.generatePlan, { prompt: promptText });
    let plan = data.plan;
    plan = plan.replace(/^```json\n?/, '').replace(/\n?```$/, '');
    const parsed = JSON.parse(plan);

    if (field === 'goal') {
      set((s) => ({
        document: { ...s.document, goal: parsed.goal || '', updatedAt: new Date().toISOString() },
        isSaved: false, isGeneratingPlan: false,
      }));
      addAIMessage(`Goal generated!\nReview in the editor panel.`, set);
    } else {
      const features = (parsed.features || []).map((f, i) => ({
        id: `feature-${Date.now()}-${i}`,
        name: f.name || '', description: f.description || '',
        priority: f.priority || 'medium', details: f.details || [],
      }));
      set((s) => ({
        document: { ...s.document, features, updatedAt: new Date().toISOString() },
        isSaved: false, isGeneratingPlan: false,
      }));
      addAIMessage(`${features.length} features generated!\nReview in the editor panel.`, set);
    }
  } catch (err) {
    set({ isGeneratingPlan: false });
    addAIMessage(`${label} generation failed: ${err.message || 'Server connection error'}`, set);
  }
}

// --- Server AI: Generate Description ---
async function generateDescriptionFromServer(get, set) {
  const doc = get().document;
  set({ isGeneratingPlan: true });
  try {
    addAIMessage('Generating description... (Claude AI processing)', set);

    const ctx = buildContext(doc, 'description');
    const { data } = await http.post(endpoints.ai.generatePlan, {
      prompt: `Based on the following project information, generate a detailed project description.\n\n${ctx}\n\nInclude:\n- System overview and core concepts\n- Target users\n- Key technical requirements\n- Business rules and constraints\n- Expected scale and performance requirements\n\nOutput JSON format: {"description": "the full description text"}\nOutput valid JSON only.`,
    });

    let plan = data.plan;
    plan = plan.replace(/^```json\n?/, '').replace(/\n?```$/, '');
    const parsed = JSON.parse(plan);
    const desc = parsed.description || (typeof parsed === 'string' ? parsed : JSON.stringify(parsed, null, 2));

    set((s) => ({
      document: { ...s.document, description: desc, updatedAt: new Date().toISOString() },
      isSaved: false, isGeneratingPlan: false,
    }));
    addAIMessage(`Description generated!\nReview in the editor panel.`, set);
  } catch (err) {
    set({ isGeneratingPlan: false });
    addAIMessage(`Description generation failed: ${err.message || 'Server connection error'}`, set);
  }
}

// --- Server AI: Generate Tech/Abstract Spec ---
async function generateSpecFromServer(type, get, set) {
  const doc = get().document;
  const isTech = type === 'tech';
  const label = isTech ? 'Tech Spec' : 'Abstract Spec';

  set({ isGeneratingPlan: true });
  try {
    addAIMessage(`Generating ${label}... (Claude AI processing)`, set);

    const ctx = buildContext(doc, isTech ? 'techSpec' : 'abstractSpec');
    let specPrompt;
    if (isTech) {
      specPrompt = `Based on the following project information, generate a Technical Specification.\n\n${ctx}\n\nOutput JSON format:\n{"techStack":"...","architecture":"...","folderStructure":"use tree characters for hierarchy (display only)","framework":"...","codingConventions":"...","boundedContexts":"...","structuredSkeleton":{"module":{"type":"go|node|python","name":"module name"},"files":[{"path":"cmd/server/main.go","purpose":"Server entry point"}]}}\nAll values must be strings EXCEPT structuredSkeleton which is an object.\nFor folderStructure use tree-drawing characters (for display).\nFor structuredSkeleton.files list EVERY file with exact relative path and purpose.\nFor boundedContexts describe each context with name, responsibility, aggregates.\nOutput valid JSON only.`;
    } else {
      specPrompt = `Based on the following project information, generate an Abstract/Domain Specification.\n\n${ctx}\n\nOutput JSON format:\n{"domainOverview":"...","aggregates":"...","events":"...","workflows":"..."}\nAll values must be strings. Describe domain concepts, aggregate roots, domain events (name, trigger, effect), and key workflows step-by-step. Output valid JSON only.`;
    }

    const { data } = await http.post(endpoints.ai.generatePlan, { prompt: specPrompt });
    let plan = data.plan;
    plan = plan.replace(/^```json\n?/, '').replace(/\n?```$/, '');
    const parsed = JSON.parse(plan);

    // Claude may return flat fields or nested under techSpec/abstractSpec key.
    const resolve = (...keys) => {
      for (const k of keys) {
        const v = parsed[k] ?? parsed.techSpec?.[k] ?? parsed.abstractSpec?.[k];
        if (v != null) {
          if (typeof v === 'string') return v;
          if (Array.isArray(v)) return v.map((item) => typeof item === 'object' ? JSON.stringify(item) : String(item)).join('\n');
          if (typeof v === 'object') return Object.entries(v).map(([k2, v2]) => `${k2}: ${typeof v2 === 'object' ? JSON.stringify(v2) : v2}`).join('\n');
          return String(v);
        }
      }
      return '';
    };

    if (isTech) {
      // Extract structuredSkeleton separately (it's an object, not a string).
      const skeleton = parsed.structuredSkeleton || parsed.structured_skeleton || null;
      set((s) => ({
        document: {
          ...s.document,
          techSpec: {
            techStack: resolve('techStack', 'tech_stack'),
            architecture: resolve('architecture'),
            folderStructure: resolve('folderStructure', 'folder_structure'),
            framework: resolve('framework'),
            codingConventions: resolve('codingConventions', 'coding_conventions'),
            boundedContexts: resolve('boundedContexts', 'bounded_contexts'),
          },
          ...(skeleton ? { structuredSkeleton: skeleton } : {}),
          updatedAt: new Date().toISOString(),
        },
        isSaved: false, isGeneratingPlan: false,
      }));
    } else {
      set((s) => ({
        document: {
          ...s.document,
          abstractSpec: {
            domainOverview: resolve('domainOverview', 'domain_overview'),
            aggregates: resolve('aggregates'),
            events: resolve('events'),
            workflows: resolve('workflows'),
          },
          updatedAt: new Date().toISOString(),
        },
        isSaved: false, isGeneratingPlan: false,
      }));
    }

    addAIMessage(`${label} generated!\nReview in the editor panel.`, set);
  } catch (err) {
    set({ isGeneratingPlan: false });
    addAIMessage(`${label} generation failed: ${err.message || 'Server connection error'}`, set);
  }
}

// --- Server AI: Convert to Tree ---
async function convertToTreeFromServer(get, set) {
  const { document } = get();
  if (!document.title && document.features.length === 0) {
    addAIMessage('Document is empty. Please write a plan first.', set);
    return;
  }

  set({ isConvertingTree: true });
  try {
    addAIMessage('Converting to Node Tree... (Claude AI processing)', set);

    const planDoc = JSON.stringify(document);
    const { data } = await http.post(endpoints.ai.convertToTree, { plan_document: planDoc });
    let tree = data.tree;

    tree = tree.replace(/^```json\n?/, '').replace(/\n?```$/, '');
    const parsed = JSON.parse(tree);

    // Save tree to server.
    const planId = get().document.id;
    try {
      await http.post(endpoints.plans.saveTree(planId), parsed);
    } catch (err) {
      console.error('[planning] Save tree to server failed:', err);
    }
    set({ isConvertingTree: false });

    const nodeCount = parsed.nodes?.length || 0;
    addAIMessage(
      `Node Tree conversion complete!\n\n` +
      `${nodeCount} nodes created\n\n` +
      `View at Task Tree page.\n-> /task-tree`,
      set
    );
  } catch (err) {
    set({ isConvertingTree: false });
    addAIMessage(`Tree conversion failed: ${err.message || 'Server connection error'}`, set);
  }
}

function addAIMessage(content, set) {
  const aiMsg = {
    id: `msg-${Date.now()}-${Math.random().toString(36).slice(2, 5)}`,
    sender: 'sentinel',
    content,
    timestamp: new Date().toISOString(),
  };
  set((s) => ({ messages: [...s.messages, aiMsg], isTyping: false }));
}

function parseAndApply(text, get, set) {
  const titleMatch = text.match(/(?:title|project)\s*[:：]\s*(.+)/i);
  if (titleMatch) {
    set((s) => ({
      document: { ...s.document, title: titleMatch[1].trim(), updatedAt: new Date().toISOString() },
      isSaved: false,
    }));
  }
}

function generateLocalResponse(input, doc) {
  const lower = input.toLowerCase();
  if (lower.includes('title') || lower.includes('project')) {
    return `Project title set to "${doc.title || '(not set)'}".`;
  }
  if (lower.includes('goal')) {
    return `Goal recorded.`;
  }
  if (lower.includes('feature')) {
    return `Feature list updated. ${doc.features.length} registered.`;
  }
  if (lower.includes('help') || lower.includes('command')) {
    return `Commands:\n• Title: [title] — Set title\n• Goal: [trigger] — AI generate goal\n• Description: [trigger] — AI generate description\n• Features: [trigger] — AI generate features\n• Tech Spec [trigger] — AI generate tech spec\n• Abstract [trigger] — AI generate abstract spec\n• Tree / Convert — Convert to Node Tree\n• Help — This list`;
  }
  return `Input received. Type "help" for available commands.`;
}
