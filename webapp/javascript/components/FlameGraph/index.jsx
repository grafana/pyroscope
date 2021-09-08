/* eslint-disable react/no-unused-state */
/* eslint-disable no-bitwise */
/* eslint-disable react/no-access-state-in-setstate */
/* eslint-disable react/jsx-props-no-spreading */
/* eslint-disable react/destructuring-assignment */
/* eslint-disable no-nested-ternary */

import { connect } from "react-redux";
import { bindActionCreators } from "redux";
import { withShortcut } from "react-keybind";
import { buildDiffRenderURL, buildRenderURL } from "../../util/updateRequests";

import FlameGraphRenderer from "./FlameGraphRenderer";

const mapStateToProps = (state) => ({
  ...state,
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

export default connect(
  mapStateToProps,
  mapDispatchToProps
)(withShortcut(FlameGraphRenderer));
