import { createSlice, PayloadAction } from '@reduxjs/toolkit';
import type { RootState } from '../store';

interface ViewState {
  value: number;
}

// Define the initial state using that type
const initialState: ViewState = {
  value: 0,
};

export const viewsSlice = createSlice({
  name: 'views',
  initialState,
  reducers: {},
});

export const selectCount = (state: RootState) => state.views.value;

export default viewsSlice.reducer;
