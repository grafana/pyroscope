// TODO reenable spreading lint
/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import { useAppSelector, useAppDispatch } from '@webapp/redux/hooks';
import {
  actions,
  selectAppNames,
  selectAppNamesState,
  reloadAppNames,
  selectQueries,
} from '@webapp/redux/reducers/continuous';
import LoadingSpinner from '@webapp/ui/LoadingSpinner';
import Button from '@webapp/ui/Button';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import Dropdown, {
  MenuItem,
  FocusableItem,
  MenuGroup,
} from '@webapp/ui/Dropdown';
import Input from '@webapp/ui/Input';
import { queryFromAppName, queryToAppName, Query } from '@webapp/models/query';
import styles from './NameSelector.module.scss';

interface NameSelectorProps {
  /** allows to overwrite what to happen when a name is selected, by default it dispatches 'actions.setQuery' */
  onSelectedName?: (name: Query) => void;
}
function NameSelector({ onSelectedName }: NameSelectorProps) {
  const appNamesState = useAppSelector(selectAppNamesState);
  const appNames = useAppSelector(selectAppNames);
  const dispatch = useAppDispatch();
  const { query } = useAppSelector(selectQueries);

  const [filter, setFilter] = useState('');

  const selectAppName = (name: string) => {
    const query = queryFromAppName(name);
    if (onSelectedName) {
      onSelectedName(query);
    } else {
      dispatch(actions.setQuery(query));
    }
  };

  const selectedValue = queryToAppName(query).mapOr('', (q) => {
    if (appNames.indexOf(q) !== -1) {
      return q;
    }
    return '';
  });

  const filterOptions = (n: string) => {
    const f = filter.trim().toLowerCase();
    return n.toLowerCase().includes(f);
  };

  // TODO figure out this any
  const options = appNames.filter(filterOptions).map((name) => (
    <MenuItem
      key={name}
      value={name}
      onClick={() => selectAppName(name)}
      className={selectedValue === name ? 'active' : ''}
    >
      {name}
    </MenuItem>
  )) as ShamefulAny;

  const noApp = (
    appNames.length > 0 ? null : <MenuItem>No App available</MenuItem>
  ) as JSX.Element;

  return (
    <div className={styles.container}>
      Application:&nbsp;
      <Dropdown
        label="Select application"
        data-testid="app-name-selector"
        value={selectedValue}
        overflow="auto"
        position="anchor"
        menuButtonClassName={styles.menuButton}
      >
        {noApp}
        <FocusableItem>
          {({ ref }) => (
            <Input
              name="application seach"
              ref={ref}
              type="text"
              placeholder="Type an app"
              value={filter}
              onChange={(e) => setFilter(e.target.value)}
            />
          )}
        </FocusableItem>
        <MenuGroup takeOverflow>{options}</MenuGroup>
      </Dropdown>
      <Button
        aria-label="Refresh Apps"
        icon={faSyncAlt}
        onClick={() => {
          dispatch(reloadAppNames());
        }}
      />
      <Loading {...appNamesState} />
    </div>
  );
}

function Loading({ type }: ReturnType<typeof selectAppNamesState>) {
  switch (type) {
    case 'reloading': {
      return <LoadingSpinner />;
    }

    default: {
      return null;
    }
  }
}
export default NameSelector;
