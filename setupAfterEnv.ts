import '@testing-library/jest-dom/extend-expect';
import '@testing-library/jest-dom';
import 'jest-canvas-mock';
import timezoneMock from 'timezone-mock';
import nodeFetch from 'node-fetch';
import 'regenerator-runtime/runtime';

timezoneMock.register('UTC');

globalThis.fetch = nodeFetch as unknown as typeof fetch;

// When testing redux we can assume this will be populated
// Which will be used for setting up the initialState
(globalThis.window as any).initialState = {
  appNames: [],
};
