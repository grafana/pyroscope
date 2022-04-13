import {
  createAsyncThunk as libCreateAsyncThunk,
  AsyncThunkPayloadCreator,
  AsyncThunk,
  SerializedError,
} from '@reduxjs/toolkit';

// eslint-disable-next-line @typescript-eslint/no-empty-interface
interface ThunkAPIConfig {}

// https://github.com/reduxjs/redux-toolkit/issues/486
// eslint-disable-next-line import/prefer-default-export
export const createAsyncThunk = <Returned, ThunkArg = any>(
  type: string,
  thunk: AsyncThunkPayloadCreator<Returned, ThunkArg>
): AsyncThunk<Returned, ThunkArg, ThunkAPIConfig> => {
  return libCreateAsyncThunk<Returned, ThunkArg, ThunkAPIConfig>(type, thunk, {
    // Return the error as is (without)
    // So that the components can use features like instanceof, and accessing other fields that would otherwise be ignored
    // https://github.com/reduxjs/redux-toolkit/blob/db0d7dc20939b62f8c59631cc030575b78642296/packages/toolkit/src/createAsyncThunk.ts#L94
    serializeError: (x) => x as SerializedError,
  });
};
