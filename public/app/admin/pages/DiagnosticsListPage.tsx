import React, { useEffect, useState } from 'react';
import { Header } from '../components/Header';
import { fetchTenants, listDiagnostics } from '../services/api';
import { formatBytes, formatMs, formatTime } from '../utils';
import type { DiagnosticSummary } from '../types';

export function DiagnosticsListPage() {
  const [tenants, setTenants] = useState<string[]>([]);
  const [selectedTenant, setSelectedTenant] = useState<string | null>(null);
  const [diagnostics, setDiagnostics] = useState<DiagnosticSummary[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    async function loadTenants() {
      try {
        const tenantList = await fetchTenants();
        setTenants(tenantList);
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : 'Failed to load tenants'
        );
      }
    }
    loadTenants();
  }, []);

  useEffect(() => {
    const urlParams = new URLSearchParams(window.location.search);
    const tenant = urlParams.get('tenant');
    if (tenant) {
      setSelectedTenant(tenant);
    }
  }, []);

  useEffect(() => {
    if (!selectedTenant) {
      setDiagnostics([]);
      return;
    }

    async function loadDiagnostics() {
      setLoading(true);
      setError(null);
      try {
        const diagList = await listDiagnostics(selectedTenant!);
        setDiagnostics(diagList);
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : 'Failed to load diagnostics'
        );
        setDiagnostics([]);
      } finally {
        setLoading(false);
      }
    }
    loadDiagnostics();
  }, [selectedTenant]);

  const handleTenantSelect = (tenant: string) => {
    setSelectedTenant(tenant);
    window.history.pushState({}, '', `?tenant=${encodeURIComponent(tenant)}`);
  };

  return (
    <main>
      <div className="container-fluid mt-4">
        <Header
          title="Stored Diagnostics"
          subtitle="Browse and view stored query diagnostics"
          showNewQueryLink={true}
          showStoredDiagnosticsLink={false}
        />

        {error && (
          <div className="alert alert-danger mt-3" role="alert">
            <strong>Error:</strong> {error}
          </div>
        )}

        <div className="row mt-4">
          <div className="col-12 col-md-3 col-lg-2">
            <div className="card">
              <div className="card-header">
                <h6 className="mb-0">Tenants</h6>
              </div>
              <div className="card-body p-0">
                {tenants.length > 0 ? (
                  <div className="tenant-list">
                    {tenants.map((tenant) => (
                      <a
                        key={tenant}
                        href={`?tenant=${encodeURIComponent(tenant)}`}
                        className={`tenant-item ${selectedTenant === tenant ? 'active' : ''}`}
                        onClick={(e) => {
                          e.preventDefault();
                          handleTenantSelect(tenant);
                        }}
                      >
                        {tenant}
                      </a>
                    ))}
                  </div>
                ) : (
                  <div className="p-3 text-muted">
                    <small>No diagnostics stored yet</small>
                  </div>
                )}
              </div>
            </div>
            <div className="mt-3">
              <a
                href="/query-diagnostics"
                className="btn btn-primary btn-sm w-100"
              >
                <i className="bi bi-plus-circle"></i> New Query
              </a>
            </div>
          </div>

          <div className="col-12 col-md-9 col-lg-10 mt-4 mt-md-0">
            <div className="card">
              <div className="card-header d-flex justify-content-between align-items-center">
                <h6 className="mb-0">
                  {selectedTenant ? (
                    <>
                      Diagnostics for <code>{selectedTenant}</code>
                    </>
                  ) : (
                    'Select a tenant to view diagnostics'
                  )}
                </h6>
                {diagnostics.length > 0 && (
                  <span className="badge bg-secondary">
                    {diagnostics.length} entries
                  </span>
                )}
              </div>
              <div className="card-body">
                {selectedTenant ? (
                  loading ? (
                    <div className="text-center py-5">
                      <div className="spinner-border" role="status">
                        <span className="visually-hidden">Loading...</span>
                      </div>
                    </div>
                  ) : diagnostics.length > 0 ? (
                    <div className="table-responsive">
                      <table className="table table-hover diagnostics-table mb-0">
                        <thead>
                          <tr>
                            <th className="col-narrow">Created</th>
                            <th className="col-narrow">Method</th>
                            <th className="col-payload">Payload</th>
                            <th className="col-narrow">Time</th>
                            <th className="col-narrow">Size</th>
                            <th className="col-narrow"></th>
                          </tr>
                        </thead>
                        <tbody>
                          {diagnostics.map((diag) => (
                            <tr key={diag.id}>
                              <td className="col-narrow">
                                <small>{formatTime(diag.created_at)}</small>
                              </td>
                              <td className="col-narrow">
                                <span className="badge bg-info query-type-badge">
                                  {diag.method}
                                </span>
                              </td>
                              <td className="col-payload">
                                {diag.request ? (
                                  <code
                                    className="payload-json"
                                    title={JSON.stringify(diag.request)}
                                  >
                                    {JSON.stringify(diag.request)}
                                  </code>
                                ) : (
                                  <span className="text-muted">-</span>
                                )}
                              </td>
                              <td className="col-narrow">
                                {diag.response_time_ms ? (
                                  <span className="badge bg-success">
                                    {formatMs(diag.response_time_ms)}
                                  </span>
                                ) : (
                                  <span className="text-muted">-</span>
                                )}
                              </td>
                              <td className="col-narrow">
                                <span className="badge bg-secondary">
                                  {formatBytes(diag.response_size_bytes)}
                                </span>
                              </td>
                              <td className="col-narrow">
                                <a
                                  href={`/query-diagnostics?load=${diag.id}&tenant=${selectedTenant}`}
                                  className="btn btn-sm btn-outline-primary"
                                >
                                  <i className="bi bi-eye"></i> View
                                </a>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <div className="text-center text-muted py-5">
                      <i className="bi bi-inbox display-4"></i>
                      <p className="mt-3">
                        No diagnostics stored for this tenant
                      </p>
                    </div>
                  )
                ) : (
                  <div className="text-center text-muted py-5">
                    <i className="bi bi-arrow-left display-4"></i>
                    <p className="mt-3">
                      Select a tenant from the list to view their stored
                      diagnostics
                    </p>
                  </div>
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    </main>
  );
}
