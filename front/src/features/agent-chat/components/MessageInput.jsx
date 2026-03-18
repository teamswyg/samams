import { useComposerStore } from '../store/composerStore';
import { Button } from '../../../shared/components/ui/Button';
import styles from './MessageInput.module.css';

export function MessageInput({ onSend, disabled }) {
  const draft = useComposerStore((s) => s.draft);
  const setDraft = useComposerStore((s) => s.setDraft);

  const handleSubmit = (e) => {
    e.preventDefault();
    if (!draft.trim()) return;
    onSend(draft);
  };

  const handleKeyDown = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSubmit(e);
    }
  };

  return (
    <form className={styles.form} onSubmit={handleSubmit}>
      <textarea
        className={styles.input}
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Type a message..."
        rows={1}
        disabled={disabled}
      />
      <Button type="submit" disabled={disabled || !draft.trim()} size="sm">
        Send
      </Button>
    </form>
  );
}
