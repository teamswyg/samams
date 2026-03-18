import { Button } from '../ui/Button';

export function ErrorFallback({ error, resetErrorBoundary }) {
  return (
    <div style={{ padding: 40, textAlign: 'center' }}>
      <h2>Something went wrong</h2>
      <p style={{ color: 'var(--color-text-secondary)', margin: '12px 0' }}>
        {error?.message || 'Unknown error'}
      </p>
      {resetErrorBoundary && (
        <Button onClick={resetErrorBoundary}>Try again</Button>
      )}
    </div>
  );
}
