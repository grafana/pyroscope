import React from 'react';
import { render, screen } from '@testing-library/react';
import AnnotationTooltipBody, { THRESHOLD } from './Annotation';

const mockAnnotations = [
  {
    timestamp: 1663000000,
    content: 'annotation 1',
  },
];

describe('AnnotationTooltipBody', () => {
  it('return null when theres no annotation', () => {
    const { container } = render(
      <AnnotationTooltipBody annotations={[]} pointOffset={jest.fn()} />
    );

    expect(container.querySelector('div')).toBeNull();
  });

  it('return nothing when theres no timestamp', () => {
    const { container } = render(
      <AnnotationTooltipBody
        annotations={mockAnnotations}
        pointOffset={jest.fn()}
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
    const values = [{ closest: [0] }];

    const pointOffset = jest.fn();

    // reference position
    pointOffset.mockReturnValueOnce({ left: 100 });
    // our annotation position, point is to be outside the threshold
    pointOffset.mockReturnValueOnce({ left: 100 + THRESHOLD });

    const { container } = render(
      <AnnotationTooltipBody
        annotations={annotations}
        values={values}
        pointOffset={pointOffset}
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
      const values = [{ closest: [1663000000] }];
      const pointOffset = jest.fn();

      // reference position
      pointOffset.mockReturnValueOnce({ left: 100 });
      // our annotation position
      pointOffset.mockReturnValueOnce({ left: 99 });

      render(
        <AnnotationTooltipBody
          values={values}
          annotations={annotations}
          pointOffset={pointOffset}
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
      const pointOffset = jest.fn();

      pointOffset.mockImplementation((a) => {
        // our reference point
        if (a.x === values[0].closest[0]) {
          return { left: 98 };
        }

        // furthest
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
          values={values}
          annotations={annotations}
          pointOffset={pointOffset}
        />
      );

      expect(screen.queryByText(/annotation closest/i)).toBeInTheDocument();
    });
  });
});
