import { usePlanningStore } from '../store/planningStore';
import styles from './FeatureCard.module.css';

const priorityColors = {
  high: styles.priorityHigh,
  medium: styles.priorityMedium,
  low: styles.priorityLow,
};

export function FeatureCard({ feature, index }) {
  const updateFeature = usePlanningStore((s) => s.updateFeature);
  const removeFeature = usePlanningStore((s) => s.removeFeature);
  const addDetail = usePlanningStore((s) => s.addDetail);
  const updateDetail = usePlanningStore((s) => s.updateDetail);
  const removeDetail = usePlanningStore((s) => s.removeDetail);

  return (
    <div className={styles.card}>
      <div className={styles.top}>
        <span className={styles.index}>{index + 1}</span>
        <div className={styles.fields}>
          <input
            className={styles.nameInput}
            value={feature.name}
            onChange={(e) => updateFeature(feature.id, { name: e.target.value })}
            placeholder="Enter feature name"
          />
          <textarea
            className={styles.descInput}
            rows={2}
            value={feature.description}
            onChange={(e) => updateFeature(feature.id, { description: e.target.value })}
            placeholder="Enter feature description"
          />
        </div>
        <select
          className={`${styles.prioritySelect} ${priorityColors[feature.priority] || ''}`}
          value={feature.priority}
          onChange={(e) => updateFeature(feature.id, { priority: e.target.value })}
        >
          <option value="high">HIGH</option>
          <option value="medium">MEDIUM</option>
          <option value="low">LOW</option>
        </select>
        <button className={styles.deleteBtn} onClick={() => removeFeature(feature.id)}>
          &#128465;
        </button>
      </div>

      {/* Details */}
      <div className={styles.detailsSection}>
        <div className={styles.detailsHeader}>
          <span className={styles.detailsLabel}>Details</span>
          <button className={styles.addDetailBtn} onClick={() => addDetail(feature.id)}>
            + Add
          </button>
        </div>
        {feature.details.map((detail, i) => (
          <div key={i} className={styles.detailRow}>
            <span className={styles.detailDot} />
            <input
              className={styles.detailInput}
              value={detail}
              onChange={(e) => updateDetail(feature.id, i, e.target.value)}
              placeholder="Enter detail"
            />
            <button className={styles.detailDeleteBtn} onClick={() => removeDetail(feature.id, i)}>
              &#10005;
            </button>
          </div>
        ))}
      </div>
    </div>
  );
}
