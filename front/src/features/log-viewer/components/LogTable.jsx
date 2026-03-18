import { useState } from 'react';
import { useLogViewerStore } from '../store/logViewerStore';
import styles from './LogTable.module.css';

const PAGE_SIZES = [25, 50, 100];

const eventColorMap = {
  TASK_START: 'start',
  TASK_PROGRESS: 'progress',
  TASK_COMPLETE: 'complete',
  TOKEN_WARNING: 'warning',
  ERROR: 'error',
  CONFLICT_DETECTED: 'conflict',
  AGENT_PAUSED: 'paused',
};

const statusBadgeMap = {
  active: 'badgeActive',
  completed: 'badgeCompleted',
  warning: 'badgeWarning',
  error: 'badgeError',
  paused: 'badgePaused',
};

export function LogTable() {
  const getFilteredLogs = useLogViewerStore((s) => s.getFilteredLogs);
  const isLoading = useLogViewerStore((s) => s.isLoading);
  const selectedLogId = useLogViewerStore((s) => s.selectedLogId);
  const selectLog = useLogViewerStore((s) => s.selectLog);

  const [page, setPage] = useState(0);
  const [pageSize, setPageSize] = useState(50);

  const allLogs = getFilteredLogs();
  const totalPages = Math.max(1, Math.ceil(allLogs.length / pageSize));
  const safePage = Math.min(page, totalPages - 1);
  const logs = allLogs.slice(safePage * pageSize, (safePage + 1) * pageSize);

  return (
    <div className={styles.tableWrap}>
      {isLoading && allLogs.length === 0 && (
        <div className={styles.loadingOverlay} role="status" aria-label="Loading logs">
          <span className={styles.loadingText}>Loading logs...</span>
        </div>
      )}

      <table className={styles.table} aria-label="Agent activity logs">
        <thead>
          <tr>
            <th className={styles.th}>Timestamp</th>
            <th className={styles.th}>Agent</th>
            <th className={styles.th}>Event Type</th>
            <th className={styles.th}>Task UID</th>
            <th className={styles.th}>Description</th>
            <th className={styles.th}>Status</th>
          </tr>
        </thead>
        <tbody>
          {logs.map((log) => {
            const rowTint = eventColorMap[log.eventType] || '';
            const isSelected = selectedLogId === log.id;
            return (
              <tr
                key={log.id}
                className={`${styles.row} ${styles[rowTint] || ''} ${isSelected ? styles.selected : ''}`}
                onClick={() => selectLog(log.id)}
                aria-selected={isSelected}
                tabIndex={0}
                onKeyDown={(e) => e.key === 'Enter' && selectLog(log.id)}
              >
                <td className={styles.td}>
                  <span className={styles.mono}>{log.timestamp.split(' ')[1]}</span>
                </td>
                <td className={styles.td}>{log.agent}</td>
                <td className={styles.td}>
                  <span className={`${styles.eventTag} ${styles[rowTint] || ''}`}>
                    {log.eventType.replace(/_/g, ' ')}
                  </span>
                </td>
                <td className={styles.td}>
                  <span className={styles.mono}>{log.taskUid}</span>
                </td>
                <td className={`${styles.td} ${styles.descCell}`}>{log.description}</td>
                <td className={styles.td}>
                  <span className={`${styles.statusBadge} ${styles[statusBadgeMap[log.status]] || ''}`}>
                    {log.status.toUpperCase()}
                  </span>
                </td>
              </tr>
            );
          })}
          {allLogs.length === 0 && !isLoading && (
            <tr>
              <td className={styles.td} colSpan={6}>
                <div className={styles.empty}>No log entries match the current filters.</div>
              </td>
            </tr>
          )}
        </tbody>
      </table>

      {/* Pagination controls */}
      {allLogs.length > 0 && (
        <div className={styles.pagination} aria-label="Log pagination">
          <span className={styles.pageInfo}>
            {safePage * pageSize + 1}–{Math.min((safePage + 1) * pageSize, allLogs.length)} of {allLogs.length}
          </span>
          <div className={styles.pageControls}>
            <button
              className={styles.pageBtn}
              onClick={() => setPage(0)}
              disabled={safePage === 0}
              aria-label="First page"
            >
              &laquo;
            </button>
            <button
              className={styles.pageBtn}
              onClick={() => setPage((p) => Math.max(0, p - 1))}
              disabled={safePage === 0}
              aria-label="Previous page"
            >
              &lsaquo;
            </button>
            <span className={styles.pageNum}>
              Page {safePage + 1} / {totalPages}
            </span>
            <button
              className={styles.pageBtn}
              onClick={() => setPage((p) => Math.min(totalPages - 1, p + 1))}
              disabled={safePage >= totalPages - 1}
              aria-label="Next page"
            >
              &rsaquo;
            </button>
            <button
              className={styles.pageBtn}
              onClick={() => setPage(totalPages - 1)}
              disabled={safePage >= totalPages - 1}
              aria-label="Last page"
            >
              &raquo;
            </button>
          </div>
          <select
            className={styles.pageSizeSelect}
            value={pageSize}
            onChange={(e) => { setPageSize(Number(e.target.value)); setPage(0); }}
            aria-label="Rows per page"
          >
            {PAGE_SIZES.map((s) => (
              <option key={s} value={s}>{s} rows</option>
            ))}
          </select>
        </div>
      )}
    </div>
  );
}
