import { useTaskTreeStore } from '../store/taskTreeStore';
import styles from './CanvasToolbar.module.css';

export function CanvasToolbar() {
  const zoom = useTaskTreeStore((s) => s.zoom);
  const zoomIn = useTaskTreeStore((s) => s.zoomIn);
  const zoomOut = useTaskTreeStore((s) => s.zoomOut);
  const fitView = useTaskTreeStore((s) => s.fitView);
  const resetView = useTaskTreeStore((s) => s.resetView);

  return (
    <div className={styles.toolbar} role="toolbar" aria-label="Canvas controls">
      <button className={styles.btn} onClick={zoomIn} title="Zoom In" aria-label="Zoom in">+</button>
      <span className={styles.zoomLabel} aria-live="polite">{Math.round(zoom * 100)}%</span>
      <button className={styles.btn} onClick={zoomOut} title="Zoom Out" aria-label="Zoom out">&minus;</button>
      <span className={styles.sep} />
      <button className={styles.btn} onClick={fitView} title="Fit View" aria-label="Fit all nodes in view">Fit</button>
      <button className={styles.btn} onClick={resetView} title="Reset View" aria-label="Reset to default view">Reset</button>
      <span className={styles.sep} />
      <span className={styles.hint}>Drag to pan, Scroll to zoom</span>
    </div>
  );
}
