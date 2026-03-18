import styles from './LogViewerHeader.module.css';

export function LogViewerHeader() {
  return (
    <header className={styles.header}>
      <div className={styles.left}>
        <div className={styles.logo}>S</div>
        <div className={styles.brand}>
          <span className={styles.title}>MAAL Log Viewer</span>
          <span className={styles.subtitle}>Multi-Agent Activity Log &middot; Real-time Monitoring</span>
        </div>
      </div>
      <nav className={styles.nav}>
        <a href="/" className={styles.navLink}>
          <span className={styles.navIcon}>&#9783;</span> Dashboard
        </a>
        <a href="/planning" className={styles.navLink}>
          <span className={styles.navIcon}>&#9998;</span> Planning
        </a>
        <a href="/task-tree" className={styles.navLink}>
          <span className={styles.navIcon}>&#9783;</span> Task Tree
        </a>
      </nav>
    </header>
  );
}
