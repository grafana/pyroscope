import React from 'react';
import { render, screen } from '@testing-library/react';
import AnnotationTooltipBody, { THRESHOLD } from './Annotation';

describe('AnnotationTooltipBody', () => {
  it('return null when theres no annotation', () => {
    const { container } = render(
      <AnnotationTooltipBody
        timezone="utc"
        annotations={[]}
        coordsToCanvasPos={jest.fn()}
        canvasX={0}
      />
    );

    expect(container.querySelector('div')).toBeNull();
  });

  it('return nothing when no annotation match', () => {
    const annotations = [
      {
        timestamp: 0,
        content: 'annotation 1',
      },
    ];
    const coordsToCanvasPos = jest.fn();

    // reference position
    coordsToCanvasPos.mockReturnValueOnce({ left: 100 });
    // our annotation position, point is to be outside the threshold
    coordsToCanvasPos.mockReturnValueOnce({ left: 100 + THRESHOLD });

    const { container } = render(
      <AnnotationTooltipBody
        timezone="utc"
        annotations={annotations}
        coordsToCanvasPos={coordsToCanvasPos}
        canvasX={200}
      />
    );

    expect(container.querySelector('div')).toBeNull();
  });

  describe('rendering annotation', () => {
    it('return an annotation', () => {
      const annotations = [
        {
          timestamp: 1663000000,
          content: 'annotation 1',
        },
      ];
      const coordsToCanvasPos = jest.fn();

      // reference position
      coordsToCanvasPos.mockReturnValueOnce({ left: 100 });

      render(
        <AnnotationTooltipBody
          timezone="utc"
          annotations={annotations}
          coordsToCanvasPos={coordsToCanvasPos}
          canvasX={100}
        />
      );

      expect(screen.queryByText(/annotation 1/i)).toBeInTheDocument();
    });

    it('renders the closest annotation', () => {
      const furthestAnnotation = {
        timestamp: 1663000010,
        content: 'annotation 1',
      };
      const closestAnnotation = {
        timestamp: 1663000009,
        content: 'annotation closest',
      };
      const annotations = [furthestAnnotation, closestAnnotation];
      const values = [{ closest: [1663000000] }];
      const coordsToCanvasPos = jest.fn();

      coordsToCanvasPos.mockImplementation((a) => {
        // our reference point
        if (a.x === furthestAnnotation.timestamp) {
          return { left: 100 };
        }

        // closest
        if (a.x === closestAnnotation.timestamp) {
          return { left: 99 };
        }
      });

      render(
        <AnnotationTooltipBody
          timezone="utc"
          annotations={annotations}
          coordsToCanvasPos={coordsToCanvasPos}
          canvasX={98}
        />
      );

      expect(screen.queryByText(/annotation closest/i)).toBeInTheDocument();
    });
  });
});
