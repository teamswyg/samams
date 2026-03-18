import { usePlanningStore } from '../store/planningStore';
import styles from './PlanningHeader.module.css';

export function PlanningHeader() {
  const showConvertButton = usePlanningStore((s) => s.showConvertButton);
  const isConvertingTree = usePlanningStore((s) => s.isConvertingTree);
  const convertToTreeWithAI = usePlanningStore((s) => s.convertToTreeWithAI);

  return (
    <header className={styles.header}>
      <div className={styles.left}>
        <div className={styles.logo}>S</div>
        <div className={styles.brand}>
          <span className={styles.title}>Planning Editor</span>
          <span className={styles.subtitle}>AI-Assisted Project Planning &amp; Documentation</span>
        </div>
      </div>
      <div className={styles.right}>
        {showConvertButton && (
          <button
            className={styles.convertBtn}
            onClick={convertToTreeWithAI}
            disabled={isConvertingTree}
          >
            {isConvertingTree ? 'Converting...' : 'Convert to Node Tree'}
          </button>
        )}
        <a href="/task-tree" className={styles.backLink}>
          &#9782; Task Tree
        </a>
        <a href="/" className={styles.backLink}>
          &#8592; Dashboard
        </a>
      </div>
    </header>
  );
}
