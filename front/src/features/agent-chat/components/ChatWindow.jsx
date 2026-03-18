import { MessageList } from './MessageList';
import { MessageInput } from './MessageInput';
import styles from './ChatWindow.module.css';

export function ChatWindow({ messages, onSend, loading }) {
  return (
    <div className={styles.window}>
      <MessageList messages={messages} />
      <MessageInput onSend={onSend} disabled={loading} />
    </div>
  );
}
