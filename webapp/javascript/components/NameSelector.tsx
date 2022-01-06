// TODO reenable spreading lint
/* eslint-disable react/jsx-props-no-spreading */
import React from 'react';
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
import { setQuery } from '../redux/actions';

const defKey = 'Select an app...';

function NameSelector(props) {
  const { actions, names, query } = props;
  const appNamesState = useAppSelector(selectAppNamesState);
  const appNames = useAppSelector(selectAppNames);

  const selectAppName = (event) => {
    const query = appNameToQuery(event.target.value);
    actions.setQuery(query);
  };

  const dispatch = useAppDispatch();

  let defaultValue = (query || '').replace(/\{.*/g, '');
  if (names && names.indexOf(defaultValue) === -1) {
    defaultValue = defKey;
  }

  const options = appNames.mapOrElse(
    () => null,
    (names) => {
      return names.map((name) => (
        <option key={name} value={name}>
          {name}
        </option>
      ));
    }
  );

  return (
    <span>
      Application:&nbsp;
      <select
        className="label-select"
        data-testid="app-name-selector"
        value={defaultValue}
        onChange={selectAppName}
      >
        <option disabled key={defKey} value="Select an app...">
          Select an app...
        </option>
        {options}
      </select>
      <Button
        icon={faSyncAlt}
        onClick={() => {
          dispatch(reloadAppNames());
        }}
      />
      <Loading {...appNamesState} />
    </span>
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
