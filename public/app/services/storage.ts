import type { StoreType } from '@pyroscope/redux/store';

let _store: StoreType | null = null;

export function setStore(store: StoreType) {
  _store = store;
}

export function getStore() {
  return _store;
}

export function tenantIDFromStorage(): string {
  return getStore()?.getState().tenant.tenantID || '';
}
