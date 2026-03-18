import { useQuery } from '@tanstack/react-query';
import { queryKeys } from '../../../shared/constants/queryKeys';
import { getSentinelStatusApi } from '../api/getSentinelStatus';
import { useSentinelUiStore } from '../store/sentinelUiStore';

export function useSentinelStatus() {
  const autoRefresh = useSentinelUiStore((s) => s.autoRefresh);

  return useQuery({
    queryKey: queryKeys.sentinel.status,
    queryFn: getSentinelStatusApi,
    refetchInterval: autoRefresh ? 5000 : false,
  });
}
