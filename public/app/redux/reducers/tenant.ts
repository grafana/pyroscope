import { createSlice, type PayloadAction } from '@reduxjs/toolkit';
import { createAsyncThunk } from '@webapp/redux/async-thunk';
import type { RootState } from '@webapp/redux/store';
import {
  isMultiTenancyEnabled,
  tenantIDFromStorage,
} from '@phlare/services/tenant';
import storage from 'redux-persist/lib/storage';
import { PersistConfig } from 'redux-persist/lib/types';

export const persistConfig: PersistConfig<TenantState> = {
  key: 'pyroscope:tenant',
  version: 0,
  storage,
  whitelist: ['tenantID'],
};

interface TenantState {
  tenancy:
    | 'unknown'
    | 'loading'
    | 'needs_tenant_id'
    | 'wants_to_change'
    | 'single_tenant'
    | 'multi_tenant';
  tenantID?: string;
}

const initialState: TenantState = {
  tenancy: 'unknown',
  tenantID: undefined,
};

export const checkTenancyIsRequired = createAsyncThunk<
  { tenancy: TenantState['tenancy']; tenantID?: string },
  void,
  { state: { tenant: TenantState } }
>(
  'checkTenancyIsRequired',
  async () => {
    const tenantID = tenantIDFromStorage();

    // Try to hit the server and see the response
    const multitenancy = await isMultiTenancyEnabled();

    if (multitenancy && !tenantID) {
      return Promise.resolve({ tenancy: 'needs_tenant_id', tenantID });
    }

    if (multitenancy && tenantID) {
      return Promise.resolve({ tenancy: 'multi_tenant', tenantID });
    }

    return Promise.resolve({ tenancy: 'single_tenant', tenantID });
  },
  {
    // This check is only valid if we don't know what's the tenancy status yet
    condition: (query, thunkAPI) => {
      const state = thunkAPI.getState().tenant;

      return state.tenancy === 'unknown';
    },
  }
);

const tenantSlice = createSlice({
  name: 'tenant',
  initialState,
  reducers: {
    deleteTenancy(state) {
      state.tenancy = 'unknown';
      state.tenantID = undefined;
    },
    setTenantID(state, action: PayloadAction<string>) {
      state.tenancy = 'multi_tenant';
      state.tenantID = action.payload;
    },
    setWantsToChange(state) {
      state.tenancy = 'wants_to_change';
    },
    setTenancy(state, action: PayloadAction<TenantState['tenancy']>) {
      state.tenancy = action.payload;
    },
  },
  extraReducers: (builder) => {
    // This thunk will never reject
    builder.addCase(checkTenancyIsRequired.fulfilled, (state, action) => {
      state.tenancy = action.payload.tenancy;
      state.tenantID = action.payload.tenantID;
    });
    builder.addCase(checkTenancyIsRequired.pending, (state) => {
      state.tenancy = 'loading';
    });
  },
});

export const { actions } = tenantSlice;

export const selectTenancy = (state: RootState) => state.tenant.tenancy;

export const selectIsMultiTenant = (state: RootState) =>
  state.tenant.tenancy === 'multi_tenant' ||
  state.tenant.tenancy === 'wants_to_change';

export const selectTenantID = (state: RootState) => state.tenant.tenantID;

export default tenantSlice.reducer;
