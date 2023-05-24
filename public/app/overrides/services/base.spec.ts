import * as ogBase from '@pyroscope/webapp/javascript/services/base';
import { requestWithOrgID } from '@webapp/services/base';
import * as tenantSvc from '@phlare/services/tenant';

jest.mock('@pyroscope/webapp/javascript/services/base', () => {
  return {
    __esModule: true,
    ...jest.requireActual('@pyroscope/webapp/javascript/services/base'),
  };
});

jest.mock('@phlare/services/tenant', () => {
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

  it('uses X-Scope-OrgID if set manually', () => {
    const spy = jest.spyOn(ogBase, 'request');

    requestWithOrgID('/', {
      headers: {
        'X-Scope-OrgID': 'myID',
      },
    });

    expect(spy).toHaveBeenCalledWith('/', {
      headers: {
        'X-Scope-OrgID': 'myID',
      },
    });
  });

  it('does not set X-Scope-OrgID if tenantID is not available', () => {
    const spy = jest.spyOn(ogBase, 'request');
    const tenantIdSpy = jest.spyOn(tenantSvc, 'tenantIDFromStorage');

    tenantIdSpy.mockReturnValueOnce('');

    requestWithOrgID('/');

    expect(spy).toHaveBeenCalledWith('/', {
      headers: {},
    });
  });

  it('sets X-Scope-OrgID if tenantID is available', () => {
    const spy = jest.spyOn(ogBase, 'request');
    const tenantIdSpy = jest.spyOn(tenantSvc, 'tenantIDFromStorage');

    tenantIdSpy.mockReturnValueOnce('myid');

    requestWithOrgID('/');

    expect(spy).toHaveBeenCalledWith('/', {
      headers: {
        'X-Scope-OrgID': 'myid',
      },
    });
  });
});
