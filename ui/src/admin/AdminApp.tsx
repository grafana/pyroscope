import { QueryDiagnosticsPage } from './pages/QueryDiagnosticsPage';
import { DiagnosticsListPage } from './pages/DiagnosticsListPage';

export function AdminApp() {
  const path = window.location.pathname;

  if (path.endsWith('/query-diagnostics/list')) {
    return <DiagnosticsListPage />;
  }

  return <QueryDiagnosticsPage />;
}
