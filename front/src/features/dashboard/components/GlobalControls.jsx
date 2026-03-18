import { useCallback } from 'react';
import { useDashboardStore } from '../store/dashboardStore';
import styles from './GlobalControls.module.css';

export function GlobalControls() {
  const pauseAll = useDashboardStore((s) => s.pauseAll);
  const resumeAll = useDashboardStore((s) => s.resumeAll);
  const openMeeting = useDashboardStore((s) => s.openMeeting);
  const pollMeetingStatus = useDashboardStore((s) => s.pollMeetingStatus);

  const handleMeeting = useCallback(async () => {
    // Fetch current meeting status before opening modal.
    await pollMeetingStatus();
    openMeeting();
  }, [pollMeetingStatus, openMeeting]);

  return (
    <div className={styles.controls} role="toolbar" aria-label="Global agent controls">
      <span className={styles.label}>Global Controls</span>
      <div className={styles.buttons}>
        <button className={styles.btnWarning} onClick={pauseAll} aria-label="Pause all agents">
          Pause All
        </button>
        <button className={styles.btnPrimary} onClick={resumeAll} aria-label="Resume all agents">
          Resume All
        </button>
        <button className={styles.btnMeeting} onClick={handleMeeting} aria-label="Open strategy meeting">
          Strategy Meeting
        </button>
      </div>
    </div>
  );
}
