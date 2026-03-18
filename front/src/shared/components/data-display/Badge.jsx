import styles from './Badge.module.css';

const colorMap = {
  idle: 'gray',
  running: 'green',
  starting: 'blue',
  paused: 'yellow',
  waiting: 'yellow',
  meeting: 'purple',
  stopped: 'orange',
  terminated: 'red',
  error: 'red',
  info: 'blue',
  warning: 'yellow',
  critical: 'red',
  success: 'green',
};

export function Badge({ label, variant }) {
  const color = colorMap[variant] || 'gray';
  return <span className={`${styles.badge} ${styles[color]}`}>{label}</span>;
}
