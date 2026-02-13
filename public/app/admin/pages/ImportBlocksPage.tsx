import React, { useCallback, useEffect, useRef, useState } from 'react';

import { Header } from '../components/Header';
import { fetchTenants, importBlock } from '../services/api';
import { formatBytes } from '../utils';

interface TarEntry {
  name: string;
  size: number;
  data: Uint8Array;
  status: 'pending' | 'uploading' | 'done' | 'error';
  error?: string;
}

function parseTarEntries(buffer: ArrayBuffer): TarEntry[] {
  const view = new Uint8Array(buffer);
  const entries: TarEntry[] = [];
  let offset = 0;

  while (offset + 512 <= view.length) {
    const header = view.slice(offset, offset + 512);
    // Check for end-of-archive (two consecutive zero blocks).
    if (header.every((b) => b === 0)) {
      break;
    }

    const name = new TextDecoder()
      .decode(header.slice(0, 100))
      .replace(/\0.*$/, '');
    const sizeStr = new TextDecoder()
      .decode(header.slice(124, 136))
      .replace(/\0.*$/, '')
      .trim();
    const typeFlag = header[156];
    const size = parseInt(sizeStr, 8) || 0;

    offset += 512;

    // typeFlag 48 = '0' (regular file), 0 = legacy regular file
    if ((typeFlag === 48 || typeFlag === 0) && name.endsWith('.bin')) {
      const data = view.slice(offset, offset + size);
      entries.push({ name, size, data, status: 'pending' });
    }

    // Advance past file data (padded to 512-byte boundary).
    offset += Math.ceil(size / 512) * 512;
  }

  return entries;
}

async function decompressGzip(file: File): Promise<ArrayBuffer> {
  const ds = new DecompressionStream('gzip');
  const decompressedStream = file.stream().pipeThrough(ds);
  const reader = decompressedStream.getReader();
  const chunks: Uint8Array[] = [];
  let totalSize = 0;

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    chunks.push(value);
    totalSize += value.length;
  }

  const result = new Uint8Array(totalSize);
  let pos = 0;
  for (const chunk of chunks) {
    result.set(chunk, pos);
    pos += chunk.length;
  }
  return result.buffer;
}

export function ImportBlocksPage() {
  const [tenants, setTenants] = useState<string[]>([]);
  const [tenant, setTenant] = useState('');
  const [selectedFile, setSelectedFile] = useState<File | null>(null);
  const [entries, setEntries] = useState<TarEntry[]>([]);
  const [isParsing, setIsParsing] = useState(false);
  const [isImporting, setIsImporting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    async function loadTenants() {
      try {
        const tenantList = await fetchTenants();
        setTenants(tenantList);
      } catch (err) {
        setError(
          err instanceof Error ? err.message : 'Failed to load tenants'
        );
      }
    }
    loadTenants();
  }, []);

  const handleFileSelect = useCallback(() => {
    fileInputRef.current?.click();
  }, []);

  const handleFileChange = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const file = e.target.files?.[0];
      if (!file) return;

      setSelectedFile(file);
      setEntries([]);
      setError(null);
      setIsParsing(true);

      try {
        const tarBuffer = await decompressGzip(file);
        const parsed = parseTarEntries(tarBuffer);
        if (parsed.length === 0) {
          setError('No .bin block files found in the archive.');
        }
        setEntries(parsed);
      } catch (err) {
        setError(
          err instanceof Error
            ? err.message
            : 'Failed to decompress/parse archive'
        );
      } finally {
        setIsParsing(false);
      }
    },
    []
  );

  const handleImport = useCallback(async () => {
    if (!tenant || entries.length === 0) return;

    setIsImporting(true);
    setError(null);

    for (let i = 0; i < entries.length; i++) {
      setEntries((prev) =>
        prev.map((e, idx) => (idx === i ? { ...e, status: 'uploading' } : e))
      );

      try {
        await importBlock(tenant, entries[i].data);
        setEntries((prev) =>
          prev.map((e, idx) => (idx === i ? { ...e, status: 'done' } : e))
        );
      } catch (err) {
        const msg =
          err instanceof Error ? err.message : 'Failed to import block';
        setEntries((prev) =>
          prev.map((e, idx) =>
            idx === i ? { ...e, status: 'error', error: msg } : e
          )
        );
      }
    }

    setIsImporting(false);
  }, [tenant, entries]);

  const doneCount = entries.filter((e) => e.status === 'done').length;
  const errorCount = entries.filter((e) => e.status === 'error').length;
  const totalSize = entries.reduce((sum, e) => sum + e.size, 0);

  return (
    <div className="diagnostics-list-page">
      <div className="page-header">
        <Header
          title="Import Blocks"
          subtitle="Import exported L3 blocks into a target tenant"
          showNewQueryLink={true}
          showStoredDiagnosticsLink={true}
          showImportBlocksLink={false}
        />
      </div>

      <div style={{ flex: 1, overflow: 'auto', padding: '1rem' }}>
        {error && (
          <div className="alert alert-danger mb-3" role="alert">
            <strong>Error:</strong> {error}
          </div>
        )}
        <div className="row">
          <div className="col-md-5">
            <div className="card">
              <div className="card-header">
                <h6 className="mb-0">Import Settings</h6>
              </div>
              <div className="card-body">
                <div className="mb-3">
                  <label htmlFor="tenant-input" className="form-label">
                    Target Tenant
                  </label>
                  <input
                    id="tenant-input"
                    type="text"
                    className="form-control"
                    placeholder="Enter or select a tenant..."
                    value={tenant}
                    onChange={(e) => setTenant(e.target.value)}
                    list="tenant-list"
                  />
                  <datalist id="tenant-list">
                    {tenants.map((t) => (
                      <option key={t} value={t} />
                    ))}
                  </datalist>
                </div>

                <div className="mb-3">
                  <label className="form-label">Blocks Archive (tar.gz)</label>
                  <div className="d-flex align-items-center gap-2">
                    <input
                      type="file"
                      ref={fileInputRef}
                      style={{ display: 'none' }}
                      accept=".tar.gz,.gz"
                      onChange={handleFileChange}
                    />
                    <button
                      type="button"
                      className="btn btn-outline-secondary"
                      onClick={handleFileSelect}
                      disabled={isParsing}
                    >
                      {isParsing ? (
                        <>
                          <span
                            className="spinner-border spinner-border-sm me-2"
                            role="status"
                            aria-hidden="true"
                          ></span>
                          Extracting...
                        </>
                      ) : (
                        'Choose File'
                      )}
                    </button>
                    <span className="text-muted">
                      {selectedFile ? selectedFile.name : 'No file selected'}
                    </span>
                  </div>
                </div>

                <button
                  type="button"
                  className="btn btn-primary"
                  disabled={
                    !tenant || entries.length === 0 || isImporting || isParsing
                  }
                  onClick={handleImport}
                >
                  {isImporting ? (
                    <>
                      <span
                        className="spinner-border spinner-border-sm me-2"
                        role="status"
                        aria-hidden="true"
                      ></span>
                      Importing {doneCount}/{entries.length}...
                    </>
                  ) : (
                    'Import Blocks'
                  )}
                </button>

                {doneCount > 0 && !isImporting && (
                  <div className="alert alert-success mt-3 mb-0" role="alert">
                    Imported <strong>{doneCount}</strong> block
                    {doneCount !== 1 ? 's' : ''}
                    {errorCount > 0 && (
                      <>
                        , <strong>{errorCount}</strong> failed
                      </>
                    )}
                    .
                  </div>
                )}
              </div>
            </div>
          </div>

          <div className="col-md-7 mt-3 mt-md-0">
            {entries.length > 0 && (
              <div className="card">
                <div className="card-header d-flex justify-content-between align-items-center">
                  <h6 className="mb-0">
                    Archive Contents ({entries.length} block
                    {entries.length !== 1 ? 's' : ''},{' '}
                    {formatBytes(totalSize)})
                  </h6>
                  {isImporting && (
                    <span className="badge bg-primary">
                      {doneCount}/{entries.length}
                    </span>
                  )}
                </div>
                <div className="card-body p-0">
                  <table className="table table-sm table-hover mb-0">
                    <thead>
                      <tr>
                        <th style={{ width: '30px' }}></th>
                        <th>Block</th>
                        <th style={{ width: '100px' }}>Size</th>
                      </tr>
                    </thead>
                    <tbody>
                      {entries.map((entry, idx) => (
                        <tr key={idx}>
                          <td className="text-center">
                            <StatusIcon status={entry.status} />
                          </td>
                          <td>
                            <code className="small">{entry.name}</code>
                            {entry.error && (
                              <div className="text-danger small mt-1">
                                {entry.error}
                              </div>
                            )}
                          </td>
                          <td className="text-muted small">
                            {formatBytes(entry.size)}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

function StatusIcon({ status }: { status: TarEntry['status'] }) {
  switch (status) {
    case 'pending':
      return <i className="bi bi-circle text-muted"></i>;
    case 'uploading':
      return (
        <span
          className="spinner-border spinner-border-sm text-primary"
          role="status"
        ></span>
      );
    case 'done':
      return <i className="bi bi-check-circle-fill text-success"></i>;
    case 'error':
      return <i className="bi bi-x-circle-fill text-danger"></i>;
  }
}
