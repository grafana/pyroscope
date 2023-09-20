import React, { ReactNode } from 'react';

export function PageContentWrapper({ children }: { children: ReactNode }) {
  return <div className="main-wrapper">{children}</div>;
}
