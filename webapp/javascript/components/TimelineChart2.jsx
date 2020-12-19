import React from 'react';
import { connect } from "react-redux";
import { setDateRange, receiveJSON } from "../redux/actions";
import "react-dom";

import ReactFlot from 'react-flot';
import 'react-flot/flot/jquery.flot.time.min';
import 'react-flot/flot/jquery.flot.selection.min';
import { bindActionCreators } from "redux";
import { buildRenderURL, fetchJSON } from '../util/update_requests';

let currentJSONController = null;
class TimelineChart extends ReactFlot {

  constructor(props) {
    super(props);

    this.fetchJSON = fetchJSON.bind(this);
    this.buildRenderURL = buildRenderURL.bind(this);
  }

  componentDidMount() {
    this.draw();
    $(`#${this.props.id}`).bind('plotselected', (event, ranges) => {
      console.log('setting date range:', this);
      this.props.actions.setDateRange(Math.round(ranges.xaxis.from / 1000), Math.round(ranges.xaxis.to / 1000))
      let renderURL = this.buildRenderURL();
      this.fetchJSON(renderURL + '&format=json');
    });
  }
}

const mapStateToProps = state => ({
  ...state,
});

const mapDispatchToProps = dispatch => ({
  actions: bindActionCreators(
    {
      setDateRange,
      receiveJSON,
    },
    dispatch,
  ),
});

export default connect(
  mapStateToProps,
  mapDispatchToProps,
)(TimelineChart);
