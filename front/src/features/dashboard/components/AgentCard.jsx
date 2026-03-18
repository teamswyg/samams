import { useState, useEffect } from 'react';
import { useDashboardStore } from '../store/dashboardStore';
import http from '../../../shared/api/http';
import { endpoints } from '../../../shared/api/endpoints';
import styles from './AgentCard.module.css';

const statusConfig = {
  active: { label: 'ACTIVE', color: 'var(--color-primary)', glow: true },
  idle: { label: 'IDLE', color: 'var(--color-warning)', glow: false },
  paused: { label: 'PAUSED', color: 'var(--color-text-muted)', glow: false },
  error: { label: 'ERROR', color: 'var(--color-error)', glow: true },
};

export function AgentCard({ agent }) {
  const pauseAgent = useDashboardStore((s) => s.pauseAgent);
  const killAgent = useDashboardStore((s) => s.killAgent);
  const config = statusConfig[agent.status] || statusConfig.idle;
  const tokenPercent = agent.tokenMax > 0 ? Math.round((agent.tokenUsed / agent.tokenMax) * 100) : 0;
  const [showLogs, setShowLogs] = useState(false);
  const [logLines, setLogLines] = useState([]);

  // Poll agent logs when modal is open.
  useEffect(() => {
    if (!showLogs) return;
    let active = true;
    const fetchLogs = async () => {
      try {
        const { data } = await http.get(endpoints.run.agentLogs(agent.id));
        if (active && data?.logs) setLogLines(data.logs);
      } catch (err) {
        console.warn('[AgentCard] Failed to fetch logs:', err.message);
      }
    };
    fetchLogs();
    const timer = setInterval(fetchLogs, 3000);
    return () => { active = false; clearInterval(timer); };
  }, [showLogs, agent.id]);

  return (
    <>
      <div
        className={styles.card}
        style={{
          borderColor: `${config.color}33`,
          boxShadow: config.glow ? `0 0 20px ${config.color}15` : 'none',
        }}
      >
        <div className={styles.header}>
          <div className={styles.nameRow}>
            <span className={styles.name}>{agent.name}</span>
            {agent.mode === 'watch' ? (
              <span className={styles.watchBadge}>watch</span>
            ) : agent.agentTypeBadge ? (
              <span className={styles.typeBadge}>{agent.agentTypeBadge}</span>
            ) : null}
            <button className={styles.menuBtn} aria-label={`Agent ${agent.name} menu`} aria-haspopup="true">&#8943;</button>
          </div>
          <div className={styles.statusRow}>
            <span
              className={`${styles.statusDot} ${agent.status === 'active' ? styles.pulse : ''}`}
              style={{ background: config.color }}
            />
            <span className={styles.statusText} style={{ color: config.color }}>
              {config.label}
            </span>
          </div>
        </div>

        <div className={styles.taskSection}>
          <span className={styles.label}>Current Task:</span>
          {agent.nodeUid && (
            <span className={styles.nodeUid}>{agent.nodeUid}</span>
          )}
          <p className={styles.taskText}>
            {agent.currentTask || 'No active task'}
          </p>
        </div>

        <div className={styles.progressSection}>
          <div className={styles.progressHeader}>
            <span className={styles.label}>Progress</span>
            <span className={styles.progressValue}>{agent.progress}%</span>
          </div>
          <div className={styles.progressTrack} role="progressbar" aria-valuenow={agent.progress} aria-valuemin={0} aria-valuemax={100} aria-label="Task progress">
            <div className={styles.progressFill} style={{ width: `${agent.progress}%` }} />
          </div>
        </div>

        <div className={styles.tokenSection}>
          <div className={styles.progressHeader}>
            <span className={styles.label}>Token Usage</span>
            <span className={styles.tokenValue}>
              {agent.tokenUsed.toLocaleString()} / {agent.tokenMax.toLocaleString()}
            </span>
          </div>
          <div className={styles.tokenTrack} role="progressbar" aria-valuenow={tokenPercent} aria-valuemin={0} aria-valuemax={100} aria-label="Token usage">
            <div className={styles.tokenFill} style={{ width: `${tokenPercent}%` }} />
          </div>
        </div>

        <div className={styles.actions}>
          <button
            className={styles.actionBtn}
            style={{ borderColor: 'var(--color-primary)', color: 'var(--color-primary)' }}
            onClick={() => pauseAgent(agent.id)}
            aria-label={`Pause agent ${agent.name}`}
          >
            Pause
          </button>
          <button
            className={styles.actionBtn}
            style={{ borderColor: 'var(--color-error)', color: 'var(--color-error)' }}
            onClick={() => killAgent(agent.id)}
            aria-label={`Kill agent ${agent.name}`}
          >
            Kill
          </button>
          <button
            className={styles.actionBtn}
            style={{ borderColor: 'var(--color-info)', color: 'var(--color-info)' }}
            onClick={() => setShowLogs(true)}
            aria-label={`View CLI logs for ${agent.name}`}
          >
            Log
          </button>
        </div>
      </div>

      {/* CLI Log Modal */}
      {showLogs && (
        <div className={styles.logOverlay} onClick={() => setShowLogs(false)}>
          <div className={styles.logModal} onClick={(e) => e.stopPropagation()} role="dialog" aria-label={`CLI logs for ${agent.name}`}>
            <div className={styles.logHeader}>
              <span className={styles.logTitle}>{agent.name} — CLI Output</span>
              <button className={styles.logClose} onClick={() => setShowLogs(false)} aria-label="Close log viewer">&times;</button>
            </div>
            <pre className={styles.logContent}>
              {logLines.length > 0 ? logLines.join('\n') : 'No output yet...'}
            </pre>
          </div>
        </div>
      )}
    </>
  );
}
