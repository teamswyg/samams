import { Spinner } from '../ui/Spinner';

export function LoadingState({ message = 'Loading...' }) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 12, padding: 40 }}>
      <Spinner size={32} />
      <span style={{ color: 'var(--color-text-secondary)', fontSize: 14 }}>{message}</span>
    </div>
  );
}
