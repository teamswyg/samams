import { useEffect } from 'react';
import { usePlanningStore } from '../../features/planning/store/planningStore';
import { PlanningHeader } from '../../features/planning/components/PlanningHeader';
import { PlanningChat } from '../../features/planning/components/PlanningChat';
import { PlanningEditor } from '../../features/planning/components/PlanningEditor';
import styles from './PlanningPage.module.css';

export function PlanningPage() {
  const initFromServer = usePlanningStore((s) => s.initFromServer);
  useEffect(() => { initFromServer(); }, [initFromServer]);

  return (
    <div className={styles.page}>
      <PlanningHeader />
      <div className={styles.main}>
        <PlanningChat />
        <PlanningEditor />
      </div>
    </div>
  );
}
