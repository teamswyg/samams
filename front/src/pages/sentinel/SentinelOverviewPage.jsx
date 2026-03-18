import { useState, useEffect } from 'react';
import { useDashboardStore } from '../../features/dashboard/store/dashboardStore';
import { StatusCard } from '../../features/sentinel-monitor/components/StatusCard';
import { AlertList } from '../../features/sentinel-monitor/components/AlertList';
import { FilterBar } from '../../features/sentinel-monitor/components/FilterBar';
import http from '../../shared/api/http';
import { endpoints } from '../../shared/api/endpoints';
import styles from './SentinelOverviewPage.module.css';

export function SentinelOverviewPage() {
  const agents = useDashboardStore((s) => s.agents);
  const proxyConnected = useDashboardStore((s) => s.proxyConnected);
  const [alerts, setAlerts] = useState([]);

  useEffect(() => {
    async function loadAlerts() {
      try {
        const { data } = await http.get(endpoints.sentinel.alerts);
        setAlerts(Array.isArray(data) ? data : []);
      } catch {}
    }
    loadAlerts();
    const timer = setInterval(loadAlerts, 5000);
    return () => clearInterval(timer);
  }, []);

  const activeAgents = agents.filter((a) => a.status === 'active').length;
  const errorAgents = agents.filter((a) => a.status === 'error').length;
  const systemStatus = proxyConnected ? 'running' : 'disconnected';

  return (
    <div className={styles.page}>
      <h2 className={styles.title}>Sentinel Overview</h2>
      <div className={styles.grid}>
        <StatusCard title="System Status" value={systemStatus} variant={proxyConnected ? 'running' : 'error'} />
        <StatusCard title="Active Agents" value={String(activeAgents)} variant="info" />
        <StatusCard title="Errors" value={String(errorAgents)} variant={errorAgents > 0 ? 'error' : 'info'} />
        <StatusCard title="Alerts" value={String(alerts.length)} variant={alerts.length > 0 ? 'warning' : 'info'} />
      </div>
      <FilterBar />
      <AlertList alerts={alerts} />
    </div>
  );
}
