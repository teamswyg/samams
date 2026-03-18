import { useMutation, useQueryClient } from '@tanstack/react-query';
import { queryKeys } from '../../../shared/constants/queryKeys';
import { sendMessageApi } from '../api/sendMessage';
import { useComposerStore } from '../store/composerStore';

export function useSendMessage(sessionId) {
  const queryClient = useQueryClient();
  const clearDraft = useComposerStore((s) => s.clearDraft);

  return useMutation({
    mutationFn: (message) => sendMessageApi(sessionId, message),
    onSuccess: () => {
      clearDraft();
      queryClient.invalidateQueries({ queryKey: queryKeys.sessions.messages(sessionId) });
    },
  });
}
