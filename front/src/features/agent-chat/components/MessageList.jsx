import styles from './MessageList.module.css';

export function MessageList({ messages = [] }) {
  if (messages.length === 0) {
    return <div className={styles.empty}>No messages yet. Start a conversation.</div>;
  }

  return (
    <div className={styles.list}>
      {messages.map((msg, i) => (
        <div key={msg.id || i} className={`${styles.message} ${styles[msg.role]}`}>
          <span className={styles.role}>{msg.role}</span>
          <div className={styles.content}>{msg.content}</div>
        </div>
      ))}
    </div>
  );
}
