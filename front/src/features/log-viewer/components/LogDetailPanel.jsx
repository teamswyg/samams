import { useLogViewerStore } from '../store/logViewerStore';
import styles from './LogDetailPanel.module.css';

export function LogDetailPanel() {
  const selectedLogId = useLogViewerStore((s) => s.selectedLogId);
  const logs = useLogViewerStore((s) => s.logs);
  const selectLog = useLogViewerStore((s) => s.selectLog);

  const log = logs.find((l) => l.id === selectedLogId);

  if (!log) {
    return (
      <aside className={styles.panel}>
        <div className={styles.empty}>
          <span className={styles.emptyIcon}>&#128196;</span>
          <p>Select a log entry to view details</p>
        </div>
      </aside>
    );
  }

  const meta = log.detail?.metadata || {};

  const handleCopy = () => {
    const text = JSON.stringify(log, null, 2);
    navigator.clipboard.writeText(text);
  };

  return (
    <aside className={styles.panel}>
      <div className={styles.header}>
        <span className={styles.headerTitle}>Log Detail</span>
        <button className={styles.closeBtn} onClick={() => selectLog(null)}>&#10005;</button>
      </div>

      <div className={styles.body}>
        <div className={styles.section}>
          <div className={styles.field}>
            <span className={styles.fieldLabel}>Log ID</span>
            <span className={styles.fieldValue}>{log.id}</span>
          </div>
          <div className={styles.field}>
            <span className={styles.fieldLabel}>Timestamp</span>
            <span className={styles.fieldValue}>{log.timestamp}</span>
          </div>
          <div className={styles.field}>
            <span className={styles.fieldLabel}>Agent</span>
            <span className={styles.fieldValue}>{log.agent}</span>
          </div>
          <div className={styles.field}>
            <span className={styles.fieldLabel}>Event Type</span>
            <span className={styles.fieldValue}>{log.eventType}</span>
          </div>
          <div className={styles.field}>
            <span className={styles.fieldLabel}>Task UID</span>
            <span className={styles.fieldValue}>{log.taskUid}</span>
          </div>
          <div className={styles.field}>
            <span className={styles.fieldLabel}>Status</span>
            <span className={styles.fieldValue}>{log.status.toUpperCase()}</span>
          </div>
        </div>

        <div className={styles.descSection}>
          <span className={styles.fieldLabel}>Description</span>
          <p className={styles.descText}>{log.description}</p>
        </div>

        {log.detail && (
          <div className={styles.section}>
            <div className={styles.sectionTitle}>Additional Information</div>
            {log.detail.module && (
              <div className={styles.field}>
                <span className={styles.fieldLabel}>Module</span>
                <span className={styles.fieldValue}>{log.detail.module}</span>
              </div>
            )}
            {log.detail.branch && (
              <div className={styles.field}>
                <span className={styles.fieldLabel}>Branch</span>
                <span className={styles.fieldValue}>{log.detail.branch}</span>
              </div>
            )}
            {log.detail.progress !== undefined && (
              <div className={styles.field}>
                <span className={styles.fieldLabel}>Progress</span>
                <span className={styles.fieldValue}>{log.detail.progress}%</span>
              </div>
            )}
            {log.detail.tokensUsed !== undefined && (
              <div className={styles.field}>
                <span className={styles.fieldLabel}>Tokens Used</span>
                <span className={styles.fieldValue}>{log.detail.tokensUsed.toLocaleString()}</span>
              </div>
            )}
            {log.detail.duration && (
              <div className={styles.field}>
                <span className={styles.fieldLabel}>Duration</span>
                <span className={styles.fieldValue}>{log.detail.duration}</span>
              </div>
            )}
            {log.detail.errorCode && (
              <div className={styles.field}>
                <span className={styles.fieldLabel}>Error Code</span>
                <span className={`${styles.fieldValue} ${styles.errorVal}`}>{log.detail.errorCode}</span>
              </div>
            )}
            {Object.keys(meta).length > 0 && (
              <div className={styles.metaBlock}>
                <span className={styles.fieldLabel}>Metadata</span>
                <pre className={styles.metaJson}>{JSON.stringify(meta, null, 2)}</pre>
              </div>
            )}
          </div>
        )}
      </div>

      <div className={styles.footer}>
        <button className={styles.footerBtn} onClick={handleCopy}>&#128203; Copy</button>
        <button className={styles.footerBtn}>&#128279; Share</button>
      </div>
    </aside>
  );
}
