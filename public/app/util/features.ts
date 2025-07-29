import { z } from 'zod';

const featuresSchema = z.object({
  googleEnabled: z.boolean().default(false),
  gitlabEnabled: z.boolean().default(false),
  githubEnabled: z.boolean().default(false),
  internalAuthEnabled: z.boolean().default(false),
  signupEnabled: z.boolean().default(false),
  isAuthRequired: z.boolean().default(false),
  exportToFlamegraphDotComEnabled: z.boolean().default(true),
});

function hasFeatures(
  window: unknown
): window is typeof window & { features: unknown } {
  if (typeof window === 'object') {
    if (window && window.hasOwnProperty('features')) {
      return true;
    }
  }

  return false;
}

// Parse features at startup
const features = featuresSchema.parse(
  hasFeatures(window) ? window.features : {}
);

// Re-export with more friendly names
export const isGoogleEnabled = features.googleEnabled;
export const isGitlabEnabled = features.gitlabEnabled;
export const isGithubEnabled = features.githubEnabled;
export const isInternalAuthEnabled = features.internalAuthEnabled;
export const isSignupEnabled = features.signupEnabled;
export const isExportToFlamegraphDotComEnabled =
  features.exportToFlamegraphDotComEnabled;
export const isAuthRequired = features.isAuthRequired;

export const isGrafanaFlamegraphEnabled = true;
