import { Outlet } from 'react-router-dom';
import { Sidebar } from '../../features/workspace/components/Sidebar';
import { Header } from '../../features/workspace/components/Header';
import styles from './DashboardLayout.module.css';

export function DashboardLayout() {
  return (
    <div className={styles.layout}>
      <Sidebar />
      <div className={styles.main}>
        <Header />
        <div className={styles.content}>
          <Outlet />
        </div>
      </div>
    </div>
  );
}
