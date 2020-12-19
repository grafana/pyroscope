import React from 'react';
import { connect } from "react-redux";
import { setDateRange, receiveJSON } from "../redux/actions";
import "react-dom";

import ReactFlot from 'react-flot';
import 'react-flot/flot/jquery.flot.time.min';
import 'react-flot/flot/jquery.flot.selection.min';
import {bindActionCreators} from "redux";

let currentJSONController = null;
class TimelineChart extends ReactFlot {
  componentDidMount() {
    this.draw();
    $(`#${this.props.id}`).bind('plotselected', (event, ranges) => {
      console.log('setting date range:', this);
      this.props.actions.setDateRange(Math.round(ranges.xaxis.from / 1000), Math.round(ranges.xaxis.to / 1000))
      let renderURL = this.buildRenderURL();
      this.fetchJSON(renderURL + '&format=json');
    });
  }

  fetchJSON(url) {
    console.log('fetching json (timeline chart)', url);
    if (currentJSONController) {
      currentJSONController.abort();
    }
    currentJSONController = new AbortController();
    fetch(url, {signal: currentJSONController.signal})
      .then((response) => {
        return response.json()
      })
      .then((data) => {
        console.log('data:', data);
        console.log('this: ', this);
        console.dir(this);
        this.props.actions.receiveJSON(data)
      })
      .finally();
  }

  buildRenderURL() {
    let width = document.body.clientWidth - 30;
    let url = `/render?from=${encodeURIComponent(this.props.from)}&until=${encodeURIComponent(this.props.until)}&width=${width}`;
    let nameLabel = this.props.labels.find(x => x.name == "__name__");
    if (nameLabel) {
      url += "&name="+nameLabel.value+"{";
    } else {
      url += "&name=unknown{";
    }

    url += this.props.labels.filter(x => x.name != "__name__").map(x => `${x.name}=${x.value}`).join(",");
    url += "}";
    if(this.props.refreshToken){
      url += `&refreshToken=${this.props.refreshToken}`
    }
    url += `&max-nodes=${this.props.maxNodes}`
    return url;
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
