// Features refers to configurations sent from the backend
interface Features {
  enableAdhocUI: boolean;
  googleEnabled: boolean;
  gitlabEnabled: boolean;
  githubEnabled: boolean;
  internalAuthEnabled: boolean;
  signupEnabled: boolean;
  isAuthRequired: boolean;

  exportToFlamegraphDotComEnabled: boolean;
}

function hasFeatures(
  window: unknown
): window is typeof window & { features: Features } {
  if (typeof window === 'object') {
    if (window && window.hasOwnProperty('features')) {
      return true;
    }
  }

  return false;
}

// Features
export const isAdhocUIEnabled = hasFeatures(window)
  ? window.features.enableAdhocUI
  : true;

export const isGoogleEnabled = hasFeatures(window)
  ? window.features.googleEnabled
  : true;

export const isGitlabEnabled = hasFeatures(window)
  ? window.features.gitlabEnabled
  : true;

export const isGithubEnabled = hasFeatures(window)
  ? window.features.githubEnabled
  : true;

export const isInternalAuthEnabled = hasFeatures(window)
  ? window.features.internalAuthEnabled
  : true;

export const isSignupEnabled = hasFeatures(window)
  ? window.features.signupEnabled
  : true;

export const isExportToFlamegraphDotComEnabled = hasFeatures(window)
  ? window.features.exportToFlamegraphDotComEnabled
  : true;

export const isAuthRequired = hasFeatures(window)
  ? window.features.isAuthRequired
  : false;
