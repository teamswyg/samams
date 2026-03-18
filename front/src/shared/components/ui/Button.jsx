import styles from './Button.module.css';

export function Button({ children, variant = 'primary', size = 'md', disabled, ...props }) {
  return (
    <button
      className={`${styles.btn} ${styles[variant]} ${styles[size]}`}
      disabled={disabled}
      {...props}
    >
      {children}
    </button>
  );
}
