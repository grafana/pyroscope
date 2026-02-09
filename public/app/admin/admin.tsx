import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryDiagnosticsPage } from './pages/QueryDiagnosticsPage';
import { DiagnosticsListPage } from './pages/DiagnosticsListPage';
import './styles.css';

function AdminApp() {
  const path = window.location.pathname;

  if (path.endsWith('/query-diagnostics/list')) {
    return <DiagnosticsListPage />;
  }

  return <QueryDiagnosticsPage />;
}

const container = document.getElementById('root');
if (container) {
  const root = ReactDOM.createRoot(container);
  root.render(<AdminApp />);
}
