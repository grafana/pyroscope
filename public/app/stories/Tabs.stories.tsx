import React, { useState } from 'react';
import { Tabs, Tab, TabPanel } from '@pyroscope/ui/Tabs';

const LIGHT_COLOR_MODE = 'Light Color Mode';

export default {
  title: 'Components/Tabs',
  args: {
    [LIGHT_COLOR_MODE]: false,
  },
};

export const StoryTabs = (props: any) => {
  const [value, setTab] = useState(0);

  return (
    <body
      style={{ padding: 20 }}
      data-theme={props[LIGHT_COLOR_MODE] ? 'light' : 'dark'}
    >
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
    </body>
  );
};
