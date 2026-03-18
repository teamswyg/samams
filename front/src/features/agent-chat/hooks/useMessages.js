import { useQuery } from '@tanstack/react-query';
import { queryKeys } from '../../../shared/constants/queryKeys';
import { getMessagesApi } from '../api/getMessages';

export function useMessages(sessionId) {
  return useQuery({
    queryKey: queryKeys.sessions.messages(sessionId),
    queryFn: () => getMessagesApi(sessionId),
    enabled: !!sessionId,
  });
}
