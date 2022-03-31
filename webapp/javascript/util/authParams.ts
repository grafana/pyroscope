interface AuthParams {
  isRequired?: boolean;
  internalEnabled?: boolean;
  internalSignupEnabled?: boolean;
  googleEnabled?: boolean;
  githubEnabled?: boolean;
  gitlabEnabled?: boolean;
}

function hasAuthParams(
  window: unknown
): window is typeof window & { auth: AuthParams } {
  if (typeof window === 'object') {
    if (window && window.hasOwnProperty('auth')) {
      return true;
    }
  }

  return false;
}

export const isAuthRequired = hasAuthParams(window)
  ? window.auth.isRequired
  : true;

export const isInternalAuthEnabled = hasAuthParams(window)
  ? window.auth.internalEnabled
  : true;

export const isInternalSignupEnabled = hasAuthParams(window)
  ? window.auth.internalSignupEnabled
  : true;

export const isGoogleAuthEnabled = hasAuthParams(window)
  ? window.auth.googleEnabled
  : true;

export const isGithubAuthEnabled = hasAuthParams(window)
  ? window.auth.githubEnabled
  : true;

export const isGitlabAuthEnabled = hasAuthParams(window)
  ? window.auth.gitlabEnabled
  : true;
