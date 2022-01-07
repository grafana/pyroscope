// TODO reenable spreading lint
/* eslint-disable react/jsx-props-no-spreading */
import React, { useState } from 'react';
import { connect } from 'react-redux';
import { bindActionCreators } from 'redux';
import { useAppSelector, useAppDispatch } from '@pyroscope/redux/hooks';
import { appNameToQuery, queryToAppName } from '@utils/query';
import {
  selectAppNames,
  selectAppNamesState,
  reloadAppNames,
} from '@pyroscope/redux/reducers/newRoot';
import Spinner from 'react-svg-spinner';
import Button from '@ui/Button';
import { faSyncAlt } from '@fortawesome/free-solid-svg-icons/faSyncAlt';
import Dropdown, { MenuItem, FocusableItem } from '@ui/Dropdown';
import { Maybe } from '@utils/fp';
import { setQuery } from '../redux/actions';
import styles from './NameSelector.module.scss';

const defKey = 'Select an app...';

function NameSelector(props) {
  const { actions, names, query } = props;
  const appNamesState = useAppSelector(selectAppNamesState);
  const appNames = useAppSelector(selectAppNames);

  const [filter, setFilter] = useState<Maybe<string>>(Maybe.nothing());

  const selectAppName = (name: string) => {
    const query = appNameToQuery(name);
    actions.setQuery(query);
  };

  const dispatch = useAppDispatch();

  let defaultValue = queryToAppName(query).mapOr('', (q) => q);
  // TODO: don't do this and instead always have a defined query
  if (names && names.indexOf(defaultValue) === -1) {
    defaultValue = defKey;
  }

  const filterOptions = (n: string) => {
    const f = filter.mapOr('', (v) => v.trim().toLowerCase());
    return n.toLowerCase().includes(f);
  };

  const options = appNames.mapOrElse(
    () => null,
    (names) => {
      return names.filter(filterOptions).map((name) => (
        <MenuItem key={name} value={name} onClick={() => selectAppName(name)}>
          {name}
        </MenuItem>
      ));
    }
  );

  const noApp = <MenuItem>No App available</MenuItem>;

  return (
    <div className={styles.container}>
      Application:&nbsp;
      <Dropdown
        label="Select application"
        data-testid="app-name-selector"
        value={defaultValue}
        overflow="auto"
        position="anchor"
        menuButtonClassName={styles.menuButton}
      >
        {appNames.mapOr(noApp, (names) => (names.length > 0 ? null : noApp))}
        <FocusableItem>
          {({ ref }) => (
            <input
              ref={ref}
              type="text"
              placeholder="Type an app"
              value={filter.mapOr('', (v) => v)}
              onChange={(e) => setFilter(Maybe.just(e.target.value))}
            />
          )}
        </FocusableItem>

        {options}
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
      return <Spinner color="rgba(255,255,255,0.6)" size="20px" />;
    }

    default: {
      return null;
    }
  }
}

const mapStateToProps = (state) => ({
  ...state.root,
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators(
    {
      setQuery,
    },
    dispatch
  ),
});

export default connect(mapStateToProps, mapDispatchToProps)(NameSelector);
