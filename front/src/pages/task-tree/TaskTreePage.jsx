import { useRef, useCallback, useEffect } from 'react';
import { useTaskTreeStore } from '../../features/task-tree/store/taskTreeStore';
import { TaskTreeHeader } from '../../features/task-tree/components/TaskTreeHeader';
import { CanvasEdges } from '../../features/task-tree/components/CanvasEdges';
import { NodeCard } from '../../features/task-tree/components/NodeCard';
import { CanvasToolbar } from '../../features/task-tree/components/CanvasToolbar';
import { CanvasLegend } from '../../features/task-tree/components/CanvasLegend';
import { NodeDetailPanel } from '../../features/task-tree/components/NodeDetailPanel';
import { LoadingState } from '../../shared/components/feedback/LoadingState';
import styles from './TaskTreePage.module.css';

export function TaskTreePage() {
  const nodes = useTaskTreeStore((s) => s.nodes);
  const positions = useTaskTreeStore((s) => s.positions);
  const zoom = useTaskTreeStore((s) => s.zoom);
  const pan = useTaskTreeStore((s) => s.pan);
  const setZoom = useTaskTreeStore((s) => s.setZoom);
  const setPan = useTaskTreeStore((s) => s.setPan);
  const selectNode = useTaskTreeStore((s) => s.selectNode);
  const isLoading = useTaskTreeStore((s) => s.isLoading);
  const nodeCount = nodes.length;

  const canvasRef = useRef(null);
  const dragging = useRef(false);
  const lastMouse = useRef({ x: 0, y: 0 });

  // Pan: mouse drag
  const onMouseDown = useCallback((e) => {
    if (e.target === canvasRef.current || e.target.dataset.canvas) {
      dragging.current = true;
      lastMouse.current = { x: e.clientX, y: e.clientY };
      canvasRef.current.style.cursor = 'grabbing';
    }
  }, []);

  const onMouseMove = useCallback((e) => {
    if (!dragging.current) return;
    const dx = e.clientX - lastMouse.current.x;
    const dy = e.clientY - lastMouse.current.y;
    lastMouse.current = { x: e.clientX, y: e.clientY };
    setPan({ x: pan.x + dx, y: pan.y + dy });
  }, [pan, setPan]);

  const onMouseUp = useCallback(() => {
    dragging.current = false;
    if (canvasRef.current) canvasRef.current.style.cursor = 'grab';
  }, []);

  // Zoom: mouse wheel
  const onWheel = useCallback((e) => {
    e.preventDefault();
    const delta = e.deltaY > 0 ? -0.05 : 0.05;
    setZoom(zoom + delta);
  }, [zoom, setZoom]);

  useEffect(() => {
    const el = canvasRef.current;
    if (!el) return;
    el.addEventListener('wheel', onWheel, { passive: false });
    return () => el.removeEventListener('wheel', onWheel);
  }, [onWheel]);

  // Load tree from server on mount.
  const initFromServer = useTaskTreeStore((s) => s.initFromServer);
  useEffect(() => { initFromServer(); }, [initFromServer]);

  // Poll proxy for task status updates (every 3s)
  const syncFromProxy = useTaskTreeStore((s) => s.syncFromProxy);
  useEffect(() => {
    syncFromProxy();
    const timer = setInterval(syncFromProxy, 3000);
    return () => clearInterval(timer);
  }, [syncFromProxy]);

  // Click on canvas background deselects
  const onCanvasClick = (e) => {
    if (e.target === canvasRef.current || e.target.dataset.canvas) {
      selectNode(null);
    }
  };

  // Dot grid background
  const gridSize = 32 * zoom;
  const offsetX = pan.x % gridSize;
  const offsetY = pan.y % gridSize;
  const dotBg = `radial-gradient(circle, rgba(0,245,160,0.15) 1px, transparent 1px)`;

  if (isLoading) {
    return (
      <div className={styles.page}>
        <TaskTreeHeader />
        <LoadingState message="Loading task tree..." />
      </div>
    );
  }

  return (
    <div className={styles.page}>
      <TaskTreeHeader />
      <div
        ref={canvasRef}
        className={styles.canvas}
        role="application"
        aria-label="Task tree canvas"
        onMouseDown={onMouseDown}
        onMouseMove={onMouseMove}
        onMouseUp={onMouseUp}
        onMouseLeave={onMouseUp}
        onClick={onCanvasClick}
      >
        {/* Dot grid */}
        <div
          className={styles.dotGrid}
          data-canvas="true"
          style={{
            backgroundImage: dotBg,
            backgroundSize: `${gridSize}px ${gridSize}px`,
            backgroundPosition: `${offsetX}px ${offsetY}px`,
          }}
        />

        {/* Transform layer */}
        <div
          className={styles.transformLayer}
          style={{
            transform: `translate(${pan.x}px, ${pan.y}px) scale(${zoom})`,
            transformOrigin: '0 0',
          }}
        >
          <CanvasEdges />
          {nodes.map((node, i) => {
            const pos = positions[node.id];
            if (!pos) return null;
            return <NodeCard key={node.id} node={node} position={pos} index={i} />;
          })}
        </div>

        {/* Overlays */}
        <CanvasLegend />
        <NodeDetailPanel />
        <CanvasToolbar />

        {/* Node count badge */}
        <div className={styles.nodeCount}>
          {nodeCount} nodes
        </div>
      </div>
    </div>
  );
}
