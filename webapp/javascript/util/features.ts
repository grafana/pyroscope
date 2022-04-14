// Features represents the list of UI experimental features
// This interfaces serves for when we introduce
// something more sofisticated than simply booleans
interface Features {
  enableAdhocUI?: boolean;
  googleEnabled?: boolean;
  gitlabEnabled?: boolean;
  githubEnabled?: boolean;
  signupEnabled?: boolean;
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

export const isSignupEnabled = hasFeatures(window)
  ? window.features.signupEnabled
  : true;
