// Client capabilities tell the backend which optional API behaviors this
// client supports. The backend reads them from media type parameters in the
// Accept header (pkg/featureflags/client_capability.go).
export const CLIENT_CAPABILITIES = [
  // Keep labels outside the legacy charset (e.g. service.namespace) in
  // LabelNames responses and in the label sets returned by Series.
  'allow-utf8-labelnames',
];

export const CLIENT_CAPABILITIES_ACCEPT_HEADER = [
  'application/json',
  ...CLIENT_CAPABILITIES.map((capability) => `${capability}=true`),
].join('; ');
