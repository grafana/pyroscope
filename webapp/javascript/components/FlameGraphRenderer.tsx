import React, {useEffect} from 'react';
import {connect} from 'react-redux';

import Label from './Label';
import {fetchJSON} from '../redux/actions';
import render from '../flamebearer';

class FlameGraphRenderer extends React.Component {

  componentDidMount() {
    this.maybeFetchSVG();
    render();
  }

  componentDidUpdate(prevProps) {
    this.maybeFetchSVG()
    if (window.init) {
      try {
        window.init();
      } catch(e) {
        console.log(e);
      }
    }
    if (window.unzoom) {
      try {
        window.unzoom();
      } catch(e) {
        console.log(e);
      }
    }
  }

  maybeFetchSVG(){
    let url = this.props.renderURL;
    if(this.lastRequestedURL != url) {
      this.lastRequestedURL = url
      this.props.fetchJSON(url);
    }
  }

  render() {
    return (
      <div className="canvas-renderer">
        <canvas id="flamegraph-canvas" height="0"></canvas>

        <div id="header">
          <div id="controls">
            <input id="search" placeholder="Search..." />
            <button id="reset">Reset view</button>
          </div>
        </div>
        <div id="highlight"></div>
        <div id="tooltip"></div>
        <div id="intro"></div>
      </div>
    );
  }
}



export default connect(
  (x) => x,
  { fetchJSON }
)(FlameGraphRenderer);

