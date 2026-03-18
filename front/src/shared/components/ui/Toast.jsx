import { useEffect } from 'react';
import { useNotificationStore } from '../../stores/notificationStore';
import styles from './Toast.module.css';

export function ToastContainer() {
  const notifications = useNotificationStore((s) => s.notifications);
  const removeNotification = useNotificationStore((s) => s.removeNotification);

  return (
    <div className={styles.container}>
      {notifications.map((n) => (
        <ToastItem key={n.id} notification={n} onClose={() => removeNotification(n.id)} />
      ))}
    </div>
  );
}

function ToastItem({ notification, onClose }) {
  useEffect(() => {
    const timer = setTimeout(onClose, 4000);
    return () => clearTimeout(timer);
  }, [onClose]);

  return (
    <div className={`${styles.toast} ${styles[notification.type]}`}>
      <span>{notification.message}</span>
      <button className={styles.close} onClick={onClose}>x</button>
    </div>
  );
}
