import { z } from 'zod';

const featuresSchema = z.object({
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

const features = featuresSchema.parse(
  hasFeatures(window) ? window.features : {}
);

export const isExportToFlamegraphDotComEnabled =
  features.exportToFlamegraphDotComEnabled;
