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
    render(<AnnotationTooltipBody annotations={[]} />);

    expect(screen.queryByRole('section')).toBeNull();
  });

  it('return nothing when theres no timestamp', () => {
    render(<AnnotationTooltipBody annotations={mockAnnotations} />);

    expect(screen.queryByRole('section')).toBeNull();
  });

  it('return nothing when no annotation match', () => {
    const annotations = [
      {
        timestamp: 1663000000,
        content: 'annotation 1',
      },
    ];
    const values = [{ closest: [annotations[0].timestamp + THRESHOLD + 1] }];

    render(<AnnotationTooltipBody annotations={annotations} values={values} />);

    expect(screen.queryByRole('section')).toBeNull();
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

      render(
        <AnnotationTooltipBody values={values} annotations={annotations} />
      );

      expect(screen.queryByText(/annotation 1/i)).toBeInTheDocument();
    });

    it('renders the closest annotation', () => {
      const annotations = [
        {
          timestamp: 1663000010,
          content: 'annotation 1',
        },
        {
          timestamp: 1663000009,
          content: 'annotation closest',
        },
      ];
      const values = [{ closest: [1663000000] }];

      render(
        <AnnotationTooltipBody values={values} annotations={annotations} />
      );

      expect(screen.queryByText(/annotation closest/i)).toBeInTheDocument();
    });
  });
});
