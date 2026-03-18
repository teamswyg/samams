import { Link } from 'react-router-dom';
import { Button } from '../../shared/components/ui/Button';

export function NotFoundPage() {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', minHeight: '100vh', gap: 16 }}>
      <h1 style={{ fontSize: 48, fontWeight: 700 }}>404</h1>
      <p style={{ color: 'var(--color-text-secondary)' }}>Page not found</p>
      <Link to="/dashboard"><Button>Go to Dashboard</Button></Link>
    </div>
  );
}
