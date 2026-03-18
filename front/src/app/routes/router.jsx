import { createBrowserRouter, useRouteError } from 'react-router-dom';
import { DashboardLayout } from '../layouts/DashboardLayout';
import { AuthLayout } from '../layouts/AuthLayout';
import { AuthGuard } from '../../features/auth/components/AuthGuard';
import { ErrorFallback } from '../../shared/components/feedback/ErrorFallback';
import { LoginPage } from '../../pages/auth/LoginPage';
import { DashboardPage } from '../../pages/dashboard/DashboardPage';
import { AgentChatPage } from '../../pages/agent/AgentChatPage';
import { SentinelOverviewPage } from '../../pages/sentinel/SentinelOverviewPage';
import { SettingsPage } from '../../pages/settings/SettingsPage';
import { LogViewerPage } from '../../pages/log-viewer/LogViewerPage';
import { PlanningPage } from '../../pages/planning/PlanningPage';
import { TaskTreePage } from '../../pages/task-tree/TaskTreePage';
import { NotFoundPage } from '../../pages/not-found/NotFoundPage';

function RouterErrorBoundary() {
  const error = useRouteError();
  return <ErrorFallback error={error} resetErrorBoundary={() => window.location.reload()} />;
}

export const router = createBrowserRouter([
  {
    errorElement: <RouterErrorBoundary />,
    children: [
      {
        path: '/login',
        element: <AuthLayout />,
        children: [
          { index: true, element: <LoginPage /> },
        ],
      },
      {
        path: '/',
        element: <AuthGuard><DashboardPage /></AuthGuard>,
      },
      {
        path: '/dashboard',
        element: <AuthGuard><DashboardPage /></AuthGuard>,
      },
      {
        path: '/log-viewer',
        element: <AuthGuard><LogViewerPage /></AuthGuard>,
      },
      {
        path: '/planning',
        element: <AuthGuard><PlanningPage /></AuthGuard>,
      },
      {
        path: '/task-tree',
        element: <AuthGuard><TaskTreePage /></AuthGuard>,
      },
      {
        path: '/',
        element: <AuthGuard><DashboardLayout /></AuthGuard>,
        children: [
          { path: 'agent/sessions', element: <AgentChatPage /> },
          { path: 'agent/sessions/:sessionId', element: <AgentChatPage /> },
          { path: 'sentinel/overview', element: <SentinelOverviewPage /> },
          { path: 'sentinel/alerts', element: <SentinelOverviewPage /> },
          { path: 'sentinel/logs', element: <SentinelOverviewPage /> },
          { path: 'settings', element: <SettingsPage /> },
          { path: '*', element: <NotFoundPage /> },
        ],
      },
    ],
  },
]);
