import React, { RefObject } from 'react';
import { render, screen } from '@testing-library/react';

import HeatmapTooltip from './HeatmapTooltip';
import { heatmapMockData } from '../../services/exemplarsTestData';

const canvasEl = document.createElement('canvas');
const canvasRef = { current: canvasEl } as RefObject<HTMLCanvasElement>;

describe('Component: HeatmapTooltip', () => {
  const renderTooltip = () => {
    render(
      <HeatmapTooltip
        dataSourceElRef={canvasRef}
        heatmapW={400}
        heatmap={heatmapMockData}
        timezone="browser"
        sampleRate={100}
      />
    );
  };

  it('should render initial tooltip (not active)', () => {
    renderTooltip();

    expect(screen.getByTestId('heatmap-tooltip')).toBeInTheDocument();
  });
});
