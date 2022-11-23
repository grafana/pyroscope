import React from 'react';
import { render, screen } from '@testing-library/react';

import ModalWithToggle, { ModalWithToggleProps } from './ModalWithToggle';

const defaultProps = {
  isModalOpen: false,
  setModalOpenStatus: jest.fn(),
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

  describe('structure', () => {
    it('should display toggler button', () => {
      renderComponent(defaultProps);

      expect(screen.getByTestId('toggler')).toHaveTextContent(
        defaultProps.toggleText
      );
    });

    it('should display modal after toggler click', () => {
      renderComponent({ ...defaultProps, isModalOpen: true });

      expect(screen.getByTestId('modal')).toBeInTheDocument();
      expect(screen.getByTestId('modal-header')).toBeInTheDocument();
      expect(screen.getByTestId('modal-body')).toBeInTheDocument();
      expect(screen.getByTestId('modal-footer')).toBeInTheDocument();
    });
  });

  describe('optional props', () => {
    it('props: noDataEl', () => {
      renderComponent({
        ...defaultProps,
        isModalOpen: true,
        noDataEl: <div data-testid="no-data" />,
      });
      expect(screen.getByTestId('no-data')).toBeInTheDocument();
    });

    it('props: modalClassName', () => {
      renderComponent({
        ...defaultProps,
        isModalOpen: true,
        modalClassName: 'test-class-name',
      });
      expect(screen.getByTestId('modal').getAttribute('class')).toContain(
        'test-class-name'
      );
    });

    it('props: modalHeight', () => {
      renderComponent({
        ...defaultProps,
        modalHeight: '100px',
        isModalOpen: true,
      });
      expect(
        screen
          .getByTestId('modal')
          .querySelector('.side')
          ?.getAttribute('style')
      ).toContain('100px');
    });
  });
});
