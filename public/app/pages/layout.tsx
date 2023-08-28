// Layout wrapper components that will allow us to override these layout components with alternative styling

import React, { ReactNode } from 'react';

export function PageContentWrapper({ children }: { children: ReactNode }) {
  return <div className="main-wrapper">{children}</div>;
}
