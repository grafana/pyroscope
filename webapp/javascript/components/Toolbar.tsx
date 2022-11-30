import React from 'react';
import 'react-dom';

import { Query } from '@webapp/models/query';
import classNames from 'classnames';
import DateRangePicker from './DateRangePicker';
import RefreshButton from './RefreshButton';
import AppSelector from './AppSelector';

interface ToolbarProps {
  /** callback to be called when an app is selected via the dropdown */
  onSelectedApp: (name: Query) => void;

  filterApp?: React.ComponentProps<typeof AppSelector>['filterApp'];
}
function Toolbar({ onSelectedApp: onSelectedName, filterApp }: ToolbarProps) {
  return (
    <>
      <div className="navbar">
        <div className={classNames('labels')}>
          <AppSelector onSelectedName={onSelectedName} filterApp={filterApp} />
        </div>
        <div className="navbar-space-filler" />
        <RefreshButton />
        &nbsp;
        <DateRangePicker />
      </div>
    </>
  );
}

export default Toolbar;
