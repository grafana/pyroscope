import React from 'react';
import 'react-dom';

import Spinner from 'react-svg-spinner';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  selectIsLoadingData,
  selectAppTags,
  actions,
  fetchTagValues,
  selectQueries,
} from '@webapp/redux/reducers/continuous';
import { Query } from '@webapp/models/query';
import classNames from 'classnames';
import DateRangePicker from './DateRangePicker';
import RefreshButton from './RefreshButton';
import TagsBar from './TagsBar';
import AppSelector from './AppSelector';

interface ToolbarProps {
  // TODO: refactor this
  /* hide tags bar, useful for comparison view */
  hideTagsBar?: boolean;
  /** allows to overwrite what to happen when a name is selected, by default it dispatches 'actions.setQuery' */
  onSelectedName?: (name: Query) => void;
}
function Toolbar({ hideTagsBar, onSelectedName }: ToolbarProps) {
  const dispatch = useAppDispatch();
  const isLoadingData = useAppSelector(selectIsLoadingData);
  const { query } = useAppSelector(selectQueries);
  const tags = useAppSelector(selectAppTags(query));

  return (
    <>
      <div className="navbar">
        <div className={classNames('labels')}>
          <AppSelector onSelectedName={onSelectedName} />
        </div>
        <div className="navbar-space-filler" />
        <div
          className={classNames('spinner-container', {
            visible: isLoadingData,
            loaded: !isLoadingData,
          })}
        >
          {isLoadingData && (
            <Spinner color="rgba(255,255,255,0.6)" size="20px" />
          )}
        </div>
        &nbsp;
        <RefreshButton />
        &nbsp;
        <DateRangePicker />
      </div>
      {!hideTagsBar && (
        <TagsBar
          query={query}
          tags={tags}
          onSetQuery={(q) => {
            dispatch(actions.setQuery(q));

            // It's the same query, so components' useEffect won't pick up a change
            if (q === query) {
              dispatch(actions.refresh());
            }
          }}
          onSelectedLabel={(label, query) => {
            dispatch(
              fetchTagValues({
                query,
                label,
              })
            );
          }}
        />
      )}
    </>
  );
}

export default Toolbar;
