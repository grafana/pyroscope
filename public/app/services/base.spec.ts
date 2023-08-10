import { requestWithOrgID } from '@phlare/services/base';
import * as storageSvc from '@phlare/services/storage';

jest.mock('@phlare/services/base', () => {
  return {
    __esModule: true,
    ...jest.requireActual('@phlare/services/base'),
  };
});

jest.mock('@phlare/services/storage', () => {
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
    requestWithOrgID('/', {
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

    requestWithOrgID('/');

    expect(global.fetch).toHaveBeenCalledWith('http://localhost/', {
      headers: {},
    });
  });

  it('sets X-Scope-OrgID if tenantID is available', () => {
    const tenantIdSpy = jest.spyOn(storageSvc, 'tenantIDFromStorage');

    tenantIdSpy.mockReturnValueOnce('myid');

    requestWithOrgID('/');

    expect(global.fetch).toHaveBeenCalledWith('http://localhost/', {
      headers: {
        'X-Scope-OrgID': 'myid',
      },
    });
  });
});
