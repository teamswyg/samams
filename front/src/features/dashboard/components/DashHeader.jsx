import { useDashboardStore } from '../store/dashboardStore';
import { useAuthStore } from '../../auth/store/authStore';
import styles from './DashHeader.module.css';

export function DashHeader() {
  const toggleChat = useDashboardStore((s) => s.toggleChat);
  const chatOpen = useDashboardStore((s) => s.chatOpen);
  const agents = useDashboardStore((s) => s.agents);
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);

  return (
    <header className={styles.header}>
      <div className={styles.left}>
        <img src="/logo.png" alt="The BAT" className={styles.logo} />
        <div className={styles.brand}>
          <span className={styles.title}>SAMAMS</span>
          <span className={styles.subtitle}>Sentinel Automated Multiple AI Management System</span>
        </div>
      </div>
      <nav className={styles.nav}>
        <a href="/planning" className={styles.navLink}>
          <span className={styles.navIcon}>&#9998;</span> Planning
        </a>
        <a href="/task-tree" className={styles.navLink}>
          <span className={styles.navIcon}>&#9783;</span> Task Tree
        </a>
        <a href="/log-viewer" className={styles.navLink}>
          <span className={styles.navIcon}>&#9776;</span> Log Viewer
        </a>
        {agents.length > 0 && (
          <button
            className={`${styles.sentinelBtn} ${chatOpen ? styles.sentinelBtnActive : ''}`}
            onClick={toggleChat}
          >
            <span className={styles.pulseDot}>
              <span className={styles.pingRing} />
            </span>
            SENTINEL AI
          </button>
        )}
        {user && (
          <div className={styles.userArea}>
            {user.photoURL && (
              <img src={user.photoURL} alt="" className={styles.avatar} />
            )}
            <span className={styles.userName}>{user.displayName || user.email}</span>
            <button className={styles.logoutBtn} onClick={logout}>Logout</button>
          </div>
        )}
      </nav>
    </header>
  );
}
