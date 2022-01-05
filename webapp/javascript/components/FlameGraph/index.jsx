/* eslint-disable react/no-unused-state */
/* eslint-disable no-bitwise */
/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/jsx-props-no-spreading */
/* eslint-disable react/destructuring-assignment */
/* eslint-disable no-nested-ternary */

import React from 'react';
import { connect } from 'react-redux';
import { bindActionCreators, compose } from 'redux';
import { withDisplayName } from '@pyroscope/redux/utils';
import { buildDiffRenderURL, buildRenderURL } from '../../util/updateRequests';
import { withNamedUpdateableView } from './enchancers';

import { FlameGraphRenderer } from './FlameGraphRenderer';

const mapStateToProps = (state) => ({
  ...state.root,
  renderURL: buildRenderURL(state),
  leftRenderURL: buildRenderURL(state, state.leftFrom, state.leftUntil),
  rightRenderURL: buildRenderURL(state, state.rightFrom, state.rightUntil),
  diffRenderURL: buildDiffRenderURL(
    state,
    state.leftFrom,
    state.leftUntil,
    state.rightFrom,
    state.rightUntil
  ),
});

const mapDispatchToProps = (dispatch) => ({
  actions: bindActionCreators({}, dispatch),
});

export default withDisplayName('NamedFlameGraphRenderer')((props) => {
  const { name } = props;
  if (name) {
    const Component = compose(
      connect(mapStateToProps, mapDispatchToProps),
      withNamedUpdateableView(name)
    )(FlameGraphRenderer);
    return <Component {...props} />;
  }
  // Name here is used to identify different instances to separate persisted data
  console.error('Please specify `name` property for FlameGraph component');
  return null;
});
