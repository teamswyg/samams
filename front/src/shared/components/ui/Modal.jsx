import styles from './Modal.module.css';

export function Modal({ open, onClose, title, children }) {
  if (!open) return null;

  return (
    <div className={styles.overlay} onClick={onClose}>
      <div className={styles.modal} onClick={(e) => e.stopPropagation()}>
        <div className={styles.header}>
          <h3>{title}</h3>
          <button className={styles.close} onClick={onClose}>x</button>
        </div>
        <div className={styles.body}>{children}</div>
      </div>
    </div>
  );
}
