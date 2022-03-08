import { Flamebearer, FlamebearerProfile } from '@pyroscope/models';
import { createSlice, createAsyncThunk } from '@reduxjs/toolkit';
import { renderSingle } from '../../services/render';

type SingleView =
  | { type: 'pristine' }
  | { type: 'loading' }
  | {
      type: 'loaded';
      raw: Flamebearer;
      // TODO: type this
      timeline: any;
      flamebearer: Flamebearer;
    }
  | {
      type: 'reloading';
      raw: Flamebearer;
      // TODO: type this
      timeline: any;
      flamebearer: Flamebearer;
    };

export const fetchSingleView = createAsyncThunk(
  'continuous/single',
  async (a, thunkAPI) => {
    const state = thunkAPI.getState();
    console.log('state', state);
    //    const res = await renderSingle();
    return Promise.reject(new Error('error'));
  }
);

interface ContinuousState {
  from: string;
  until: string;
  leftFrom: string;
  leftUntil: string;
  rightFrom: string;
  rightUntil: string;
  query: string;
  maxNodes: string;

  singleView: SingleView;
}

const initialState: ContinuousState = {
  from: 'now-1h',
  until: 'now',
  leftFrom: 'now-1h',
  leftUntil: 'now-30m',
  rightFrom: 'now-30m',
  rightUntil: 'now',
  query: '',
  maxNodes: '1024',

  singleView: { type: 'pristine' },
};

export const continuousSlice = createSlice({
  name: 'continuous',
  initialState,
  reducers: {},
  extraReducers: {},
});

export default continuousSlice.reducer;
