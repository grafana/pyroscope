import React from 'react';
import { connect } from 'react-redux';
import 'react-dom';

import Spinner from 'react-svg-spinner';

import classNames from 'classnames';
import DateRangePicker from './DateRangePicker';
import RefreshButton from './RefreshButton';
import NameSelector from './NameSelector';
import TagsBar from './TagsBar';

import { fetchNames } from '../redux/actions';

function Header(props) {
  const { areNamesLoading, isJSONLoading, query } = props;

  // This component initializes using a value frmo the redux store (query)
  // Which doesn't work well when the 'query' changes in the store (see https://reactjs.org/docs/forms.html#controlled-components)
  // This is a workaround to force the component to always remount
  // TODO: move the state from this component into the redux store
  const tagsBar = <TagsBar key={query} />;

  return (
    <>
      <div className="navbar">
        <div
          className={classNames('labels', {
            visible: !areNamesLoading,
          })}
        >
          <NameSelector />
        </div>
        <div className="navbar-space-filler" />
        <div
          className={classNames('spinner-container', {
            visible: isJSONLoading,
          })}
        >
          <Spinner color="rgba(255,255,255,0.6)" size="20px" />
        </div>
        &nbsp;
        <RefreshButton />
        &nbsp;
        <DateRangePicker />
      </div>
      {tagsBar}
    </>
  );
}

const mapStateToProps = (state) => ({
  ...state.root,
});

export default connect(mapStateToProps, { fetchNames })(Header);
