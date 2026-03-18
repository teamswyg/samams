import { useState, useEffect, useCallback } from 'react';
import { useDashboardStore } from '../store/dashboardStore';
import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';
import styles from './StrategyMeetingModal.module.css';

const STAGES = ['pausing', 'analyzing', 'dispatching'];
const STAGE_LABELS = {
  idle: 'Idle',
  pausing: 'Interrupting & Analyzing',
  analyzing: 'LLM Decision',
  dispatching: 'Applying Actions',
};
const STAGE_DESC = {
  pausing: 'Interrupting agents (1x SIGINT), watch agents analyzing worktrees...',
  analyzing: 'Claude is synthesizing all analyses and making a restructuring decision...',
  dispatching: 'Applying per-task actions: keep / reset & retry / cancel...',
};
const DECISION_LABELS = {
  restructure: 'Restructured',
};

export function StrategyMeetingModal() {
  const meetingOpen = useDashboardStore((s) => s.meetingOpen);
  const closeMeeting = useDashboardStore((s) => s.closeMeeting);
  const meetingStatus = useDashboardStore((s) => s.meetingStatus);
  const meetingError = useDashboardStore((s) => s.meetingError);
  const meetingSessionId = useDashboardStore((s) => s.meetingSessionId);
  const pollMeetingStatus = useDashboardStore((s) => s.pollMeetingStatus);
  const startStrategyMeeting = useDashboardStore((s) => s.startStrategyMeeting);
  const [selectedProject, setSelectedProject] = useState('');
  const [projects, setProjects] = useState([]);
  const [loadingProjects, setLoadingProjects] = useState(false);
  const [elapsed, setElapsed] = useState(0);

  const status = meetingStatus?.status || 'idle';
  const isActive = status !== 'idle' && meetingStatus;
  // Only show decision if it belongs to the current session (prevents stale decisions).
  const hasDecision = status === 'idle' && meetingStatus?.decision
    && (!meetingSessionId || meetingStatus?.sessionId === meetingSessionId);

  // Poll meeting status while active (max 15 minutes to prevent infinite polling).
  const MAX_POLL_COUNT = 450; // 15min at 2s interval
  useEffect(() => {
    if (!meetingOpen || !isActive) return;
    let pollCount = 0;
    const interval = setInterval(() => {
      pollCount++;
      if (pollCount > MAX_POLL_COUNT) {
        clearInterval(interval);
        return;
      }
      pollMeetingStatus();
    }, 2000);
    return () => clearInterval(interval);
  }, [meetingOpen, isActive, pollMeetingStatus]);

  // Elapsed timer.
  useEffect(() => {
    if (!isActive) { setElapsed(0); return; }
    const start = meetingStatus?.createdAt || Date.now();
    const tick = () => setElapsed(Math.floor((Date.now() - start) / 1000));
    tick();
    const interval = setInterval(tick, 1000);
    return () => clearInterval(interval);
  }, [isActive, meetingStatus?.createdAt]);

  // Load status + project list on open.
  useEffect(() => {
    if (!meetingOpen) return;
    pollMeetingStatus();
    setLoadingProjects(true);
    http.get(endpoints.run.resumable)
      .then(({ data }) => {
        const list = (data.resumable || []).filter((p) => p.running > 0);
        setProjects(list);
        if (list.length === 1) setSelectedProject(list[0].projectName);
      })
      .catch(() => setProjects([]))
      .finally(() => setLoadingProjects(false));
  }, [meetingOpen, pollMeetingStatus]);

  const handleStart = useCallback(() => {
    if (selectedProject) startStrategyMeeting(selectedProject);
  }, [selectedProject, startStrategyMeeting]);

  if (!meetingOpen) return null;

  const participants = meetingStatus?.participantNodeUids || [];
  const discussionContexts = meetingStatus?.discussionContexts || {};
  const contextCount = Object.keys(discussionContexts).length;
  const currentStageIdx = STAGES.indexOf(status);

  const formatTime = (s) => `${Math.floor(s / 60)}:${String(s % 60).padStart(2, '0')}`;

  return (
    <div className={styles.overlay}>
      <div className={styles.modal}>
        {/* Header */}
        <div className={styles.header}>
          <div className={styles.headerLeft}>
            <span className={styles.warnIcon}>{isActive ? '\u26A1' : hasDecision ? '\u2705' : '\u2699'}</span>
            <span className={styles.headerTitle}>Strategy Meeting</span>
            {isActive && (
              <span className={styles.badge}>{STAGE_LABELS[status] || status}</span>
            )}
            {hasDecision && (
              <span className={styles.badgeComplete}>{DECISION_LABELS[meetingStatus.decision] || meetingStatus.decision}</span>
            )}
          </div>
          <button className={styles.closeBtn} onClick={closeMeeting}>{'\u2715'}</button>
        </div>

        {/* Not started and no decision — show start form */}
        {!isActive && !hasDecision && (
          <div className={styles.startForm}>
            {meetingError && <div className={styles.errorMsg}>{meetingError}</div>}
            <label className={styles.inputLabel}>
              Running Project
              {loadingProjects ? (
                <div className={styles.input} style={{ opacity: 0.5 }}>Loading projects...</div>
              ) : projects.length === 0 ? (
                <div className={styles.input} style={{ opacity: 0.5 }}>No running projects found</div>
              ) : (
                <select
                  className={styles.input}
                  value={selectedProject}
                  onChange={(e) => setSelectedProject(e.target.value)}
                >
                  {projects.length > 1 && <option value="">Select a project...</option>}
                  {projects.map((p) => (
                    <option key={p.projectName} value={p.projectName}>
                      {p.projectName} ({p.running} running, {p.completed}/{p.totalTasks} done)
                    </option>
                  ))}
                </select>
              )}
            </label>
            <p className={styles.description}>
              Starts a strategy meeting: all agents pause, analyze their worktrees for conflicts,
              and Claude decides how to restructure work.
            </p>
            <button className={styles.startBtn} onClick={handleStart} disabled={!selectedProject || projects.length === 0}>
              Start Strategy Meeting
            </button>
          </div>
        )}

        {/* Decision result — meeting completed */}
        {hasDecision && (
          <div className={styles.decisionResult}>
            <div className={styles.decisionHeader}>
              <span className={styles.decisionLabel}>Decision</span>
              <span className={`${styles.decisionBadge} ${styles[`decision_${meetingStatus.decision}`] || ''}`}>
                {DECISION_LABELS[meetingStatus.decision] || meetingStatus.decision}
              </span>
            </div>
            {meetingStatus.decisionReasoning && (
              <p className={styles.decisionReasoning}>{meetingStatus.decisionReasoning}</p>
            )}
            <div className={styles.decisionMeta}>
              <span>Session: {meetingStatus.sessionId}</span>
              <span>Participants: {participants.length}</span>
            </div>
          </div>
        )}

        {/* Error during active meeting */}
        {isActive && meetingError && (
          <div style={{ padding: '0 24px' }}>
            <div className={styles.errorMsg}>{meetingError}</div>
          </div>
        )}

        {/* Active — show progress */}
        {isActive && (
          <>
            {/* Stage progress bar */}
            <div className={styles.stageBar}>
              {STAGES.map((stage, i) => {
                const isDone = i < currentStageIdx;
                const isCurrent = i === currentStageIdx;
                return (
                  <div key={stage} className={`${styles.stage} ${isDone ? styles.stageDone : ''} ${isCurrent ? styles.stageCurrent : ''}`}>
                    <div className={styles.stageDot}>
                      {isDone ? '\u2713' : i + 1}
                    </div>
                    <span className={styles.stageLabel}>{STAGE_LABELS[stage]}</span>
                  </div>
                );
              })}
            </div>

            {/* Status info */}
            <div className={styles.conflictInfo}>
              <div className={styles.conflictRow}>
                <span>Session: <strong>{meetingStatus?.sessionId || '\u2014'}</strong></span>
                <span className={styles.timer}>
                  Elapsed: <span className={styles.countdown}>{formatTime(elapsed)}</span>
                </span>
              </div>
              <p className={styles.description}>{STAGE_DESC[status] || 'Processing...'}</p>
            </div>

            {/* Participants */}
            <div className={styles.participantList}>
              <div className={styles.participantHeader}>
                <span>Participants ({participants.length})</span>
                {contextCount > 0 && (
                  <span className={styles.progressText}>
                    {contextCount}/{participants.length} analyzed
                  </span>
                )}
              </div>
              {participants.map((uid) => (
                <div key={uid} className={styles.participantRow}>
                  <span className={styles.participantUid}>{uid}</span>
                </div>
              ))}
            </div>

            {/* Analysis progress bar */}
            {status === 'pausing' && participants.length > 0 && (
              <div className={styles.progressBar}>
                <div
                  className={styles.progressFill}
                  style={{ width: `${(contextCount / participants.length) * 100}%` }}
                />
              </div>
            )}
          </>
        )}

        {/* Footer */}
        <div className={styles.actions}>
          <button className={styles.dismissBtn} onClick={closeMeeting}>
            {isActive ? 'Hide (continues in background)' : 'Close'}
          </button>
        </div>
      </div>
    </div>
  );
}
