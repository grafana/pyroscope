import React from 'react';
import { render } from '@testing-library/react';
import FlamegraphComponent from './index';
import TestData from './testData';

describe('FlamegraphComponent', () => {
  // the leafs have already been tested
  // this is just to guarantee code is compiling
  it('renders', () => {
    const onZoom = jest.fn();
    const onReset = jest.fn();
    const isDirty = jest.fn();

    render(
      <FlamegraphComponent
        viewType="single"
        fitMode="HEAD"
        zoom={{ i: -1, j: -1 }}
        topLevel={0}
        selectedLevel={0}
        query=""
        onZoom={onZoom}
        onReset={onReset}
        isDirty={isDirty}
        flamebearer={TestData.SimpleTree}
      />
    );
  });
});
