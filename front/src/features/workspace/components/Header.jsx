import { useAuthStore } from '../../auth/store/authStore';
import { Button } from '../../../shared/components/ui/Button';
import styles from './Header.module.css';

export function Header() {
  const user = useAuthStore((s) => s.user);
  const clearAuth = useAuthStore((s) => s.clearAuth);

  return (
    <header className={styles.header}>
      <div className={styles.left}>
        <h1 className={styles.title}>Sentinel Control</h1>
      </div>
      <div className={styles.right}>
        {user && <span className={styles.user}>{user.email || user.id}</span>}
        <Button variant="ghost" size="sm" onClick={clearAuth}>
          Logout
        </Button>
      </div>
    </header>
  );
}
