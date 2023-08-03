import { act, render, screen } from '@testing-library/react';
import React, { useState } from 'react';
import userEvent from '@testing-library/user-event';
import { Tab, TabPanel, Tabs } from './Tabs';

function TabsComponent() {
  const [value, setTab] = useState(0);

  return (
    <>
      <Tabs value={value} onChange={(e, value) => setTab(value)}>
        <Tab label="Tab_1" />
        <Tab label="Tab_2" />
      </Tabs>
      <TabPanel value={value} index={0}>
        Tab_1_Content
      </TabPanel>
      <TabPanel value={value} index={1}>
        Tab_2_Content
      </TabPanel>
    </>
  );
}

describe('Tabs', () => {
  beforeEach(() => {
    render(<TabsComponent />);
  });

  it('shows all tabs', () => {
    expect(screen.getByText('Tab_1')).toBeVisible();
    expect(screen.getByText('Tab_2')).toBeVisible();
  });

  it('toggling tabs works', () => {
    expect(screen.getByText('Tab_1_Content')).toBeVisible();
    expect(screen.queryByText('Tab_2_Content')).toBeNull();

    const tab2 = screen.getByText('Tab_2');
    act(() => userEvent.click(tab2, { button: 1 }));

    expect(screen.getByText('Tab_2_Content')).toBeVisible();
    expect(screen.queryByText('Tab_1_Content')).toBeNull();
  });
});
