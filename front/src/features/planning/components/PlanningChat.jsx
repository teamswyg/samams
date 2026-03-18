import { useRef, useEffect } from 'react';
import Markdown from 'react-markdown';
import { usePlanningStore } from '../store/planningStore';
import styles from './PlanningChat.module.css';

const quickCommands = [
  { label: '1. Set Title', text: 'Title: ' },
  { label: '2. Generate Goal', text: 'Goal: generate' },
  { label: '3. Generate Description', text: 'Description: generate' },
  { label: '4. Generate Tech Spec', text: 'Tech Spec generate' },
  { label: '5. Generate Abstract', text: 'Abstract generate' },
  { label: '6. Generate Features', text: 'Features: generate' },
  { label: '7. Convert to Tree', text: 'Convert to Node Tree' },
  { label: 'Help', text: 'help' },
];

export function PlanningChat() {
  const messages = usePlanningStore((s) => s.messages);
  const chatInput = usePlanningStore((s) => s.chatInput);
  const isTyping = usePlanningStore((s) => s.isTyping);
  const setChatInput = usePlanningStore((s) => s.setChatInput);
  const sendChat = usePlanningStore((s) => s.sendChat);
  const clearChat = usePlanningStore((s) => s.clearChat);

  const feedRef = useRef(null);
  const inputRef = useRef(null);

  useEffect(() => {
    if (feedRef.current) {
      feedRef.current.scrollTop = feedRef.current.scrollHeight;
    }
  }, [messages, isTyping]);

  const handleKey = (e) => {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      sendChat(chatInput);
    }
  };

  const handleQuick = (text) => {
    setChatInput(text);
    inputRef.current?.focus();
  };

  const formatTime = (iso) => {
    const d = new Date(iso);
    return d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit' });
  };

  return (
    <div className={styles.panel}>
      <div className={styles.chatHeader}>
        <div className={styles.aiId}>
          <div className={styles.avatarWrap}>
            <span className={styles.avatarIcon}>&#129302;</span>
            <span className={styles.statusDot} />
          </div>
          <div className={styles.aiInfo}>
            <span className={styles.aiName}>SENTINEL AI</span>
            <span className={styles.aiRole}>Planning Assistant</span>
          </div>
        </div>
        <button className={styles.clearBtn} onClick={clearChat}>
          &#128465; Clear
        </button>
      </div>

      <div className={styles.quickBar}>
        {quickCommands.map((cmd) => (
          <button
            key={cmd.label}
            className={styles.quickBtn}
            onClick={() => handleQuick(cmd.text)}
          >
            {cmd.label}
          </button>
        ))}
      </div>

      <div className={styles.feed} ref={feedRef}>
        {messages.map((msg) => (
          <div
            key={msg.id}
            className={`${styles.msgRow} ${msg.sender === 'user' ? styles.userRow : styles.sentinelRow}`}
          >
            <div
              className={`${styles.bubble} ${msg.sender === 'user' ? styles.userBubble : styles.sentinelBubble}`}
            >
              {msg.sender === 'sentinel' ? (
                <div className={styles.msgText}>
                  <Markdown>{msg.content}</Markdown>
                </div>
              ) : (
                <pre className={styles.msgText}>{msg.content}</pre>
              )}
              <span className={styles.msgTime}>{formatTime(msg.timestamp)}</span>
            </div>
          </div>
        ))}
        {isTyping && (
          <div className={`${styles.msgRow} ${styles.sentinelRow}`}>
            <div className={`${styles.bubble} ${styles.sentinelBubble}`}>
              <div className={styles.typingDots}>
                <span style={{ animationDelay: '0ms' }} />
                <span style={{ animationDelay: '150ms' }} />
                <span style={{ animationDelay: '300ms' }} />
              </div>
            </div>
          </div>
        )}
      </div>

      <div className={styles.inputBar}>
        <textarea
          ref={inputRef}
          className={styles.input}
          rows={3}
          value={chatInput}
          onChange={(e) => setChatInput(e.target.value)}
          onKeyDown={handleKey}
          placeholder="Enter planning details... (Shift+Enter: newline)"
        />
        <button
          className={styles.sendBtn}
          onClick={() => sendChat(chatInput)}
          disabled={!chatInput.trim()}
        >
          &#9654;
        </button>
      </div>
    </div>
  );
}
