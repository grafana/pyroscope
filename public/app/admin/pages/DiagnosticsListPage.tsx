import React, {
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';

import { Header } from '../components/Header';
import {
  fetchTenants,
  listDiagnostics,
  importDiagnostic,
} from '../services/api';
import { formatBytes, formatMs, formatTime, getBasePath } from '../utils';
import type { DiagnosticSummary } from '../types';

type SortField =
  | 'created_at'
  | 'method'
  | 'response_time_ms'
  | 'response_size_bytes';
type SortDirection = 'asc' | 'desc';

export function DiagnosticsListPage() {
  const [tenants, setTenants] = useState<string[]>([]);
  const [selectedTenant, setSelectedTenant] = useState<string | null>(null);
  const [diagnostics, setDiagnostics] = useState<DiagnosticSummary[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);

  const [sortField, setSortField] = useState<SortField>('created_at');
  const [sortDirection, setSortDirection] = useState<SortDirection>('desc');
  const [filterText, setFilterText] = useState('');
  const [methodFilter, setMethodFilter] = useState<string>('');
  const [tenantSearch, setTenantSearch] = useState('');

  useEffect(() => {
    async function loadTenants() {
      try {
        const tenantList = await fetchTenants();
        setTenants(tenantList);
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load tenants');
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
          err instanceof Error ? err.message : 'Failed to load diagnostics'
        );
        setDiagnostics([]);
      } finally {
        setLoading(false);
      }
    }
    loadDiagnostics();
  }, [selectedTenant]);

  const filteredTenants = useMemo(() => {
    if (!tenantSearch) {
      return tenants;
    }
    const lowerSearch = tenantSearch.toLowerCase();
    return tenants.filter((t) => t.toLowerCase().includes(lowerSearch));
  }, [tenants, tenantSearch]);

  const availableMethods = useMemo(() => {
    const methods = new Set(diagnostics.map((d) => d.method));
    return Array.from(methods).sort();
  }, [diagnostics]);

  const filteredAndSortedDiagnostics = useMemo(() => {
    let result = [...diagnostics];

    if (filterText) {
      const lowerFilter = filterText.toLowerCase();
      result = result.filter((diag) => {
        const payloadStr = diag.request
          ? JSON.stringify(diag.request).toLowerCase()
          : '';
        return (
          diag.method.toLowerCase().includes(lowerFilter) ||
          payloadStr.includes(lowerFilter) ||
          diag.id.toLowerCase().includes(lowerFilter)
        );
      });
    }

    if (methodFilter) {
      result = result.filter((diag) => diag.method === methodFilter);
    }

    result.sort((a, b) => {
      let comparison = 0;
      switch (sortField) {
        case 'created_at':
          comparison =
            new Date(a.created_at).getTime() - new Date(b.created_at).getTime();
          break;
        case 'method':
          comparison = a.method.localeCompare(b.method);
          break;
        case 'response_time_ms':
          comparison = (a.response_time_ms || 0) - (b.response_time_ms || 0);
          break;
        case 'response_size_bytes':
          comparison =
            (a.response_size_bytes || 0) - (b.response_size_bytes || 0);
          break;
      }
      return sortDirection === 'asc' ? comparison : -comparison;
    });

    return result;
  }, [diagnostics, filterText, methodFilter, sortField, sortDirection]);

  const handleSort = (field: SortField) => {
    if (sortField === field) {
      setSortDirection(sortDirection === 'asc' ? 'desc' : 'asc');
    } else {
      setSortField(field);
      setSortDirection('desc');
    }
  };

  const handleTenantSelect = (tenant: string) => {
    setSelectedTenant(tenant);
    setFilterText('');
    setMethodFilter('');
    window.history.pushState({}, '', `?tenant=${encodeURIComponent(tenant)}`);
  };

  const handleImportClick = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleFileChange = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file || !selectedTenant) {
        return;
      }

      setIsImporting(true);
      setError(null);

      try {
        const result = await importDiagnostic(selectedTenant, file);
        // Navigate to the imported diagnostic
        window.location.href = `${getBasePath()}/query-diagnostics?load=${
          result.id
        }&tenant=${encodeURIComponent(selectedTenant)}`;
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'Failed to import diagnostic'
        );
      } finally {
        setIsImporting(false);
        // Reset file input
        if (fileInputRef.current) {
          fileInputRef.current.value = '';
        }
      }
    },
    [selectedTenant]
  );

  const SortIcon = ({ field }: { field: SortField }) => {
    if (sortField !== field) {
      return <span className="sort-icon text-muted">⇅</span>;
    }
    return (
      <span className="sort-icon active">
        {sortDirection === 'asc' ? '↑' : '↓'}
      </span>
    );
  };

  return (
    <div className="diagnostics-list-page">
      <div className="page-header">
        <Header
          title="Stored Diagnostics"
          subtitle="Browse and view stored query diagnostics"
          showNewQueryLink={true}
          showStoredDiagnosticsLink={false}
        />
      </div>

      {error && (
        <div className="alert alert-danger mx-3 mt-3" role="alert">
          <strong>Error:</strong> {error}
        </div>
      )}

      <div className="page-content">
        <div className="tenants-panel">
          <div className="card">
            <div className="card-header">
              <div className="d-flex justify-content-between align-items-center mb-2">
                <h6 className="mb-0">Tenants</h6>
                {tenants.length > 0 && (
                  <span className="badge bg-secondary">
                    {filteredTenants.length !== tenants.length
                      ? `${filteredTenants.length}/`
                      : ''}
                    {tenants.length}
                  </span>
                )}
              </div>
              {tenants.length > 0 && (
                <input
                  type="text"
                  className="form-control form-control-sm"
                  placeholder="Search tenants..."
                  value={tenantSearch}
                  onChange={(e) => setTenantSearch(e.target.value)}
                />
              )}
            </div>
            <div className="card-body p-0">
              {tenants.length > 0 ? (
                filteredTenants.length > 0 ? (
                  <div className="tenant-list">
                    {filteredTenants.map((tenant) => (
                      <a
                        key={tenant}
                        href={`?tenant=${encodeURIComponent(tenant)}`}
                        className={`tenant-item ${
                          selectedTenant === tenant ? 'active' : ''
                        }`}
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
                  <div className="p-3 text-muted text-center">
                    <small>No matching tenants</small>
                  </div>
                )
              ) : (
                <div className="p-3 text-muted">
                  <small>No diagnostics stored yet</small>
                </div>
              )}
            </div>
          </div>
        </div>

        <div className="diagnostics-panel">
          <div className="card h-100">
            <div className="card-header">
              <div className="d-flex justify-content-between align-items-center">
                <h6 className="mb-0">
                  {selectedTenant ? (
                    <>
                      Diagnostics for <code>{selectedTenant}</code>
                    </>
                  ) : (
                    'Select a tenant to view diagnostics'
                  )}
                </h6>
                <div className="d-flex align-items-center gap-2">
                  {filteredAndSortedDiagnostics.length > 0 && (
                    <span className="badge bg-secondary">
                      {filteredAndSortedDiagnostics.length}
                      {filteredAndSortedDiagnostics.length !==
                        diagnostics.length && <> / {diagnostics.length}</>}{' '}
                      entries
                    </span>
                  )}
                  {selectedTenant && (
                    <>
                      <input
                        type="file"
                        ref={fileInputRef}
                        style={{ display: 'none' }}
                        accept=".zip"
                        onChange={handleFileChange}
                      />
                      <button
                        type="button"
                        className="btn btn-sm btn-outline-primary"
                        onClick={handleImportClick}
                        disabled={isImporting}
                        title="Import diagnostic from zip file"
                      >
                        {isImporting ? 'Importing...' : 'Import'}
                      </button>
                    </>
                  )}
                </div>
              </div>

              {selectedTenant && diagnostics.length > 0 && (
                <div className="filters-row mt-3">
                  <div className="filter-group">
                    <input
                      type="text"
                      className="form-control form-control-sm"
                      placeholder="Search in payload, method, or ID..."
                      value={filterText}
                      onChange={(e) => setFilterText(e.target.value)}
                    />
                  </div>
                  <div className="filter-group">
                    <select
                      className="form-select form-select-sm"
                      value={methodFilter}
                      onChange={(e) => setMethodFilter(e.target.value)}
                    >
                      <option value="">All Methods</option>
                      {availableMethods.map((method) => (
                        <option key={method} value={method}>
                          {method}
                        </option>
                      ))}
                    </select>
                  </div>
                  {(filterText || methodFilter) && (
                    <button
                      type="button"
                      className="btn btn-sm btn-outline-secondary"
                      onClick={() => {
                        setFilterText('');
                        setMethodFilter('');
                      }}
                    >
                      Clear
                    </button>
                  )}
                </div>
              )}
            </div>
            <div className="card-body p-0">
              {selectedTenant ? (
                loading ? (
                  <div className="text-center py-5">
                    <div className="spinner-border" role="status">
                      <span className="visually-hidden">Loading...</span>
                    </div>
                  </div>
                ) : diagnostics.length > 0 ? (
                  filteredAndSortedDiagnostics.length > 0 ? (
                    <div className="table-responsive">
                      <table className="table table-hover diagnostics-table mb-0">
                        <thead>
                          <tr>
                            <th
                              className="col-narrow sortable"
                              onClick={() => handleSort('created_at')}
                            >
                              Created <SortIcon field="created_at" />
                            </th>
                            <th
                              className="col-narrow sortable"
                              onClick={() => handleSort('method')}
                            >
                              Method <SortIcon field="method" />
                            </th>
                            <th className="col-payload">Payload</th>
                            <th
                              className="col-narrow sortable"
                              onClick={() => handleSort('response_time_ms')}
                            >
                              Time <SortIcon field="response_time_ms" />
                            </th>
                            <th
                              className="col-narrow sortable"
                              onClick={() => handleSort('response_size_bytes')}
                            >
                              Size <SortIcon field="response_size_bytes" />
                            </th>
                            <th className="col-narrow"></th>
                          </tr>
                        </thead>
                        <tbody>
                          {filteredAndSortedDiagnostics.map((diag) => (
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
                                  href={`${getBasePath()}/query-diagnostics?load=${
                                    diag.id
                                  }&tenant=${selectedTenant}`}
                                  className="btn btn-sm btn-outline-primary"
                                >
                                  View
                                </a>
                              </td>
                            </tr>
                          ))}
                        </tbody>
                      </table>
                    </div>
                  ) : (
                    <div className="text-center text-muted py-5">
                      <p className="mb-0">No results match your filters</p>
                      <button
                        type="button"
                        className="btn btn-sm btn-link mt-2"
                        onClick={() => {
                          setFilterText('');
                          setMethodFilter('');
                        }}
                      >
                        Clear filters
                      </button>
                    </div>
                  )
                ) : (
                  <div className="text-center text-muted py-5">
                    <p className="mt-3">
                      No diagnostics stored for this tenant
                    </p>
                  </div>
                )
              ) : (
                <div className="text-center text-muted py-5">
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
  );
}
