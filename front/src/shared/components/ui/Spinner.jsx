import styles from './Spinner.module.css';

export function Spinner({ size = 24 }) {
  return (
    <div className={styles.spinner} role="status" aria-label="Loading" style={{ width: size, height: size }} />
  );
}
