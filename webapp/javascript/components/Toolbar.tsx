import React, { useEffect } from 'react';
import 'react-dom';

import Spinner from 'react-svg-spinner';

import { useAppDispatch, useAppSelector } from '@webapp/redux/hooks';
import {
  selectIsLoadingData,
  selectAppTags,
  actions,
  fetchTags,
  fetchTagValues,
} from '@webapp/redux/reducers/continuous';
import classNames from 'classnames';
import DateRangePicker from './DateRangePicker';
import RefreshButton from './RefreshButton';
import NameSelector from './NameSelector';
import TagsBar from './TagsBar';

interface ToolbarProps {
  // TODO: refactor this
  /* hide tags bar, useful for comparison view */
  hideTagsBar?: boolean;
  /** allows to overwrite what to happen when a name is selected, by default it dispatches 'actions.setQuery' */
  onSelectedName?: (name: string) => void;
}
function Toolbar({ hideTagsBar, onSelectedName }: ToolbarProps) {
  const dispatch = useAppDispatch();
  const isLoadingData = useAppSelector(selectIsLoadingData);
  const { query } = useAppSelector((state) => state.continuous);
  const tags = useAppSelector(selectAppTags(query));

  useEffect(() => {
    dispatch(fetchTags(query));
  }, [query]);

  return (
    <>
      <div className="navbar">
        <div className={classNames('labels')}>
          <NameSelector onSelectedName={onSelectedName} />
        </div>
        <div className="navbar-space-filler" />
        <div
          className={classNames('spinner-container', {
            visible: isLoadingData,
          })}
        >
          <Spinner color="rgba(255,255,255,0.6)" size="20px" />
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
