import React from 'react';
import { render } from '@testing-library/react';
import { Maybe } from 'true-myth';
import * as buildInfo from '../util/buildInfo';
import Footer from './Footer';

const mockDate = new Date('2021-12-21T12:44:01.741Z');

jest.mock('../util/buildInfo.ts');
const actual = jest.requireActual('../util/buildInfo.ts');

const basicBuildInfo = {
  goos: '',
  goarch: '',
  goVersion: '',
  version: '',
  time: '',
  gitDirty: '',
  gitSHA: '',
  jsVersion: '',
  useEmbeddedAssets: '',
};

describe('Footer', function () {
  beforeEach(() => {
    jest.useFakeTimers().setSystemTime(mockDate.getTime());
  });
  afterEach(() => {
    jest.restoreAllMocks();
  });

  describe('trademark', function () {
    beforeEach(() => {
      const buildInfoMock = jest.spyOn(buildInfo, 'buildInfo');
      const latestVersionMock = jest.spyOn(buildInfo, 'latestVersionInfo');

      buildInfoMock.mockImplementation(() => ({ ...basicBuildInfo }));
      latestVersionMock.mockImplementation(() => actual.latestVersionInfo());
    });

    it('shows current year correctly', function () {
      const { queryByText } = render(<Footer />);

      expect(queryByText(/Pyroscope 2020 – 2021/i)).toBeInTheDocument();
    });
  });

  describe('latest version', function () {
    test.each([
      // smaller
      ['0.0.1', '1.0.0', true],
      ['v0.0.1', 'v1.0.0', true],
      ['v9.0.1', 'v10.0.0', true],

      // same version
      ['1.0.0', '1.0.0', false],
      ['v1.0.0', 'v1.0.0', false],
      // current is bigger (bug with the server most likely)
      ['1.0.0', '0.0.1', false],
      ['v1.0.0', 'v0.0.1', false],
      ['v10.0.1', 'v1.0.0', false],
    ])(
      `currVer (%s), latestVer(%s) should show update available? '%s'`,
      (v1, v2, display) => {
        const buildInfoMock = jest.spyOn(buildInfo, 'buildInfo');
        const latestVersionMock = jest.spyOn(buildInfo, 'latestVersionInfo');

        buildInfoMock.mockImplementation(() => ({
          ...basicBuildInfo,
          version: v1,
        }));

        latestVersionMock.mockImplementation(() =>
          Maybe.of({ latest_version: v2 })
        );

        const { queryByText } = render(<Footer />);

        if (display) {
          expect(queryByText(/Newer Version Available/i)).toBeInTheDocument();
        } else {
          expect(
            queryByText(/Newer Version Available/i)
          ).not.toBeInTheDocument();
        }
      }
    );

    it('does not crash when version is not available', () => {
      const buildInfoMock = jest.spyOn(buildInfo, 'buildInfo');
      const latestVersionMock = jest.spyOn(buildInfo, 'latestVersionInfo');

      buildInfoMock.mockImplementation(() => ({ ...basicBuildInfo }));
      latestVersionMock.mockImplementation(() => Maybe.nothing());

      const { queryByRole } = render(<Footer />);
      expect(queryByRole('contentinfo')).toBeInTheDocument();
    });
  });
});
