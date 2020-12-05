import React, {useEffect} from 'react';
import {connect} from 'react-redux';

import Label from './Label';
import {fetchSVG} from '../redux/actions';

class SVGRenderer extends React.Component {

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
  }

  maybeFetchSVG(){
    let url = this.props.renderURL;
    if(this.lastRequestedURL != url) {
      this.lastRequestedURL = url
      this.props.fetchSVG(url);
    }
  }

  render() {
    return (
      <div className="svg-renderer">
        <div className="samples">{
          (this.props.samples || []).map((x) => <div key={x.ts}>{`${x.ts}-${x.samples}`}</div>)
        }</div>
        <div className="svg-container" dangerouslySetInnerHTML={{__html: this.props.svg}}></div>
        {/* <img className="svg-container" src={`data:image/svg+xml;utf8,${this.props.svg}`} /> */}
        {/* {this.props.from} */}
        {/* {this.props.until} */}
      </div>
    );
  }
}



export default connect(
  (x) => x,
  { fetchSVG }
)(SVGRenderer);

