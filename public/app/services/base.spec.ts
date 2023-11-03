import { request } from '@pyroscope/services/base';
import * as storageSvc from '@pyroscope/services/storage';

jest.mock('@pyroscope/services/base', () => {
  return {
    __esModule: true,
    ...jest.requireActual('@pyroscope/services/base'),
  };
});

jest.mock('@pyroscope/services/storage', () => {
  return {
    __esModule: true,
    tenantIDFromStorage: jest.fn(),
  };
});

describe('base', () => {
  afterEach(() => {
    // restore the spy created with spyOn
    jest.restoreAllMocks();
  });
  beforeEach(() => {
    global.fetch = jest.fn().mockImplementation(
      () =>
        new Promise((resolve) => {
          resolve({
            ok: true,
            text: () =>
              new Promise((resolve) => {
                resolve('');
              }),
          });
        })
    );
  });

  it('uses X-Scope-OrgID if set manually', () => {
    request('/', {
      headers: {
        'X-Scope-OrgID': 'myID',
      },
    });

    expect(global.fetch).toHaveBeenCalledWith('http://localhost/', {
      headers: {
        'X-Scope-OrgID': 'myID',
      },
    });
  });

  it('does not set X-Scope-OrgID if tenantID is not available', () => {
    const tenantIdSpy = jest.spyOn(storageSvc, 'tenantIDFromStorage');

    tenantIdSpy.mockReturnValueOnce('');

    request('/');

    expect(global.fetch).toHaveBeenCalledWith('http://localhost/', {
      headers: {},
    });
  });

  it('sets X-Scope-OrgID if tenantID is available', () => {
    const tenantIdSpy = jest.spyOn(storageSvc, 'tenantIDFromStorage');

    tenantIdSpy.mockReturnValueOnce('myid');

    request('/');

    expect(global.fetch).toHaveBeenCalledWith('http://localhost/', {
      headers: {
        'X-Scope-OrgID': 'myid',
      },
    });
  });
});
