import { usePlanningStore } from '../store/planningStore';
import { FeatureCard } from './FeatureCard';
import styles from './PlanningEditor.module.css';

const techSpecFields = [
  { key: 'techStack', label: 'Tech Stack', placeholder: 'e.g. Go, React, PostgreSQL, Redis, gRPC...' },
  { key: 'architecture', label: 'Architecture', placeholder: 'e.g. Hexagonal Architecture (Ports & Adapters), CQRS...' },
  { key: 'folderStructure', label: 'Folder Structure', placeholder: 'e.g.\n/cmd/server/main.go\n/internal/domain/\n/internal/application/port/\n/internal/infra/' },
  { key: 'framework', label: 'Framework & Libraries', placeholder: 'e.g. Vite, Zustand, Axios, net/http, AWS Lambda...' },
  { key: 'codingConventions', label: 'Coding Conventions', placeholder: 'e.g. camelCase for JS, snake_case for Go, Feature-based folder structure...' },
  { key: 'boundedContexts', label: 'DDD Bounded Contexts', placeholder: 'e.g.\n- User BC: Auth, Google OAuth\n- AI BC: OpenAI/Claude/Gemini integration\n- Execution BC: Task Node execution, Cascading' },
];

const abstractSpecFields = [
  { key: 'domainOverview', label: 'Domain Overview', placeholder: 'e.g. AI Agent management system. Sentinel orchestrates multiple AI agents...' },
  { key: 'aggregates', label: 'Aggregates & Entities', placeholder: 'e.g.\n- TaskNode (Aggregate Root): id, summary, status, parentId\n- Agent: id, name, status, currentTask\n- PlanDocument: title, goal, features' },
  { key: 'events', label: 'Domain Events', placeholder: 'e.g.\n- TaskAssigned: task assigned to agent\n- TaskCompleted: execution complete, propagate summary\n- FrontierGenerated: child frontier command generated' },
  { key: 'workflows', label: 'Workflows & Use Cases', placeholder: 'e.g.\n1. Planning -> AI plan generation -> Tree conversion\n2. Cascading Execution: Parent complete -> Build child context -> Generate frontier -> Assign child' },
];

export function PlanningEditor() {
  const document = usePlanningStore((s) => s.document);
  const isSaved = usePlanningStore((s) => s.isSaved);
  const setTitle = usePlanningStore((s) => s.setTitle);
  const setGoal = usePlanningStore((s) => s.setGoal);
  const setDescription = usePlanningStore((s) => s.setDescription);
  const addFeature = usePlanningStore((s) => s.addFeature);
  const saveDocument = usePlanningStore((s) => s.saveDocument);
  const updateTechSpec = usePlanningStore((s) => s.updateTechSpec);
  const updateAbstractSpec = usePlanningStore((s) => s.updateAbstractSpec);
  const planList = usePlanningStore((s) => s.planList);
  const showPlanList = usePlanningStore((s) => s.showPlanList);
  const togglePlanList = usePlanningStore((s) => s.togglePlanList);
  const loadPlan = usePlanningStore((s) => s.loadPlan);
  const deletePlan = usePlanningStore((s) => s.deletePlan);
  const newPlan = usePlanningStore((s) => s.newPlan);

  const techSpec = document.techSpec || {};
  const abstractSpec = document.abstractSpec || {};

  const formatDate = (iso) => {
    if (!iso) return '-';
    return new Date(iso).toLocaleString('en-US', {
      year: 'numeric', month: 'short', day: 'numeric',
      hour: '2-digit', minute: '2-digit', second: '2-digit',
    });
  };

  return (
    <div className={styles.editor}>
      <div className={styles.inner}>
        <div className={styles.saveBar}>
          <div className={styles.saveBarLeft}>
            <button className={styles.saveBtn} onClick={saveDocument}>
              &#128190; Save
            </button>
            <button className={styles.newBtn} onClick={newPlan}>
              + New
            </button>
            <div className={styles.saveStatus}>
              <span className={`${styles.saveDot} ${isSaved ? styles.saved : styles.unsaved}`} />
              <span className={styles.saveText}>{isSaved ? 'Saved' : 'Unsaved'}</span>
            </div>
          </div>
          <button
            className={`${styles.listBtn} ${showPlanList ? styles.listBtnActive : ''}`}
            onClick={togglePlanList}
          >
            &#128203; List ({planList.length})
          </button>
        </div>

        {showPlanList && (
          <div className={styles.planListPanel}>
            <div className={styles.planListHeader}>
              <span className={styles.planListTitle}>Saved Plans</span>
            </div>
            {planList.length === 0 ? (
              <div className={styles.planListEmpty}>No saved plans yet.</div>
            ) : (
              <div className={styles.planListItems}>
                {planList.map((plan) => (
                  <div
                    key={plan.id}
                    className={`${styles.planListItem} ${plan.id === document.id ? styles.planListItemActive : ''}`}
                  >
                    <div className={styles.planListItemInfo} onClick={() => loadPlan(plan.id)}>
                      <span className={styles.planListItemTitle}>
                        {plan.title || 'Untitled'}
                      </span>
                      <span className={styles.planListItemDate}>
                        {new Date(plan.updatedAt).toLocaleDateString()}
                      </span>
                    </div>
                    <button
                      className={styles.planListItemDelete}
                      onClick={() => deletePlan(plan.id)}
                      title="Delete"
                    >
                      &times;
                    </button>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        <div className={styles.section}>
          <label className={styles.label}>Project Title</label>
          <input
            className={styles.titleInput}
            value={document.title}
            onChange={(e) => setTitle(e.target.value)}
            placeholder="Enter project title"
          />
        </div>

        <div className={styles.section}>
          <label className={styles.label}>Main Goal</label>
          <textarea
            className={styles.goalInput}
            rows={4}
            value={document.goal}
            onChange={(e) => setGoal(e.target.value)}
            placeholder="What do you want to achieve with this project?"
          />
        </div>

        <div className={styles.section}>
          <label className={styles.label}>Description</label>
          <textarea
            className={styles.descInput}
            rows={8}
            value={document.description || ''}
            onChange={(e) => setDescription(e.target.value)}
            placeholder={"Detailed project description.\n\ne.g.:\n- System overview & core concepts\n- Target users\n- Key technical requirements\n- Business rules & constraints\n- Expected scale & performance"}
          />
        </div>

        <div className={styles.specSection}>
          <div className={styles.specHeader}>
            <span className={styles.specIcon}>&#9881;</span>
            <h3 className={styles.specTitle}>Tech Specification</h3>
            <span className={styles.specBadge}>Technical Specification</span>
          </div>
          <div className={styles.specGrid}>
            {techSpecFields.map((field) => (
              <div key={field.key} className={styles.specField}>
                <label className={styles.label}>{field.label}</label>
                <textarea
                  className={styles.specInput}
                  rows={field.key === 'folderStructure' || field.key === 'boundedContexts' ? 8 : 4}
                  value={techSpec[field.key] || ''}
                  onChange={(e) => updateTechSpec(field.key, e.target.value)}
                  placeholder={field.placeholder}
                />
              </div>
            ))}
          </div>
        </div>

        <div className={styles.specSection}>
          <div className={styles.specHeader}>
            <span className={styles.specIcon}>&#9670;</span>
            <h3 className={styles.specTitle}>Abstract Specification</h3>
            <span className={styles.specBadge}>Domain & Event Specification</span>
          </div>
          <div className={styles.specGrid}>
            {abstractSpecFields.map((field) => (
              <div key={field.key} className={styles.specField}>
                <label className={styles.label}>{field.label}</label>
                <textarea
                  className={styles.specInput}
                  rows={field.key === 'events' || field.key === 'workflows' ? 8 : 5}
                  value={abstractSpec[field.key] || ''}
                  onChange={(e) => updateAbstractSpec(field.key, e.target.value)}
                  placeholder={field.placeholder}
                />
              </div>
            ))}
          </div>
        </div>

        <div className={styles.section}>
          <div className={styles.featureHeader}>
            <label className={styles.label}>Features ({document.features.length})</label>
            <button className={styles.addFeatureBtn} onClick={addFeature}>
              + Add Feature
            </button>
          </div>

          {document.features.length === 0 ? (
            <div className={styles.emptyFeatures}>
              <span className={styles.emptyIcon}>&#128196;</span>
              <p>No features added yet.</p>
              <p className={styles.emptyHint}>Chat with Sentinel AI or add manually.</p>
            </div>
          ) : (
            <div className={styles.featureList}>
              {document.features.map((feature, i) => (
                <FeatureCard key={feature.id} feature={feature} index={i} />
              ))}
            </div>
          )}
        </div>

        <div className={styles.metaFooter}>
          <span>Created: {formatDate(document.createdAt)}</span>
          <span>Updated: {formatDate(document.updatedAt)}</span>
        </div>
      </div>
    </div>
  );
}
