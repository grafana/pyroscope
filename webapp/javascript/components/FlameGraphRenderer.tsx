import React, {useEffect} from 'react';
import {connect} from 'react-redux';

import Label from './Label';
import {fetchJSON} from '../redux/actions';
import render from '../flamebearer';
import MaxNodesSelector from "./MaxNodesSelector";
import clsx from "clsx";

class FlameGraphRenderer extends React.Component {
  constructor (){
    super();
  }

  componentDidMount() {
    this.maybeFetchSVG();
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
    if(this.props.flamebearer) {
      render(this.props.flamebearer);
    }
  }

  maybeFetchSVG(){
    let url = this.props.renderURL;
    if(this.lastRequestedURL != url) {
      this.lastRequestedURL = url
      this.props.fetchJSON(url);
    }
  }

  handleSearchChange = (e) => {
    this.setState({searchValue: e.target.value})
  }

  render() {
    return (
      <div className="canvas-renderer">
        <div className="canvas-container">
          <div className="navbar-2">
            <input id="search" name="flamegraph-search" placeholder="Search..." />
            &nbsp;
            <button className={clsx('btn')} style={{visibility:'hidden'}} id="reset">Reset View</button>
            <div className="navbar-space-filler"></div>
            <MaxNodesSelector />
          </div>
          <canvas id="flamegraph-canvas" height="0"></canvas>
        </div>

        <div id="highlight"></div>
        <div id="tooltip"></div>
      </div>
    );
  }
}



export default connect(
  (x) => x,
  { fetchJSON }
)(FlameGraphRenderer);

