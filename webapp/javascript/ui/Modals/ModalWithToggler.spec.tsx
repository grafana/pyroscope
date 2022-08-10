import React from 'react';
import { render, screen, cleanup } from '@testing-library/react';

import ModalWithToggle, { ModalWithToggleProps } from './ModalWithToggle';

const defaultProps = {
  toggleText: 'toggle-test-text',
  headerEl: <div />,
  leftSideEl: <div />,
  rightSideEl: <div />,
  footerEl: <div />,
};

describe('Component: ModalWithToggle', () => {
  const renderComponent = (props: ModalWithToggleProps) => {
    render(<ModalWithToggle {...props} />);
  };

  describe('structure and toggle behavior', () => {
    beforeEach(() => {
      renderComponent(defaultProps);
    });

    it('should display toggler button', () => {
      expect(screen.getByTestId('toggler')).toHaveTextContent(
        defaultProps.toggleText
      );
    });

    it('should display modal after toggler click', () => {
      // upper testcase triggers click
      expect(screen.getByTestId('modal')).toBeInTheDocument();
      expect(screen.getByTestId('modal-header')).toBeInTheDocument();
      expect(screen.getByTestId('modal-body')).toBeInTheDocument();
      expect(screen.getByTestId('modal-footer')).toBeInTheDocument();
    });
  });

  describe('optional props', () => {
    beforeEach(() => {
      cleanup();
    });

    it('props: noDataEl', () => {
      renderComponent({
        ...defaultProps,
        noDataEl: <div data-testid="no-data" />,
      });
      expect(screen.getByTestId('no-data')).toBeInTheDocument();
    });

    it('props: modalClassName', () => {
      renderComponent({ ...defaultProps, modalClassName: 'test-class-name' });
      expect(screen.getByTestId('modal').getAttribute('class')).toContain(
        'test-class-name'
      );
    });

    it('props: modalHeight', () => {
      renderComponent({ ...defaultProps, modalHeight: '100px' });
      expect(
        screen
          .getByTestId('modal')
          .querySelector('.side')
          ?.getAttribute('style')
      ).toContain('100px');
    });
  });
});
