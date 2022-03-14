import React from 'react';
import 'react-dom';

import Spinner from 'react-svg-spinner';

import { useAppSelector } from '@pyroscope/redux/hooks';
import { selectIsLoadingData } from '@pyroscope/redux/reducers/continuous';
import classNames from 'classnames';
import DateRangePicker from './DateRangePicker';
import RefreshButton from './RefreshButton';
import NameSelector from './NameSelector';
import TagsBar from './TagsBar';

interface ToolbarProps {
  // TODO: refactor this
  /* hide tags bar, useful for comparison view */
  hideTagsBar?: boolean;
}
function Toolbar({ hideTagsBar }: ToolbarProps) {
  const isLoadingData = useAppSelector(selectIsLoadingData);

  // This component initializes using a value frmo the redux store (query)
  // Which doesn't work well when the 'query' changes in the store (see https://reactjs.org/docs/forms.html#controlled-components)
  // This is a workaround to force the component to always remount
  // TODO: move the state from this component into the redux store
  //  const tagsBar = <TagsBar key={query} />;

  return (
    <>
      <div className="navbar">
        <div className={classNames('labels')}>
          <NameSelector />
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
      {!hideTagsBar && <TagsBar />}
    </>
  );
}

export default Toolbar;
