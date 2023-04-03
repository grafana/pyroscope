import React from 'react';
import ReactDOM from 'react-dom/client';
import Icon from '@webapp/ui/Icon';
import Box from '@webapp/ui/Box';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';

const root = ReactDOM.createRoot(document.getElementById('reactRoot'));
root.render(
  <div>
    <Box>
      <Icon icon={faClock} />
    </Box>
  </div>
);
