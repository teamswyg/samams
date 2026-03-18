import { Badge } from '../../../shared/components/data-display/Badge';

export function RunStatusBadge({ status }) {
  return <Badge label={status} variant={status} />;
}
