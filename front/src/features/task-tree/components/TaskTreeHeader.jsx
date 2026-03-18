import { useTaskTreeStore } from '../store/taskTreeStore';
import styles from './TaskTreeHeader.module.css';

export function TaskTreeHeader() {
  const isFromPlanning = useTaskTreeStore((s) => s.isFromPlanning);
  const isConverting = useTaskTreeStore((s) => s.isConverting);
  const loadFromServer = useTaskTreeStore((s) => s.loadFromServer);

  const handleAILoad = async () => {
    const ok = await loadFromServer();
    if (!ok) alert('AI conversion failed. Save your plan and try again.');
  };

  return (
    <header className={styles.header}>
      <div className={styles.left}>
        <div className={styles.logo}>S</div>
        <div className={styles.brand}>
          <span className={styles.title}>Task Tree View</span>
          <span className={styles.subtitle}>Miro Board &middot; Visual Task Hierarchy</span>
        </div>
      </div>
      <div className={styles.right}>
        {isFromPlanning && (
          <span className={styles.badge}>&#9989; Generated from Plan</span>
        )}
        <button className={styles.loadBtn} onClick={handleAILoad} disabled={isConverting}>
          {isConverting ? 'Converting...' : 'AI Tree Convert'}
        </button>
        <a href="/planning" className={styles.link}>&#9998; Planning</a>
        <a href="/" className={styles.link}>&#8592; Dashboard</a>
      </div>
    </header>
  );
}
