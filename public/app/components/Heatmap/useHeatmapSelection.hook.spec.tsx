import React, { RefObject } from 'react';
import { renderHook } from '@testing-library/react';
import { Provider } from 'react-redux';
import { configureStore } from '@reduxjs/toolkit';
import { continuousReducer } from '@pyroscope/redux/reducers/continuous';
import tracingReducer from '@pyroscope/redux/reducers/tracing';

import { useHeatmapSelection } from './useHeatmapSelection.hook';
import { heatmapMockData } from '../../services/exemplarsTestData';
import { setStore } from '@pyroscope/services/storage';
const canvasEl = document.createElement('canvas');
const divEl = document.createElement('div');
const canvasRef = { current: canvasEl } as RefObject<HTMLCanvasElement>;
const resizedSelectedAreaRef = { current: divEl } as RefObject<HTMLDivElement>;

function createStore(preloadedState: any) {
  const store = configureStore({
    reducer: {
      continuous: continuousReducer,
      tracing: tracingReducer,
    },
    preloadedState,
  });

  setStore(store);
  return store;
}

describe('Hook: useHeatmapSelection', () => {
  const render = () =>
    renderHook(
      () =>
        useHeatmapSelection({
          canvasRef,
          resizedSelectedAreaRef,
          heatmapW: 1234,
          heatmap: heatmapMockData,
          onSelection: () => ({}),
        }),
      {
        wrapper: ({ children }) => (
          <Provider
            store={createStore({
              continuous: {},
              tracing: {
                exemplarsSingleView: {},
              },
            })}
          >
            {children}
          </Provider>
        ),
      }
    ).result;

  it('should return initial selection values', () => {
    const { current } = render();

    expect(current).toMatchObject({
      selectedCoordinates: { start: null, end: null },
      selectedAreaToHeatmapRatio: 1,
      resetSelection: expect.any(Function),
    });
  });
});
