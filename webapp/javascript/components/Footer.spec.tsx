import React from 'react';
import { render } from '@testing-library/react';
import Footer from './Footer';

const mockDate = new Date('2021-12-21T12:44:01.741Z');

describe('Footer', function () {
  describe('trademark', function () {
    beforeEach(() => {
      jest.useFakeTimers().setSystemTime(mockDate.getTime());
    });

    it('shows current year correctly', function () {
      const { queryByText } = render(<Footer />);

      expect(queryByText(/Pyroscope 2020 – 2021/i)).toBeInTheDocument();
    });
  });
});
