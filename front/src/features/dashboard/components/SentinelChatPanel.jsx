import { useState, useRef, useEffect } from 'react';
import { useDashboardStore } from '../store/dashboardStore';
import styles from './SentinelChatPanel.module.css';

const quickCommands = ['STATUS', 'PAUSE ALL', 'RESUME ALL', 'CONFLICT SCAN', 'HELP'];

export function SentinelChatPanel() {
  const chatOpen = useDashboardStore((s) => s.chatOpen);
  const toggleChat = useDashboardStore((s) => s.toggleChat);
  const chatMessages = useDashboardStore((s) => s.chatMessages);
  const sendChat = useDashboardStore((s) => s.sendChat);
  const [input, setInput] = useState('');
  const [typing, setTyping] = useState(false);
  const listRef = useRef(null);
  const prevLen = useRef(chatMessages.length);

  useEffect(() => {
    if (listRef.current) {
      listRef.current.scrollTop = listRef.current.scrollHeight;
    }
    if (chatMessages.length > prevLen.current) {
      const last = chatMessages[chatMessages.length - 1];
      if (last.sender === 'user') {
        setTyping(true);
        setTimeout(() => setTyping(false), 750);
      }
    }
    prevLen.current = chatMessages.length;
  }, [chatMessages]);

  if (!chatOpen) return null;

  const handleSend = () => {
    if (!input.trim()) return;
    sendChat(input.trim());
    setInput('');
  };

  const handleKey = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  };

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <div className={styles.headerLeft}>
          <span className={styles.dot} />
          <span className={styles.headerTitle}>SENTINEL AI</span>
          <span className={styles.headerStatus}>Online &middot; Monitoring</span>
        </div>
        <div className={styles.headerRight}>
          <button className={styles.clearBtn} title="Clear chat">&#128465;</button>
          <button className={styles.closeBtn} onClick={toggleChat}>&#10005;</button>
        </div>
      </div>

      <div className={styles.messages} ref={listRef}>
        {chatMessages.map((msg) => (
          <div
            key={msg.id}
            className={`${styles.msgRow} ${msg.sender === 'user' ? styles.userRow : styles.sentinelRow}`}
          >
            <div className={styles.msgIcon}>
              {msg.sender === 'sentinel' ? '&#129302;' : '&#128100;'}
            </div>
            <div
              className={`${styles.msgBubble} ${msg.sender === 'user' ? styles.userBubble : styles.sentinelBubble}`}
            >
              <pre className={styles.msgText}>{msg.text}</pre>
            </div>
          </div>
        ))}
        {typing && (
          <div className={`${styles.msgRow} ${styles.sentinelRow}`}>
            <div className={styles.msgIcon}>&#129302;</div>
            <div className={`${styles.msgBubble} ${styles.sentinelBubble}`}>
              <div className={styles.typingDots}>
                <span style={{ animationDelay: '0ms' }} />
                <span style={{ animationDelay: '150ms' }} />
                <span style={{ animationDelay: '300ms' }} />
              </div>
            </div>
          </div>
        )}
      </div>

      <div className={styles.quickCmds}>
        {quickCommands.map((cmd) => (
          <button key={cmd} className={styles.quickBtn} onClick={() => sendChat(cmd)}>
            {cmd}
          </button>
        ))}
      </div>

      <div className={styles.inputRow}>
        <span className={styles.inputPrompt}>&gt;_</span>
        <input
          className={styles.input}
          value={input}
          onChange={(e) => setInput(e.target.value)}
          onKeyDown={handleKey}
          placeholder="Enter command or ask Sentinel AI..."
        />
        <button className={styles.sendBtn} onClick={handleSend}>
          &#9654;
        </button>
      </div>
    </div>
  );
}
