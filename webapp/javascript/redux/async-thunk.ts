/* eslint-disable import/prefer-default-export */
import {
  createAsyncThunk as libCreateAsyncThunk,
  SerializedError,
} from '@reduxjs/toolkit';

// eslint-disable-next-line @typescript-eslint/ban-ts-comment
// @ts-ignore
export const createAsyncThunk: typeof libCreateAsyncThunk = (
  ...args: Parameters<typeof libCreateAsyncThunk>
) => {
  const [typePrefix, payloadCreator, options] = args;
  return libCreateAsyncThunk(typePrefix, payloadCreator, {
    ...options,
    // Return the error as is (without)
    // So that the components can use features like instanceof, and accessing other fields that would otherwise be ignored
    // https://github.com/reduxjs/redux-toolkit/blob/db0d7dc20939b62f8c59631cc030575b78642296/packages/toolkit/src/createAsyncThunk.ts#L94
    serializeError: (x) => x as SerializedError,
  });
};
