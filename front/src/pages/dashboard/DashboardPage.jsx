import { useState, useEffect, useCallback, useRef } from 'react';
import { useDashboardStore } from '../../features/dashboard/store/dashboardStore';
import { useAuthStore } from '../../features/auth/store/authStore';
import { DashHeader } from '../../features/dashboard/components/DashHeader';
import { TopStatusBar } from '../../features/dashboard/components/TopStatusBar';
import { AgentCard } from '../../features/dashboard/components/AgentCard';
import { MaalLogStream } from '../../features/dashboard/components/MaalLogStream';
import { GlobalControls } from '../../features/dashboard/components/GlobalControls';
import { StrategyMeetingModal } from '../../features/dashboard/components/StrategyMeetingModal';
import { SentinelChatPanel } from '../../features/dashboard/components/SentinelChatPanel';
import http from '../../shared/api/http';
import { endpoints } from '../../shared/api/endpoints';
import styles from './DashboardPage.module.css';

const MAX_AGENTS = 6;

export function DashboardPage() {
  const agents = useDashboardStore((s) => s.agents);
  const hasActiveTasks = useDashboardStore((s) => s.hasActiveTasks);
  const startPolling = useDashboardStore((s) => s.startPolling);
  const stopPolling = useDashboardStore((s) => s.stopPolling);
  const user = useAuthStore((s) => s.user);
  const hasAgents = agents.length > 0;

  // Poll proxy for running agents
  useEffect(() => {
    startPolling();
    return () => stopPolling();
  }, [startPolling, stopPolling]);

  const [planList, setPlanList] = useState([]);
  const [resumableList, setResumableList] = useState([]);
  const [plansLoading, setPlansLoading] = useState(true);
  const [showResumeDropdown, setShowResumeDropdown] = useState(false);
  const [showDropdown, setShowDropdown] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState(null);
  const [isRunning, setIsRunning] = useState(false);
  const [runningPlanId, setRunningPlanId] = useState(null);
  const [progress, setProgress] = useState(null);
  const dropdownRef = useRef(null);

  useEffect(() => {
    async function loadPlans() {
      try {
        const { data } = await http.get(endpoints.plans.list);
        const list = (Array.isArray(data) ? data : []).filter((p) => p.title || p.goal).slice(0, 6);
        setPlanList(list);
      } catch (err) {
        console.error('[DashboardPage] Failed to load plans:', err);
      } finally {
        setPlansLoading(false);
      }
    }
    async function loadResumable() {
      try {
        const { data } = await http.get(endpoints.run.resumable);
        setResumableList(data.resumable || []);
      } catch (err) {
        console.error('[DashboardPage] Failed to load resumable:', err);
      }
    }
    loadPlans();
    loadResumable();

    // Check progress on page load (survives refresh).
    async function checkInitialProgress() {
      try {
        const { data } = await http.get(endpoints.run.progress);
        if (data.stage && data.stage !== 'idle' && data.stage !== 'running') {
          setProgress(data);
          setIsRunning(true);
        }
      } catch {}
    }
    checkInitialProgress();
  }, []);

  // Poll progress while preparing (skeleton → Claude → dispatch).
  useEffect(() => {
    if (!isRunning && !progress) return;
    const timer = setInterval(async () => {
      try {
        const { data } = await http.get(endpoints.run.progress);
        if (data.stage && data.stage !== 'idle') {
          setProgress(data);
        } else {
          setProgress(null);
        }
        if (data.stage === 'running') {
          setIsRunning(false);
          clearInterval(timer);
        }
      } catch {
        // ignore
      }
    }, 2000);
    return () => clearInterval(timer);
  }, [isRunning, progress]);

  // Close dropdown on outside click
  useEffect(() => {
    function handleClickOutside(e) {
      if (dropdownRef.current && !dropdownRef.current.contains(e.target)) {
        setShowDropdown(false);
        setSelectedPlan(null);
        setShowResumeDropdown(false);
      }
    }
    if (showDropdown) {
      document.addEventListener('mousedown', handleClickOutside);
      return () => document.removeEventListener('mousedown', handleClickOutside);
    }
  }, [showDropdown]);

  const hasPlan = planList.length > 0;

  const handleSelectPlan = useCallback((plan) => {
    setSelectedPlan(plan);
  }, []);

  const [addingAgent, setAddingAgent] = useState(false);
  const handleAddAgent = useCallback(async () => {
    setAddingAgent(true);
    try {
      const { data } = await http.post(endpoints.run.addAgent, {});
      console.log('[Add Agent]', data);
    } catch (err) {
      console.error('[Add Agent] Failed:', err);
    } finally {
      setAddingAgent(false);
    }
  }, []);

  const handleSelectAgentCount = useCallback(async (count) => {
    if (!selectedPlan) return;
    setShowDropdown(false);
    setIsRunning(true);
    setRunningPlanId(selectedPlan.id);

    try {
      // 1. Try to load existing tree first. Only generate if none exists.
      let parsed = null;
      try {
        const { data: existingTree } = await http.get(endpoints.plans.getTree(selectedPlan.id));
        if (existingTree?.nodes && existingTree.nodes.length > 0) {
          parsed = existingTree;
          // console.log('[Run SAMAMS] Using existing tree (%d nodes)', parsed.nodes.length);
        }
      } catch {
        // No existing tree — will generate below.
      }

      if (!parsed) {
        // Generate tree via AI.
        // console.log('[Run SAMAMS] No existing tree, generating via AI...');
        const planDoc = JSON.stringify(selectedPlan);
        const { data: treeData } = await http.post(endpoints.ai.convertToTree, { plan_document: planDoc });
        let tree = treeData.tree;
        tree = tree.replace(/^```json\n?/, '').replace(/\n?```$/, '');
        parsed = JSON.parse(tree);
      }

      if (!parsed.nodes || parsed.nodes.length === 0) {
        alert('Tree is empty.');
        setIsRunning(false);
        setRunningPlanId(null);
        setSelectedPlan(null);
        return;
      }

      // Save tree to server (idempotent).
      // Track ID = sanitized project name (must match server's sanitizeTrackID).
      // Server: alphanumeric only, others → '-', no leading/trailing '-', max 50 chars.
      const trackId = (selectedPlan.title || 'untitled')
        .replace(/[^a-zA-Z0-9]+/g, '-').replace(/^-|-$/g, '').slice(0, 50).replace(/-$/, '').toLowerCase();
      try {
        await http.post(endpoints.plans.saveTree(selectedPlan.id), parsed);
        sessionStorage.setItem('samams_active_plan_id', trackId);
      } catch (err) {
        // console.error('[Run SAMAMS] Save tree failed:', err);
      }

      // 2. Dispatch to proxy with max_agents limit + plan spec for setup agent
      const { data } = await http.post(endpoints.run.start, {
        nodes: parsed.nodes,
        max_agents: count,
        project_name: selectedPlan.title || 'Untitled Project',
        plan: {
          title: selectedPlan.title || '',
          goal: selectedPlan.goal || '',
          description: selectedPlan.description || '',
          techSpec: selectedPlan.techSpec || {},
          abstractSpec: selectedPlan.abstractSpec || {},
          structuredSkeleton: selectedPlan.structuredSkeleton || null,
        },
      });
      // console.log('[Run SAMAMS]', data);
      // Don't set isRunning=false here — progress polling will handle it
      // when stage becomes "running" (agents dispatched).
      setRunningPlanId(null);
      setSelectedPlan(null);
    } catch (err) {
      // console.error('[Run SAMAMS] Failed:', err);
      alert('Failed to start: ' + (err.message || 'Unknown error'));
      setIsRunning(false);
      setRunningPlanId(null);
      setSelectedPlan(null);
    }
  }, [selectedPlan]);

  return (
    <div className={styles.page}>
      <DashHeader />
      <TopStatusBar />

      <div className={styles.mainContent}>
        {/* Top bar: always visible */}
        <div className={styles.agentSection}>
          <div className={styles.agentSectionHeader}>
            {/* Progress banner during preparation */}
            {(isRunning || (progress && progress.stage !== 'idle' && progress.stage !== 'running')) && (
              <div style={{ width: '100%', padding: '12px 16px', marginBottom: 8, background: 'rgba(0,245,160,0.08)', border: '1px solid rgba(0,245,160,0.2)', borderRadius: 8, display: 'flex', alignItems: 'center', gap: 10 }}>
                <div style={{ width: 16, height: 16, border: '2.5px solid rgba(0,245,160,0.3)', borderTopColor: 'var(--color-primary)', borderRadius: '50%', animation: 'spin 0.8s linear infinite', flexShrink: 0 }} />
                <span style={{ color: 'var(--color-primary)', fontSize: 13, fontWeight: 600 }}>
                  {progress?.message || 'Preparing project...'}
                </span>
              </div>
            )}

            {/* Resume & Run Plan: hidden when agents are active */}
            {!hasAgents && !isRunning && !(progress && progress.stage !== 'idle' && progress.stage !== 'running') && resumableList.length > 0 && (
              <div style={{ position: 'relative', marginRight: 8 }}>
                <button
                  className={styles.runBtn}
                  style={{ background: 'var(--color-primary)', color: '#fff', fontWeight: 700, fontSize: 15, padding: '10px 24px' }}
                  onClick={() => setShowResumeDropdown((v) => !v)}
                >
                  Resume Project ({resumableList.length})
                </button>
                {showResumeDropdown && (
                  <div className={styles.dropdown} style={{ minWidth: 320 }}>
                    <div className={styles.dropdownHeader}>Resume Project</div>
                    {resumableList.map((proj) => (
                      <button
                        key={proj.trackId}
                        className={styles.dropdownItem}
                        onClick={() => {
                          sessionStorage.setItem('samams_active_plan_id', proj.trackId);
                          setShowResumeDropdown(false);
                          window.location.href = '/task-tree';
                        }}
                      >
                        <span className={styles.dropdownTitle}>
                          {proj.projectName || 'Untitled'}
                        </span>
                        <span className={styles.dropdownMeta}>
                          {proj.completed}/{proj.totalTasks} done &middot; {proj.pending} pending
                        </span>
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Add Agent: assigns to highest-priority pending task */}
            <button
              className={styles.runBtn}
              onClick={handleAddAgent}
              disabled={addingAgent || !hasActiveTasks || agents.length >= MAX_AGENTS}
              title={!hasActiveTasks ? 'Run a plan first to start agents' : agents.length >= MAX_AGENTS ? `Agent limit reached (${MAX_AGENTS})` : ''}
            >
              {addingAgent ? 'Adding...' : '+ Add Agent'}
            </button>

            {/* Run SAMAMS: plan → tree → full dispatch (hidden when agents active) */}
            {hasPlan && !hasAgents && !isRunning && !(progress && progress.stage !== 'idle' && progress.stage !== 'running') && (
              <div className={styles.runWrapper} ref={dropdownRef}>
                <button
                  className={styles.runBtn}
                  onClick={() => setShowDropdown((v) => !v)}
                  disabled={isRunning}
                  style={{ marginLeft: 8 }}
                >
                  {isRunning ? 'Dispatching...' : 'Run Plan'}
                </button>
                {showDropdown && (
                  <div className={styles.dropdown}>
                    {!selectedPlan ? (
                      <>
                        <div className={styles.dropdownHeader}>Select a Plan</div>
                        {planList.map((plan) => (
                          <button
                            key={plan.id}
                            className={styles.dropdownItem}
                            onClick={() => handleSelectPlan(plan)}
                          >
                            <span className={styles.dropdownTitle}>
                              {plan.title || 'Untitled Plan'}
                            </span>
                            <span className={styles.dropdownMeta}>
                              {plan.features?.length || 0} features
                            </span>
                          </button>
                        ))}
                      </>
                    ) : (
                      <>
                        <div className={styles.dropdownHeader}>
                          <button
                            className={styles.backBtn}
                            onClick={() => setSelectedPlan(null)}
                          >
                            ← Back
                          </button>
                          Agent Count
                        </div>
                        <div className={styles.agentCountGrid}>
                          {Array.from({ length: MAX_AGENTS }, (_, i) => i + 1).map((n) => (
                            <button
                              key={n}
                              className={styles.agentCountBtn}
                              onClick={() => handleSelectAgentCount(n)}
                              disabled={isRunning}
                            >
                              {n}
                            </button>
                          ))}
                        </div>
                        <div className={styles.dropdownHint}>
                          Max {selectedPlan.title ? `"${selectedPlan.title}"` : 'plan'} agents
                        </div>
                      </>
                    )}
                  </div>
                )}
              </div>
            )}
            {!hasPlan && (
              <a href="/planning" className={styles.planningBtn}>
                + Start Planning
              </a>
            )}
            <span className={styles.agentCount}>
              {hasAgents ? `${agents.length} Agent${agents.length > 1 ? 's' : ''}` : 'No agents'}
            </span>
            <a href="/task-tree" className={styles.checkMapBtn}>Task Map →</a>
            <a href="/planning" className={styles.checkMapBtn}>Edit Plan →</a>
          </div>

          {/* Agent cards */}
          {hasAgents ? (
            <div className={styles.agentGrid}>
              {agents.map((agent) => (
                <AgentCard key={agent.id} agent={agent} />
              ))}
            </div>
          ) : (
            <div className={styles.emptyState}>
              <img src="/logo.png" alt="The BAT" className={styles.emptyLogo} />
              <h2 className={styles.emptyTitle}>SENTINEL</h2>
              <p className={styles.emptyDesc}>
                {user?.displayName ? `Welcome, ${user.displayName}. ` : ''}
                {hasPlan ? 'Select a plan and click Run SAMAMS to start.' : 'Start by creating a project plan.'}
              </p>
            </div>
          )}
        </div>

        <MaalLogStream />
      </div>

      <GlobalControls />
      <StrategyMeetingModal />
      <SentinelChatPanel />
    </div>
  );
}
