let cachedProfileTypes = [];

function updateQueryParams() {
    const method = document.getElementById('method').value;

    // Hide all query params first
    document.querySelectorAll('.query-param').forEach(el => {
        el.style.display = 'none';
    });
    document.getElementById('diff-params').style.display = 'none';

    // Show relevant params based on method
    switch (method) {
        case 'SelectMergeStacktraces':
        case 'SelectMergeProfile':
            document.getElementById('param-profile-type-id').style.display = 'block';
            document.getElementById('param-max-nodes').style.display = 'block';
            document.getElementById('param-format').style.display = 'block';
            break;
        case 'SelectMergeSpanProfile':
            document.getElementById('param-profile-type-id').style.display = 'block';
            document.getElementById('param-max-nodes').style.display = 'block';
            document.getElementById('param-format').style.display = 'block';
            document.getElementById('param-span-selector').style.display = 'block';
            break;
        case 'SelectSeries':
            document.getElementById('param-profile-type-id').style.display = 'block';
            document.getElementById('param-step').style.display = 'block';
            document.getElementById('param-group-by').style.display = 'block';
            document.getElementById('param-aggregation').style.display = 'block';
            document.getElementById('param-limit').style.display = 'block';
            document.getElementById('param-exemplar-type').style.display = 'block';
            break;
        case 'SelectHeatmap':
            document.getElementById('param-profile-type-id').style.display = 'block';
            document.getElementById('param-step').style.display = 'block';
            document.getElementById('param-group-by').style.display = 'block';
            document.getElementById('param-limit').style.display = 'block';
            document.getElementById('param-heatmap-query-type').style.display = 'block';
            document.getElementById('param-exemplar-type').style.display = 'block';
            break;
        case 'Diff':
            document.getElementById('diff-params').style.display = 'block';
            break;
        case 'LabelValues':
            document.getElementById('param-label-name').style.display = 'block';
            break;
        case 'LabelNames':
        case 'ProfileTypes':
            // No additional parameters
            break;
        case 'Series':
            document.getElementById('param-label-names').style.display = 'block';
            break;
    }
}

function onTenantChange() {
    const tenantId = document.getElementById('tenant_id').value;
    if (!tenantId) {
        updateProfileTypeSelects([]);
        return;
    }
    const startTime = document.getElementById('start_time').value;
    const endTime = document.getElementById('end_time').value;
    fetchProfileTypes(tenantId, startTime, endTime);
}

async function fetchProfileTypes(tenantId, startTime, endTime) {
    const selects = document.querySelectorAll('#profile_type_id, .profile-type-select');
    selects.forEach(select => {
        select.innerHTML = '<option value="">Loading...</option>';
        select.disabled = true;
    });

    try {
        // Parse times to milliseconds for the Connect API
        let startMs = 0, endMs = 0;
        if (startTime) {
            const d = new Date(startTime);
            if (!isNaN(d.getTime())) startMs = d.getTime();
        }
        if (endTime) {
            const d = new Date(endTime);
            if (!isNaN(d.getTime())) endMs = d.getTime();
        }

        // Call the existing Connect RPC endpoint
        const response = await fetch('/querier.v1.QuerierService/ProfileTypes', {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Scope-OrgID': tenantId
            },
            body: JSON.stringify({ start: startMs, end: endMs })
        });
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}`);
        }
        const data = await response.json();
        cachedProfileTypes = (data.profileTypes || []).map(pt => pt.ID);
        updateProfileTypeSelects(cachedProfileTypes);
    } catch (error) {
        console.error('Failed to fetch profile types:', error);
        selects.forEach(select => {
            select.innerHTML = '<option value="">Failed to load</option>';
            select.disabled = false;
        });
    }
}

function updateProfileTypeSelects(profileTypes) {
    const selects = document.querySelectorAll('#profile_type_id, .profile-type-select');
    selects.forEach(select => {
        // Use data-initial-value if set, otherwise use current value
        const initialValue = select.dataset.initialValue;
        const currentValue = initialValue || select.value;
        select.innerHTML = '<option value="">-- Select profile type --</option>';
        profileTypes.forEach(pt => {
            const option = document.createElement('option');
            option.value = pt;
            option.textContent = pt;
            if (pt === currentValue) {
                option.selected = true;
            }
            select.appendChild(option);
        });
        select.disabled = false;
        // Clear initial value after first use
        if (initialValue) {
            delete select.dataset.initialValue;
        }
    });
}

function parseTime(timeStr) {
    if (!timeStr) return 0;
    const now = Date.now();
    const match = timeStr.match(/^now(-(\d+)([smhd]))?$/);
    if (match) {
        if (!match[1]) return now;
        const value = parseInt(match[2], 10);
        const unit = match[3];
        const multipliers = { s: 1000, m: 60000, h: 3600000, d: 86400000 };
        return now - value * (multipliers[unit] || 0);
    }
    const d = new Date(timeStr);
    return isNaN(d.getTime()) ? 0 : d.getTime();
}

function calculateDefaultStep(startMs, endMs) {
    const minStep = 15; // minimum step in seconds, same as Grafana default
    const targetDataPoints = 100; // aim for ~100 data points
    const rangeSeconds = (endMs - startMs) / 1000;
    const calculatedStep = rangeSeconds / targetDataPoints;
    return Math.max(minStep, Math.ceil(calculatedStep));
}

function buildRequestBody(method) {
    const startMs = parseTime(document.getElementById('start_time').value);
    const endMs = parseTime(document.getElementById('end_time').value);
    const labelSelector = document.getElementById('label_selector').value;
    const profileTypeId = document.getElementById('profile_type_id').value;

    switch (method) {
        case 'SelectMergeStacktraces':
        case 'SelectMergeProfile': {
            const body = { start: startMs, end: endMs, labelSelector, profileTypeID: profileTypeId };
            const maxNodes = document.getElementById('max_nodes').value;
            if (maxNodes) body.maxNodes = parseInt(maxNodes, 10);
            const format = document.getElementById('format').value;
            if (format) body.format = format;
            return body;
        }
        case 'SelectMergeSpanProfile': {
            const body = { start: startMs, end: endMs, labelSelector, profileTypeID: profileTypeId };
            const maxNodes = document.getElementById('max_nodes').value;
            if (maxNodes) body.maxNodes = parseInt(maxNodes, 10);
            const format = document.getElementById('format').value;
            if (format) body.format = format;
            const spanSelector = document.getElementById('span_selector').value;
            if (spanSelector) body.spanSelector = spanSelector.split(',').map(s => s.trim()).filter(Boolean);
            return body;
        }
        case 'SelectSeries': {
            const body = { start: startMs, end: endMs, labelSelector, profileTypeID: profileTypeId };
            const stepInput = document.getElementById('step').value;
            body.step = stepInput ? parseInt(stepInput, 10) : calculateDefaultStep(startMs, endMs);
            const groupBy = document.getElementById('group_by').value;
            if (groupBy) body.groupBy = groupBy.split(',').map(s => s.trim()).filter(Boolean);
            const aggregation = document.getElementById('aggregation').value;
            if (aggregation === 'sum') body.aggregation = 'TIME_SERIES_AGGREGATION_TYPE_SUM';
            else if (aggregation === 'avg') body.aggregation = 'TIME_SERIES_AGGREGATION_TYPE_AVERAGE';
            const limit = document.getElementById('limit').value;
            if (limit) body.limit = parseInt(limit, 10);
            const exemplarType = document.getElementById('exemplar_type').value;
            if (exemplarType) body.exemplarType = exemplarType;
            return body;
        }
        case 'SelectHeatmap': {
            const body = {
                start: startMs, end: endMs, labelSelector, profileTypeID: profileTypeId,
                queryType: document.getElementById('heatmap_query_type').value,
            };
            const stepInput = document.getElementById('step').value;
            body.step = stepInput ? parseInt(stepInput, 10) : calculateDefaultStep(startMs, endMs);
            const groupBy = document.getElementById('group_by').value;
            if (groupBy) body.groupBy = groupBy.split(',').map(s => s.trim()).filter(Boolean);
            const limit = document.getElementById('limit').value;
            if (limit) body.limit = parseInt(limit, 10);
            const exemplarType = document.getElementById('exemplar_type').value;
            if (exemplarType) body.exemplarType = exemplarType;
            return body;
        }
        case 'Diff': {
            const leftStartMs = parseTime(document.getElementById('diff_left_start').value);
            const leftEndMs = parseTime(document.getElementById('diff_left_end').value);
            const rightStartMs = parseTime(document.getElementById('diff_right_start').value);
            const rightEndMs = parseTime(document.getElementById('diff_right_end').value);
            return {
                left: {
                    start: leftStartMs, end: leftEndMs,
                    labelSelector: document.getElementById('diff_left_selector').value,
                    profileTypeID: document.getElementById('diff_left_profile_type').value,
                },
                right: {
                    start: rightStartMs, end: rightEndMs,
                    labelSelector: document.getElementById('diff_right_selector').value,
                    profileTypeID: document.getElementById('diff_right_profile_type').value,
                },
            };
        }
        case 'LabelNames':
            return { start: startMs, end: endMs, matchers: labelSelector ? [labelSelector] : [] };
        case 'LabelValues':
            return { start: startMs, end: endMs, name: document.getElementById('label_name').value, matchers: labelSelector ? [labelSelector] : [] };
        case 'Series': {
            const body = { start: startMs, end: endMs, matchers: labelSelector ? [labelSelector] : [] };
            const labelNames = document.getElementById('label_names').value;
            if (labelNames) body.labelNames = labelNames.split(',').map(s => s.trim()).filter(Boolean);
            return body;
        }
        case 'ProfileTypes':
            return { start: startMs, end: endMs };
        default:
            return {};
    }
}

async function executeQuery(event) {
    event.preventDefault();

    const btn = document.getElementById('execute-btn');
    const errorDiv = document.getElementById('query-error');
    errorDiv.style.display = 'none';

    const tenantId = document.getElementById('tenant_id').value;
    if (!tenantId) {
        errorDiv.textContent = 'Tenant ID is required';
        errorDiv.style.display = 'block';
        return false;
    }

    const method = document.getElementById('method').value;
    const endpoint = '/querier.v1.QuerierService/' + method;
    const body = buildRequestBody(method);

    const originalContent = btn.innerHTML;
    btn.innerHTML = '<span class="spinner-border spinner-border-sm" role="status"></span> Executing...';
    btn.disabled = true;

    try {
        const response = await fetch(endpoint, {
            method: 'POST',
            headers: {
                'Content-Type': 'application/json',
                'X-Scope-OrgID': tenantId,
                'X-Pyroscope-Collect-Diagnostics': 'true',
            },
            body: JSON.stringify(body),
        });

        const diagnosticsId = response.headers.get('X-Pyroscope-Diagnostics-Id');
        if (!response.ok) {
            const text = await response.text();
            throw new Error(text || `HTTP ${response.status}`);
        }

        if (diagnosticsId) {
            window.location.href = `/query-diagnostics?load=${diagnosticsId}&tenant=${tenantId}`;
        } else {
            errorDiv.textContent = 'Query succeeded but no diagnostics ID was returned';
            errorDiv.style.display = 'block';
            btn.innerHTML = originalContent;
            btn.disabled = false;
        }
    } catch (err) {
        errorDiv.textContent = 'Query failed: ' + err.message;
        errorDiv.style.display = 'block';
        btn.innerHTML = originalContent;
        btn.disabled = false;
    }
    return false;
}

// Initialize on page load
document.addEventListener('DOMContentLoaded', function() {
    updateQueryParams();
    // If tenant is already selected, fetch profile types
    const tenantId = document.getElementById('tenant_id').value;
    if (tenantId) {
        const startTime = document.getElementById('start_time').value;
        const endTime = document.getElementById('end_time').value;
        fetchProfileTypes(tenantId, startTime, endTime);
    }
});
