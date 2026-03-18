import { useState, useEffect } from 'react';
import { useParams } from 'react-router-dom';
import { ChatWindow } from '../../features/agent-chat/components/ChatWindow';
import http from '../../shared/api/http';
import { endpoints } from '../../shared/api/endpoints';
import styles from './AgentChatPage.module.css';

export function AgentChatPage() {
  const { sessionId } = useParams();
  const [messages, setMessages] = useState([]);
  const [loading, setLoading] = useState(false);

  // Load session messages on mount.
  useEffect(() => {
    if (!sessionId) return;
    async function load() {
      try {
        const { data } = await http.get(endpoints.sessions.messages(sessionId));
        if (Array.isArray(data)) setMessages(data);
      } catch {}
    }
    load();
  }, [sessionId]);

  const handleSend = async (content) => {
    const userMsg = { id: Date.now(), role: 'user', content };
    setMessages((prev) => [...prev, userMsg]);
    setLoading(true);

    try {
      const { data } = await http.post(
        sessionId ? endpoints.sessions.send(sessionId) : endpoints.ai.chat,
        sessionId ? { content } : { message: content, context: '' }
      );

      const reply = data.reply || data.content || data.message || JSON.stringify(data);
      setMessages((prev) => [
        ...prev,
        { id: Date.now() + 1, role: 'assistant', content: reply },
      ]);
    } catch (err) {
      setMessages((prev) => [
        ...prev,
        { id: Date.now() + 1, role: 'assistant', content: `Error: ${err.message || 'Failed to send'}` },
      ]);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className={styles.page}>
      <ChatWindow messages={messages} onSend={handleSend} loading={loading} />
    </div>
  );
}
