import { combineReducers, createSlice, PayloadAction } from '@reduxjs/toolkit';
import type { RootState } from '../store';
import { SET_COLLAPSE_UI } from '../actions';

interface UIState {
  sidebar: boolean;
}
interface ViewState {
  value: number;
}

const windowWidth =
  window.innerWidth ||
  document.documentElement.clientWidth ||
  document.body.clientWidth;

const initialUIState: UIState = {
  sidebar: windowWidth < 1200,
};

// Define the initial state using that type
const initialViewState: ViewState = {
  value: 0,
};

export const uiReducer = (state: UIState = initialUIState, action) => {
  switch (action.type) {
    case 'SET_COLLAPSE_UI':
      return { ...state, [action.payload.path]: action.payload.value };
    default:
      return state;
  }
  return state;
};

export const viewsSlice = createSlice({
  name: 'views',
  initialState: initialViewState,
  reducers: {},
});

export const selectCount = (state: RootState) => state.views.value;
export const selectUIState = (state: RootState) => (path) =>
  state.views.ui[path];

export default combineReducers({ value: viewsSlice.reducer, ui: uiReducer });
