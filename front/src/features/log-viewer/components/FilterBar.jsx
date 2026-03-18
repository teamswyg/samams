import { useLogViewerStore } from '../store/logViewerStore';
import styles from './FilterBar.module.css';

export function FilterBar() {
  const filters = useLogViewerStore((s) => s.filters);
  const setFilter = useLogViewerStore((s) => s.setFilter);
  const resetFilters = useLogViewerStore((s) => s.resetFilters);
  const eventTypes = useLogViewerStore((s) => s.eventTypes);
  const agentNames = useLogViewerStore((s) => s.agentNames);
  const getFilteredLogs = useLogViewerStore((s) => s.getFilteredLogs);

  const count = getFilteredLogs().length;

  return (
    <div className={styles.bar}>
      <div className={styles.filters}>
        <div className={styles.filterGroup}>
          <label className={styles.label}>Agent</label>
          <select
            className={styles.select}
            value={filters.agent}
            onChange={(e) => setFilter('agent', e.target.value)}
          >
            {agentNames.map((name) => (
              <option key={name} value={name}>{name === 'ALL' ? 'All Agents' : name}</option>
            ))}
          </select>
        </div>

        <div className={styles.filterGroup}>
          <label className={styles.label}>Event Type</label>
          <select
            className={styles.select}
            value={filters.eventType}
            onChange={(e) => setFilter('eventType', e.target.value)}
          >
            {eventTypes.map((type) => (
              <option key={type} value={type}>{type === 'ALL' ? 'All Events' : type.replace(/_/g, ' ')}</option>
            ))}
          </select>
        </div>

        <div className={styles.filterGroup}>
          <label className={styles.label}>Date Range</label>
          <select
            className={styles.select}
            value={filters.dateRange}
            onChange={(e) => setFilter('dateRange', e.target.value)}
          >
            <option value="ALL">All Time</option>
            <option value="1h">Last 1 Hour</option>
            <option value="6h">Last 6 Hours</option>
            <option value="24h">Last 24 Hours</option>
            <option value="7d">Last 7 Days</option>
          </select>
        </div>

        <button className={styles.resetBtn} onClick={resetFilters}>Reset</button>
      </div>

      <div className={styles.right}>
        <span className={styles.count}>{count} entries</span>
        <button className={styles.actionBtn}>&#8635; Refresh</button>
        <button className={styles.actionBtn}>&#8681; Export</button>
      </div>
    </div>
  );
}
