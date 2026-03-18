export function EmptyState({ message = 'No data found' }) {
  return (
    <div style={{ padding: 40, textAlign: 'center', color: 'var(--color-text-secondary)' }}>
      <p>{message}</p>
    </div>
  );
}
