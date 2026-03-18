import { NavLink } from 'react-router-dom';
import { useWorkspaceStore } from '../store/workspaceStore';
import styles from './Sidebar.module.css';

const navItems = [
  { path: '/dashboard', label: 'Dashboard' },
  { path: '/agent/sessions', label: 'Agent Chat' },
  { path: '/sentinel/overview', label: 'Sentinel' },
  { path: '/jobs', label: 'Jobs' },
  { path: '/settings', label: 'Settings' },
];

export function Sidebar() {
  const collapsed = useWorkspaceStore((s) => s.sidebarCollapsed);
  const toggle = useWorkspaceStore((s) => s.toggleSidebar);

  return (
    <aside className={`${styles.sidebar} ${collapsed ? styles.collapsed : ''}`}>
      <div className={styles.brand}>
        <button className={styles.toggle} onClick={toggle}>
          {collapsed ? '>' : '<'}
        </button>
        {!collapsed && <span className={styles.logo}>Sentinel</span>}
      </div>
      <nav className={styles.nav}>
        {navItems.map((item) => (
          <NavLink
            key={item.path}
            to={item.path}
            className={({ isActive }) => `${styles.link} ${isActive ? styles.active : ''}`}
          >
            {!collapsed && item.label}
          </NavLink>
        ))}
      </nav>
    </aside>
  );
}
