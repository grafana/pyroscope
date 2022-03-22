// Features represents the list of UI experimental features
// This interfaces serves for when we introduce
// something more sofisticated than simply booleans
interface Features {
  enableAdhocUI?: boolean;
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
