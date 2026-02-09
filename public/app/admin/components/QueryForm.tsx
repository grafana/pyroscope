import React, { useCallback, useEffect, useState } from 'react';
import type { QueryMethod, QueryParams } from '../types';
import { QUERY_METHODS } from '../types';
import { fetchProfileTypes } from '../services/api';

interface QueryFormProps {
  tenants: string[];
  params: QueryParams;
  onParamsChange: (params: QueryParams) => void;
  onSubmit: () => void;
  isSubmitting: boolean;
  error: string | null;
}

function parseTimeForApi(timeStr: string): number {
  if (!timeStr) {
    return 0;
  }
  const now = Date.now();
  const match = timeStr.match(/^now(-(\d+)([smhd]))?$/);
  if (match) {
    if (!match[1]) {
      return now;
    }
    const value = parseInt(match[2], 10);
    const unit = match[3];
    const multipliers: Record<string, number> = {
      s: 1000,
      m: 60000,
      h: 3600000,
      d: 86400000,
    };
    return now - value * (multipliers[unit] || 0);
  }
  const d = new Date(timeStr);
  return isNaN(d.getTime()) ? 0 : d.getTime();
}

export function QueryForm({
  tenants,
  params,
  onParamsChange,
  onSubmit,
  isSubmitting,
  error,
}: QueryFormProps) {
  const [profileTypes, setProfileTypes] = useState<string[]>([]);
  const [profileTypesLoading, setProfileTypesLoading] = useState(false);
  const [profileTypesError, setProfileTypesError] = useState<string | null>(
    null
  );

  const updateParam = useCallback(
    <K extends keyof QueryParams>(key: K, value: QueryParams[K]) => {
      onParamsChange({ ...params, [key]: value });
    },
    [params, onParamsChange]
  );

  const loadProfileTypes = useCallback(async () => {
    if (!params.tenantId) {
      setProfileTypes([]);
      return;
    }
    setProfileTypesLoading(true);
    setProfileTypesError(null);
    try {
      const startMs = parseTimeForApi(params.startTime);
      const endMs = parseTimeForApi(params.endTime);
      const types = await fetchProfileTypes(params.tenantId, startMs, endMs);
      setProfileTypes(types);
    } catch (err) {
      setProfileTypesError(
        err instanceof Error ? err.message : 'Failed to load profile types'
      );
      setProfileTypes([]);
    } finally {
      setProfileTypesLoading(false);
    }
  }, [params.tenantId, params.startTime, params.endTime]);

  useEffect(() => {
    loadProfileTypes();
  }, [loadProfileTypes]);

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onSubmit();
  };

  const showProfileTypeId =
    params.method === 'SelectMergeStacktraces' ||
    params.method === 'SelectMergeProfile' ||
    params.method === 'SelectMergeSpanProfile' ||
    params.method === 'SelectSeries' ||
    params.method === 'SelectHeatmap';

  const showMaxNodes =
    params.method === 'SelectMergeStacktraces' ||
    params.method === 'SelectMergeProfile' ||
    params.method === 'SelectMergeSpanProfile';

  const showFormat =
    params.method === 'SelectMergeStacktraces' ||
    params.method === 'SelectMergeProfile' ||
    params.method === 'SelectMergeSpanProfile';

  const showSpanSelector = params.method === 'SelectMergeSpanProfile';

  const showStep =
    params.method === 'SelectSeries' || params.method === 'SelectHeatmap';

  const showGroupBy =
    params.method === 'SelectSeries' || params.method === 'SelectHeatmap';

  const showAggregation = params.method === 'SelectSeries';

  const showLimit =
    params.method === 'SelectSeries' || params.method === 'SelectHeatmap';

  const showHeatmapQueryType = params.method === 'SelectHeatmap';

  const showExemplarType =
    params.method === 'SelectSeries' || params.method === 'SelectHeatmap';

  const showLabelName = params.method === 'LabelValues';

  const showLabelNames = params.method === 'Series';

  const showDiff = params.method === 'Diff';

  return (
    <div className="card">
      <div className="card-header">
        <h5 className="mb-0">Query Parameters</h5>
      </div>
      <div className="card-body">
        <form onSubmit={handleSubmit}>
          <div className="form-section">
            <label htmlFor="tenant_id" className="form-label">
              Tenant ID
            </label>
            <select
              className="form-select"
              id="tenant_id"
              value={params.tenantId}
              onChange={(e) => updateParam('tenantId', e.target.value)}
              required
            >
              <option value="">-- Select tenant --</option>
              {tenants.map((tenant) => (
                <option key={tenant} value={tenant}>
                  {tenant}
                </option>
              ))}
            </select>
          </div>

          <div className="form-section">
            <label htmlFor="method" className="form-label">
              API Method
            </label>
            <select
              className="form-select"
              id="method"
              value={params.method}
              onChange={(e) =>
                updateParam('method', e.target.value as QueryMethod)
              }
            >
              {QUERY_METHODS.map((method) => (
                <option key={method} value={method}>
                  {method}
                </option>
              ))}
            </select>
          </div>

          <div className="row form-section">
            <div className="col-6">
              <label htmlFor="start_time" className="form-label">
                Start Time
              </label>
              <input
                type="text"
                className="form-control"
                id="start_time"
                value={params.startTime}
                onChange={(e) => updateParam('startTime', e.target.value)}
                placeholder="now-1h"
                required
              />
            </div>
            <div className="col-6">
              <label htmlFor="end_time" className="form-label">
                End Time
              </label>
              <input
                type="text"
                className="form-control"
                id="end_time"
                value={params.endTime}
                onChange={(e) => updateParam('endTime', e.target.value)}
                placeholder="now"
                required
              />
            </div>
          </div>

          <div className="form-section">
            <label htmlFor="label_selector" className="form-label">
              Label Selector
            </label>
            <input
              type="text"
              className="form-control"
              id="label_selector"
              value={params.labelSelector}
              onChange={(e) => updateParam('labelSelector', e.target.value)}
              placeholder='{service_name="my-service"}'
            />
          </div>

          {showProfileTypeId && (
            <div className="form-section">
              <label htmlFor="profile_type_id" className="form-label">
                Profile Type ID
              </label>
              <select
                className="form-select"
                id="profile_type_id"
                value={params.profileTypeId}
                onChange={(e) => updateParam('profileTypeId', e.target.value)}
                disabled={profileTypesLoading}
              >
                <option value="">
                  {profileTypesLoading
                    ? 'Loading...'
                    : profileTypesError
                    ? 'Failed to load'
                    : params.tenantId
                    ? '-- Select profile type --'
                    : '-- Select tenant first --'}
                </option>
                {profileTypes.map((pt) => (
                  <option key={pt} value={pt}>
                    {pt}
                  </option>
                ))}
              </select>
              <div className="form-text">
                Select a tenant to load available profile types
              </div>
            </div>
          )}

          {showMaxNodes && (
            <div className="form-section">
              <label htmlFor="max_nodes" className="form-label">
                Max Nodes
              </label>
              <input
                type="number"
                className="form-control"
                id="max_nodes"
                value={params.maxNodes}
                onChange={(e) => updateParam('maxNodes', e.target.value)}
                placeholder="16384"
              />
              <div className="form-text">
                Maximum number of nodes in the result
              </div>
            </div>
          )}

          {showFormat && (
            <div className="form-section">
              <label htmlFor="format" className="form-label">
                Profile Format
              </label>
              <select
                className="form-select"
                id="format"
                value={params.format}
                onChange={(e) => updateParam('format', e.target.value)}
              >
                <option value="PROFILE_FORMAT_FLAMEGRAPH">Flame Graph</option>
                <option value="PROFILE_FORMAT_TREE">Tree</option>
              </select>
              <div className="form-text">Format of the profile response</div>
            </div>
          )}

          {showSpanSelector && (
            <div className="form-section">
              <label htmlFor="span_selector" className="form-label">
                Span IDs (comma-separated)
              </label>
              <input
                type="text"
                className="form-control"
                id="span_selector"
                value={params.spanSelector}
                onChange={(e) => updateParam('spanSelector', e.target.value)}
                placeholder="9a517183f26a089d, 5a4fe264a9c987fe"
              />
              <div className="form-text">List of Span IDs to query</div>
            </div>
          )}

          {showLabelName && (
            <div className="form-section">
              <label htmlFor="label_name" className="form-label">
                Label Name
              </label>
              <input
                type="text"
                className="form-control"
                id="label_name"
                value={params.labelName}
                onChange={(e) => updateParam('labelName', e.target.value)}
                placeholder="service_name"
              />
              <div className="form-text">Label name to get values for</div>
            </div>
          )}

          {showLabelNames && (
            <div className="form-section">
              <label htmlFor="label_names" className="form-label">
                Label Names (comma-separated)
              </label>
              <input
                type="text"
                className="form-control"
                id="label_names"
                value={params.labelNames}
                onChange={(e) => updateParam('labelNames', e.target.value)}
                placeholder="service_name, namespace"
              />
              <div className="form-text">
                Filter which label names to return (empty = all)
              </div>
            </div>
          )}

          {showStep && (
            <div className="form-section">
              <label htmlFor="step" className="form-label">
                Step (seconds)
              </label>
              <input
                type="number"
                className="form-control"
                id="step"
                value={params.step}
                onChange={(e) => updateParam('step', e.target.value)}
                placeholder="auto"
                min="1"
              />
              <div className="form-text">
                Time step between data points (empty = auto, min 15s, ~100
                points)
              </div>
            </div>
          )}

          {showGroupBy && (
            <div className="form-section">
              <label htmlFor="group_by" className="form-label">
                Group By (comma-separated)
              </label>
              <input
                type="text"
                className="form-control"
                id="group_by"
                value={params.groupBy}
                onChange={(e) => updateParam('groupBy', e.target.value)}
                placeholder="service_name"
              />
              <div className="form-text">Labels to group time series by</div>
            </div>
          )}

          {showAggregation && (
            <div className="form-section">
              <label htmlFor="aggregation" className="form-label">
                Aggregation
              </label>
              <select
                className="form-select"
                id="aggregation"
                value={params.aggregation}
                onChange={(e) => updateParam('aggregation', e.target.value)}
              >
                <option value="">None</option>
                <option value="sum">Sum</option>
                <option value="avg">Average</option>
              </select>
              <div className="form-text">Aggregation type for time series</div>
            </div>
          )}

          {showLimit && (
            <div className="form-section">
              <label htmlFor="limit" className="form-label">
                Limit
              </label>
              <input
                type="number"
                className="form-control"
                id="limit"
                value={params.limit}
                onChange={(e) => updateParam('limit', e.target.value)}
                placeholder="0"
                min="0"
              />
              <div className="form-text">
                Maximum number of series (0 = unlimited)
              </div>
            </div>
          )}

          {showHeatmapQueryType && (
            <div className="form-section">
              <label htmlFor="heatmap_query_type" className="form-label">
                Heatmap Query Type
              </label>
              <select
                className="form-select"
                id="heatmap_query_type"
                value={params.heatmapQueryType}
                onChange={(e) =>
                  updateParam('heatmapQueryType', e.target.value)
                }
              >
                <option value="HEATMAP_QUERY_TYPE_INDIVIDUAL">
                  Individual Profiles
                </option>
                <option value="HEATMAP_QUERY_TYPE_SPAN">Span Profiles</option>
              </select>
              <div className="form-text">Type of profiles to query</div>
            </div>
          )}

          {showExemplarType && (
            <div className="form-section">
              <label htmlFor="exemplar_type" className="form-label">
                Exemplar Type
              </label>
              <select
                className="form-select"
                id="exemplar_type"
                value={params.exemplarType}
                onChange={(e) => updateParam('exemplarType', e.target.value)}
              >
                <option value="">None</option>
                <option value="EXEMPLAR_TYPE_INDIVIDUAL">Individual</option>
                <option value="EXEMPLAR_TYPE_SPAN">Span</option>
              </select>
              <div className="form-text">
                Type of exemplars to include in the response
              </div>
            </div>
          )}

          {showDiff && (
            <div className="diff-params">
              <h6 className="mb-3">Diff Parameters</h6>
              <div className="row mb-2">
                <div className="col-6">
                  <strong>Left (baseline)</strong>
                </div>
                <div className="col-6">
                  <strong>Right (comparison)</strong>
                </div>
              </div>
              <div className="row form-section">
                <div className="col-6">
                  <label htmlFor="diff_left_selector" className="form-label">
                    Label Selector
                  </label>
                  <input
                    type="text"
                    className="form-control form-control-sm"
                    id="diff_left_selector"
                    value={params.diffLeftSelector}
                    onChange={(e) =>
                      updateParam('diffLeftSelector', e.target.value)
                    }
                    placeholder='{service_name="my-service"}'
                  />
                </div>
                <div className="col-6">
                  <label htmlFor="diff_right_selector" className="form-label">
                    Label Selector
                  </label>
                  <input
                    type="text"
                    className="form-control form-control-sm"
                    id="diff_right_selector"
                    value={params.diffRightSelector}
                    onChange={(e) =>
                      updateParam('diffRightSelector', e.target.value)
                    }
                    placeholder='{service_name="my-service"}'
                  />
                </div>
              </div>
              <div className="row form-section">
                <div className="col-6">
                  <label
                    htmlFor="diff_left_profile_type"
                    className="form-label"
                  >
                    Profile Type
                  </label>
                  <select
                    className="form-select form-select-sm"
                    id="diff_left_profile_type"
                    value={params.diffLeftProfileType}
                    onChange={(e) =>
                      updateParam('diffLeftProfileType', e.target.value)
                    }
                    disabled={profileTypesLoading}
                  >
                    <option value="">
                      {profileTypesLoading
                        ? 'Loading...'
                        : params.tenantId
                        ? '-- Select profile type --'
                        : '-- Select tenant first --'}
                    </option>
                    {profileTypes.map((pt) => (
                      <option key={pt} value={pt}>
                        {pt}
                      </option>
                    ))}
                  </select>
                </div>
                <div className="col-6">
                  <label
                    htmlFor="diff_right_profile_type"
                    className="form-label"
                  >
                    Profile Type
                  </label>
                  <select
                    className="form-select form-select-sm"
                    id="diff_right_profile_type"
                    value={params.diffRightProfileType}
                    onChange={(e) =>
                      updateParam('diffRightProfileType', e.target.value)
                    }
                    disabled={profileTypesLoading}
                  >
                    <option value="">
                      {profileTypesLoading
                        ? 'Loading...'
                        : params.tenantId
                        ? '-- Select profile type --'
                        : '-- Select tenant first --'}
                    </option>
                    {profileTypes.map((pt) => (
                      <option key={pt} value={pt}>
                        {pt}
                      </option>
                    ))}
                  </select>
                </div>
              </div>
              <div className="row form-section">
                <div className="col-3">
                  <label htmlFor="diff_left_start" className="form-label">
                    Start
                  </label>
                  <input
                    type="text"
                    className="form-control form-control-sm"
                    id="diff_left_start"
                    value={params.diffLeftStart}
                    onChange={(e) =>
                      updateParam('diffLeftStart', e.target.value)
                    }
                    placeholder="now-2h"
                  />
                </div>
                <div className="col-3">
                  <label htmlFor="diff_left_end" className="form-label">
                    End
                  </label>
                  <input
                    type="text"
                    className="form-control form-control-sm"
                    id="diff_left_end"
                    value={params.diffLeftEnd}
                    onChange={(e) => updateParam('diffLeftEnd', e.target.value)}
                    placeholder="now-1h"
                  />
                </div>
                <div className="col-3">
                  <label htmlFor="diff_right_start" className="form-label">
                    Start
                  </label>
                  <input
                    type="text"
                    className="form-control form-control-sm"
                    id="diff_right_start"
                    value={params.diffRightStart}
                    onChange={(e) =>
                      updateParam('diffRightStart', e.target.value)
                    }
                    placeholder="now-1h"
                  />
                </div>
                <div className="col-3">
                  <label htmlFor="diff_right_end" className="form-label">
                    End
                  </label>
                  <input
                    type="text"
                    className="form-control form-control-sm"
                    id="diff_right_end"
                    value={params.diffRightEnd}
                    onChange={(e) =>
                      updateParam('diffRightEnd', e.target.value)
                    }
                    placeholder="now"
                  />
                </div>
              </div>
            </div>
          )}

          <div className="d-grid gap-2 mt-3">
            <button
              type="submit"
              className="btn btn-success"
              disabled={isSubmitting}
            >
              {isSubmitting ? (
                <>
                  <span
                    className="spinner-border spinner-border-sm"
                    role="status"
                  ></span>{' '}
                  Executing...
                </>
              ) : (
                <>
                  <i className="bi bi-play-fill"></i> Execute Query
                </>
              )}
            </button>
          </div>
          {error && (
            <div className="alert alert-danger mt-3" role="alert">
              {error}
            </div>
          )}
        </form>
      </div>
    </div>
  );
}
