import { createSlice } from '@reduxjs/toolkit';
import type { RootState } from '../store';

const initialState = {
  isSearchLinked: false,
  linkedSearchQuery: '',
  resetLinkedSearchSide: '',
};

export const searchSlice = createSlice({
  name: 'search',
  initialState,
  reducers: {
    setSearchQuery: (state, action) => {
      state.linkedSearchQuery = action.payload;
    },

    toggleLinkedSearch: (state, action) => {
      const { isSearchLinked } = state;

      if (isSearchLinked === false) {
        state.isSearchLinked = true;
        state.resetLinkedSearchSide = '';
      } else {
        switch (action.payload) {
          case 'left':
            state.resetLinkedSearchSide = 'right';
            break;

          case 'right':
            state.resetLinkedSearchSide = 'left';

            break;

          case 'both':
            state.isSearchLinked = false;
            state.resetLinkedSearchSide = '';
            break;

          default:
            break;
        }
      }
    },
  },
});
export const { setSearchQuery, toggleLinkedSearch } = searchSlice.actions;

export const isSearchLinked = (state: RootState) => state.search.isSearchLinked;
export const linkedSearchQuery = (state: RootState) =>
  state.search.linkedSearchQuery;
export const resetLinkedSearchSide = (state: RootState) =>
  state.search.resetLinkedSearchSide;

export default searchSlice.reducer;
