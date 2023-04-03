import React from 'react';
import ReactDOM from 'react-dom/client';
import Icon from '@webapp/ui/Icon';
import { faClock } from '@fortawesome/free-solid-svg-icons/faClock';

const root = ReactDOM.createRoot(document.getElementById('reactRoot'));
root.render(
  <div>
    <Icon icon={faClock} />
  </div>
);
