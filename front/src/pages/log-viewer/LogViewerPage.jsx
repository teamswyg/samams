import { useEffect } from 'react';
import { useLogViewerStore } from '../../features/log-viewer/store/logViewerStore';
import { LogViewerHeader } from '../../features/log-viewer/components/LogViewerHeader';
import { FilterBar } from '../../features/log-viewer/components/FilterBar';
import { LogTable } from '../../features/log-viewer/components/LogTable';
import { LogDetailPanel } from '../../features/log-viewer/components/LogDetailPanel';
import styles from './LogViewerPage.module.css';

export function LogViewerPage() {
  const loadLogs = useLogViewerStore((s) => s.loadLogs);
  useEffect(() => {
    loadLogs();
    const timer = setInterval(loadLogs, 5000);
    return () => clearInterval(timer);
  }, [loadLogs]);

  return (
    <div className={styles.page}>
      <LogViewerHeader />
      <FilterBar />
      <div className={styles.content}>
        <LogTable />
        <LogDetailPanel />
      </div>
    </div>
  );
}
