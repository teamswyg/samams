import { useCallback } from 'react';
import { useNotificationStore } from '../stores/notificationStore';

export function useToast() {
  const addNotification = useNotificationStore((s) => s.addNotification);

  const toast = useCallback((message, type = 'info') => {
    addNotification({ message, type, id: Date.now().toString() });
  }, [addNotification]);

  return { toast };
}
