import { useDashboardStore } from '../store/dashboardStore';
import styles from './TopStatusBar.module.css';

export function TopStatusBar() {
  const agents = useDashboardStore((s) => s.agents);
  const proxyConnected = useDashboardStore((s) => s.proxyConnected);
  const lastSync = useDashboardStore((s) => s.lastSync);

  const counts = {
    total: agents.length,
    active: agents.filter((a) => a.status === 'active').length,
    idle: agents.filter((a) => a.status === 'idle').length,
    paused: agents.filter((a) => a.status === 'paused').length,
    error: agents.filter((a) => a.status === 'error').length,
  };

  const proxyLabel = proxyConnected === null ? 'Checking...' : proxyConnected ? 'Connected' : 'Disconnected';
  const proxyColor = proxyConnected === null ? 'var(--color-text-muted)' : proxyConnected ? 'var(--color-primary)' : 'var(--color-error)';

  const elapsed = lastSync ? Math.round((Date.now() - lastSync) / 1000) : null;
  const syncText = elapsed !== null ? `${elapsed}s ago` : '—';

  return (
    <div className={styles.bar}>
      <div className={styles.items}>
        <StatusItem label="Proxy" value={proxyLabel} color={proxyColor} />
        <StatusItem label="Total Agents" value={counts.total} color="var(--color-primary)" />
        <StatusItem label="Active" value={counts.active} color="var(--color-primary)" />
        <StatusItem label="Idle" value={counts.idle} color="var(--color-warning)" />
        <StatusItem label="Paused" value={counts.paused} color="var(--color-text-muted)" />
        <StatusItem label="Error" value={counts.error} color="var(--color-error)" />
      </div>
      <div className={styles.sync}>
        <span className={styles.syncIcon}>&#9201;</span>
        Last sync: {syncText}
      </div>
    </div>
  );
}

function StatusItem({ label, value, color }) {
  return (
    <div className={styles.item}>
      <span className={styles.dot} style={{ background: color }} />
      <span className={styles.label}>{label}</span>
      <span className={styles.value} style={{ color }}>{value}</span>
    </div>
  );
}
